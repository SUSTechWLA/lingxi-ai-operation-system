#!/usr/bin/env python3
"""Blender-side contracts for staged refined main-IP master publication."""

from __future__ import annotations

import copy
import hashlib
import json
import sys
import tempfile
import traceback
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


def _publication_report(staged: Path, capabilities: dict) -> dict:
    return {
        "schemaVersion": "tangying-refined-master-publication/v1",
        "stagedSha256": hashlib.sha256(staged.read_bytes()).hexdigest(),
        "capabilityReport": capabilities,
        "publicationGates": {
            "masterCapabilities": True,
            "sourceSurfacePreservation": True,
            "visemePerformanceQa": True,
            "renderQa": True,
            "poseCollisions": True,
            "comparisonEvidence": True,
            "handPixelMargins": True,
            "visualInspection": True,
        },
    }


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
        published = master_asset.publish_refined_master(staging, final, report)
        assert published["published"] is True
        assert final.read_bytes() == staging.read_bytes()
        assert approved.read_bytes() == b"approved-master"
        _expect_runtime_error(
            "legacy approved master",
            lambda: master_asset.publish_refined_master(staging, approved, report),
        )
        invented = copy.deepcopy(report)
        invented["publicationGates"]["inventedGate"] = True
        _expect_runtime_error(
            "publication gate schema",
            lambda: master_asset.publish_refined_master(staging, final, invented),
        )
        missing = copy.deepcopy(report)
        missing["publicationGates"].pop("renderQa")
        _expect_runtime_error(
            "publication gate schema",
            lambda: master_asset.publish_refined_master(staging, final, missing),
        )
        failed = copy.deepcopy(report)
        failed["publicationGates"]["renderQa"] = False
        _expect_runtime_error(
            "publication gates failed",
            lambda: master_asset.publish_refined_master(staging, final, failed),
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
