#!/usr/bin/env python3
"""Contract tests for the local one-click deploy entrypoint."""

from __future__ import annotations

import os
from pathlib import Path
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "one-click-deploy.sh"
DESKTOP_BUILD_SCRIPT = ROOT / "scripts" / "build-local-desktop.sh"
COMPOSE = ROOT / "cloud-backend" / "docker-compose.yml"
DOCKERIGNORE = ROOT / "cloud-backend" / ".dockerignore"
DOCKERFILE = ROOT / "cloud-backend" / "Dockerfile"


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

    def test_entrypoint_exists_and_is_executable(self) -> None:
        self.assertTrue(SCRIPT.is_file(), "scripts/one-click-deploy.sh is missing")
        self.assertTrue(os.access(SCRIPT, os.X_OK), "one-click deploy entrypoint is not executable")

    def test_help_documents_safe_lifecycle(self) -> None:
        result = self.run_script("--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        for contract in ("up", "status", "down", "--dry-run", "--skip-package"):
            self.assertIn(contract, result.stdout)

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
        ):
            self.assertIn(contract, source)
        self.assertGreaterEqual(source.count("healthcheck:"), 4)
        self.assertNotIn(":latest", source)

    def test_deploy_persists_separate_observability_sealing_key(self) -> None:
        script = SCRIPT.read_text(encoding="utf-8")
        compose = COMPOSE.read_text(encoding="utf-8")
        self.assertIn("OBSERVABILITY_SEALING_KEY", script)
        self.assertIn("OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1", script)
        self.assertIn("openssl rand -hex 32", script)
        self.assertIn("OBSERVABILITY_SEALING_KEY is required", compose)
        self.assertIn("OBSERVABILITY_SEALING_DOMAIN", compose)

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
