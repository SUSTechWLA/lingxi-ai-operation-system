#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import pathlib
import tempfile
import unittest
import wave

from reference_voice import synthesize_reference_voice_service


def _write_wav(path: pathlib.Path, *, sample_rate: int = 48000) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with wave.open(str(path), "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(sample_rate)
        wav_file.writeframes(b"\x00\x00" * 4800)


def _sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class FakeClient:
    def __init__(self, *, returned_path: pathlib.Path | None = None) -> None:
        self.calls: list[dict[str, object]] = []
        self.returned_path = returned_path

    def synthesize(self, **kwargs):
        self.calls.append(kwargs)
        output = pathlib.Path(str(kwargs["output_path"]))
        _write_wav(output, sample_rate=24000)
        actual = self.returned_path or output
        if self.returned_path:
            _write_wav(actual, sample_rate=24000)
        return {
            "audioPath": str(actual),
            "provider": "gpt_sovits_local",
            "voiceId": str(kwargs["voice_id"]),
            "referenceAudioSha256": _sha256(pathlib.Path(str(kwargs["ref_audio_path"]))),
            "bundleReady": True,
            "productionReady": True,
        }


class ReferenceVoiceServiceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)
        self.reference = self.root / "reference.wav"
        self.gpt = self.root / "gpt.ckpt"
        self.sovits = self.root / "sovits.pth"
        _write_wav(self.reference)
        self.gpt.write_bytes(b"gpt")
        self.sovits.write_bytes(b"sovits")
        self.profile = self.root / "character-profile.json"
        self.profile.write_text(json.dumps({
            "schemaVersion": "tangying-ip-character/v1",
            "voice": {
                "provider": "gpt_sovits_local",
                "voiceId": "main_ip_warm_knowledge_host_v1",
                "language": "zh-CN",
                "speed": 0.94,
                "gptSovitsLocal": {
                    "endpoint": "http://127.0.0.1:9880",
                    "referenceAudioPath": self.reference.name,
                    "expectedReferenceAudioSha256": _sha256(self.reference),
                    "promptText": "这是经过核对的默认参考录音原文。",
                    "promptTextVerified": True,
                    "promptLanguage": "zh",
                    "gptWeightsPath": self.gpt.name,
                    "expectedGptWeightsSha256": _sha256(self.gpt),
                    "sovitsWeightsPath": self.sovits.name,
                    "expectedSovitsWeightsSha256": _sha256(self.sovits),
                    "modelVersion": "v2ProPlus",
                    "seed": 20260714,
                    "settings": {},
                },
            },
        }), encoding="utf-8")

    def tearDown(self) -> None:
        self.tmp.cleanup()

    @staticmethod
    def _master(raw_path: pathlib.Path, output_path: pathlib.Path):
        del raw_path
        _write_wav(output_path)
        return ({"integratedLufs": -16.0, "truePeakDbtp": -1.6, "loudnessRangeLu": 2.1}, _sha256(output_path))

    def _call(self, client: FakeClient, **overrides):
        arguments = {
            "text": "欢迎使用唐影。",
            "output_dir": str(self.root / "output"),
            "mode": "default_ip",
            "character_profile_path": str(self.profile),
            "provider": "gpt_sovits_local",
            "voice_id": "",
            "reference_audio_path": "",
            "reference_text": "",
            "reference_text_verified": False,
            "usage_rights_confirmed": False,
            "language": "zh",
            "speed": 1.0,
            "expected_reference_audio_sha256": "",
            "client_factory": lambda _config: client,
            "master_audio": self._master,
        }
        arguments.update(overrides)
        return synthesize_reference_voice_service(**arguments)

    def test_default_ip_loads_pinned_profile_reference_and_writes_safe_provenance(self) -> None:
        client = FakeClient()
        result = self._call(client)

        self.assertEqual(result["schemaVersion"], "tangying-reference-voice-result/v1")
        self.assertEqual(result["mode"], "default_ip")
        self.assertEqual(result["voiceId"], "main_ip_warm_knowledge_host_v1")
        self.assertEqual(pathlib.Path(client.calls[0]["ref_audio_path"]), self.reference.resolve())
        self.assertEqual(result["masteredFileSha256"], _sha256(pathlib.Path(result["audioPath"])))
        with wave.open(result["audioPath"], "rb") as wav_file:
            self.assertEqual((wav_file.getframerate(), wav_file.getnchannels(), wav_file.getsampwidth()), (48000, 1, 2))
        sidecar = json.loads(pathlib.Path(result["provenancePath"]).read_text(encoding="utf-8"))
        self.assertEqual(sidecar["schemaVersion"], "tangying-production-audio-provenance/v1")
        self.assertEqual(sidecar["outputFileSha256"], result["masteredFileSha256"])
        serialized = json.dumps({"result": result, "sidecar": sidecar}, ensure_ascii=False)
        self.assertNotIn("经过核对", serialized)
        self.assertNotIn(str(self.gpt), serialized)
        self.assertNotIn("audioBytes", serialized)

    def test_reference_clone_requires_exact_text_verification_and_consent_before_client(self) -> None:
        cases = [
            {"reference_text": "", "reference_text_verified": True, "usage_rights_confirmed": True},
            {"reference_text": "逐字原文", "reference_text_verified": False, "usage_rights_confirmed": True},
            {"reference_text": "逐字原文", "reference_text_verified": True, "usage_rights_confirmed": False},
        ]
        for values in cases:
            with self.subTest(values=values):
                client = FakeClient()
                with self.assertRaises(ValueError):
                    self._call(client, mode="reference_clone", reference_audio_path=str(self.reference), **values)
                self.assertEqual(client.calls, [])

    def test_rejects_unapproved_provider_before_client(self) -> None:
        client = FakeClient()
        with self.assertRaisesRegex(ValueError, "gpt_sovits_local"):
            self._call(client, provider="chattts_local")
        self.assertEqual(client.calls, [])

    def test_reference_hash_mismatch_fails_before_synthesis(self) -> None:
        client = FakeClient()
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            self._call(
                client,
                mode="reference_clone",
                reference_audio_path=str(self.reference),
                reference_text="逐字原文",
                reference_text_verified=True,
                usage_rights_confirmed=True,
                expected_reference_audio_sha256="0" * 64,
            )
        self.assertEqual(client.calls, [])

    def test_rejects_client_output_outside_output_directory(self) -> None:
        client = FakeClient(returned_path=self.root / "escaped.wav")
        with self.assertRaisesRegex(ValueError, "outside outputDir"):
            self._call(client)


if __name__ == "__main__":
    unittest.main()
