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
            target_height=blender_renderer.scene_target_height(data),
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
        driven_visemes = {"Mouth_A", "Mouth_E", "Mouth_O", "Mouth_U"}
        for driven_role in ("tongue", "oral_cavity"):
            role_shape_keys = oral[driven_role].data.shape_keys
            self.assertIsNotNone(role_shape_keys)
            self.assertTrue(
                driven_visemes.issubset(
                    {key.name for key in role_shape_keys.key_blocks}
                )
            )
            driver_paths = {
                driver.data_path
                for driver in role_shape_keys.animation_data.drivers
            }
            self.assertEqual(
                driver_paths,
                {f'key_blocks["{name}"].value' for name in driven_visemes},
            )
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
        blender_renderer.setup_authored_scene(data, dimensions, mode_objects)
        blender_renderer.place_character_in_authored_scene(
            meshes,
            objects,
            armature,
            face,
            dimensions,
            mode_objects,
        )
        scene = bpy.context.scene
        qa._configure_close_cameras(scene, armature, bone_map)
        scene.camera = bpy.data.objects[qa.FACE_CLOSE_CAMERA]
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

        def mask_components(path, roles):
            width, height, pixels = qa._read_color_mask(path)
            selected_bits = {
                tuple(channel > 0.5 for channel in qa.MASK_COLORS[role][:3])
                for role in roles
            }
            active = set()
            for pixel_index, offset in enumerate(range(0, len(pixels), 4)):
                red, green, blue, alpha = pixels[offset : offset + 4]
                if alpha < 0.05:
                    continue
                bits = (red >= 0.08, green >= 0.08, blue >= 0.08)
                if bits in selected_bits:
                    active.add(pixel_index)
            components = []
            while active:
                stack = [active.pop()]
                points = []
                while stack:
                    pixel_index = stack.pop()
                    x = pixel_index % width
                    y = pixel_index // width
                    points.append((x, y))
                    for neighbor_y in range(max(0, y - 1), min(height, y + 2)):
                        for neighbor_x in range(max(0, x - 1), min(width, x + 2)):
                            neighbor = neighbor_y * width + neighbor_x
                            if neighbor in active:
                                active.remove(neighbor)
                                stack.append(neighbor)
                components.append(
                    {
                        "size": len(points),
                        "bounds": (
                            min(x for x, _y in points),
                            min(y for _x, y in points),
                            max(x for x, _y in points),
                            max(y for _x, y in points),
                        ),
                    }
                )
            return sorted(components, key=lambda component: component["size"])

        def mask_component_sizes(path, roles):
            return [
                component["size"]
                for component in mask_components(path, roles)
            ]

        with tempfile.TemporaryDirectory() as temp_dir:
            for viseme, (shape_name, jaw_radians) in extrema.items():
                qa._reset_armature_pose(scene, armature)
                qa._activate_shape_keys(shape_owners, {shape_name: 1.0})
                jaw.rotation_mode = "XYZ"
                jaw.rotation_euler = (jaw_radians, 0.0, 0.0)
                bpy.context.view_layer.update()
                mask_path = Path(temp_dir) / f"{viseme}.png"
                mask = qa._render_pixel_mask(
                    scene,
                    mask_path,
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
                    12,
                    f"{viseme} must retain a distinct tongue region",
                )
                component_sizes = mask_component_sizes(mask_path, oral)
                self.assertGreaterEqual(
                    min(component_sizes),
                    24,
                    f"{viseme} must not expose detached oral-mask specks: {component_sizes}",
                )
                cavity_components = mask_components(
                    mask_path,
                    {"oral_cavity"},
                )
                self.assertLessEqual(
                    len(cavity_components),
                    2,
                    f"{viseme} cavity mask has detached fragments: {cavity_components}",
                )
                self.assertGreaterEqual(
                    min(component["size"] for component in cavity_components),
                    12,
                    f"{viseme} cavity mask has a tiny fragment: {cavity_components}",
                )
                foreground_components = mask_components(
                    mask_path,
                    set(oral).difference({"oral_cavity"}),
                )
                foreground_bounds = (
                    min(component["bounds"][0] for component in foreground_components),
                    min(component["bounds"][1] for component in foreground_components),
                    max(component["bounds"][2] for component in foreground_components),
                    max(component["bounds"][3] for component in foreground_components),
                )
                mouth_margin = 12
                for component in cavity_components:
                    left, bottom, right, top = component["bounds"]
                    self.assertGreaterEqual(left, foreground_bounds[0] - mouth_margin)
                    self.assertGreaterEqual(bottom, foreground_bounds[1] - mouth_margin)
                    self.assertLessEqual(right, foreground_bounds[2] + mouth_margin)
                    self.assertLessEqual(top, foreground_bounds[3] + mouth_margin)
                trees = {role: evaluated_tree(obj) for role, obj in oral.items()}
                for left, right in (
                    ("tongue", "upper_teeth"),
                    ("tongue", "lower_teeth"),
                    ("tongue", "upper_gum"),
                    ("tongue", "oral_cavity"),
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
                0.014,
            )
            self.assertLessEqual(
                metrics["Aroll_Greeting_Wave"]["tipSeparation"],
                0.025,
            )
            self.assertGreaterEqual(
                metrics["Aroll_OpenPalm_Explain"]["palmFacing"],
                0.90,
            )
            self.assertGreaterEqual(
                metrics["Aroll_OpenPalm_Explain"]["tipSeparation"],
                0.022,
            )
            self.assertLessEqual(
                metrics["Aroll_OpenPalm_Explain"]["tipSeparation"],
                0.032,
            )
            self.assertGreaterEqual(
                metrics["Aroll_Count_Three"]["tipSeparation"],
                0.024,
            )
            self.assertLessEqual(
                metrics["Aroll_Count_Three"]["tipSeparation"],
                0.035,
            )
            self.assertGreater(
                metrics["Aroll_OpenPalm_Explain"]["tipSeparation"],
                metrics["Aroll_Greeting_Wave"]["tipSeparation"],
            )
            self.assertGreater(
                metrics["Aroll_Count_Three"]["tipSeparation"],
                metrics["Aroll_OpenPalm_Explain"]["tipSeparation"],
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

            def evaluated_hand_state(action_name, frame):
                qa._set_action_sample(scene, armature, action_name, frame)
                hand = armature.pose.bones[bone_map["hand_r"]]
                upper_arm = armature.pose.bones[bone_map["upper_arm_r"]]
                forearm = armature.pose.bones[bone_map["forearm_r"]]
                hand_world = armature.matrix_world @ hand.matrix
                hand_center = armature.matrix_world @ ((hand.head + hand.tail) * 0.5)
                camera_vector = (scene.camera.location - hand_center).normalized()
                palm_normal = (
                    hand_world.to_3x3() @ Vector((0.0, 0.0, 1.0))
                ).normalized()
                tip_world = {
                    digit: armature.matrix_world
                    @ armature.pose.bones[
                        bone_map[f"finger_{digit}_tip_r"]
                    ].tail
                    for digit in (1, 2, 3)
                }
                tip_screen = {
                    digit: world_to_camera_view(
                        scene,
                        scene.camera,
                        tip,
                    )
                    for digit, tip in tip_world.items()
                }
                return {
                    "orientation": hand_world.to_quaternion(),
                    "palmFacingSigned": palm_normal.dot(camera_vector),
                    "tipWorld": tip_world,
                    "tipScreenCenterX": sum(
                        float(point.x) for point in tip_screen.values()
                    )
                    / 3.0,
                    "upperOrientation": upper_arm.matrix.to_quaternion(),
                    "forearmOrientation": forearm.matrix.to_quaternion(),
                    "elbowScreen": world_to_camera_view(
                        scene,
                        scene.camera,
                        armature.matrix_world @ forearm.head,
                    ),
                }

            wave_frames = [
                specs["Aroll_Greeting_Wave"][index][0]
                for index in (1, 2, 3)
            ]
            wave_states = [
                evaluated_hand_state("Aroll_Greeting_Wave", frame)
                for frame in wave_frames
            ]
            for state in wave_states:
                self.assertGreaterEqual(abs(state["palmFacingSigned"]), 0.75)
                self.assertGreaterEqual(float(state["elbowScreen"].x), 0.02)
                self.assertLessEqual(float(state["elbowScreen"].x), 0.98)
                self.assertGreaterEqual(float(state["elbowScreen"].y), 0.02)
                self.assertLessEqual(float(state["elbowScreen"].y), 0.98)
            self.assertTrue(
                all(
                    state["palmFacingSigned"] * wave_states[0]["palmFacingSigned"] > 0.0
                    for state in wave_states[1:]
                ),
                "wave palm normal must not invert between extrema",
            )
            alternating_rotation = [
                wave_states[index]["orientation"]
                .rotation_difference(wave_states[index + 1]["orientation"])
                .angle
                for index in (0, 1)
            ]
            self.assertLessEqual(
                wave_states[0]["orientation"]
                .rotation_difference(wave_states[2]["orientation"])
                .angle,
                0.01,
                "wave must return to the first alternating wrist extremum",
            )
            alternating_lateral = [
                abs(
                    wave_states[index]["tipScreenCenterX"]
                    - wave_states[index + 1]["tipScreenCenterX"]
                )
                for index in (0, 1)
            ]
            for role in ("upperOrientation", "forearmOrientation"):
                self.assertLessEqual(
                    max(
                        wave_states[0][role]
                        .rotation_difference(state[role])
                        .angle
                        for state in wave_states[1:]
                    ),
                    0.03,
                    f"wave should oscillate at the wrist, not dislocate {role}",
                )

            hold_frame = specs["Aroll_Pinch_Detail"][2][0]
            hand_states = {
                "open": evaluated_hand_state("Aroll_OpenPalm_Explain", hold_frame),
                "count_three": evaluated_hand_state("Aroll_Count_Three", hold_frame),
                "pinch": evaluated_hand_state("Aroll_Pinch_Detail", hold_frame),
            }
            chain_length = sum(
                qa._digit_chain_length(armature, bone_map, "r", digit)
                for digit in (1, 3)
            ) / 2.0

            def normalized_tip_distance(state, left, right):
                return (
                    state["tipWorld"][left] - state["tipWorld"][right]
                ).length / max(chain_length, 1e-9)

            outer_tip_distance = {
                name: normalized_tip_distance(state, 1, 3)
                for name, state in hand_states.items()
            }
            with self.subTest("pinch convergence"):
                self.assertLessEqual(
                    outer_tip_distance["pinch"],
                    min(
                        outer_tip_distance["open"],
                        outer_tip_distance["count_three"],
                    )
                    * 0.58,
                    f"pinch outer tips do not converge: {outer_tip_distance}",
                )
                self.assertGreaterEqual(
                    outer_tip_distance["pinch"],
                    0.08,
                    "pinch tips must not collide or collapse",
                )
            for left, right in ((1, 2), (1, 3), (2, 3)):
                with self.subTest("pinch separation", left=left, right=right):
                    self.assertGreaterEqual(
                        normalized_tip_distance(hand_states["pinch"], left, right),
                        0.05,
                        f"pinch digits {left}/{right} collapse into one point",
                    )
            with self.subTest("wave alternating rotation"):
                self.assertGreaterEqual(
                    min(alternating_rotation),
                    0.20,
                    f"wave wrist rotation is unreadable: {alternating_rotation}",
                )
            with self.subTest("wave lateral travel"):
                self.assertGreaterEqual(
                    min(alternating_lateral),
                    0.008,
                    f"wave fingertip centroid does not move laterally: {alternating_lateral}",
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
                "outputs/final/refined-aroll-evidence/production-third-review-v3/"
                "ip_layer.mp4"
            ),
        )
        self.assertEqual(
            demo.FINAL_VIDEO_RELATIVE_PATH,
            Path("outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4"),
        )
        self.assertEqual(
            demo.SIGNED_OFF_VIDEO_SHA256,
            "73faca148d127e0e0f1e990846b9d6933c3f1dcebcdcfea52948dc2b8c7e4448",
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
