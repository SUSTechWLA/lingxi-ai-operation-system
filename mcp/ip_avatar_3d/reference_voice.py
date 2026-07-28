"""Production-only GPT-SoVITS reference voice synthesis and provenance."""

from __future__ import annotations

import hashlib
import json
import math
import os
import re
import shutil
import subprocess
import tempfile
import wave
from pathlib import Path
from typing import Any, Callable

from gpt_sovits_client import GPTSoVITSClient


RESULT_SCHEMA = "tangying-reference-voice-result/v1"
PROVENANCE_SCHEMA = "tangying-production-audio-provenance/v1"
PROVIDER = "gpt_sovits_local"
MASTER_SAMPLE_RATE = 48000
MASTER_CHANNELS = 1
MASTER_CODEC = "pcm_s16le"
TARGET_INTEGRATED_LUFS = -16.0
TARGET_TRUE_PEAK_DBTP = -1.5
LOUDNESS_TOLERANCE = 0.5
PRE_MASTER_FILTER = (
    f"aresample={MASTER_SAMPLE_RATE},"
    "highpass=f=55,lowpass=f=18000,"
    "acompressor=threshold=-20dB:ratio=2.5:attack=15:release=180:knee=2.5:makeup=1"
)
MASTER_FILTER = f"{PRE_MASTER_FILTER},loudnorm=I=-16:TP=-1.5:LRA=7"


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _require_sha256(value: str, label: str) -> str:
    normalized = str(value or "").strip().lower()
    if len(normalized) != 64 or any(char not in "0123456789abcdef" for char in normalized):
        raise ValueError(f"{label} must be a lowercase SHA-256 hex digest")
    return normalized


def _atomic_write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temp_name = tempfile.mkstemp(
        prefix=f".{path.name}.", suffix=".tmp", dir=str(path.parent)
    )
    temp_path = Path(temp_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, ensure_ascii=False, indent=2)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temp_path, path)
    finally:
        if temp_path.exists():
            temp_path.unlink()


def _profile_asset(profile_path: Path, raw: Any) -> str:
    if not raw:
        return ""
    candidate = Path(os.path.expandvars(str(raw))).expanduser()
    if not candidate.is_absolute():
        candidate = profile_path.parent / candidate
    return str(candidate.resolve())


def resolve_gpt_sovits_config(
    profile_path: Path, voice_config: dict[str, Any]
) -> dict[str, Any]:
    raw_config = voice_config.get("gptSovitsLocal") or {}
    if not isinstance(raw_config, dict):
        raise ValueError("voice.gptSovitsLocal must be an object")
    resolved = dict(raw_config)
    for key in ("referenceAudioPath", "gptWeightsPath", "sovitsWeightsPath"):
        if resolved.get(key):
            resolved[key] = _profile_asset(profile_path, resolved[key])
    return resolved


def gpt_sovits_language(raw_language: str) -> str:
    normalized = str(raw_language or "").strip().lower().replace("_", "-")
    if normalized.startswith("zh"):
        return "zh"
    if normalized.startswith("en"):
        return "en"
    if normalized.startswith("ja") or normalized.startswith("jp"):
        return "ja"
    if normalized.startswith("ko"):
        return "ko"
    if not normalized:
        raise ValueError("language is required")
    return normalized


def gpt_sovits_client(config: dict[str, Any]) -> GPTSoVITSClient:
    return GPTSoVITSClient(
        endpoint=str(config.get("endpoint") or "http://127.0.0.1:9880"),
        timeout_sec=float(config.get("timeoutSec") or 120),
        allow_remote=bool(config.get("allowRemoteEndpoint", False)),
    )


def gpt_sovits_bundle_kwargs(
    config: dict[str, Any], *, voice_id: str, speed: float
) -> dict[str, Any]:
    settings = dict(config.get("settings") or {})
    settings["speed_factor"] = speed
    return {
        "ref_audio_path": str(config.get("referenceAudioPath") or ""),
        "prompt_text": str(config.get("promptText") or ""),
        "prompt_lang": gpt_sovits_language(str(config.get("promptLanguage") or "zh")),
        "prompt_text_verified": bool(config.get("promptTextVerified", False)),
        "gpt_weights_path": str(config.get("gptWeightsPath") or ""),
        "sovits_weights_path": str(config.get("sovitsWeightsPath") or ""),
        "voice_id": str(voice_id or "").strip(),
        "model_version": str(config.get("modelVersion") or ""),
        "seed": int(config.get("seed") if config.get("seed") is not None else 24680),
        "settings": settings,
        "expected_reference_audio_sha256": str(config.get("expectedReferenceAudioSha256") or ""),
        "expected_gpt_weights_sha256": str(config.get("expectedGptWeightsSha256") or ""),
        "expected_sovits_weights_sha256": str(config.get("expectedSovitsWeightsSha256") or ""),
    }


def validate_master_wav(path: Path) -> None:
    try:
        with wave.open(str(path), "rb") as wav_file:
            valid = (
                wav_file.getframerate() == MASTER_SAMPLE_RATE
                and wav_file.getnchannels() == MASTER_CHANNELS
                and wav_file.getsampwidth() == 2
                and wav_file.getnframes() > 0
                and wav_file.getcomptype() == "NONE"
            )
    except (EOFError, OSError, wave.Error) as exc:
        raise RuntimeError("production master must be a valid 48 kHz mono PCM16 WAV") from exc
    if not valid:
        raise RuntimeError("production master must be a valid 48 kHz mono PCM16 WAV")


def _run(args: list[str], timeout: int = 180) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        args,
        check=False,
        text=True,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or f"exit {completed.returncode}"
        raise RuntimeError(detail)
    return completed


def _parse_loudnorm(output: str, *, analysis: bool) -> dict[str, float]:
    fields = (
        {
            "input_i": "inputIntegratedLufs",
            "input_tp": "inputTruePeakDbtp",
            "input_lra": "inputLoudnessRangeLu",
            "input_thresh": "inputThresholdLufs",
            "target_offset": "targetOffsetLufs",
        }
        if analysis
        else {
            "input_i": "integratedLufs",
            "input_tp": "truePeakDbtp",
            "input_lra": "loudnessRangeLu",
        }
    )
    for raw in reversed(re.findall(r"\{[^{}]*\}", output or "", flags=re.DOTALL)):
        try:
            data = json.loads(raw)
            parsed = {target: float(data[source]) for source, target in fields.items()}
        except (KeyError, TypeError, ValueError, json.JSONDecodeError):
            continue
        if all(math.isfinite(value) for value in parsed.values()):
            return parsed
    raise RuntimeError("ffmpeg did not return valid loudnorm data")


def master_gpt_sovits_audio(raw_path: Path, output_path: Path) -> tuple[dict[str, float], str]:
    ffmpeg = str(os.environ.get("TANGYING_FFMPEG_BIN") or "").strip() or shutil.which("ffmpeg")
    if not ffmpeg:
        raise RuntimeError("ffmpeg is required for GPT-SoVITS mastering")
    output_path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temp_name = tempfile.mkstemp(
        prefix=f".{output_path.name}.", suffix=".wav", dir=str(output_path.parent)
    )
    os.close(descriptor)
    staging_path = Path(temp_name)
    staging_path.unlink()
    try:
        completed = _run([
            ffmpeg, "-hide_banner", "-nostats", "-i", str(raw_path),
            "-af", f"{MASTER_FILTER}:print_format=json", "-f", "null", "-",
        ])
        analysis = _parse_loudnorm(f"{completed.stdout}\n{completed.stderr}", analysis=True)
        loudnorm_filter = (
            "loudnorm=I=-16:TP=-1.5:LRA=7:"
            f"measured_I={analysis['inputIntegratedLufs']}:"
            f"measured_TP={analysis['inputTruePeakDbtp']}:"
            f"measured_LRA={analysis['inputLoudnessRangeLu']}:"
            f"measured_thresh={analysis['inputThresholdLufs']}:"
            f"offset={analysis['targetOffsetLufs']}:linear=true"
        )
        _run([
            ffmpeg, "-y", "-i", str(raw_path), "-af", f"{PRE_MASTER_FILTER},{loudnorm_filter}",
            "-ar", str(MASTER_SAMPLE_RATE), "-ac", str(MASTER_CHANNELS),
            "-c:a", MASTER_CODEC, str(staging_path),
        ])
        validate_master_wav(staging_path)
        measured = _run([
            ffmpeg, "-hide_banner", "-nostats", "-i", str(staging_path),
            "-af", "loudnorm=I=-16:TP=-1.5:LRA=7:print_format=json", "-f", "null", "-",
        ])
        loudness = _parse_loudnorm(f"{measured.stdout}\n{measured.stderr}", analysis=False)
        if (
            abs(loudness["integratedLufs"] - TARGET_INTEGRATED_LUFS) > LOUDNESS_TOLERANCE
            or loudness["truePeakDbtp"] > TARGET_TRUE_PEAK_DBTP
        ):
            raise RuntimeError("production master is outside the loudness target")
        mastered_hash = _sha256_file(staging_path)
        os.replace(staging_path, output_path)
        return loudness, mastered_hash
    finally:
        if staging_path.exists():
            staging_path.unlink()


def _default_profile_path() -> Path:
    configured = str(os.environ.get("TANGYING_IP_AVATAR_PROFILE") or "").strip()
    if configured:
        return Path(configured).expanduser().resolve()
    return (Path(__file__).resolve().parents[2] / "ip-assets/main-ip/character-profile.json").resolve()


def _load_profile(raw_path: str) -> tuple[Path, dict[str, Any]]:
    path = Path(raw_path).expanduser().resolve() if str(raw_path or "").strip() else _default_profile_path()
    if not path.is_file():
        raise FileNotFoundError(f"characterProfilePath does not exist: {path}")
    profile = json.loads(path.read_text(encoding="utf-8"))
    if profile.get("schemaVersion") != "tangying-ip-character/v1":
        raise ValueError("characterProfilePath must use schemaVersion tangying-ip-character/v1")
    return path, profile


def _duration_seconds(path: Path) -> float:
    with wave.open(str(path), "rb") as wav_file:
        return wav_file.getnframes() / float(wav_file.getframerate())


def _safe_voice_metadata(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict):
        return {}
    allowed = {
        "provider", "voiceId", "tts_provider", "voice_id", "referenceAudioSha256",
        "gptWeightsSha256", "sovitsWeightsSha256", "modelIdentifier", "modelVersion",
        "seed", "settings", "bundleReady", "productionReady", "generatedFileSha256",
    }
    return {key: value for key, value in raw.items() if key in allowed}


def synthesize_reference_voice_service(
    *,
    text: str,
    output_dir: str,
    mode: str = "default_ip",
    character_profile_path: str = "",
    provider: str = PROVIDER,
    voice_id: str = "",
    reference_audio_path: str = "",
    reference_text: str = "",
    reference_text_verified: bool = False,
    usage_rights_confirmed: bool = False,
    language: str = "zh",
    speed: float = 1.0,
    expected_reference_audio_sha256: str = "",
    client_factory: Callable[[dict[str, Any]], Any] = gpt_sovits_client,
    master_audio: Callable[[Path, Path], tuple[dict[str, float], str]] = master_gpt_sovits_audio,
) -> dict[str, Any]:
    synthesis_text = str(text or "").strip()
    if not synthesis_text:
        raise ValueError("text is required")
    normalized_mode = str(mode or "default_ip").strip().lower()
    if normalized_mode not in {"default_ip", "reference_clone"}:
        raise ValueError("mode must be default_ip or reference_clone")
    if str(provider or "").strip().lower() != PROVIDER:
        raise ValueError("provider must be gpt_sovits_local")
    if normalized_mode == "reference_clone":
        if not str(reference_text or "").strip():
            raise ValueError("reference_clone requires exact nonempty referenceText")
        if not bool(reference_text_verified):
            raise ValueError("reference_clone requires referenceTextVerified=true")
        if not bool(usage_rights_confirmed):
            raise ValueError("reference_clone requires usageRightsConfirmed=true")

    profile_path, profile = _load_profile(character_profile_path)
    voice_config = profile.get("voice") or {}
    if not isinstance(voice_config, dict):
        raise ValueError("character profile voice configuration must be an object")
    if str(voice_config.get("provider") or "").strip().lower() != PROVIDER:
        raise ValueError("character profile voice.provider must be gpt_sovits_local")
    config = resolve_gpt_sovits_config(profile_path, voice_config)
    selected_voice_id = str(voice_id or voice_config.get("voiceId") or "").strip()
    if not selected_voice_id:
        raise ValueError("voiceId is required")

    if normalized_mode == "reference_clone":
        reference_path = Path(str(reference_audio_path or "")).expanduser().resolve()
        expected_hash = _require_sha256(expected_reference_audio_sha256, "expectedReferenceAudioSha256")
        if not reference_path.is_file() or _sha256_file(reference_path) != expected_hash:
            raise ValueError("reference audio hash mismatch")
        config["referenceAudioPath"] = str(reference_path)
        config["expectedReferenceAudioSha256"] = expected_hash
        config["promptText"] = str(reference_text).strip()
        config["promptTextVerified"] = True
    else:
        reference_path = Path(str(config.get("referenceAudioPath") or "")).resolve()
        expected_hash = _require_sha256(
            str(config.get("expectedReferenceAudioSha256") or ""),
            "profile expectedReferenceAudioSha256",
        )
        if not reference_path.is_file() or _sha256_file(reference_path) != expected_hash:
            raise ValueError("reference audio hash mismatch")

    output_root = Path(str(output_dir or "")).expanduser().resolve()
    output_root.mkdir(parents=True, exist_ok=True)
    raw_path = (output_root / ".narration_raw.wav").resolve()
    master_path = (output_root / "narration_master.wav").resolve()
    provenance_path = (output_root / "narration_master.provenance.json").resolve()
    for candidate in (raw_path, master_path, provenance_path):
        try:
            candidate.relative_to(output_root)
        except ValueError as exc:
            raise ValueError("output path is outside outputDir") from exc

    effective_speed = max(0.7, min(float(speed or voice_config.get("speed") or 1.0), 1.3))
    client = client_factory(config)
    try:
        voice_result = client.synthesize(
            text=synthesis_text,
            text_lang=gpt_sovits_language(language or str(config.get("textLanguage") or "zh")),
            output_path=str(raw_path),
            **gpt_sovits_bundle_kwargs(config, voice_id=selected_voice_id, speed=effective_speed),
        )
        actual_raw_path = Path(str(voice_result.get("audioPath") or "")).expanduser().resolve()
        try:
            actual_raw_path.relative_to(output_root)
        except ValueError as exc:
            raise ValueError("GPT-SoVITS output is outside outputDir") from exc
        if actual_raw_path != raw_path or not actual_raw_path.is_file():
            raise ValueError("GPT-SoVITS output path does not match the requested project output")
        loudness, mastered_hash = master_audio(actual_raw_path, master_path)
        validate_master_wav(master_path)
        mastered_hash = _sha256_file(master_path)
        duration = _duration_seconds(master_path)
        consent = normalized_mode == "default_ip" or bool(usage_rights_confirmed)
        safe_voice = _safe_voice_metadata(voice_result)
        provenance = {
            "schemaVersion": PROVENANCE_SCHEMA,
            "sourceMode": normalized_mode,
            "provider": PROVIDER,
            "voiceId": selected_voice_id,
            "inputTextSha256": _sha256_text(synthesis_text),
            "referenceAudioSha256": expected_hash,
            "outputFileSha256": mastered_hash,
            "durationSec": round(duration, 6),
            "mastering": {
                "sampleRateHz": MASTER_SAMPLE_RATE,
                "channels": MASTER_CHANNELS,
                "sampleFormat": MASTER_CODEC,
                **loudness,
            },
            "referenceTextVerified": bool(config.get("promptTextVerified")),
            "usageRightsConfirmed": consent,
            "productionReady": True,
            "voice": safe_voice,
        }
        _atomic_write_json(provenance_path, provenance)
        return {
            "schemaVersion": RESULT_SCHEMA,
            "status": "ready",
            "success": True,
            "mode": normalized_mode,
            "provider": PROVIDER,
            "voiceId": selected_voice_id,
            "audioPath": str(master_path),
            "provenancePath": str(provenance_path),
            "durationSec": round(duration, 6),
            "inputTextSha256": provenance["inputTextSha256"],
            "referenceAudioSha256": expected_hash,
            "masteredFileSha256": mastered_hash,
            "productionReady": True,
            "usageRightsConfirmed": consent,
            "voice": safe_voice,
        }
    finally:
        if raw_path.exists():
            raw_path.unlink()
