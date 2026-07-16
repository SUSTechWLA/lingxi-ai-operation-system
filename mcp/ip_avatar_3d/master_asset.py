#!/usr/bin/env python3
"""Stable Blender collection helpers for curated IP character masters."""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import tempfile
from pathlib import Path
from typing import Any, Iterable, Mapping, MutableMapping

try:
    import bpy
except ImportError:  # pragma: no cover - pure validation tests run without Blender.
    bpy = None  # type: ignore[assignment]

try:
    import aroll_actions
    import hand_refinement
    import oral_refinement
except ImportError:  # pragma: no cover - unavailable outside the Blender package path.
    aroll_actions = None  # type: ignore[assignment]
    hand_refinement = None  # type: ignore[assignment]
    oral_refinement = None  # type: ignore[assignment]


MASTER_COLLECTION = "IP_Character_Master"
MASTER_VERSION_PROPERTY = "ip_aroll_master_version"
MASTER_VERSION = 1
REFINED_CAPABILITY_REPORT_PROPERTY = "ip_refined_master_capability_report"
ORAL_DEPTH_CALIBRATION_PROPERTY = "ip_oral_depth_calibration_version"
ORAL_DEPTH_CALIBRATION_VERSION = 1
PUBLICATION_SCHEMA_VERSION = "tangying-refined-master-publication/v1"
PUBLICATION_REPORT_FIELDS = frozenset(
    {"schemaVersion", "stagedSha256", "capabilityReport", "publicationGates"}
)
PUBLICATION_GATE_NAMES = frozenset(
    {
        "masterCapabilities",
        "sourceSurfacePreservation",
        "visemePerformanceQa",
        "renderQa",
        "poseCollisions",
        "comparisonEvidence",
        "handPixelMargins",
        "visualInspection",
    }
)
ORAL_TOPOLOGY_VERSION = "continuous_arch_v1"
REQUIRED_ORAL_ROLES = (
    "oral_cavity",
    "upper_teeth",
    "lower_teeth",
    "upper_gum",
    "lower_gum",
    "tongue",
)
ORAL_MATERIAL_ROLES = {
    "oral_cavity": ["IP_OralCavity_Material"],
    "upper_teeth": ["IP_Teeth_Material"],
    "lower_teeth": ["IP_Teeth_Material"],
    "upper_gum": ["IP_Gum_Material"],
    "lower_gum": ["IP_Gum_Material"],
    "tongue": ["IP_Tongue_Material"],
}
REQUIRED_MASTER_GESTURES = frozenset(
    {
        "Gesture_OpenHand",
        "Gesture_Fist",
        "Gesture_Pinch",
        "Gesture_Count_One",
        "Gesture_Count_Two",
        "Gesture_Count_Three",
        "Gesture_Point_Right",
        "Gesture_Wave",
    }
)
MASTER_QA_CAMERAS = {
    "Camera_Medium": {"lens": 58.0, "distance_scale": 2.1, "target_height": 0.64},
    "Camera_Wide": {"lens": 50.0, "distance_scale": 3.2, "target_height": 0.52},
}


def _require_bpy() -> Any:
    if bpy is None:
        raise RuntimeError("Blender bpy is required for master asset operations")
    return bpy


def _unique_objects(objects: Iterable[Any]) -> list[Any]:
    unique: list[Any] = []
    seen: set[int] = set()
    for obj in objects:
        identity = id(obj)
        if identity in seen:
            continue
        seen.add(identity)
        unique.append(obj)
    return unique


def _require_one_armature(objects: Iterable[Any], source: Path | str) -> Any:
    armatures = [obj for obj in _unique_objects(objects) if getattr(obj, "type", "") == "ARMATURE"]
    if not armatures:
        raise RuntimeError(f"{source} must contain exactly one Armature")
    if len(armatures) > 1:
        names = ", ".join(str(getattr(obj, "name", "<unnamed>")) for obj in armatures)
        raise RuntimeError(f"{source} contains duplicate Armatures: {names}")
    return armatures[0]


def _mesh_component_count(obj: Any) -> int:
    adjacency = {vertex.index: set() for vertex in obj.data.vertices}
    for edge in obj.data.edges:
        first, second = (int(index) for index in edge.vertices)
        adjacency[first].add(second)
        adjacency[second].add(first)
    remaining = set(adjacency)
    components = 0
    while remaining:
        components += 1
        stack = [remaining.pop()]
        while stack:
            current = stack.pop()
            neighbors = adjacency[current] & remaining
            remaining.difference_update(neighbors)
            stack.extend(neighbors)
    return components


def _capability_inputs(
    capabilities: Mapping[str, Any],
) -> tuple[list[Any], Any, dict[str, str], dict[str, Any], list[Any]]:
    objects = _unique_objects(capabilities.get("objects") or ())
    armature = capabilities.get("armature")
    if armature is not None and armature not in objects:
        objects.append(armature)
    armature = _require_one_armature(objects, "refined character master")
    bone_map = dict(capabilities.get("boneMap") or {})
    dimensions = dict(capabilities.get("dimensions") or {})
    if not bone_map or not dimensions:
        raise RuntimeError("refined character master requires boneMap and dimensions")
    hand_objects = [
        obj
        for obj in objects
        if getattr(obj, "type", "") == "MESH"
        and str(obj.get("ip_face_topology_role") or "") not in REQUIRED_ORAL_ROLES
    ]
    return objects, armature, bone_map, dimensions, hand_objects


def _sha256_json(value: Any) -> str:
    encoded = json.dumps(
        value, sort_keys=True, separators=(",", ":"), ensure_ascii=True, allow_nan=False
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _validated_source_surface_hashes(value: Any) -> dict[str, str]:
    if not isinstance(value, Mapping) or set(value) != {
        "uvSha256",
        "materialSha256",
    }:
        raise RuntimeError("refined master is missing preserved source UV/material hashes")
    hashes = {name: str(value[name]) for name in ("uvSha256", "materialSha256")}
    if any(
        len(digest) != 64
        or any(character not in "0123456789abcdef" for character in digest)
        for digest in hashes.values()
    ):
        raise RuntimeError("refined master has invalid preserved source UV/material hashes")
    return hashes


def source_surface_hashes(objects: Iterable[Any]) -> dict[str, str]:
    """Hash live source-mesh UVs and material assignments deterministically."""
    meshes = sorted(
        (
            obj
            for obj in _unique_objects(objects)
            if getattr(obj, "type", "") == "MESH"
            and str(obj.get("ip_face_topology_role") or "") not in REQUIRED_ORAL_ROLES
        ),
        key=lambda obj: obj.name,
    )
    uv_payload = []
    material_payload = []
    for obj in meshes:
        uv_payload.append(
            {
                "object": obj.name,
                "layers": [
                    {
                        "name": layer.name,
                        "coordinates": [
                            [round(float(loop.uv.x), 9), round(float(loop.uv.y), 9)]
                            for loop in layer.data
                        ],
                    }
                    for layer in obj.data.uv_layers
                ],
            }
        )
        material_payload.append(
            {
                "object": obj.name,
                "slots": [material.name if material else "" for material in obj.data.materials],
                "polygonMaterialIndices": [int(polygon.material_index) for polygon in obj.data.polygons],
            }
        )
    if not meshes:
        raise RuntimeError("refined master has no source meshes to hash")
    return {
        "uvSha256": _sha256_json(uv_payload),
        "materialSha256": _sha256_json(material_payload),
    }


def _infer_master_capabilities(objects: list[Any], armature: Any) -> dict[str, Any]:
    from mathutils import Vector

    try:
        bone_map = json.loads(str(armature["ip_avatar_bone_map"]))
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as exc:
        raise RuntimeError("refined character master has no valid ip_avatar_bone_map") from exc
    source_meshes = [
        obj
        for obj in objects
        if getattr(obj, "type", "") == "MESH"
        and str(obj.get("ip_face_topology_role") or "") not in REQUIRED_ORAL_ROLES
    ]
    points = [obj.matrix_world @ Vector(corner) for obj in source_meshes for corner in obj.bound_box]
    if not points:
        raise RuntimeError("refined character master has no source bounds")
    minimum = tuple(min(float(point[axis]) for point in points) for axis in range(3))
    maximum = tuple(max(float(point[axis]) for point in points) for axis in range(3))
    return {
        "objects": objects,
        "armature": armature,
        "boneMap": bone_map,
        "dimensions": {
            "width": maximum[0] - minimum[0],
            "depth": maximum[1] - minimum[1],
            "height": maximum[2] - minimum[2],
        },
    }


def _oral_role_objects(objects: Iterable[Any]) -> dict[str, list[Any]]:
    matches: dict[str, list[Any]] = {role: [] for role in REQUIRED_ORAL_ROLES}
    for obj in objects:
        if getattr(obj, "type", "") != "MESH":
            continue
        role = str(obj.get("ip_face_topology_role") or "")
        if role in matches:
            matches[role].append(obj)
    return matches


def _require_exact_oral_roles(objects: Iterable[Any]) -> dict[str, Any]:
    matches = _oral_role_objects(objects)
    invalid = [role for role, role_objects in matches.items() if len(role_objects) != 1]
    if invalid:
        raise RuntimeError(
            "refined master oral roles must resolve exactly once: " + ", ".join(invalid)
        )
    return {role: role_objects[0] for role, role_objects in matches.items()}


def _front_y(obj: Any) -> float:
    return min(float((obj.matrix_world @ vertex.co).y) for vertex in obj.data.vertices)


def _shift_world_y(obj: Any, distance: float) -> None:
    matrix = obj.matrix_world.copy()
    matrix.translation.y += float(distance)
    obj.matrix_world = matrix


def _calibrate_oral_depth_order(
    oral_objects: Mapping[str, Any],
    dimensions: Mapping[str, Any],
) -> None:
    """Keep teeth in front of gums and the cavity while preserving source surfaces."""
    calibration_versions = {
        role: obj.get(ORAL_DEPTH_CALIBRATION_PROPERTY) for role, obj in oral_objects.items()
    }
    if any(version is not None for version in calibration_versions.values()):
        if not all(
            type(version) is int and version == ORAL_DEPTH_CALIBRATION_VERSION
            for version in calibration_versions.values()
        ):
            raise RuntimeError("refined master has inconsistent oral depth calibration metadata")
        return

    depth = float(dimensions.get("depth", 0.0))
    dental_forward_offset = max(depth * 0.017, 1e-5)
    for role in ("upper_teeth", "lower_teeth"):
        _shift_world_y(oral_objects[role], -dental_forward_offset)

    clearance = max(depth * 0.004, 1e-5)
    dental_front = min(
        _front_y(oral_objects[role]) for role in ("upper_teeth", "lower_teeth")
    )
    for role in ("upper_gum", "lower_gum"):
        gum = oral_objects[role]
        required_front = dental_front + clearance
        current_front = _front_y(gum)
        if current_front < required_front:
            _shift_world_y(gum, required_front - current_front)

    cavity = oral_objects["oral_cavity"]
    foreground_back = max(
        _front_y(oral_objects[role])
        for role in ("upper_teeth", "lower_teeth", "upper_gum", "lower_gum")
    )
    required_cavity_front = foreground_back + clearance
    cavity_front = _front_y(cavity)
    if cavity_front < required_cavity_front:
        _shift_world_y(cavity, required_cavity_front - cavity_front)
    for obj in oral_objects.values():
        obj[ORAL_DEPTH_CALIBRATION_PROPERTY] = ORAL_DEPTH_CALIBRATION_VERSION
    if bpy is not None:
        bpy.context.view_layer.update()


def seal_master_capabilities(capabilities: MutableMapping[str, Any]) -> str:
    """Seal the completed staged build after face Shape Keys have been generated."""
    if hand_refinement is None:
        raise RuntimeError("refined master sealing requires Blender hand refinement")
    _objects, armature, bone_map, dimensions, hand_objects = _capability_inputs(capabilities)
    expected_hashes = _validated_source_surface_hashes(
        capabilities.get("sourceSurfaceHashes")
    )
    if source_surface_hashes(hand_objects) != expected_hashes:
        raise RuntimeError(
            "refined master preserved source UV/material hashes do not match captured values"
        )
    hand_report = hand_refinement._stored_aesthetic_report(armature)
    hand_regions = hand_refinement.analyze_three_digit_hands(
        armature, hand_objects, dimensions, bone_map
    )
    signature = hand_refinement._aesthetic_integrity_signature(
        hand_objects, hand_report, hand_regions
    )
    previous = armature.get(hand_refinement.HAND_AESTHETIC_SIGNATURE_KEY)
    marked_objects = [
        obj
        for obj in hand_objects
        if obj.get(hand_refinement.HAND_CONTRACT_KEY) == hand_refinement.HAND_CONTRACT_NAME
    ]
    armature[hand_refinement.HAND_AESTHETIC_SIGNATURE_KEY] = signature
    for obj in marked_objects:
        obj[hand_refinement.HAND_AESTHETIC_SIGNATURE_KEY] = signature
    try:
        hand_refinement._validate_current_aesthetic_report(
            armature, hand_objects, bone_map, hand_regions, hand_report
        )
    except Exception:
        armature[hand_refinement.HAND_AESTHETIC_SIGNATURE_KEY] = previous
        for obj in marked_objects:
            obj[hand_refinement.HAND_AESTHETIC_SIGNATURE_KEY] = previous
        raise
    return signature


def validate_master_capabilities(capabilities: Mapping[str, Any]) -> dict[str, Any]:
    """Validate refined oral and hand capabilities against live Blender data."""
    if hand_refinement is None or oral_refinement is None or aroll_actions is None:
        raise RuntimeError("refined master validation requires Blender refinement modules")
    objects, armature, bone_map, dimensions, hand_objects = _capability_inputs(capabilities)

    oral_objects = _require_exact_oral_roles(objects)
    stale_versions = [
        role
        for role, obj in oral_objects.items()
        if int(obj.get("ip_oral_refinement_version", 0))
        != int(oral_refinement.ORAL_REFINEMENT_VERSION)
    ]
    if stale_versions:
        raise RuntimeError("refined master has stale oral geometry: " + ", ".join(stale_versions))

    component_counts = {
        role: _mesh_component_count(obj) for role, obj in oral_objects.items()
    }
    legacy_clusters = sum(
        max(0, component_counts[role] - 1) for role in ("upper_teeth", "lower_teeth")
    )
    if legacy_clusters:
        raise RuntimeError(f"refined master contains {legacy_clusters} legacy tooth clusters")
    if component_counts["tongue"] != 1:
        raise RuntimeError("refined master tongue must be one connected component")
    material_roles = {
        role: [material.name for material in obj.data.materials if material is not None]
        for role, obj in oral_objects.items()
    }
    if material_roles != ORAL_MATERIAL_ROLES:
        raise RuntimeError(f"refined master oral material roles are invalid: {material_roles}")

    expected_finger_bones = {
        f"Finger_{digit:02d}_{segment}.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
        for segment in ("Proximal", "Middle", "Distal")
    }
    hand_refinement._validate_reusable_three_segment_hand_rig(
        armature,
        hand_objects,
        expected_finger_bones,
        expected_version=hand_refinement.HAND_CONTRACT_VERSION,
    )
    hand_report = hand_refinement._stored_aesthetic_report(armature)
    hand_regions = hand_refinement.analyze_three_digit_hands(
        armature, hand_objects, dimensions, bone_map
    )
    hand_refinement._validate_current_aesthetic_report(
        armature, hand_objects, bone_map, hand_regions, hand_report
    )
    expected_hashes = _validated_source_surface_hashes(
        capabilities.get("sourceSurfaceHashes")
    )
    current_hashes = source_surface_hashes(hand_objects)
    if current_hashes != expected_hashes:
        raise RuntimeError("refined master source UV/material hashes no longer match live meshes")
    required_actions = set(aroll_actions.AROLL_ACTIONS) | set(REQUIRED_MASTER_GESTURES)
    missing_actions = sorted(name for name in required_actions if bpy.data.actions.get(name) is None)
    if missing_actions:
        raise RuntimeError("refined master missing required Actions: " + ", ".join(missing_actions))

    return {
        "oralRefinementVersion": ORAL_TOPOLOGY_VERSION,
        "oralGeometryVersion": int(oral_refinement.ORAL_REFINEMENT_VERSION),
        "oralComponentCounts": component_counts,
        "legacyToothClusterCount": legacy_clusters,
        "tongueConnectedComponents": component_counts["tongue"],
        "oralMaterialRoles": material_roles,
        "handAestheticVersion": hand_report["handAestheticVersion"],
        "handTopologyVersion": int(armature[hand_refinement.HAND_CONTRACT_VERSION_KEY]),
        "handWeightStatistics": {
            field: hand_report[field]
            for field in (
                "maxInfluences",
                "unnormalizedVertices",
                "unweightedVertices",
                "neighborTipLeakageMax",
            )
        },
        "sourceSurfaceHashes": current_hashes,
        "requiredActionCount": len(required_actions),
        "rigActionGate": True,
        "capabilityGatesPassed": True,
    }


def publish_refined_master(
    staged_path: Path | str,
    final_path: Path | str,
    report: Mapping[str, Any],
) -> dict[str, Any]:
    """Atomically publish a gated refined master without touching the approved legacy master."""
    staged = Path(staged_path).expanduser().resolve()
    final = Path(final_path).expanduser().resolve()
    if final.name == "main-ip-aroll-master.blend":
        raise RuntimeError("refusing to overwrite the legacy approved master")
    if final.name != "main-ip-aroll-master-refined.blend":
        raise RuntimeError(f"unexpected refined master publish target: {final}")
    if not staged.is_file() or staged.suffix.lower() != ".blend":
        raise RuntimeError(f"staged refined master is missing: {staged}")
    with staged.open("rb") as source:
        header = source.read(7)
        if not (
            header == b"BLENDER"
            or header.startswith(b"\x28\xb5\x2f\xfd")
            or header.startswith(b"\x1f\x8b")
        ):
            raise RuntimeError(f"staged refined master has no Blender file header: {staged}")
    if staged.name != "main-ip-aroll-master-refined.blend" or staged.parent.name != "staging":
        raise RuntimeError(
            "refined master must publish from staging/main-ip-aroll-master-refined.blend"
        )
    if staged == final:
        raise RuntimeError("staged and published refined master paths must differ")
    if not isinstance(report, Mapping) or set(report) != PUBLICATION_REPORT_FIELDS:
        raise RuntimeError("refined master publication report schema is invalid")
    if report.get("schemaVersion") != PUBLICATION_SCHEMA_VERSION:
        raise RuntimeError("refined master publication report schema is invalid")
    gates = report.get("publicationGates")
    if not isinstance(gates, Mapping) or set(gates) != PUBLICATION_GATE_NAMES:
        raise RuntimeError("refined master publication gate schema is invalid")
    if not all(gates[name] is True for name in PUBLICATION_GATE_NAMES):
        raise RuntimeError(f"refined master publication gates failed: {gates!r}")
    staged_sha256 = _sha256_file(staged)
    if report.get("stagedSha256") != staged_sha256:
        raise RuntimeError("refined master staged SHA-256 mismatch")

    blender = _require_bpy()
    try:
        blender.ops.wm.open_mainfile(
            filepath=str(staged), load_ui=False, use_scripts=False
        )
    except RuntimeError as exc:
        raise RuntimeError(f"cannot load staged refined master: {staged}") from exc
    validated = validate_master_collection(
        blender.data.collections.get(MASTER_COLLECTION), staged
    )
    live_capabilities = validated.get("capabilities")
    if not isinstance(live_capabilities, Mapping):
        raise RuntimeError("staged refined master has no live capability report")
    if report.get("capabilityReport") != live_capabilities:
        raise RuntimeError("refined master live capability report mismatch")

    final.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{final.stem}.", suffix=".tmp", dir=str(final.parent)
    )
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as target, staged.open("rb") as source:
            shutil.copyfileobj(source, target)
            target.flush()
            os.fsync(target.fileno())
        os.replace(temporary, final)
        directory_descriptor = os.open(final.parent, os.O_RDONLY)
        try:
            os.fsync(directory_descriptor)
        finally:
            os.close(directory_descriptor)
    finally:
        temporary.unlink(missing_ok=True)
    final_sha256 = _sha256_file(final)
    if final_sha256 != staged_sha256:  # pragma: no cover - guarded by the atomic copy.
        raise RuntimeError("published refined master SHA-256 does not match staging")
    return {
        "published": True,
        "stagedPath": str(staged),
        "finalPath": str(final),
        "sha256": final_sha256,
        "capabilityReport": dict(live_capabilities),
    }


def validate_master_collection(collection: Any, master_path: Path | str) -> dict[str, Any]:
    """Fail closed unless a loaded master has supported metadata and one rig."""
    if collection is None or getattr(collection, "name", "") != MASTER_COLLECTION:
        raise RuntimeError(f"missing {MASTER_COLLECTION} in {master_path}")
    version = collection.get(MASTER_VERSION_PROPERTY)
    if version is None:
        raise RuntimeError(f"missing {MASTER_VERSION_PROPERTY} in {master_path}")
    if type(version) is not int:
        raise RuntimeError(
            f"invalid {MASTER_VERSION_PROPERTY}={version!r} in {master_path}; expected an integer"
        )
    resolved_version = version
    if resolved_version != MASTER_VERSION:
        raise RuntimeError(
            f"unsupported {MASTER_VERSION_PROPERTY}={resolved_version} in {master_path}; expected {MASTER_VERSION}"
        )
    objects = list(collection.all_objects)
    armature = _require_one_armature(objects, master_path)
    result = {
        "collection": MASTER_COLLECTION,
        "version": resolved_version,
        "armature": armature,
        "objects": objects,
    }
    persisted_report = collection.get(REFINED_CAPABILITY_REPORT_PROPERTY)
    if persisted_report:
        try:
            expected = json.loads(str(persisted_report))
        except (TypeError, ValueError, json.JSONDecodeError) as exc:
            raise RuntimeError(f"invalid refined capability report in {master_path}") from exc
        capabilities = _infer_master_capabilities(objects, armature)
        capabilities["sourceSurfaceHashes"] = expected.get("sourceSurfaceHashes")
        current = validate_master_capabilities(capabilities)
        if current != expected:
            raise RuntimeError(f"stale refined capability report in {master_path}")
        result["capabilities"] = current
    return result


def ensure_master_collection(character_objects: Iterable[Any], armature: Any) -> Any:
    """Move one complete character hierarchy into the named master collection."""
    blender = _require_bpy()
    objects = _unique_objects([*character_objects, armature])
    selected_armature = _require_one_armature(objects, "character master")
    if selected_armature is not armature:
        raise RuntimeError("character master armature does not match the supplied Armature")
    if blender.data.collections.get(MASTER_COLLECTION) is not None:
        raise RuntimeError(f"duplicate {MASTER_COLLECTION} collection")

    collection = blender.data.collections.new(MASTER_COLLECTION)
    blender.context.scene.collection.children.link(collection)
    for obj in objects:
        if collection.objects.get(obj.name) is None:
            collection.objects.link(obj)
        for owner in list(getattr(obj, "users_collection", [])):
            if owner != collection:
                owner.objects.unlink(obj)
    collection[MASTER_VERSION_PROPERTY] = MASTER_VERSION
    validate_master_collection(collection, "current Blender file")
    return collection


def ensure_master_qa_cameras(armature: Any) -> dict[str, Any]:
    """Create deterministic standalone cameras without adding them to the master collection."""
    blender = _require_bpy()
    from mathutils import Vector

    rig_points = [
        armature.matrix_world @ point
        for bone in armature.data.bones
        for point in (bone.head_local, bone.tail_local)
    ]
    if not rig_points:
        raise RuntimeError("character master Armature has no bones for QA camera framing")

    minimum = Vector(tuple(min(point[index] for point in rig_points) for index in range(3)))
    maximum = Vector(tuple(max(point[index] for point in rig_points) for index in range(3)))
    height = maximum.z - minimum.z
    if height <= 1e-6:
        raise RuntimeError("character master Armature has zero height for QA camera framing")

    center_x = (minimum.x + maximum.x) * 0.5
    center_y = (minimum.y + maximum.y) * 0.5
    cameras: dict[str, Any] = {}
    for name, spec in MASTER_QA_CAMERAS.items():
        camera = blender.data.objects.get(name)
        if camera is not None and camera.type != "CAMERA":
            raise RuntimeError(f"cannot create {name}: name belongs to a non-camera object")
        if camera is None:
            camera_data = blender.data.cameras.new(f"{name}_Data")
            camera = blender.data.objects.new(name, camera_data)
        if blender.context.scene.objects.get(camera.name) is None:
            blender.context.scene.collection.objects.link(camera)

        target = Vector(
            (center_x, center_y, minimum.z + height * float(spec["target_height"]))
        )
        distance = max(0.5, height * float(spec["distance_scale"]))
        camera.location = target + Vector((0.0, -distance, height * 0.025))
        camera.rotation_euler = (target - camera.location).to_track_quat("-Z", "Y").to_euler()
        camera.data.lens = float(spec["lens"])
        camera.data.clip_start = 0.01
        camera.data.clip_end = max(100.0, distance * 10.0)
        cameras[name] = camera

    blender.context.scene.camera = cameras["Camera_Medium"]
    return cameras


def save_master_collection(
    *,
    character_objects: Iterable[Any],
    armature: Any,
    output_path: Path,
    refined_intent: bool | None = None,
    expected_source_surface_hashes: Mapping[str, str] | None = None,
) -> dict[str, Any]:
    """Save a versioned, self-contained character master Blend."""
    objects = _unique_objects([*character_objects, armature])
    _require_one_armature(objects, "character master")
    output_path = Path(output_path).expanduser().resolve()
    if output_path.suffix.lower() != ".blend":
        raise RuntimeError(f"master output must be a .blend file: {output_path}")
    if refined_intent is not None and type(refined_intent) is not bool:
        raise RuntimeError("refined_intent must be an explicit boolean")
    canonical_refined_target = (
        output_path.name == "main-ip-aroll-master-refined.blend"
        and output_path.parent.name == "staging"
    )
    refined = refined_intent is True or (
        refined_intent is None and canonical_refined_target
    )
    capabilities_report: dict[str, Any] | None = None
    capabilities: dict[str, Any] | None = None
    if refined:
        if not canonical_refined_target:
            raise RuntimeError(
                "refined master must be built to staging/main-ip-aroll-master-refined.blend"
            )
        oral_objects = _require_exact_oral_roles(objects)
        capabilities = _infer_master_capabilities(objects, armature)
        _calibrate_oral_depth_order(oral_objects, capabilities["dimensions"])
        capabilities["sourceSurfaceHashes"] = _validated_source_surface_hashes(
            expected_source_surface_hashes
        )
        seal_master_capabilities(capabilities)
        capabilities_report = validate_master_capabilities(capabilities)
    elif output_path.name == "main-ip-aroll-master-refined.blend":
        raise RuntimeError(
            "refined master requires explicit intent and the canonical staging path"
        )
    elif expected_source_surface_hashes is not None:
        raise RuntimeError("source surface hashes are only valid for a refined master build")
    output_path.parent.mkdir(parents=True, exist_ok=True)
    collection = ensure_master_collection(objects, armature)
    collection[MASTER_VERSION_PROPERTY] = MASTER_VERSION
    if capabilities_report is not None:
        collection[REFINED_CAPABILITY_REPORT_PROPERTY] = json.dumps(
            capabilities_report, sort_keys=True, separators=(",", ":")
        )
    ensure_master_qa_cameras(armature)
    validate_master_collection(collection, output_path)
    _require_bpy().ops.wm.save_as_mainfile(filepath=str(output_path))
    result = {
        "collection": collection.name,
        "version": MASTER_VERSION,
        "path": str(output_path),
    }
    if capabilities_report is not None:
        result["capabilities"] = capabilities_report
    return result


def append_master_collection(master_path: Path, scene_collection: Any) -> list[Any]:
    """Append and validate a curated master, including its reusable Actions."""
    blender = _require_bpy()
    master_path = Path(master_path).expanduser().resolve()
    if master_path.suffix.lower() != ".blend" or not master_path.is_file():
        raise RuntimeError(f"invalid master Blend: {master_path}")
    if blender.data.collections.get(MASTER_COLLECTION) is not None:
        raise RuntimeError(f"duplicate {MASTER_COLLECTION} collection while appending {master_path}")

    with blender.data.libraries.load(str(master_path), link=False) as (source, target):
        matches = [name for name in source.collections if name == MASTER_COLLECTION]
        if len(matches) != 1:
            if not matches:
                raise RuntimeError(f"missing {MASTER_COLLECTION} in {master_path}")
            raise RuntimeError(f"duplicate {MASTER_COLLECTION} collections in {master_path}")
        target.collections = [MASTER_COLLECTION]
        target.actions = list(source.actions)

    collection = target.collections[0] if target.collections else None
    validated = validate_master_collection(collection, master_path)
    scene_collection.children.link(collection)
    return list(validated["objects"])
