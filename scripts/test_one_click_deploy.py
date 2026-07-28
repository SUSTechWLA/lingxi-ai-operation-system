#!/usr/bin/env python3
"""Contract tests for the local one-click deploy entrypoint."""

from __future__ import annotations

import base64
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "one-click-deploy.sh"
DESKTOP_BUILD_SCRIPT = ROOT / "scripts" / "build-local-desktop.sh"
COMPOSE = ROOT / "cloud-backend" / "docker-compose.yml"
DOCKERIGNORE = ROOT / "cloud-backend" / ".dockerignore"
DOCKERFILE = ROOT / "cloud-backend" / "Dockerfile"
SMOKE_SCRIPT = ROOT / "scripts" / "beta-smoke-check.sh"
VALID_SEALING_KEY = (
    "base64:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXpBQkNERUY="
)


class OneClickDeployContractTest(unittest.TestCase):
    def run_script(self, *arguments: str) -> subprocess.CompletedProcess[str]:
        environment = os.environ.copy()
        environment["TANGYING_DEPLOY_TEST"] = "1"
        return subprocess.run(
            ["bash", str(SCRIPT), *arguments],
            cwd=ROOT,
            env=environment,
            text=True,
            capture_output=True,
            check=False,
        )

    def write_runtime_env(
        self,
        target: Path,
        *,
        xtrace: bool = False,
        migration_from: str | None = None,
        environment_overrides: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        trace = "set -x;" if xtrace else ""
        command = (
            f'{trace} script="$1"; target="$2"; migration_from="$3"; '
            'source "$script"; runtime_env="$target"; '
            'observability_source_migration_from="$migration_from"; write_runtime_env'
        )
        environment = os.environ.copy()
        environment["TANGYING_DEPLOY_TEST"] = "1"
        environment["TANGYING_DESKTOP_DATA_ROOT"] = str(target.parent / "desktop-data")
        environment.update(environment_overrides or {})
        return subprocess.run(
            [
                "bash",
                "-c",
                command,
                "_",
                str(SCRIPT),
                str(target),
                migration_from or "",
            ],
            cwd=ROOT,
            env=environment,
            text=True,
            capture_output=True,
            check=False,
        )

    def check_smoke_release_env(
        self,
        *,
        previous: str,
        approval: str,
    ) -> subprocess.CompletedProcess[str]:
        command = r'''
function_source="$(
  awk '
    /^check_release_env\(\)/ { capture = 1 }
    capture { print }
    capture && /^}/ { exit }
  ' "$1"
)"
eval "$function_source"
FAILURES=0
WARNINGS=0
fail() { FAILURES=$((FAILURES + 1)); printf 'FAIL: %s\n' "$*"; }
warn() { WARNINGS=$((WARNINGS + 1)); printf 'WARN: %s\n' "$*"; }
check_release_env
printf 'failure_count=%s\n' "$FAILURES"
[[ "$FAILURES" == "0" ]]
'''
        environment = os.environ.copy()
        environment.update(
            {
                "GIN_MODE": "release",
                "APP_ENV": "production",
                "AUTH_TOKEN_SECRET": "test-auth-secret",
                "OBSERVABILITY_SEALING_KEY": VALID_SEALING_KEY,
                "OBSERVABILITY_SEALING_DOMAIN": "cloud-agent-terminal-v1",
                "OBSERVABILITY_SOURCE_ENVIRONMENT": "production",
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS": previous,
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED": approval,
                "POSTGRES_PASSWORD": "test-postgres-password",
                "MINIO_SECRET_KEY": "test-minio-secret",
                "CORS_ALLOWED_ORIGINS": "http://127.0.0.1:3000",
                "SANDBOX_ENABLED": "true",
                "SANDBOX_ADDRESS": "127.0.0.1:50051",
                "SANDBOX_FALLBACK": "false",
            }
        )
        return subprocess.run(
            ["bash", "-c", command, "_", str(SMOKE_SCRIPT)],
            cwd=ROOT,
            env=environment,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_entrypoint_exists_and_is_executable(self) -> None:
        self.assertTrue(SCRIPT.is_file(), "scripts/one-click-deploy.sh is missing")
        self.assertTrue(os.access(SCRIPT, os.X_OK), "one-click deploy entrypoint is not executable")

    def test_help_documents_safe_lifecycle(self) -> None:
        result = self.run_script("--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        for contract in (
            "up",
            "status",
            "down",
            "--dry-run",
            "--skip-package",
            "--migrate-observability-source-from development",
        ):
            self.assertIn(contract, result.stdout)

    def test_source_migration_option_rejects_missing_unknown_and_duplicate_values(
        self,
    ) -> None:
        cases = (
            (
                ("up", "--dry-run", "--migrate-observability-source-from"),
                "requires a source",
            ),
            (
                (
                    "up",
                    "--dry-run",
                    "--migrate-observability-source-from",
                    "staging",
                ),
                "only supported source is development",
            ),
            (
                (
                    "up",
                    "--dry-run",
                    "--migrate-observability-source-from",
                    "development",
                    "--migrate-observability-source-from",
                    "development",
                ),
                "specified more than once",
            ),
        )
        for arguments, expected_error in cases:
            with self.subTest(arguments=arguments):
                result = self.run_script(*arguments)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(expected_error, result.stderr)

    def test_dry_run_plans_fresh_clone_install_and_package(self) -> None:
        result = self.run_script("up", "--dry-run")
        self.assertEqual(result.returncode, 0, result.stderr)
        for contract in (
            "frontend: npm ci",
            "hyperframes-render-service: npm ci",
            "cloud-backend: go build for linux",
            "docker compose up --detach --build --wait",
            "scripts/build-local-desktop.sh",
            "http://127.0.0.1:8080/api/health/ready",
            "http://127.0.0.1:8787/health",
        ):
            self.assertIn(contract, result.stdout)

    def test_skip_package_dry_run_omits_desktop_build(self) -> None:
        result = self.run_script("up", "--dry-run", "--skip-package")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("scripts/build-local-desktop.sh", result.stdout)
        self.assertIn("docker compose up --detach --build --wait", result.stdout)

    def test_status_dry_run_is_read_only(self) -> None:
        result = self.run_script("status", "--dry-run")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("docker compose ps", result.stdout)
        self.assertIn("http://127.0.0.1:8080/api/health/ready", result.stdout)
        self.assertNotIn("docker compose up", result.stdout)

    def test_script_does_not_kill_unrelated_port_owners(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")
        for unsafe in ("kill -9", "pkill", "lsof -ti"):
            self.assertNotIn(unsafe, source)
        self.assertIn("port is already occupied by a non-Tangying service", source)
        self.assertIn("launchctl submit", source)
        self.assertIn("launchctl remove", source)
        self.assertIn('log "desktop_installer=$desktop_installer"', source)
        self.assertIn("desktop build completed without a packaged .dmg", source)

    def test_compose_runs_cloud_and_data_services_with_host_renderer_bridge(self) -> None:
        source = COMPOSE.read_text(encoding="utf-8")
        for contract in (
            "cloud-backend:",
            "POSTGRES_HOST: postgres",
            "REDIS_HOST: redis",
            "KAFKA_BOOTSTRAP_SERVERS: redpanda:29092",
            "internal://redpanda:29092,external://localhost:9092",
            "MINIO_ENDPOINT: minio:9000",
            "HYPERFRAMES_SERVICE_URL: http://host.docker.internal:8787",
            "AIOS_VIDEO_PIPELINE_ROOT: /app/video-pipelines",
            "host.docker.internal:host-gateway",
            "tangying-project-data:",
            "GIN_MODE: release",
            "APP_ENV: production",
            "OBSERVABILITY_SOURCE_ENVIRONMENT: ${OBSERVABILITY_SOURCE_ENVIRONMENT:-production}",
            "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS: ${OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS:-}",
            "OBSERVABILITY_SOURCE_MIGRATION_APPROVED: ${OBSERVABILITY_SOURCE_MIGRATION_APPROVED:-none}",
            'SANDBOX_ENABLED: "true"',
            'SANDBOX_FALLBACK: "false"',
        ):
            self.assertIn(contract, source)
        self.assertGreaterEqual(source.count("healthcheck:"), 4)
        self.assertNotIn(":latest", source)

    def test_release_smoke_requires_exact_source_migration_marker_pair(self) -> None:
        for previous, approval in (
            ("", "none"),
            ("development", "development->production"),
        ):
            with self.subTest(previous=previous, approval=approval):
                result = self.check_smoke_release_env(
                    previous=previous,
                    approval=approval,
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        for previous, approval, expected_error in (
            (
                "development",
                "none",
                "marker none requires an empty previous source",
            ),
            (
                "",
                "development->production",
                "approved marker requires previous source development",
            ),
            ("", "staging->production", "migration approval marker is invalid"),
        ):
            with self.subTest(previous=previous, approval=approval):
                result = self.check_smoke_release_env(
                    previous=previous,
                    approval=approval,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(expected_error, result.stdout)

    def test_deploy_persists_separate_observability_sealing_key(self) -> None:
        script = SCRIPT.read_text(encoding="utf-8")
        compose = COMPOSE.read_text(encoding="utf-8")
        self.assertIn("OBSERVABILITY_SEALING_KEY", script)
        self.assertIn("OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1", script)
        self.assertIn("OBSERVABILITY_SOURCE_ENVIRONMENT=production", script)
        self.assertIn("base64:$(openssl rand -base64 32", script)
        self.assertIn("OBSERVABILITY_SEALING_KEY is required", compose)
        self.assertIn("OBSERVABILITY_SEALING_DOMAIN", compose)

    def test_runtime_env_is_private_and_existing_valid_key_is_preserved(self) -> None:
        valid_key = "base64:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXpBQkNERUY="
        with tempfile.TemporaryDirectory() as directory:
            fresh = Path(directory) / "fresh.env"
            result = self.write_runtime_env(fresh)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(fresh.stat().st_mode & 0o777, 0o600)
            self.assertEqual(fresh.stat().st_uid, os.getuid())
            generated = next(
                line.split("=", 1)[1]
                for line in fresh.read_text().splitlines()
                if line.startswith("OBSERVABILITY_SEALING_KEY=")
            )
            self.assertTrue(generated.startswith("base64:"))
            decoded = base64.b64decode(
                generated.removeprefix("base64:"), validate=True
            )
            self.assertGreaterEqual(len(decoded), 32)
            self.assertGreaterEqual(len(set(decoded)), 16)
            self.assertIn(
                "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n",
                fresh.read_text(),
            )
            self.assertNotIn(
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\n",
                fresh.read_text(),
            )
            self.assertIn(
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=\n",
                fresh.read_text(),
            )
            self.assertIn(
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\n",
                fresh.read_text(),
            )
            fresh_before_rerun = fresh.read_bytes()
            rerun = self.write_runtime_env(fresh)
            self.assertEqual(rerun.returncode, 0, rerun.stderr)
            self.assertEqual(fresh.read_bytes(), fresh_before_rerun)

            existing = Path(directory) / "existing.env"
            existing.write_text(
                f"OBSERVABILITY_SEALING_KEY={valid_key}\n"
                "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n"
            )
            existing.chmod(0o644)
            result = self.write_runtime_env(existing)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(existing.stat().st_mode & 0o777, 0o600)
            self.assertIn(
                f"OBSERVABILITY_SEALING_KEY={valid_key}\n",
                existing.read_text(),
            )
            self.assertIn(
                "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n",
                existing.read_text(),
            )
            self.assertIn(
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=\n",
                existing.read_text(),
            )
            self.assertIn(
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\n",
                existing.read_text(),
            )
            self.assertNotIn(
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\n",
                existing.read_text(),
            )
            existing_before_rerun = existing.read_bytes()
            rerun = self.write_runtime_env(existing)
            self.assertEqual(rerun.returncode, 0, rerun.stderr)
            self.assertEqual(existing.read_bytes(), existing_before_rerun)

    def test_explicit_development_migration_is_persisted_and_rerun_is_stable(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "round-430.env"
            target.write_text(f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n")
            result = self.write_runtime_env(target, migration_from="development")
            self.assertEqual(result.returncode, 0, result.stderr)
            payload = target.read_text()
            self.assertIn(
                "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n",
                payload,
            )
            self.assertIn(
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\n",
                payload,
            )
            self.assertIn(
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=development->production\n",
                payload,
            )
            self.assertIn(
                "approved observability source migration: development->production",
                result.stdout,
            )
            self.assertNotIn(VALID_SEALING_KEY, result.stdout + result.stderr)

            before_rerun = target.read_bytes()
            rerun = self.write_runtime_env(target)
            self.assertEqual(rerun.returncode, 0, rerun.stderr)
            self.assertEqual(target.read_bytes(), before_rerun)

    def test_source_migration_opt_in_requires_an_existing_sealing_environment(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "missing.env"
            result = self.write_runtime_env(
                target,
                migration_from="development",
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "restore the existing environment and sealing key",
                result.stderr,
            )
            self.assertFalse(target.exists())

    def test_unapproved_legacy_development_value_stops_without_mutation(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "legacy-auto.env"
            target.write_text(
                f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n"
                "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n"
                "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n"
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\n"
            )
            before = target.read_bytes()
            result = self.write_runtime_env(target)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(
                "--migrate-observability-source-from development",
                result.stderr,
            )
            self.assertEqual(target.read_bytes(), before)

            approved = self.write_runtime_env(
                target,
                migration_from="development",
            )
            self.assertEqual(approved.returncode, 0, approved.stderr)
            self.assertIn(
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=development->production\n",
                target.read_text(),
            )

    def test_crlf_upgrade_is_normalized_without_rotating_key(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "crlf.env"
            target.write_bytes(
                (
                    f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\r\n"
                    "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\r\n"
                ).encode()
            )
            result = self.write_runtime_env(target)
            self.assertEqual(result.returncode, 0, result.stderr)
            payload = target.read_bytes()
            self.assertNotIn(b"\r", payload)
            self.assertIn(
                f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n".encode(),
                payload,
            )
            self.assertIn(b"OBSERVABILITY_SOURCE_ENVIRONMENT=production\n", payload)
            self.assertIn(
                b"OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=\n",
                payload,
            )
            self.assertIn(
                b"OBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\n",
                payload,
            )

    def test_duplicate_source_migration_environment_stops_upgrade(self) -> None:
        for duplicate_lines in (
            f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n",
            "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\nOBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n",
            "OBSERVABILITY_SOURCE_ENVIRONMENT=production\nOBSERVABILITY_SOURCE_ENVIRONMENT=production\n",
            "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\nOBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\n",
            "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\nOBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\n",
            "TOOL_REGISTRATION_INTERNAL_TOKEN=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nTOOL_REGISTRATION_INTERNAL_TOKEN=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n",
        ):
            with self.subTest(duplicate_lines=duplicate_lines):
                with tempfile.TemporaryDirectory() as directory:
                    target = Path(directory) / "duplicate.env"
                    target.write_text(
                        f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n"
                        + duplicate_lines
                    )
                    before = target.read_bytes()
                    result = self.write_runtime_env(target)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("duplicated", result.stderr)
                    self.assertEqual(target.read_bytes(), before)

    def test_invalid_current_source_and_domain_stop_without_mutation(self) -> None:
        cases = (
            (
                "OBSERVABILITY_SEALING_DOMAIN=foreign-domain\n"
                "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n",
                "OBSERVABILITY_SEALING_DOMAIN is invalid",
            ),
            (
                "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n"
                "OBSERVABILITY_SOURCE_ENVIRONMENT=development\n",
                "OBSERVABILITY_SOURCE_ENVIRONMENT is invalid",
            ),
        )
        for identity, expected_error in cases:
            with self.subTest(identity=identity):
                with tempfile.TemporaryDirectory() as directory:
                    target = Path(directory) / "invalid-identity.env"
                    target.write_text(
                        f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n"
                        + identity
                    )
                    before = target.read_bytes()
                    result = self.write_runtime_env(target)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(expected_error, result.stderr)
                    self.assertEqual(target.read_bytes(), before)

    def test_ambiguous_previous_source_list_stops_upgrade(self) -> None:
        for previous_environments in (
            ",development",
            "development,",
            "development,,staging",
            "staging",
        ):
            with self.subTest(previous_environments=previous_environments):
                with tempfile.TemporaryDirectory() as directory:
                    target = Path(directory) / "ambiguous.env"
                    target.write_text(
                        f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n"
                        "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n"
                        "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n"
                        "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS="
                        f"{previous_environments}\n"
                    )
                    before = target.read_bytes()
                    result = self.write_runtime_env(target)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("invalid", result.stderr)
                    self.assertEqual(target.read_bytes(), before)

    def test_marker_and_previous_source_must_be_an_exact_pair(self) -> None:
        cases = (
            (
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=development\n"
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\n",
                "marker none requires an empty previous source",
            ),
            (
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=\n"
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=development->production\n",
                "approved marker requires previous source development",
            ),
            (
                "OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=\n"
                "OBSERVABILITY_SOURCE_MIGRATION_APPROVED=staging->production\n",
                "migration approval marker is invalid",
            ),
        )
        for lineage, expected_error in cases:
            with self.subTest(lineage=lineage):
                with tempfile.TemporaryDirectory() as directory:
                    target = Path(directory) / "invalid-lineage.env"
                    target.write_text(
                        f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n"
                        "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n"
                        "OBSERVABILITY_SOURCE_ENVIRONMENT=production\n"
                        + lineage
                    )
                    before = target.read_bytes()
                    result = self.write_runtime_env(target)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(expected_error, result.stderr)
                    self.assertEqual(target.read_bytes(), before)

    def test_crlf_validation_failure_keeps_original_bytes(self) -> None:
        invalid_key = "not-a-canonical-key"
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "invalid-crlf.env"
            target.write_bytes(
                (
                    f"OBSERVABILITY_SEALING_KEY={invalid_key}\r\n"
                    "OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\r\n"
                ).encode()
            )
            before = target.read_bytes()
            result = self.write_runtime_env(target)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(target.read_bytes(), before)
            self.assertNotIn(invalid_key, result.stdout + result.stderr)

    def test_atomic_replacement_failure_keeps_original_bytes_and_secret_private(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "atomic-failure.env"
            target.write_text(f"OBSERVABILITY_SEALING_KEY={VALID_SEALING_KEY}\n")
            before = target.read_bytes()
            result = self.write_runtime_env(
                target,
                migration_from="development",
                xtrace=True,
                environment_overrides={
                    "TANGYING_DEPLOY_TEST_ATOMIC_RENAME_FAIL": "1",
                },
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("atomic runtime environment replacement failed", result.stderr)
            self.assertEqual(target.read_bytes(), before)
            self.assertNotIn(VALID_SEALING_KEY, result.stdout + result.stderr)

    def test_runtime_env_metadata_supports_darwin_and_linux_stat(self) -> None:
        command = r'''
source "$1"
platform="$2"
uname() { printf '%s\n' "$platform"; }
stat() {
  case "$platform:$1:$2" in
    Darwin:-f:%u) printf '501\n' ;;
    Darwin:-f:%g) printf '20\n' ;;
    Darwin:-f:%Lp) printf '600\n' ;;
    Linux:-c:%u) printf '1000\n' ;;
    Linux:-c:%g) printf '1000\n' ;;
    Linux:-c:%a) printf '600\n' ;;
    *) return 97 ;;
  esac
}
printf '%s:%s:%s\n' \
  "$(runtime_env_owner ignored)" \
  "$(runtime_env_group ignored)" \
  "$(runtime_env_mode ignored)"
'''
        for platform, expected in (
            ("Darwin", "501:20:600\n"),
            ("Linux", "1000:1000:600\n"),
        ):
            with self.subTest(platform=platform):
                result = subprocess.run(
                    ["bash", "-c", command, "_", str(SCRIPT), platform],
                    cwd=ROOT,
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, expected)

    def test_invalid_existing_key_stops_without_rotation_or_leak(self) -> None:
        invalid_key = "0123456789abcdef0123456789abcdef"
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "invalid.env"
            target.write_text(f"OBSERVABILITY_SEALING_KEY={invalid_key}\n")
            result = self.write_runtime_env(target)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("do not auto-rotate", result.stderr)
            self.assertNotIn(invalid_key, result.stdout + result.stderr)
            self.assertEqual(
                target.read_text(), f"OBSERVABILITY_SEALING_KEY={invalid_key}\n"
            )

    def test_xtrace_does_not_print_generated_observability_key(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "trace.env"
            result = self.write_runtime_env(target, xtrace=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            generated = next(
                line.split("=", 1)[1]
                for line in target.read_text().splitlines()
                if line.startswith("OBSERVABILITY_SEALING_KEY=")
            )
            self.assertNotIn(generated, result.stdout + result.stderr)

    def test_cloud_runtime_binary_is_included_in_docker_context(self) -> None:
        source = DOCKERIGNORE.read_text(encoding="utf-8")
        self.assertIn("!build/tangying-ai-os", source)
        image = DOCKERFILE.read_text(encoding="utf-8")
        self.assertIn("COPY video-pipelines ./video-pipelines", image)

    def test_desktop_build_uses_a_repo_writable_go_cache_by_default(self) -> None:
        source = DESKTOP_BUILD_SCRIPT.read_text(encoding="utf-8")
        self.assertIn('GOCACHE="${GOCACHE:-$ROOT_DIR/.run/go-build-cache}"', source)
        self.assertIn('export GOCACHE', source)
        self.assertIn('ELECTRON_BUILDER_CACHE="${ELECTRON_BUILDER_CACHE:-$ROOT_DIR/.run/electron-builder-cache}"', source)
        self.assertIn('export ELECTRON_BUILDER_CACHE', source)


if __name__ == "__main__":
    unittest.main()
