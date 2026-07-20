#!/usr/bin/env python3
"""Tests for the portable default A-roll asset manifest."""

from __future__ import annotations

import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from default_aroll_assets import SCHEMA_VERSION, validate_manifest


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class DefaultArollAssetsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        (self.root / "models").mkdir()
        (self.root / "scenes").mkdir()
        self.character = self.root / "models/character.blend"
        self.studio = self.root / "scenes/studio.blend"
        self.character.write_bytes(b"character-master")
        self.studio.write_bytes(b"studio-template")
        self.manifest_path = self.root / "default-aroll-assets.json"

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def _manifest(self) -> dict[str, object]:
        return {
            "schemaVersion": SCHEMA_VERSION,
            "characterId": "main_ip_sloth",
            "releaseId": "test-release",
            "assets": {
                "characterMaster": {
                    "path": "models/character.blend",
                    "sha256": _sha256(self.character),
                },
                "studioTemplate": {
                    "path": "scenes/studio.blend",
                    "sha256": _sha256(self.studio),
                },
            },
        }

    def _validate(self, manifest: dict[str, object]) -> dict[str, object]:
        self.manifest_path.write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2),
            encoding="utf-8",
        )
        return validate_manifest(self.manifest_path, repo_root=self.root)

    def test_valid_manifest_succeeds(self) -> None:
        result = self._validate(self._manifest())

        self.assertTrue(result["success"])
        self.assertEqual(result["errors"], [])
        self.assertEqual(
            set(result["assets"]),
            {"characterMaster", "studioTemplate"},
        )
        self.assertEqual(
            result["assets"]["characterMaster"]["sha256"],
            _sha256(self.character),
        )

    def test_schema_is_fail_closed(self) -> None:
        manifest = self._manifest()
        manifest["schemaVersion"] = "unknown/v1"

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertIn(f"schemaVersion must be {SCHEMA_VERSION}", result["errors"])

    def test_absolute_asset_path_is_rejected(self) -> None:
        manifest = self._manifest()
        manifest["assets"]["characterMaster"]["path"] = str(self.character)

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertIn(
            "characterMaster.path must be repository-relative",
            result["errors"],
        )

    def test_parent_traversal_outside_repo_is_rejected(self) -> None:
        manifest = self._manifest()
        manifest["assets"]["studioTemplate"]["path"] = "../studio.blend"

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertIn(
            "studioTemplate.path resolves outside repository root",
            result["errors"],
        )

    def test_missing_asset_is_rejected(self) -> None:
        manifest = self._manifest()
        self.studio.unlink()

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertTrue(
            any(error.startswith("studioTemplate.path does not exist:") for error in result["errors"])
        )

    def test_hash_mismatch_is_rejected(self) -> None:
        manifest = self._manifest()
        manifest["assets"]["characterMaster"]["sha256"] = "0" * 64

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertIn("characterMaster.sha256 does not match file", result["errors"])

    def test_character_and_studio_must_be_distinct_files(self) -> None:
        manifest = self._manifest()
        manifest["assets"]["studioTemplate"] = dict(
            manifest["assets"]["characterMaster"]
        )

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertIn(
            "characterMaster and studioTemplate must resolve to different files",
            result["errors"],
        )

    def test_missing_required_role_is_rejected(self) -> None:
        manifest = self._manifest()
        del manifest["assets"]["studioTemplate"]

        result = self._validate(manifest)

        self.assertFalse(result["success"])
        self.assertIn("assets.studioTemplate is required", result["errors"])


if __name__ == "__main__":
    unittest.main()
