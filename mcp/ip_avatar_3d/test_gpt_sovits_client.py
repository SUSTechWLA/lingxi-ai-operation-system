#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import io
import json
import os
import pathlib
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.parse
import wave
from unittest import mock

from gpt_sovits_client import GPTSoVITSClient, GPTSoVITSError


def wav_bytes() -> bytes:
    buffer = io.BytesIO()
    with wave.open(buffer, "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(32000)
        wav_file.writeframes(b"\x00\x00" * 320)
    return buffer.getvalue()


class FakeResponse:
    def __init__(
        self,
        body: bytes,
        *,
        status: int = 200,
        content_type: str = "application/json",
    ) -> None:
        self.body = body
        self.status = status
        self.headers = {"Content-Type": content_type}

    def read(self) -> bytes:
        return self.body

    def __enter__(self):
        return self

    def __exit__(self, _exc_type, _exc, _traceback) -> None:
        return None


class GPTSoVITSClientTests(unittest.TestCase):
    def make_inputs(self, root: pathlib.Path) -> dict[str, pathlib.Path]:
        ref_audio = root / "reference.wav"
        gpt_weights = root / "voice.ckpt"
        sovits_weights = root / "voice.pth"
        ref_audio.write_bytes(wav_bytes())
        gpt_weights.write_bytes(b"gpt checkpoint")
        sovits_weights.write_bytes(b"sovits checkpoint")
        return {
            "ref_audio": ref_audio,
            "gpt_weights": gpt_weights,
            "sovits_weights": sovits_weights,
        }

    def synthesize_kwargs(
        self,
        root: pathlib.Path,
        inputs: dict[str, pathlib.Path],
        *,
        output_name: str = "generated.wav",
    ) -> dict:
        return {
            "text": "今天分享一个清晰的判断。",
            "text_lang": "zh",
            "ref_audio_path": str(inputs["ref_audio"]),
            "prompt_text": "这是经过人工核对的参考音频原文。",
            "prompt_lang": "zh",
            "prompt_text_verified": True,
            "gpt_weights_path": str(inputs["gpt_weights"]),
            "sovits_weights_path": str(inputs["sovits_weights"]),
            "voice_id": "main-ip-gpt-sovits-v1",
            "model_version": "gpt-sovits-v2-main-ip-2026-07",
            "output_path": str(root / output_name),
            "seed": 24680,
        }

    def successful_urlopen(self, calls: list) -> callable:
        audio = wav_bytes()

        def fake_urlopen(request, timeout):
            calls.append((request, timeout))
            path = urllib.parse.urlparse(request.full_url).path
            if path == "/tts":
                return FakeResponse(audio, content_type="audio/wav")
            return FakeResponse(b'{"message":"success"}')

        return fake_urlopen

    def test_rejects_non_loopback_endpoint_before_sending_private_paths(self) -> None:
        calls = []

        with self.assertRaisesRegex(GPTSoVITSError, "loopback"):
            GPTSoVITSClient(
                endpoint="https://voice.example.com:9880",
                urlopen=lambda *args, **kwargs: calls.append((args, kwargs)),
            )

        self.assertEqual(calls, [])

    def test_success_selects_weights_then_posts_official_tts_payload_and_provenance(self) -> None:
        calls = []
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            kwargs = self.synthesize_kwargs(root, inputs)
            client = GPTSoVITSClient(
                endpoint="http://127.0.0.1:9880/",
                timeout_sec=45,
                urlopen=self.successful_urlopen(calls),
            )

            result = client.synthesize(**kwargs)

            self.assertEqual(
                [urllib.parse.urlparse(call[0].full_url).path for call in calls],
                ["/set_gpt_weights", "/set_sovits_weights", "/tts"],
            )
            self.assertEqual([call[0].get_method() for call in calls], ["GET", "GET", "POST"])
            self.assertEqual({call[1] for call in calls}, {45.0})
            self.assertEqual(
                urllib.parse.parse_qs(urllib.parse.urlparse(calls[0][0].full_url).query)["weights_path"],
                [str(inputs["gpt_weights"].resolve())],
            )
            self.assertEqual(
                urllib.parse.parse_qs(urllib.parse.urlparse(calls[1][0].full_url).query)["weights_path"],
                [str(inputs["sovits_weights"].resolve())],
            )
            payload = json.loads(calls[2][0].data.decode("utf-8"))
            self.assertEqual(payload["text"], kwargs["text"])
            self.assertEqual(payload["text_lang"], "zh")
            self.assertEqual(payload["ref_audio_path"], str(inputs["ref_audio"].resolve()))
            self.assertEqual(payload["prompt_text"], kwargs["prompt_text"])
            self.assertEqual(payload["prompt_lang"], "zh")
            self.assertEqual(payload["media_type"], "wav")
            self.assertFalse(payload["streaming_mode"])
            self.assertFalse(payload["parallel_infer"])
            self.assertEqual(payload["seed"], 24680)

            output_path = pathlib.Path(result["audioPath"])
            self.assertEqual(output_path, pathlib.Path(kwargs["output_path"]).resolve())
            self.assertTrue(output_path.is_file())
            self.assertEqual(result["provider"], "gpt_sovits_local")
            self.assertEqual(result["voiceId"], kwargs["voice_id"])
            self.assertEqual(result["tts_provider"], "gpt_sovits_local")
            self.assertEqual(result["voice_id"], kwargs["voice_id"])
            self.assertEqual(result["endpoint"], "http://127.0.0.1:9880")
            self.assertEqual(result["modelIdentifier"], "GPT-SoVITS/api_v2")
            self.assertEqual(result["modelVersion"], kwargs["model_version"])
            self.assertEqual(result["seed"], 24680)
            self.assertEqual(result["settings"]["seed"], 24680)
            self.assertEqual(result["settings"]["media_type"], "wav")
            self.assertFalse(result["settings"]["streaming_mode"])
            self.assertFalse(result["settings"]["parallel_infer"])
            self.assertNotIn("prompt_text", result["settings"])
            self.assertNotIn("ref_audio_path", result["settings"])
            self.assertEqual(
                result["referenceAudioSha256"],
                hashlib.sha256(inputs["ref_audio"].read_bytes()).hexdigest(),
            )
            self.assertEqual(
                result["gptWeightsSha256"],
                hashlib.sha256(inputs["gpt_weights"].read_bytes()).hexdigest(),
            )
            self.assertEqual(
                result["sovitsWeightsSha256"],
                hashlib.sha256(inputs["sovits_weights"].read_bytes()).hexdigest(),
            )
            self.assertEqual(
                result["generatedFileSha256"],
                hashlib.sha256(output_path.read_bytes()).hexdigest(),
            )
            self.assertTrue(result["productionReady"])

    def test_preflight_validates_bundle_without_claiming_generated_production_audio(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            kwargs = self.synthesize_kwargs(root, inputs)
            preflight_kwargs = {
                key: value
                for key, value in kwargs.items()
                if key not in {"text", "text_lang", "output_path"}
            }
            client = GPTSoVITSClient(
                urlopen=lambda *_args, **_kwargs: self.fail("preflight must not use HTTP")
            )

            result = client.preflight(**preflight_kwargs)

            self.assertTrue(result["bundleReady"])
            self.assertFalse(result["productionReady"])
            self.assertNotIn("audioPath", result)
            self.assertNotIn("generatedFileSha256", result)

    def test_expands_environment_variables_in_private_bundle_paths(self) -> None:
        calls = []
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            kwargs = self.synthesize_kwargs(root, inputs)
            kwargs.update(
                {
                    "ref_audio_path": "$GPT_SOVITS_HOME/reference.wav",
                    "gpt_weights_path": "$GPT_SOVITS_HOME/voice.ckpt",
                    "sovits_weights_path": "$GPT_SOVITS_HOME/voice.pth",
                }
            )
            client = GPTSoVITSClient(urlopen=self.successful_urlopen(calls))

            with mock.patch.dict(os.environ, {"GPT_SOVITS_HOME": str(root)}):
                result = client.synthesize(**kwargs)

            self.assertTrue(result["productionReady"])
            payload = json.loads(calls[2][0].data.decode("utf-8"))
            self.assertEqual(payload["ref_audio_path"], str(inputs["ref_audio"].resolve()))
            self.assertEqual(
                urllib.parse.parse_qs(urllib.parse.urlparse(calls[0][0].full_url).query)[
                    "weights_path"
                ],
                [str(inputs["gpt_weights"].resolve())],
            )

    def test_rejects_missing_output_path_before_http(self) -> None:
        calls = []
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            client = GPTSoVITSClient(
                urlopen=lambda *args, **kwargs: calls.append((args, kwargs))
            )

            with self.assertRaisesRegex(GPTSoVITSError, "output path is required"):
                client.synthesize(**(self.synthesize_kwargs(root, inputs) | {"output_path": ""}))

            self.assertEqual(calls, [])

    def test_requires_explicit_verified_prompt_text(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            base = self.synthesize_kwargs(root, inputs)
            client = GPTSoVITSClient(urlopen=lambda *_args, **_kwargs: self.fail("HTTP must not run"))
            invalid = [
                {"prompt_text": ""},
                {"prompt_text": "unverified transcript", "prompt_text_verified": False},
            ]

            for override in invalid:
                with self.subTest(override=override), self.assertRaisesRegex(
                    GPTSoVITSError, "verified prompt text"
                ):
                    client.synthesize(**(base | override))

    def test_rejects_reference_and_checkpoint_hash_mismatch_before_http(self) -> None:
        expected_fields = [
            "expected_reference_audio_sha256",
            "expected_gpt_weights_sha256",
            "expected_sovits_weights_sha256",
        ]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            base = self.synthesize_kwargs(root, inputs)
            for field in expected_fields:
                calls = []
                client = GPTSoVITSClient(urlopen=lambda *args, **kwargs: calls.append((args, kwargs)))
                with self.subTest(field=field), self.assertRaisesRegex(GPTSoVITSError, "hash mismatch"):
                    client.synthesize(**(base | {field: "0" * 64}))
                self.assertEqual(calls, [])

    def test_rejects_empty_or_non_wav_output_without_replacing_destination(self) -> None:
        bad_outputs = [b"", b"not a wav"]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            for index, bad_output in enumerate(bad_outputs):
                output_path = root / f"bad-{index}.wav"
                output_path.write_bytes(b"existing output")

                def fake_urlopen(request, timeout, body=bad_output):
                    if urllib.parse.urlparse(request.full_url).path == "/tts":
                        return FakeResponse(body, content_type="audio/wav")
                    return FakeResponse(b'{"message":"success"}')

                client = GPTSoVITSClient(urlopen=fake_urlopen)
                with self.subTest(index=index), self.assertRaisesRegex(GPTSoVITSError, "valid nonempty WAV"):
                    client.synthesize(
                        **self.synthesize_kwargs(root, inputs, output_name=output_path.name)
                    )
                self.assertEqual(output_path.read_bytes(), b"existing output")

    def test_wraps_timeout_and_http_errors(self) -> None:
        failures = [
            (TimeoutError("timed out"), "timed out"),
            (
                urllib.error.HTTPError(
                    "http://127.0.0.1:9880/set_gpt_weights",
                    503,
                    "unavailable",
                    {},
                    io.BytesIO(b'{"message":"model unavailable"}'),
                ),
                "HTTP 503",
            ),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            kwargs = self.synthesize_kwargs(root, inputs)
            for failure, expected in failures:
                client = GPTSoVITSClient(urlopen=lambda *_args, error=failure, **_kwargs: (_ for _ in ()).throw(error))
                with self.subTest(failure=failure), self.assertRaisesRegex(GPTSoVITSError, expected):
                    client.synthesize(**kwargs)

    def test_serializes_weight_switch_and_tts_sequence_per_endpoint(self) -> None:
        active = 0
        max_active = 0
        guard = threading.Lock()
        start = threading.Event()
        errors = []

        def slow_urlopen(request, timeout):
            nonlocal active, max_active
            with guard:
                active += 1
                max_active = max(max_active, active)
            try:
                time.sleep(0.02)
                if urllib.parse.urlparse(request.full_url).path == "/tts":
                    return FakeResponse(wav_bytes(), content_type="audio/wav")
                return FakeResponse(b'{"message":"success"}')
            finally:
                with guard:
                    active -= 1

        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)

            def worker(index: int) -> None:
                try:
                    start.wait()
                    GPTSoVITSClient(urlopen=slow_urlopen).synthesize(
                        **self.synthesize_kwargs(root, inputs, output_name=f"thread-{index}.wav")
                    )
                except Exception as exc:  # pragma: no cover - asserted below.
                    errors.append(exc)

            threads = [threading.Thread(target=worker, args=(index,)) for index in range(2)]
            for thread in threads:
                thread.start()
            start.set()
            for thread in threads:
                thread.join(timeout=5)

            self.assertEqual(errors, [])
            self.assertTrue(all(not thread.is_alive() for thread in threads))
            self.assertEqual(max_active, 1)

    def test_rejects_bundle_files_changed_during_synthesis(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            inputs = self.make_inputs(root)
            output_path = root / "changed.wav"

            def mutating_urlopen(request, timeout):
                if urllib.parse.urlparse(request.full_url).path == "/tts":
                    inputs["gpt_weights"].write_bytes(b"changed checkpoint")
                    return FakeResponse(wav_bytes(), content_type="audio/wav")
                return FakeResponse(b'{"message":"success"}')

            client = GPTSoVITSClient(urlopen=mutating_urlopen)
            with self.assertRaisesRegex(GPTSoVITSError, "changed during synthesis"):
                client.synthesize(
                    **self.synthesize_kwargs(root, inputs, output_name=output_path.name)
                )

            self.assertFalse(output_path.exists())


if __name__ == "__main__":
    unittest.main()
