"""Fail-closed stdlib client for the official GPT-SoVITS api_v2 server."""

from __future__ import annotations

import hashlib
import io
import ipaddress
import json
import os
import socket
import tempfile
import threading
import urllib.error
import urllib.parse
import urllib.request
import wave
from pathlib import Path
from typing import Any, Callable


DEFAULT_ENDPOINT = "http://127.0.0.1:9880"
MODEL_IDENTIFIER = "GPT-SoVITS/api_v2"
PROVIDER = "gpt_sovits_local"
DEFAULT_INFERENCE_SETTINGS: dict[str, Any] = {
    "top_k": 15,
    "top_p": 1.0,
    "temperature": 1.0,
    "text_split_method": "cut5",
    "batch_size": 1,
    "batch_threshold": 0.75,
    "split_bucket": True,
    "speed_factor": 1.0,
    "fragment_interval": 0.3,
    "media_type": "wav",
    "streaming_mode": False,
    "parallel_infer": False,
    "repetition_penalty": 1.35,
    "sample_steps": 32,
    "super_sampling": False,
    "overlap_length": 2,
    "min_chunk_length": 16,
}
_ALLOWED_INFERENCE_SETTINGS = frozenset(DEFAULT_INFERENCE_SETTINGS)
_ENDPOINT_LOCKS: dict[str, threading.Lock] = {}
_ENDPOINT_LOCKS_GUARD = threading.Lock()


class GPTSoVITSError(RuntimeError):
    """Raised when a local GPT-SoVITS request cannot prove production provenance."""


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _endpoint_lock(endpoint: str) -> threading.Lock:
    with _ENDPOINT_LOCKS_GUARD:
        return _ENDPOINT_LOCKS.setdefault(endpoint, threading.Lock())


def _normalize_endpoint(raw_endpoint: str, *, allow_remote: bool) -> str:
    raw = str(raw_endpoint or DEFAULT_ENDPOINT).strip()
    parsed = urllib.parse.urlsplit(raw)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise GPTSoVITSError("GPT-SoVITS endpoint must be an absolute HTTP(S) URL")
    if parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise GPTSoVITSError("GPT-SoVITS endpoint must not contain credentials, query, or fragment")
    if parsed.path not in {"", "/"}:
        raise GPTSoVITSError("GPT-SoVITS endpoint must not contain a path")

    hostname = parsed.hostname.rstrip(".").lower()
    is_loopback = hostname == "localhost"
    if not is_loopback:
        try:
            is_loopback = ipaddress.ip_address(hostname).is_loopback
        except ValueError:
            is_loopback = False
    if not allow_remote and not is_loopback:
        raise GPTSoVITSError(
            "GPT-SoVITS endpoint must be loopback unless allow_remote is explicitly enabled"
        )
    return urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, "", "", "")).rstrip("/")


def _require_readable_file(raw_path: str, label: str) -> Path:
    raw = str(raw_path or "").strip()
    if not raw:
        raise GPTSoVITSError(f"{label} path is required")
    path = Path(os.path.expandvars(raw)).expanduser().resolve()
    if not path.is_file() or not os.access(path, os.R_OK):
        raise GPTSoVITSError(f"{label} is not a readable file: {path}")
    if path.stat().st_size <= 0:
        raise GPTSoVITSError(f"{label} is empty: {path}")
    return path


def _check_expected_hash(actual: str, expected: str, label: str) -> None:
    normalized = str(expected or "").strip().lower()
    if not normalized:
        return
    if len(normalized) != 64 or any(char not in "0123456789abcdef" for char in normalized):
        raise GPTSoVITSError(f"{label} expected SHA-256 is invalid")
    if actual != normalized:
        raise GPTSoVITSError(f"{label} hash mismatch")


def _validated_settings(seed: int, overrides: dict[str, Any] | None) -> dict[str, Any]:
    try:
        deterministic_seed = int(seed)
    except (TypeError, ValueError) as exc:
        raise GPTSoVITSError("GPT-SoVITS seed must be an integer") from exc
    if deterministic_seed < 0:
        raise GPTSoVITSError("GPT-SoVITS seed must be nonnegative for deterministic production")
    supplied = dict(overrides or {})
    unsupported = sorted(set(supplied) - _ALLOWED_INFERENCE_SETTINGS)
    if unsupported:
        raise GPTSoVITSError(f"unsupported GPT-SoVITS inference settings: {unsupported}")
    settings = dict(DEFAULT_INFERENCE_SETTINGS)
    settings.update(supplied)
    settings["media_type"] = "wav"
    settings["streaming_mode"] = False
    settings["parallel_infer"] = False
    settings["seed"] = deterministic_seed
    return settings


def _validate_wav(data: bytes) -> None:
    if not data:
        raise GPTSoVITSError("GPT-SoVITS output must be a valid nonempty WAV")
    try:
        with wave.open(io.BytesIO(data), "rb") as wav_file:
            valid = (
                wav_file.getnchannels() > 0
                and wav_file.getsampwidth() > 0
                and wav_file.getframerate() > 0
                and wav_file.getnframes() > 0
            )
    except (EOFError, wave.Error) as exc:
        raise GPTSoVITSError("GPT-SoVITS output must be a valid nonempty WAV") from exc
    if not valid:
        raise GPTSoVITSError("GPT-SoVITS output must be a valid nonempty WAV")


def _assert_bundle_files_unchanged(paths: dict[str, Path], metadata: dict[str, Any]) -> None:
    current = {
        "referenceAudioSha256": _sha256_file(paths["reference"]),
        "gptWeightsSha256": _sha256_file(paths["gpt"]),
        "sovitsWeightsSha256": _sha256_file(paths["sovits"]),
    }
    if any(current[key] != metadata[key] for key in current):
        raise GPTSoVITSError("GPT-SoVITS bundle files changed during synthesis")


class GPTSoVITSClient:
    def __init__(
        self,
        endpoint: str = DEFAULT_ENDPOINT,
        *,
        timeout_sec: float = 120.0,
        allow_remote: bool = False,
        urlopen: Callable[..., Any] | None = None,
    ) -> None:
        self.endpoint = _normalize_endpoint(endpoint, allow_remote=bool(allow_remote))
        self.timeout_sec = float(timeout_sec)
        if self.timeout_sec <= 0:
            raise GPTSoVITSError("GPT-SoVITS timeout must be positive")
        self._urlopen = urlopen or urllib.request.urlopen

    def _request(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, str] | None = None,
        json_body: dict[str, Any] | None = None,
    ) -> tuple[bytes, str]:
        url = f"{self.endpoint}{path}"
        if query:
            url = f"{url}?{urllib.parse.urlencode(query)}"
        body = None
        headers = {"Accept": "application/json"}
        if json_body is not None:
            body = json.dumps(json_body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            headers = {
                "Accept": "audio/wav, application/json",
                "Content-Type": "application/json",
            }
        request = urllib.request.Request(url, data=body, headers=headers, method=method)
        try:
            with self._urlopen(request, timeout=self.timeout_sec) as response:
                response_body = response.read()
                status = int(getattr(response, "status", 200))
                content_type = str(getattr(response, "headers", {}).get("Content-Type", ""))
        except urllib.error.HTTPError as exc:
            try:
                detail = exc.read().decode("utf-8", errors="replace").strip()
            except Exception:
                detail = str(exc.reason or exc)
            raise GPTSoVITSError(f"GPT-SoVITS HTTP {exc.code}: {detail or exc.reason}") from exc
        except (TimeoutError, socket.timeout) as exc:
            raise GPTSoVITSError(f"GPT-SoVITS request timed out: {path}") from exc
        except urllib.error.URLError as exc:
            reason = exc.reason
            if isinstance(reason, (TimeoutError, socket.timeout)):
                raise GPTSoVITSError(f"GPT-SoVITS request timed out: {path}") from exc
            raise GPTSoVITSError(f"GPT-SoVITS request failed: {reason}") from exc
        except OSError as exc:
            raise GPTSoVITSError(f"GPT-SoVITS request failed: {exc}") from exc
        if status < 200 or status >= 300:
            raise GPTSoVITSError(f"GPT-SoVITS HTTP {status}: {response_body[:500]!r}")
        return response_body, content_type

    def _prepare_bundle(
        self,
        *,
        ref_audio_path: str,
        prompt_text: str,
        prompt_lang: str,
        prompt_text_verified: bool,
        gpt_weights_path: str,
        sovits_weights_path: str,
        voice_id: str,
        model_version: str,
        seed: int,
        settings: dict[str, Any] | None,
        expected_reference_audio_sha256: str,
        expected_gpt_weights_sha256: str,
        expected_sovits_weights_sha256: str,
    ) -> tuple[dict[str, Any], dict[str, Path]]:
        pinned_voice = str(voice_id or "").strip()
        if not pinned_voice:
            raise GPTSoVITSError("production voice ID must be pinned and nonempty")
        version = str(model_version or "").strip()
        if not version:
            raise GPTSoVITSError("GPT-SoVITS model version is required")
        verified_prompt = str(prompt_text or "").strip()
        if not verified_prompt or not bool(prompt_text_verified):
            raise GPTSoVITSError("nonempty verified prompt text is required")
        normalized_prompt_lang = str(prompt_lang or "").strip().lower()
        if not normalized_prompt_lang:
            raise GPTSoVITSError("GPT-SoVITS prompt language is required")

        paths = {
            "reference": _require_readable_file(ref_audio_path, "reference audio"),
            "gpt": _require_readable_file(gpt_weights_path, "GPT weights"),
            "sovits": _require_readable_file(sovits_weights_path, "SoVITS weights"),
        }
        hashes = {
            "referenceAudioSha256": _sha256_file(paths["reference"]),
            "gptWeightsSha256": _sha256_file(paths["gpt"]),
            "sovitsWeightsSha256": _sha256_file(paths["sovits"]),
        }
        _check_expected_hash(
            hashes["referenceAudioSha256"],
            expected_reference_audio_sha256,
            "reference audio",
        )
        _check_expected_hash(
            hashes["gptWeightsSha256"],
            expected_gpt_weights_sha256,
            "GPT weights",
        )
        _check_expected_hash(
            hashes["sovitsWeightsSha256"],
            expected_sovits_weights_sha256,
            "SoVITS weights",
        )
        inference_settings = _validated_settings(seed, settings)
        metadata = {
            "provider": PROVIDER,
            "voiceId": pinned_voice,
            "tts_provider": PROVIDER,
            "voice_id": pinned_voice,
            **hashes,
            "endpoint": self.endpoint,
            "modelIdentifier": MODEL_IDENTIFIER,
            "modelVersion": version,
            "seed": inference_settings["seed"],
            "settings": inference_settings,
            "bundleReady": True,
            "productionReady": False,
        }
        return metadata, paths

    def preflight(
        self,
        *,
        ref_audio_path: str,
        prompt_text: str,
        prompt_lang: str,
        prompt_text_verified: bool,
        gpt_weights_path: str,
        sovits_weights_path: str,
        voice_id: str,
        model_version: str,
        seed: int = 24680,
        settings: dict[str, Any] | None = None,
        expected_reference_audio_sha256: str = "",
        expected_gpt_weights_sha256: str = "",
        expected_sovits_weights_sha256: str = "",
    ) -> dict[str, Any]:
        metadata, _paths = self._prepare_bundle(
            ref_audio_path=ref_audio_path,
            prompt_text=prompt_text,
            prompt_lang=prompt_lang,
            prompt_text_verified=prompt_text_verified,
            gpt_weights_path=gpt_weights_path,
            sovits_weights_path=sovits_weights_path,
            voice_id=voice_id,
            model_version=model_version,
            seed=seed,
            settings=settings,
            expected_reference_audio_sha256=expected_reference_audio_sha256,
            expected_gpt_weights_sha256=expected_gpt_weights_sha256,
            expected_sovits_weights_sha256=expected_sovits_weights_sha256,
        )
        return metadata

    def synthesize(
        self,
        *,
        text: str,
        text_lang: str,
        ref_audio_path: str,
        prompt_text: str,
        prompt_lang: str,
        prompt_text_verified: bool,
        gpt_weights_path: str,
        sovits_weights_path: str,
        voice_id: str,
        model_version: str,
        output_path: str,
        seed: int = 24680,
        settings: dict[str, Any] | None = None,
        expected_reference_audio_sha256: str = "",
        expected_gpt_weights_sha256: str = "",
        expected_sovits_weights_sha256: str = "",
    ) -> dict[str, Any]:
        synthesis_text = str(text or "").strip()
        language = str(text_lang or "").strip().lower()
        if not synthesis_text:
            raise GPTSoVITSError("GPT-SoVITS synthesis text is required")
        if not language:
            raise GPTSoVITSError("GPT-SoVITS synthesis language is required")
        raw_output_path = str(output_path or "").strip()
        if not raw_output_path:
            raise GPTSoVITSError("GPT-SoVITS output path is required")
        destination = Path(os.path.expandvars(raw_output_path)).expanduser().resolve()
        metadata, paths = self._prepare_bundle(
            ref_audio_path=ref_audio_path,
            prompt_text=prompt_text,
            prompt_lang=prompt_lang,
            prompt_text_verified=prompt_text_verified,
            gpt_weights_path=gpt_weights_path,
            sovits_weights_path=sovits_weights_path,
            voice_id=voice_id,
            model_version=model_version,
            seed=seed,
            settings=settings,
            expected_reference_audio_sha256=expected_reference_audio_sha256,
            expected_gpt_weights_sha256=expected_gpt_weights_sha256,
            expected_sovits_weights_sha256=expected_sovits_weights_sha256,
        )
        prompt_language = str(prompt_lang).strip().lower()
        payload = {
            "text": synthesis_text,
            "text_lang": language,
            "ref_audio_path": str(paths["reference"]),
            "aux_ref_audio_paths": [],
            "prompt_text": str(prompt_text).strip(),
            "prompt_lang": prompt_language,
            **metadata["settings"],
        }

        with _endpoint_lock(self.endpoint):
            _assert_bundle_files_unchanged(paths, metadata)
            self._request(
                "GET",
                "/set_gpt_weights",
                query={"weights_path": str(paths["gpt"])},
            )
            self._request(
                "GET",
                "/set_sovits_weights",
                query={"weights_path": str(paths["sovits"])},
            )
            audio_bytes, _content_type = self._request(
                "POST",
                "/tts",
                json_body=payload,
            )
            _assert_bundle_files_unchanged(paths, metadata)

        _validate_wav(audio_bytes)
        destination.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temp_name = tempfile.mkstemp(
            prefix=f".{destination.name}.",
            suffix=".tmp",
            dir=str(destination.parent),
        )
        temp_path = Path(temp_name)
        try:
            with os.fdopen(descriptor, "wb") as handle:
                handle.write(audio_bytes)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temp_path, destination)
        finally:
            if temp_path.exists():
                temp_path.unlink()

        return {
            "audioPath": str(destination),
            **metadata,
            "generatedFileSha256": hashlib.sha256(audio_bytes).hexdigest(),
            "productionReady": True,
        }
