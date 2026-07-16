#!/usr/bin/env python3
"""Contract tests for the formal front-facing AIOS talking-head demo."""

from __future__ import annotations

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
    def test_runtime_appended_oral_extrema_preserve_proportions_and_render_readably(self) -> None:
        try:
            import bpy
            import blender_renderer
            import render_aroll_master_qa as qa
            from mathutils.bvhtree import BVHTree
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
        native_oral_sizes = {
            str(obj.get("ip_face_topology_role")): tuple(obj.dimensions)
            for obj in meshes
            if str(obj.get("ip_face_topology_role"))
            in {
                "oral_cavity",
                "upper_teeth",
                "lower_teeth",
                "upper_gum",
                "lower_gum",
                "tongue",
            }
        }
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
        for role, obj in oral.items():
            native = native_oral_sizes[role]
            self.assertGreaterEqual(
                float(obj.dimensions.x) / float(native[0]),
                0.95,
                f"{role} runtime fit must preserve dental/tongue width",
            )
            self.assertGreaterEqual(
                float(obj.dimensions.z) / float(native[2]),
                0.95,
                f"{role} runtime fit must preserve dental/tongue thickness",
            )

        def evaluated_tree(obj):
            depsgraph = bpy.context.evaluated_depsgraph_get()
            evaluated = obj.evaluated_get(depsgraph)
            mesh = evaluated.to_mesh(
                preserve_all_data_layers=True,
                depsgraph=depsgraph,
            )
            try:
                vertices = [
                    evaluated.matrix_world @ vertex.co for vertex in mesh.vertices
                ]
                mesh.calc_loop_triangles()
                triangles = [
                    tuple(int(index) for index in triangle.vertices)
                    for triangle in mesh.loop_triangles
                ]
                return BVHTree.FromPolygons(
                    vertices,
                    triangles,
                    all_triangles=True,
                    epsilon=1e-6,
                )
            finally:
                evaluated.to_mesh_clear()

        blender_renderer.create_action_library(armature, face, bone_map, 30)
        scene = bpy.context.scene
        qa._configure_close_cameras(scene, armature, bone_map)
        scene.camera = bpy.data.objects[qa.FACE_CLOSE_CAMERA]
        scene.render.engine = "BLENDER_EEVEE"
        scene.render.resolution_x = 384
        scene.render.resolution_y = 216
        scene.render.resolution_percentage = 100
        scene.render.image_settings.file_format = "PNG"
        scene.render.image_settings.color_mode = "RGBA"
        scene.render.film_transparent = True
        for obj in scene.objects:
            if obj not in objects and obj.type not in {"CAMERA", "LIGHT"}:
                obj.hide_render = True

        shape_owners = qa._shape_key_owners(scene)
        jaw = armature.pose.bones[bone_map["jaw"]]
        extrema = {
            "rest": ("Mouth_Rest", 0.0),
            "mbp": ("Mouth_MBP", 0.0),
            "a": ("Mouth_A", 0.25),
            "e": ("Mouth_E", 0.16),
            "o": ("Mouth_O", 0.22),
            "u": ("Mouth_U", 0.18),
        }
        with tempfile.TemporaryDirectory() as temp_dir:
            for viseme, (shape_name, jaw_radians) in extrema.items():
                qa._reset_armature_pose(scene, armature)
                qa._activate_shape_keys(shape_owners, {shape_name: 1.0})
                jaw.rotation_mode = "XYZ"
                jaw.rotation_euler = (jaw_radians, 0.0, 0.0)
                bpy.context.view_layer.update()
                mask = qa._render_pixel_mask(
                    scene,
                    Path(temp_dir) / f"{viseme}.png",
                    meshes,
                    bone_map,
                    "",
                )["rolePixelCounts"]

                if viseme in {"rest", "mbp"}:
                    self.assertEqual(
                        sum(mask[role] for role in oral),
                        0,
                        f"{viseme} must fully occlude all oral roles",
                    )
                    continue

                self.assertGreaterEqual(mask["oral_cavity"], 150, viseme)
                self.assertGreaterEqual(
                    mask["upper_teeth"] + mask["lower_teeth"],
                    10,
                    f"{viseme} must retain a readable dental region",
                )
                self.assertGreaterEqual(
                    mask["tongue"],
                    6,
                    f"{viseme} must retain a distinct tongue region",
                )
                trees = {role: evaluated_tree(obj) for role, obj in oral.items()}
                for left, right in (
                    ("tongue", "upper_teeth"),
                    ("tongue", "upper_gum"),
                    ("upper_teeth", "lower_teeth"),
                ):
                    self.assertEqual(
                        trees[left].overlap(trees[right]),
                        [],
                        f"{viseme} has a forbidden {left}/{right} intersection",
                    )

    def test_required_gestures_have_front_readable_rendered_hand_silhouettes(self) -> None:
        try:
            import bpy
            import aroll_actions
            import blender_renderer
            import render_aroll_master_qa as qa
            from bpy_extras.object_utils import world_to_camera_view
            from mathutils import Vector
        except ImportError:
            self.skipTest("requires Blender's bpy runtime")

        root = Path(__file__).resolve().parents[2]
        studio = root / "ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
        master = root / "ip形象/main_ip/models/main-ip-aroll-master-refined.blend"
        bpy.ops.wm.open_mainfile(filepath=str(studio), load_ui=False)
        data = {
            "sceneBlendPath": str(studio),
            "presentationMode": "standing",
            "cameraPlan": [{"frame": 1, "camera": "Camera_Medium"}],
            "motionPlan": {"motionEvents": []},
            "fps": 30,
            "durationSec": 15.06,
            "resolution": {"width": 1920, "height": 1080},
            "lightingPreset": "editorial_soft",
        }
        mode_objects = blender_renderer.resolve_authored_scene_mode_objects(data)
        objects = blender_renderer.append_runtime_master_collection(
            master,
            bpy.context.scene.collection,
        )
        meshes = [obj for obj in objects if obj.type == "MESH"]
        armatures = [obj for obj in objects if obj.type == "ARMATURE"]
        dimensions = blender_renderer.prepare_character(
            meshes,
            target_height=blender_renderer.scene_target_height(data),
            preserve_hierarchy=True,
            asset_objects=objects,
        )
        blender_renderer.setup_authored_scene(data, dimensions, mode_objects)
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
        blender_renderer.create_action_library(armature, face, bone_map, 30)
        blender_renderer.place_character_in_authored_scene(
            meshes,
            objects,
            armature,
            face,
            dimensions,
            mode_objects,
        )
        scene = bpy.context.scene
        scene.camera = mode_objects["cameras"]["medium"]
        scene.render.engine = "BLENDER_EEVEE"
        scene.render.resolution_x = 768
        scene.render.resolution_y = 432
        scene.render.resolution_percentage = 100
        scene.render.image_settings.file_format = "PNG"
        scene.render.image_settings.color_mode = "RGBA"
        scene.render.film_transparent = True
        for obj in scene.objects:
            if obj not in objects and obj.type not in {"CAMERA", "LIGHT"}:
                obj.hide_render = True

        specs = aroll_actions.build_aroll_action_specs(True, 30)
        gestures = (
            "Aroll_Greeting_Wave",
            "Aroll_OpenPalm_Explain",
            "Aroll_Count_Three",
            "Aroll_Pinch_Detail",
        )
        metrics = {}
        with tempfile.TemporaryDirectory() as temp_dir:
            for action_name in gestures:
                qa._set_action_sample(
                    scene,
                    armature,
                    action_name,
                    specs[action_name][2][0],
                )
                bpy.context.view_layer.update()
                path = Path(temp_dir) / f"{action_name}.png"
                mask = qa._render_pixel_mask(
                    scene,
                    path,
                    meshes,
                    bone_map,
                    "r",
                )
                tips = [
                    world_to_camera_view(
                        scene,
                        scene.camera,
                        armature.matrix_world
                        @ armature.pose.bones[
                            bone_map[f"finger_{digit}_tip_r"]
                        ].tail,
                    )
                    for digit in (1, 2, 3)
                ]
                minimum_tip_separation = min(
                    ((left.x - right.x) ** 2 + (left.y - right.y) ** 2) ** 0.5
                    for index, left in enumerate(tips)
                    for right in tips[index + 1 :]
                )
                hand = armature.pose.bones[bone_map["hand_r"]]
                hand_center = armature.matrix_world @ ((hand.head + hand.tail) * 0.5)
                camera_vector = (scene.camera.location - hand_center).normalized()
                palm_normal = (
                    (armature.matrix_world @ hand.matrix).to_3x3()
                    @ Vector((0.0, 0.0, 1.0))
                ).normalized()
                metrics[action_name] = {
                    "path": path,
                    "pixels": mask["rolePixelCounts"]["hand"],
                    "tipSeparation": minimum_tip_separation,
                    "palmFacing": abs(palm_normal.dot(camera_vector)),
                    "tips": tips,
                }

            self.assertGreaterEqual(
                metrics["Aroll_Greeting_Wave"]["palmFacing"],
                0.75,
            )
            self.assertGreaterEqual(
                metrics["Aroll_Greeting_Wave"]["tipSeparation"],
                0.02,
            )
            self.assertGreaterEqual(
                metrics["Aroll_OpenPalm_Explain"]["palmFacing"],
                0.90,
            )
            self.assertGreaterEqual(
                metrics["Aroll_OpenPalm_Explain"]["tipSeparation"],
                0.035,
            )
            self.assertGreaterEqual(
                metrics["Aroll_Count_Three"]["tipSeparation"],
                0.05,
            )
            for action_name in gestures:
                self.assertGreater(metrics[action_name]["pixels"], 1000)
                for tip in metrics[action_name]["tips"]:
                    self.assertGreaterEqual(float(tip.x), 0.02, action_name)
                    self.assertLessEqual(float(tip.x), 0.98, action_name)
                    self.assertGreaterEqual(float(tip.y), 0.02, action_name)
                    self.assertLessEqual(float(tip.y), 0.98, action_name)

            self.assertGreaterEqual(
                qa.role_mask_difference(
                    metrics["Aroll_Count_Three"]["path"],
                    metrics["Aroll_Pinch_Detail"]["path"],
                    "hand",
                ),
                0.0025,
            )

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

        self.assertEqual(spec["durationSec"], 15.06)
        self.assertEqual(spec["resolution"], {"width": 1920, "height": 1080})
        self.assertEqual(spec["fps"], 30)
        self.assertEqual(spec["profileId"], "talking_head")
        self.assertEqual(spec["presentationMode"], "standing")
        self.assertEqual(spec["cameraPreset"], "front_talking")
        self.assertEqual(
            spec["actionSequence"],
            [
                "Aroll_Greeting_Wave",
                "Aroll_OpenPalm_Explain",
                "Aroll_Count_Three",
                "Aroll_Pinch_Detail",
                "Aroll_Transition_Reset",
            ],
        )
        self.assertEqual(spec["requiredVisemes"], ["A", "E", "O", "MBP"])
        self.assertEqual(spec["brollWindows"], [])
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
        self.assertEqual(context["requiredVisemes"], demo.DEMO_SPEC["requiredVisemes"])
        self.assertEqual(context["targetDurationSec"], demo.DEMO_SPEC["durationSec"])

    def test_project_payload_uses_supported_provider_generation_mode(self) -> None:
        payload = demo.build_project_payload("/tmp/front-demo-project")

        self.assertEqual(payload["mode"], "voice_visual")
        self.assertEqual(payload["generationMode"], "provider_api")
        self.assertEqual(payload["targetDurationSec"], 15.06)
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
                "outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/"
                "ip_layer.mp4"
            ),
        )
        self.assertEqual(
            demo.FINAL_VIDEO_RELATIVE_PATH,
            Path("outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4"),
        )
        self.assertEqual(
            demo.SIGNED_OFF_VIDEO_SHA256,
            "f32056457a73f3467e580b2ea21cad40da67cf667ce560aa969c3a238d8770eb",
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

    def test_mastered_voice_compose_keeps_shared_baseline_gain(self) -> None:
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
