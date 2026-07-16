#!/usr/bin/env python3
"""Contract tests for the formal front-facing AIOS talking-head demo."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import front_talking_demo as demo


class FrontTalkingDemoTests(unittest.TestCase):
    def test_runtime_appended_oral_extrema_stay_inside_source_muzzle(self) -> None:
        try:
            import bpy
            import blender_renderer
        except ImportError:
            self.skipTest("requires Blender's bpy runtime")

        root = Path(__file__).resolve().parents[2]
        studio = root / "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
        master = root / "ip形象/main_ip/models/main-ip-aroll-master-refined.blend"
        bpy.ops.wm.open_mainfile(filepath=str(studio), load_ui=False)
        objects = blender_renderer.append_runtime_master_collection(
            master,
            bpy.context.scene.collection,
        )
        meshes = [obj for obj in objects if obj.type == "MESH"]
        armatures = [obj for obj in objects if obj.type == "ARMATURE"]
        dimensions = blender_renderer.prepare_character(
            meshes,
            target_height=blender_renderer.scene_target_height(
                {"presentationMode": "standing"}
            ),
            preserve_hierarchy=True,
            asset_objects=objects,
        )
        runtime = {
            "useMasterAsset": True,
            "preserveExistingRig": True,
            "rigMode": "auto",
            "enhanceExistingRig": False,
        }
        armature, _stats, bone_map = blender_renderer.choose_character_rig(
            runtime,
            armatures,
            meshes,
            dimensions,
        )
        face = blender_renderer.setup_face(
            runtime,
            dimensions,
            armature,
            meshes,
            bone_map,
        )
        mouth = face["mouth"]
        oral = {
            role: face[role]
            for role in (
                "oral_cavity",
                "upper_teeth",
                "lower_teeth",
                "upper_gum",
                "lower_gum",
                "tongue",
            )
        }
        boundary_indices = sorted(
            set(json.loads(str(mouth["mouth_upper_boundary_indices"])))
            | set(json.loads(str(mouth["mouth_lower_boundary_indices"])))
        )

        def evaluated_world_vertices(obj):
            depsgraph = bpy.context.evaluated_depsgraph_get()
            evaluated = obj.evaluated_get(depsgraph)
            mesh = evaluated.to_mesh()
            try:
                return [evaluated.matrix_world @ vertex.co for vertex in mesh.vertices]
            finally:
                evaluated.to_mesh_clear()

        keys = mouth.data.shape_keys.key_blocks
        jaw = armature.pose.bones[bone_map["jaw"]]
        extrema = {
            "closed": ("Mouth_Rest", 0.0),
            "mbp": ("Mouth_MBP", 0.0),
            "a": ("Mouth_A", 0.25),
            "e": ("Mouth_E", 0.16),
            "o": ("Mouth_O", 0.22),
            "u": ("Mouth_U", 0.18),
        }
        failures = []
        for viseme, (shape_name, jaw_radians) in extrema.items():
            for key in keys:
                if key.name.startswith("Mouth_"):
                    key.value = 1.0 if key.name == shape_name else 0.0
            jaw.rotation_mode = "XYZ"
            jaw.rotation_euler = (jaw_radians, 0.0, 0.0)
            bpy.context.view_layer.update()

            source_vertices = evaluated_world_vertices(mouth)
            boundary = [source_vertices[index] for index in boundary_indices]
            minimum_x = min(point.x for point in boundary)
            maximum_x = max(point.x for point in boundary)
            minimum_z = min(point.z for point in boundary)
            maximum_z = max(point.z for point in boundary)
            margin_x = max((maximum_x - minimum_x) * 0.08, 1e-5)
            margin_z = max((maximum_z - minimum_z) * 0.08, 1e-5)
            front_y = min(point.y for point in boundary)
            depth_margin = max(float(dimensions["depth"]) * 0.008, 1e-5)

            for role, obj in oral.items():
                points = evaluated_world_vertices(obj)
                outside = [
                    point
                    for point in points
                    if not (
                        minimum_x - margin_x <= point.x <= maximum_x + margin_x
                        and minimum_z - margin_z <= point.z <= maximum_z + margin_z
                    )
                ]
                if outside:
                    failures.append(
                        f"{viseme}:{role} has {len(outside)}/{len(points)} vertices "
                        "outside the evaluated source-mouth envelope"
                    )
                if viseme in {"closed", "mbp"}:
                    foreground = [point for point in points if point.y < front_y + depth_margin]
                    if foreground:
                        failures.append(
                            f"{viseme}:{role} has {len(foreground)}/{len(points)} vertices "
                            "in front of the closed source-muzzle occlusion depth"
                        )

        self.assertEqual(failures, [])

    def test_refined_master_appends_into_warm_studio_before_validation(self) -> None:
        try:
            import bpy
            import blender_renderer
        except ImportError:
            self.skipTest("requires Blender's bpy runtime")

        root = Path(__file__).resolve().parents[2]
        studio = root / "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
        master = root / "ip形象/main_ip/models/main-ip-aroll-master-refined.blend"
        bpy.ops.wm.open_mainfile(filepath=str(studio), load_ui=False)

        objects = blender_renderer.append_runtime_master_collection(
            master,
            bpy.context.scene.collection,
        )

        armature = next(obj for obj in objects if obj.type == "ARMATURE")
        self.assertIsNotNone(bpy.context.scene.collection.children.get("IP_Character_Master"))
        self.assertLess(max(armature.matrix_world.to_scale()), 0.02)

    def test_runtime_master_append_rolls_back_datablocks_after_validation_failure(self) -> None:
        try:
            import bpy
            import blender_renderer
        except ImportError:
            self.skipTest("requires Blender's bpy runtime")

        root = Path(__file__).resolve().parents[2]
        studio = root / "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
        master = root / "ip形象/main_ip/models/main-ip-aroll-master-refined.blend"
        bpy.ops.wm.open_mainfile(filepath=str(studio), load_ui=False)
        baseline = {
            "collections": {item.name for item in bpy.data.collections},
            "objects": {item.name for item in bpy.data.objects},
            "actions": {item.name for item in bpy.data.actions},
            "ids": {
                (item.bl_rna.identifier, item.name_full)
                for item in bpy.data.user_map()
            },
        }

        with mock.patch.object(
            blender_renderer.master_asset,
            "validate_master_collection",
            side_effect=RuntimeError("forced validation failure"),
        ):
            with self.assertRaisesRegex(RuntimeError, "forced validation failure"):
                blender_renderer.append_runtime_master_collection(
                    master,
                    bpy.context.scene.collection,
                )

        self.assertEqual(
            {item.name for item in bpy.data.collections}, baseline["collections"]
        )
        self.assertEqual({item.name for item in bpy.data.objects}, baseline["objects"])
        self.assertEqual({item.name for item in bpy.data.actions}, baseline["actions"])
        self.assertEqual(
            {
                (item.bl_rna.identifier, item.name_full)
                for item in bpy.data.user_map()
            },
            baseline["ids"],
        )

    def test_demo_spec_is_publishable_front_talking_aroll(self) -> None:
        spec = demo.DEMO_SPEC

        self.assertGreaterEqual(spec["durationSec"], 15.0)
        self.assertLessEqual(spec["durationSec"], 30.0)
        self.assertEqual(spec["resolution"], {"width": 1920, "height": 1080})
        self.assertEqual(spec["fps"], 30)
        self.assertEqual(spec["profileId"], "talking_head")
        self.assertEqual(spec["presentationMode"], "standing")
        self.assertEqual(spec["cameraPreset"], "front_talking")
        self.assertTrue(spec["actionSequence"])
        self.assertFalse(
            any("Transition" in action for action in spec["actionSequence"])
        )

        broll_duration = sum(
            item["endSec"] - item["startSec"] for item in spec["brollWindows"]
        )
        self.assertLessEqual(broll_duration / spec["durationSec"], 0.20)
        self.assertEqual(demo.validate_demo_spec(spec), [])

    def test_agent_run_payload_carries_ip_shooting_contract(self) -> None:
        payload = demo.build_agent_run_payload("project-front-demo")

        self.assertEqual(payload["domain"], "video_creation")
        self.assertEqual(payload["mode"], "dynamic_agent")
        context = payload["context"]
        self.assertEqual(context["projectId"], "project-front-demo")
        self.assertEqual(context["profileId"], "talking_head")
        self.assertEqual(context["cameraPreset"], "front_talking")
        self.assertEqual(context["aigcProvider"], "disabled")
        self.assertEqual(context["presentationMode"], "standing")
        self.assertEqual(context["actionSequence"], demo.DEMO_SPEC["actionSequence"])
        self.assertEqual(context["targetDurationSec"], demo.DEMO_SPEC["durationSec"])

    def test_project_payload_uses_supported_provider_generation_mode(self) -> None:
        payload = demo.build_project_payload("/tmp/front-demo-project")

        self.assertEqual(payload["mode"], "voice_visual")
        self.assertEqual(payload["generationMode"], "provider_api")
        self.assertEqual(payload["targetDurationSec"], 20)
        self.assertEqual(payload["localPathHint"], "/tmp/front-demo-project")
        self.assertEqual(payload["config"]["cameraPreset"], "front_talking")

    def test_media_probe_validates_publishable_delivery_contract(self) -> None:
        report = demo.validate_media_probe(
            {
                "streams": [
                    {
                        "codec_type": "video",
                        "codec_name": "h264",
                        "width": 1920,
                        "height": 1080,
                        "r_frame_rate": "30/1",
                        "avg_frame_rate": "30/1",
                    },
                    {
                        "codec_type": "audio",
                        "codec_name": "aac",
                        "sample_rate": "48000",
                        "channels": 2,
                    },
                ],
                "format": {"duration": "20.032"},
            }
        )

        self.assertTrue(report["passed"])
        self.assertEqual(report["errors"], [])
        self.assertEqual(report["metrics"]["fps"], 30.0)
        self.assertTrue(report["metrics"]["constantFrameRate"])

    def test_media_probe_rejects_wrong_resolution_and_missing_audio(self) -> None:
        report = demo.validate_media_probe(
            {
                "streams": [
                    {
                        "codec_type": "video",
                        "codec_name": "vp9",
                        "width": 1280,
                        "height": 720,
                        "r_frame_rate": "24/1",
                        "avg_frame_rate": "23/1",
                    }
                ],
                "format": {"duration": "8"},
            }
        )

        self.assertFalse(report["passed"])
        self.assertIn("video must be 1920x1080", report["errors"])
        self.assertIn("audio codec must be AAC", report["errors"])
        self.assertIn("video must use constant 30 fps", report["errors"])

    def test_media_probe_rejects_non_48khz_audio(self) -> None:
        report = demo.validate_media_probe(
            {
                "streams": [
                    {
                        "codec_type": "video",
                        "codec_name": "h264",
                        "width": 1920,
                        "height": 1080,
                        "r_frame_rate": "30/1",
                        "avg_frame_rate": "30/1",
                    },
                    {
                        "codec_type": "audio",
                        "codec_name": "aac",
                        "sample_rate": "44100",
                        "channels": 1,
                    },
                ],
                "format": {"duration": "15.033333"},
            }
        )

        self.assertFalse(report["passed"])
        self.assertIn("audio sample rate must be 48 kHz", report["errors"])

    def test_final_publisher_is_bound_to_signed_off_corrected_artifact(self) -> None:
        self.assertEqual(
            demo.SIGNED_OFF_STAGING_RELATIVE_PATH,
            Path(
                "outputs/final/refined-aroll-evidence/production-oral-fixed/"
                "ip_layer.mp4"
            ),
        )
        self.assertEqual(
            demo.FINAL_VIDEO_RELATIVE_PATH,
            Path("outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4"),
        )
        self.assertEqual(
            demo.SIGNED_OFF_VIDEO_SHA256,
            "0ce20701c6df6498ca471d64f6d3dd5d41c385f8c1387e821798a8e82cadd8c1",
        )

        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            staging = root / demo.SIGNED_OFF_STAGING_RELATIVE_PATH
            staging.parent.mkdir(parents=True)
            staging.write_bytes(b"not-the-signed-off-render")

            with self.assertRaisesRegex(RuntimeError, "signed-off staging video SHA-256"):
                demo.publish_signed_off_video(root)

            self.assertFalse((root / demo.FINAL_VIDEO_RELATIVE_PATH).exists())

    def test_final_publisher_preserves_staging_permissions(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            staging = root / demo.SIGNED_OFF_STAGING_RELATIVE_PATH
            staging.parent.mkdir(parents=True)
            staging.write_bytes(b"signed-off-test-render")
            staging.chmod(0o644)

            with mock.patch.object(
                demo,
                "SIGNED_OFF_VIDEO_SHA256",
                demo._sha256_file(staging),
            ):
                report = demo.publish_signed_off_video(root)

            final = root / demo.FINAL_VIDEO_RELATIVE_PATH
            self.assertTrue(staging.is_file())
            self.assertEqual(final.read_bytes(), staging.read_bytes())
            self.assertEqual(final.stat().st_mode & 0o777, 0o644)
            self.assertEqual(report["finalPath"], str(final.resolve()))

    def test_mastered_voice_compose_offsets_aac_loudness_loss(self) -> None:
        import server

        command = server._build_compose_video_args(
            frames_dir=Path("/tmp/refined-aroll-frames"),
            audio_path="/tmp/refined-aroll-master.wav",
            output_path=Path("/tmp/refined-aroll.mp4"),
            duration_sec=15.06,
            fps=30,
            width=1920,
            height=1080,
            audio_mastered=True,
        )

        self.assertIn(
            "volume=0.4dB,alimiter=limit=0.75:attack=5:release=50:level=false",
            " ".join(command),
        )


if __name__ == "__main__":
    unittest.main(argv=[sys.argv[0]])
