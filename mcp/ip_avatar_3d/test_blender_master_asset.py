#!/usr/bin/env python3
"""Blender-side contracts for staged refined main-IP master publication."""

from __future__ import annotations

import copy
import hashlib
import json
import struct
import sys
import tempfile
import traceback
import zlib
from pathlib import Path

try:
    import bpy
except ModuleNotFoundError as exc:
    raise RuntimeError("requires Blender's bpy runtime") from exc

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import blender_renderer
import hand_refinement
import master_asset
import render_aroll_master_qa
from test_blender_character_rig import load_enhanced_fbx_character


def build_master_fixture(*, seal: bool = True):
    character_objects, dimensions, armature, _rig_stats, bone_map, _removed = (
        load_enhanced_fbx_character()
    )
    source_hashes = master_asset.source_surface_hashes(character_objects)
    face = blender_renderer.setup_face(
        {
            "characterId": "main_ip_sloth",
            "faceScreenMode": "source",
            "mouthMode": "source_mesh_visemes",
            "facialDetailMode": "rich",
            "facialTopologyMode": "source_retopology",
            "mouthHeightRatio": 0.805,
            "mouthScale": 1.0,
        },
        dimensions,
        armature,
        character_objects,
        bone_map,
    )
    blender_renderer.create_action_library(armature, face, bone_map, 30)
    fixture = {
        "objects": list(dict.fromkeys([*character_objects, *face.values()])),
        "armature": armature,
        "boneMap": bone_map,
        "dimensions": dimensions,
        "sourceSurfaceHashes": source_hashes,
    }
    if seal:
        master_asset.seal_master_capabilities(fixture)
    return fixture


def test_refined_master_requires_connected_oral_and_hand_metadata() -> None:
    fixture = build_master_fixture()
    report = master_asset.validate_master_capabilities(fixture)
    assert report["oralRefinementVersion"] == "continuous_arch_v1"
    assert report["handAestheticVersion"] == "three_digit_refined_v2"
    assert report["legacyToothClusterCount"] == 0
    assert report["tongueConnectedComponents"] == 1


def test_refined_master_reports_live_material_hash_weight_and_action_gates() -> None:
    fixture = build_master_fixture()
    report = master_asset.validate_master_capabilities(fixture)
    assert report["sourceSurfaceHashes"] == fixture["sourceSurfaceHashes"]
    assert len(report["sourceSurfaceHashes"]["uvSha256"]) == 64
    assert len(report["sourceSurfaceHashes"]["materialSha256"]) == 64
    assert report["oralMaterialRoles"] == {
        "oral_cavity": ["IP_OralCavity_Material"],
        "upper_teeth": ["IP_Teeth_Material"],
        "lower_teeth": ["IP_Teeth_Material"],
        "upper_gum": ["IP_Gum_Material"],
        "lower_gum": ["IP_Gum_Material"],
        "tongue": ["IP_Tongue_Material"],
    }
    assert report["handWeightStatistics"]["maxInfluences"] <= 4
    assert report["handWeightStatistics"]["unnormalizedVertices"] == 0
    assert report["handWeightStatistics"]["unweightedVertices"] == 0
    assert report["rigActionGate"] is True
    assert report["capabilityGatesPassed"] is True


def test_refined_master_rejects_source_uv_or_material_mutation_before_sealing() -> None:
    fixture = build_master_fixture(seal=False)
    source = next(
        obj
        for obj in fixture["objects"]
        if obj.type == "MESH" and obj.data.uv_layers and not obj.get("ip_face_topology_role")
    )
    source.data.uv_layers.active.data[0].uv.x += 0.125
    _expect_runtime_error(
        "preserved source UV/material hashes",
        lambda: master_asset.seal_master_capabilities(fixture),
    )

    fixture = build_master_fixture(seal=False)
    source = next(
        obj
        for obj in fixture["objects"]
        if obj.type == "MESH" and obj.data.polygons and not obj.get("ip_face_topology_role")
    )
    replacement = bpy.data.materials.new("QA_Mutated_Source_Material")
    source.data.materials.append(replacement)
    source.data.polygons[0].material_index = len(source.data.materials) - 1
    _expect_runtime_error(
        "preserved source UV/material hashes",
        lambda: master_asset.seal_master_capabilities(fixture),
    )


def _expect_runtime_error(fragment: str, callback) -> None:
    try:
        callback()
    except RuntimeError as exc:
        assert fragment in str(exc), str(exc)
    else:
        raise AssertionError(f"expected RuntimeError containing {fragment!r}")


def test_refined_master_fails_closed_for_live_topology_signature_and_action_damage() -> None:
    fixture = build_master_fixture()
    tongue = next(
        obj for obj in fixture["objects"] if obj.get("ip_face_topology_role") == "tongue"
    )
    tongue.data.vertices.add(1)
    _expect_runtime_error(
        "tongue must be one connected component",
        lambda: master_asset.validate_master_capabilities(fixture),
    )

    fixture = build_master_fixture()
    armature = fixture["armature"]
    hand_objects = [
        obj
        for obj in fixture["objects"]
        if obj.type == "MESH" and not obj.get("ip_face_topology_role")
    ]
    regions = hand_refinement.analyze_three_digit_hands(
        armature, hand_objects, fixture["dimensions"], fixture["boneMap"]
    )
    record = regions["L"][0].records[0]
    hand_mesh = bpy.data.objects[record.object_name]
    hand_mesh.data.vertices[record.vertex_index].co.x += 0.001
    _expect_runtime_error(
        "integrity mismatch",
        lambda: master_asset.validate_master_capabilities(fixture),
    )

    fixture = build_master_fixture()
    action = bpy.data.actions["Gesture_Fist"]
    bpy.data.actions.remove(action)
    _expect_runtime_error(
        "missing required Actions",
        lambda: master_asset.validate_master_capabilities(fixture),
    )


def _canonical_json_sha256(payload: dict) -> str:
    encoded = json.dumps(
        payload, sort_keys=True, separators=(",", ":")
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def _rgba_png(width: int, height: int, pixels: list[tuple[int, int, int, int]]) -> bytes:
    def chunk(kind: bytes, data: bytes) -> bytes:
        return (
            struct.pack(">I", len(data))
            + kind
            + data
            + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
        )

    rows = b"".join(
        b"\x00" + bytes(channel for pixel in pixels[y * width : (y + 1) * width] for channel in pixel)
        for y in range(height)
    )
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(rows, 9))
        + chunk(b"IEND", b"")
    )


PNG_BYTES = _rgba_png(8, 8, [(255, 255, 255, 255)] * 64)


def _qa_mask_fixture(label: str, kind: str) -> tuple[bytes, dict, int, int, set[int]]:
    width = height = 64
    pixels = [(0, 0, 0, 0)] * (width * height)
    hand_indices: set[int] = set()

    def fill(x0: int, y0: int, x1: int, y1: int, color: tuple[int, int, int, int]) -> set[int]:
        indices = {
            y * width + x for y in range(y0, y1 + 1) for x in range(x0, x1 + 1)
        }
        for index in indices:
            pixels[index] = color
        return indices

    dental_pixels = tongue_pixels = 0
    if label in {"A", "E", "O", "U", "Smile", "Surprise"}:
        dental_pixels = len(fill(24, 20, 31, 23, (255, 0, 0, 255)))
        dental_pixels += len(fill(32, 20, 39, 23, (255, 255, 0, 255)))
        tongue_pixels = len(fill(28, 29, 35, 32, (0, 255, 0, 255)))
    if kind in {"hand", "digit"}:
        rectangles = {
            "open": (8, 8, 55, 55),
            "fist": (20, 20, 43, 43),
            "finger_roll_r_1": (10, 10, 49, 53),
            "finger_roll_r_2": (12, 10, 51, 53),
            "finger_roll_r_3": (14, 10, 53, 53),
            "finger_roll_l_1": (14, 10, 53, 53),
            "finger_roll_l_2": (12, 10, 51, 53),
            "finger_roll_l_3": (10, 10, 49, 53),
        }
        hand_indices = fill(*rectangles.get(label, (12, 12, 51, 51)), (0, 0, 255, 255))
    if hand_indices:
        xs = [index % width for index in hand_indices]
        ys = [index // width for index in hand_indices]
        bounds = [min(xs), min(ys), max(xs), max(ys)]
        margin = min(bounds[0], bounds[1], width - 1 - bounds[2], height - 1 - bounds[3])
        hand_frame = {
            "pixelCount": len(hand_indices),
            "bounds": bounds,
            "marginPixels": margin,
            "marginFraction": round(margin / 63.0, 6),
            "borderTouching": margin == 0,
        }
    else:
        hand_frame = {}
    return _rgba_png(width, height, pixels), hand_frame, dental_pixels, tongue_pixels, hand_indices


def _publication_report(staged: Path, capabilities: dict) -> dict:
    staged_sha256 = hashlib.sha256(staged.read_bytes()).hexdigest()
    capability_sha256 = _canonical_json_sha256(capabilities)
    evidence_root = staged.parent / "publication-evidence"
    evidence_root.mkdir(parents=True, exist_ok=True)
    qa_sheet = evidence_root / "qa-sheet.png"
    comparison_sheet = evidence_root / "comparison-sheet.png"
    qa_sheet.write_bytes(PNG_BYTES)
    comparison_sheet.write_bytes(PNG_BYTES)
    qa_sheet_sha256 = hashlib.sha256(qa_sheet.read_bytes()).hexdigest()
    comparison_sha256 = hashlib.sha256(comparison_sheet.read_bytes()).hexdigest()
    samples = []
    hand_masks: dict[str, set[int]] = {}
    for contract_sample in render_aroll_master_qa.QA_SAMPLES:
        label = contract_sample.label
        mask_bytes, hand_frame, dental_pixels, tongue_pixels, hand_indices = (
            _qa_mask_fixture(label, contract_sample.kind)
        )
        sample = {
            "label": label,
            "kind": contract_sample.kind,
            "action": contract_sample.action,
            "camera": contract_sample.camera,
            "frame": contract_sample.frame,
            "path": contract_sample.path,
            "pixelMaskPath": f"masks/{contract_sample.path}",
            "dentalExposure": {"visiblePixelCount": dental_pixels},
            "tongueExposure": {"visiblePixelCount": tongue_pixels},
            "extremaFrameIntersections": [],
        }
        if contract_sample.kind in {"hand", "digit"}:
            sample["handPixelFrame"] = hand_frame
            hand_masks[label] = hand_indices
        sample_path = evidence_root / contract_sample.path
        sample_path.parent.mkdir(parents=True, exist_ok=True)
        sample_path.write_bytes(PNG_BYTES)
        sample["sha256"] = hashlib.sha256(sample_path.read_bytes()).hexdigest()
        mask_path = evidence_root / sample["pixelMaskPath"]
        mask_path.parent.mkdir(parents=True, exist_ok=True)
        mask_path.write_bytes(mask_bytes)
        sample["pixelMaskSha256"] = hashlib.sha256(mask_bytes).hexdigest()
        samples.append(sample)

    def difference(first: str, second: str) -> float:
        left = hand_masks[first]
        right = hand_masks[second]
        return round(len(left ^ right) / len(left | right), 6)

    qa_report = {
        "schemaVersion": "tangying-aroll-master-qa/v1",
        "status": "ready",
        "stagedSha256": staged_sha256,
        "capabilityReport": capabilities,
        "capabilityReportSha256": capability_sha256,
        "manifest": render_aroll_master_qa.qa_manifest_payload(),
        "manifestSha256": render_aroll_master_qa.qa_manifest_sha256(),
        "faceCapability": {"blinkCapability": "squint_only"},
        "lighting": {"preset": "qa_editorial_soft"},
        "contract": {
            "sampleCount": len(samples),
            "requiredFiles": [sample["path"] for sample in samples],
            "manifestSha256": render_aroll_master_qa.qa_manifest_sha256(),
        },
        "samples": samples,
        "comparisons": {
            "openFistPixelDifference": difference("open", "fist"),
            "fingerRollPixelDifferences": {
                "r1-r2": difference("finger_roll_r_1", "finger_roll_r_2"),
                "r2-r3": difference("finger_roll_r_2", "finger_roll_r_3"),
                "l1-l2": difference("finger_roll_l_1", "finger_roll_l_2"),
                "l2-l3": difference("finger_roll_l_2", "finger_roll_l_3"),
            },
        },
        "framemd5": {
            "hand": ["1", "2", "3"],
            "face": ["4", "5", "6"],
        },
        "sheets": {
            "qa": {"path": str(qa_sheet), "sha256": qa_sheet_sha256},
            "comparison": {
                "path": str(comparison_sheet),
                "sha256": comparison_sha256,
            },
        },
    }
    qa_report_path = evidence_root / "qa-report.json"
    qa_report_path.write_text(
        json.dumps(qa_report, sort_keys=True, separators=(",", ":")),
        encoding="utf-8",
    )
    inspection_path = evidence_root / "visual-inspection.json"
    master_asset.record_visual_inspection(
        staged,
        qa_report_path,
        "codex-visual-review",
        "2026-07-16T09:00:00+08:00",
        inspection_path,
    )
    report = master_asset.create_publication_report(
        staged,
        qa_report_path,
        inspection_path,
        evidence_root,
    )
    assert (evidence_root / "publication-report.json").is_file()
    return report


def test_atomic_publish_requires_exact_bound_report_and_real_blend() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        staging = root / "staging" / "main-ip-aroll-master-refined.blend"
        final = root / "main-ip-aroll-master-refined.blend"
        approved = root / "main-ip-aroll-master.blend"
        fixture = build_master_fixture()
        saved = master_asset.save_master_collection(
            character_objects=fixture["objects"],
            armature=fixture["armature"],
            output_path=staging,
            refined_intent=True,
            expected_source_surface_hashes=fixture["sourceSurfaceHashes"],
        )
        approved.write_bytes(b"approved-master")
        report = _publication_report(staging, saved["capabilities"])
        qa_report_path = staging.parent / "publication-evidence" / "qa-report.json"
        qa_report_payload = json.loads(qa_report_path.read_text(encoding="utf-8"))
        qa_report_payload["manifestSha256"] = "0" * 64
        qa_report_path.write_text(
            json.dumps(qa_report_payload, sort_keys=True, separators=(",", ":")),
            encoding="utf-8",
        )
        _expect_runtime_error(
            "QA report is not bound",
            lambda: master_asset.create_publication_report(
                staging,
                qa_report_path,
                staging.parent / "publication-evidence" / "visual-inspection.json",
                staging.parent / "forged-evidence",
            ),
        )
        report = _publication_report(staging, saved["capabilities"])
        qa_report_payload = json.loads(qa_report_path.read_text(encoding="utf-8"))
        malformed_sheet = Path(qa_report_payload["sheets"]["qa"]["path"])
        malformed_sheet.write_bytes(b"\x89PNG\r\n\x1a\nnot-an-image")
        qa_report_payload["sheets"]["qa"]["sha256"] = hashlib.sha256(
            malformed_sheet.read_bytes()
        ).hexdigest()
        qa_report_path.write_text(
            json.dumps(qa_report_payload, sort_keys=True, separators=(",", ":")),
            encoding="utf-8",
        )
        _expect_runtime_error(
            "PNG cannot be decoded",
            lambda: master_asset.record_visual_inspection(
                staging,
                qa_report_path,
                "codex-visual-review",
                "2026-07-16T09:00:00+08:00",
                staging.parent / "malformed-inspection.json",
            ),
        )
        report = _publication_report(staging, saved["capabilities"])
        published = master_asset.publish_refined_master(staging, final, report)
        assert published["published"] is True
        assert final.read_bytes() == staging.read_bytes()
        assert approved.read_bytes() == b"approved-master"

        original_staged_bytes = staging.read_bytes()
        race_report = _publication_report(staging, saved["capabilities"])
        original_validator = master_asset._validate_publication_evidence

        def validate_then_replace_staging(*args, **kwargs):
            original_validator(*args, **kwargs)
            staging.write_bytes(b"BLENDER concurrent replacement")

        master_asset._validate_publication_evidence = validate_then_replace_staging
        try:
            raced = master_asset.publish_refined_master(staging, final, race_report)
        finally:
            master_asset._validate_publication_evidence = original_validator
        assert raced["sha256"] == hashlib.sha256(original_staged_bytes).hexdigest()
        assert final.read_bytes() == original_staged_bytes
        assert staging.read_bytes() != original_staged_bytes
        staging.write_bytes(original_staged_bytes)
        report = _publication_report(staging, saved["capabilities"])
        _expect_runtime_error(
            "legacy approved master",
            lambda: master_asset.publish_refined_master(staging, approved, report),
        )
        invented = copy.deepcopy(report)
        invented["publicationEvidence"]["inventedGate"] = {
            "path": "invented.json",
            "sha256": "0" * 64,
        }
        _expect_runtime_error(
            "publication evidence schema",
            lambda: master_asset.publish_refined_master(staging, final, invented),
        )
        missing = copy.deepcopy(report)
        missing["publicationEvidence"].pop("renderQa")
        _expect_runtime_error(
            "publication evidence schema",
            lambda: master_asset.publish_refined_master(staging, final, missing),
        )
        tampered_hash = copy.deepcopy(report)
        tampered_hash["publicationEvidence"]["renderQa"]["sha256"] = "0" * 64
        _expect_runtime_error(
            "evidence SHA-256 mismatch",
            lambda: master_asset.publish_refined_master(
                staging, final, tampered_hash
            ),
        )
        bad_sha = copy.deepcopy(report)
        bad_sha["stagedSha256"] = "0" * 64
        _expect_runtime_error(
            "staged SHA-256 mismatch",
            lambda: master_asset.publish_refined_master(staging, final, bad_sha),
        )
        bad_capabilities = copy.deepcopy(report)
        bad_capabilities["capabilityReport"]["requiredActionCount"] -= 1
        _expect_runtime_error(
            "live capability report mismatch",
            lambda: master_asset.publish_refined_master(
                staging, final, bad_capabilities
            ),
        )
        invalid_visual = copy.deepcopy(report)
        visual_path = Path(
            invalid_visual["publicationEvidence"]["visualInspection"]["path"]
        )
        visual_payload = json.loads(visual_path.read_text(encoding="utf-8"))
        inspection_path = Path(
            visual_payload["details"]["inspectionReportPath"]
        )
        inspection_payload = json.loads(inspection_path.read_text(encoding="utf-8"))
        inspection_payload["reviewer"] = ""
        inspection_path.write_text(
            json.dumps(inspection_payload, sort_keys=True, separators=(",", ":")),
            encoding="utf-8",
        )
        visual_payload["details"]["inspectionReportSha256"] = hashlib.sha256(
            inspection_path.read_bytes()
        ).hexdigest()
        visual_path.write_text(
            json.dumps(visual_payload, sort_keys=True, separators=(",", ":")),
            encoding="utf-8",
        )
        invalid_visual["publicationEvidence"]["visualInspection"]["sha256"] = (
            hashlib.sha256(visual_path.read_bytes()).hexdigest()
        )
        _expect_runtime_error(
            "visual inspection evidence",
            lambda: master_asset.publish_refined_master(
                staging, final, invalid_visual
            ),
        )

        tampered_sample_report = _publication_report(
            staging, saved["capabilities"]
        )
        tampered_sample = (
            staging.parent / "publication-evidence" / "face" / "A.png"
        )
        tampered_sample.write_bytes(b"tampered-render")
        _expect_runtime_error(
            "QA sample PNG SHA-256 mismatch",
            lambda: master_asset.publish_refined_master(
                staging, final, tampered_sample_report
            ),
        )

        fake = root / "staging" / "not-a-blend.blend"
        fake.write_bytes(b"not-a-blend")
        fake_report = _publication_report(fake, saved["capabilities"])
        _expect_runtime_error(
            "Blender file header",
            lambda: master_asset.publish_refined_master(fake, final, fake_report),
        )


def test_refined_save_is_staging_only_and_persists_live_capability_report() -> None:
    fixture = build_master_fixture()
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp) / "models"
        staged = root / "staging" / "main-ip-aroll-master-refined.blend"
        saved = master_asset.save_master_collection(
            character_objects=fixture["objects"],
            armature=fixture["armature"],
            output_path=staged,
            refined_intent=True,
            expected_source_surface_hashes=fixture["sourceSurfaceHashes"],
        )
        assert staged.is_file()
        assert saved["capabilities"]["capabilityGatesPassed"] is True
        collection = bpy.data.collections[master_asset.MASTER_COLLECTION]
        persisted = collection[master_asset.REFINED_CAPABILITY_REPORT_PROPERTY]
        assert json.loads(persisted)["sourceSurfaceHashes"] == saved["capabilities"][
            "sourceSurfaceHashes"
        ]
        del collection[master_asset.REFINED_CAPABILITY_REPORT_PROPERTY]
        _expect_runtime_error(
            "missing refined capability report",
            lambda: master_asset.validate_master_collection(collection, staged),
        )
        collection[master_asset.REFINED_CAPABILITY_REPORT_PROPERTY] = persisted
        oral = {
            role: next(
                obj
                for obj in fixture["objects"]
                if obj.get("ip_face_topology_role") == role
            )
            for role in master_asset.REQUIRED_ORAL_ROLES
        }
        front_y = lambda obj: min(
            float((obj.matrix_world @ vertex.co).y) for vertex in obj.data.vertices
        )
        dental_front = min(front_y(oral[role]) for role in ("upper_teeth", "lower_teeth"))
        assert all(
            front_y(oral[role]) > dental_front for role in ("upper_gum", "lower_gum")
        )
        assert front_y(oral["oral_cavity"]) > max(
            front_y(oral[role])
            for role in ("upper_teeth", "lower_teeth", "upper_gum", "lower_gum")
        )

        fixture = build_master_fixture()
        _expect_runtime_error(
            "staging",
            lambda: master_asset.save_master_collection(
                character_objects=fixture["objects"],
                armature=fixture["armature"],
                output_path=root / "main-ip-aroll-master-refined.blend",
                refined_intent=True,
                expected_source_surface_hashes=fixture["sourceSurfaceHashes"],
            ),
        )


def test_refined_save_intent_rejects_any_missing_or_duplicate_oral_role() -> None:
    for missing_role in master_asset.REQUIRED_ORAL_ROLES:
        fixture = build_master_fixture()
        objects = [
            obj
            for obj in fixture["objects"]
            if obj.get("ip_face_topology_role") != missing_role
        ]
        with tempfile.TemporaryDirectory() as tmp:
            staged = (
                Path(tmp)
                / "models"
                / "staging"
                / "main-ip-aroll-master-refined.blend"
            )
            _expect_runtime_error(
                "oral roles must resolve exactly once",
                lambda: master_asset.save_master_collection(
                    character_objects=objects,
                    armature=fixture["armature"],
                    output_path=staged,
                    refined_intent=True,
                    expected_source_surface_hashes=fixture["sourceSurfaceHashes"],
                ),
            )

    fixture = build_master_fixture()
    tongue = next(
        obj for obj in fixture["objects"] if obj.get("ip_face_topology_role") == "tongue"
    )
    duplicate = tongue.copy()
    duplicate.data = tongue.data.copy()
    duplicate.name = "QA_Duplicate_Tongue"
    with tempfile.TemporaryDirectory() as tmp:
        staged = (
            Path(tmp)
            / "models"
            / "staging"
            / "main-ip-aroll-master-refined.blend"
        )
        _expect_runtime_error(
            "oral roles must resolve exactly once",
            lambda: master_asset.save_master_collection(
                character_objects=[*fixture["objects"], duplicate],
                armature=fixture["armature"],
                output_path=staged,
                refined_intent=True,
                expected_source_surface_hashes=fixture["sourceSurfaceHashes"],
            ),
        )


if __name__ == "__main__":
    tests = [
        test_refined_master_requires_connected_oral_and_hand_metadata,
        test_refined_master_reports_live_material_hash_weight_and_action_gates,
        test_refined_master_rejects_source_uv_or_material_mutation_before_sealing,
        test_refined_master_fails_closed_for_live_topology_signature_and_action_damage,
        test_atomic_publish_requires_exact_bound_report_and_real_blend,
        test_refined_save_is_staging_only_and_persists_live_capability_report,
        test_refined_save_intent_rejects_any_missing_or_duplicate_oral_role,
    ]
    failures = 0
    for test in tests:
        try:
            test()
        except Exception:
            failures += 1
            print(f"FAIL {test.__name__}")
            traceback.print_exc()
        else:
            print(f"PASS {test.__name__}")
    if failures:
        print(f"FAILED {failures} refined-master test(s)")
        sys.exit(1)
