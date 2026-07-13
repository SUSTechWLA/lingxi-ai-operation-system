#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import json
import pathlib
import sys
import tempfile
import unittest
from unittest import mock


def load_server():
    path = pathlib.Path(__file__).with_name("server.py")
    spec = importlib.util.spec_from_file_location("ip_avatar_3d_server", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


def load_rig_semantics():
    path = pathlib.Path(__file__).with_name("rig_semantics.py")
    spec = importlib.util.spec_from_file_location("ip_avatar_3d_rig_semantics", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


class IPAvatar3DMCPTests(unittest.TestCase):
    def assert_no_grouped_motion_overlaps(self, events: list[dict]) -> None:
        conflicts = []
        by_group: dict[str, list[dict]] = {}
        for event in events:
            groups = event.get("gestureGroups")
            if not groups and event.get("gestureGroup"):
                groups = [event["gestureGroup"]]
            for group in groups or []:
                by_group.setdefault(str(group), []).append(event)
        for group, grouped_events in by_group.items():
            previous_end = -1.0
            previous_label = ""
            for event in sorted(
                grouped_events,
                key=lambda item: (
                    float(item["timeSec"]),
                    str(item.get("action") or ""),
                    str(item.get("motion") or ""),
                ),
            ):
                start = float(event["timeSec"])
                end = start + float(event["duration"])
                label = str(event.get("action") or event.get("motion"))
                if start < previous_end - 1e-6:
                    conflicts.append((group, previous_label, label, round(start, 3), round(previous_end, 3)))
                previous_end = max(previous_end, end)
                previous_label = label
        self.assertEqual([], conflicts)

    def test_subprocess_runner_tolerates_mixed_encoding_tool_logs(self) -> None:
        server = load_server()

        completed = server._run(
            [sys.executable, "-c", "import os; os.write(1, b'\\xe8render complete')"],
            timeout=10,
        )

        self.assertIn("render complete", completed.stdout)

    def test_blender_timeout_scales_for_full_hd_animation(self) -> None:
        server = load_server()

        full_hd = server.estimate_blender_timeout(6, 30, 1920, 1080)
        preview = server.estimate_blender_timeout(6, 12, 640, 360)

        self.assertGreaterEqual(full_hd, 1800)
        self.assertEqual(preview, 600)

    def test_camera_plan_auto_uses_authored_studio_cameras(self) -> None:
        server = load_server()

        plan = server.build_camera_plan(duration_sec=6, fps=30, camera_preset="auto")

        self.assertEqual(plan[0], {"frame": 1, "camera": "Camera_Wide"})
        self.assertEqual([item["frame"] for item in plan], sorted({item["frame"] for item in plan}))
        self.assertIn("Camera_Medium", {item["camera"] for item in plan})
        self.assertIn("Camera_Close", {item["camera"] for item in plan})

    def test_camera_plan_auto_keeps_very_short_clips_stable(self) -> None:
        server = load_server()

        plan = server.build_camera_plan(duration_sec=2.5, fps=30, camera_preset="auto")

        self.assertEqual(plan, [{"frame": 1, "camera": "Camera_Medium"}])

    def test_motion_plan_contains_lip_sync_and_keyword_actions(self) -> None:
        server = load_server()

        plan = server.build_motion_plan("第一，重点介绍我们的开源 AI 视频创作流程。所以要让波波挥手，再动一下腿。", 8, 24)

        self.assertEqual(plan["schemaVersion"], "ip-avatar-3d-motion-plan/v1")
        self.assertGreater(len(plan["lipSync"]), 40)
        motions = {event["motion"] for event in plan["motionEvents"]}
        self.assertIn("idle_breath", motions)
        self.assertIn("blink", motions)
        self.assertIn("point_left", motions)
        self.assertIn("emphasis", motions)
        self.assertIn("nod", motions)
        self.assertIn("wave", motions)
        self.assertIn("leg_step", motions)

    def test_motion_plan_adds_subtle_full_body_weight_shift(self) -> None:
        server = load_server()

        plan = server.build_motion_plan("大家好，今天分享一个值得关注的观点。", 6, 30)

        shifts = [event for event in plan["motionEvents"] if event["motion"] == "weight_shift"]
        self.assertEqual(len(shifts), 1)
        self.assertLessEqual(shifts[0]["strength"], 0.35)
        self.assertGreaterEqual(shifts[0]["duration"], 1.0)

    def test_motion_plan_supports_distinct_cartoon_visemes(self) -> None:
        server = load_server()

        plan = server.build_motion_plan("a e o u m", 3, 24)

        visemes = {item["viseme"] for item in plan["lipSync"]}
        self.assertTrue({"a", "e", "o", "u", "mbp"}.issubset(visemes))

    def test_common_mandarin_script_does_not_collapse_to_one_open_viseme(self) -> None:
        server = load_server()
        script = "大家好，先挥手欢迎你。今天不用复杂术语，说明重点和判断。"

        mapped = [server._viseme_for_char(char)[0] for char in script if char not in "，。！？；、 "]

        self.assertTrue({"a", "e", "o", "u", "mbp"}.issubset(set(mapped)))
        self.assertLess(mapped.count("a") / len(mapped), 0.65)

    def test_motion_plan_uses_extended_presenter_articulation_keywords(self) -> None:
        server = load_server()

        plan = server.build_motion_plan(
            "大家一起分析：第一看左边，第二看右边。请张开手、转动手腕、动动手指，最后轻轻握拳。也许这个判断还需要思考。",
            8,
            30,
        )

        motions = {event["motion"] for event in plan["motionEvents"]}
        self.assertTrue(
            {
                "open_arms", "think", "shrug", "point_left", "point_right",
                "open_hand", "wrist_twist", "finger_wave", "fist",
            }.issubset(motions),
            motions,
        )

    def test_motion_plan_coalesces_nearby_duplicate_gestures(self) -> None:
        server = load_server()

        plan = server.build_motion_plan("我们分析、思考、判断并考虑这个问题。", 4, 30)

        think_events = [event for event in plan["motionEvents"] if event["motion"] == "think"]
        self.assertLessEqual(len(think_events), 2)
        self.assertTrue(all(event["strength"] <= 1.0 for event in think_events))

    def test_finalize_motion_events_reserves_legacy_and_multi_group_events(self) -> None:
        server = load_server()

        resolved = server._finalize_motion_events(
            [
                {"timeSec": 0.0, "motion": "finger_wave", "duration": 0.8, "strength": 0.8, "gestureGroup": "right_hand"},
                {
                    "timeSec": 0.2,
                    "motion": "present",
                    "duration": 0.8,
                    "strength": 0.9,
                    "action": "Aroll_OpenPalm_Explain",
                    "semantic": "explanation",
                    "gestureGroup": "right_hand",
                    "gestureGroups": ["right_hand", "left_hand"],
                },
                {"timeSec": 0.3, "motion": "point_left", "duration": 0.5, "strength": 0.7, "gestureGroup": "left_hand"},
                {"timeSec": 0.3, "motion": "nod", "duration": 0.4, "strength": 0.7, "gestureGroup": "head"},
            ],
            4.0,
        )

        self.assert_no_grouped_motion_overlaps(resolved)
        by_label = {str(event.get("action") or event.get("motion")): event for event in resolved}
        self.assertEqual(0.0, by_label["finger_wave"]["timeSec"])
        self.assertEqual(0.8, by_label["Aroll_OpenPalm_Explain"]["timeSec"])
        self.assertEqual(1.6, by_label["point_left"]["timeSec"])
        self.assertEqual(0.3, by_label["nod"]["timeSec"])

    def test_motion_plan_maps_aroll_semantics_without_same_group_conflicts(self) -> None:
        server = load_server()
        script = (
            "大家好，第一，我们先解释这个流程；第二，具体看这个细节；第三，总结风险。"
            "我同意这个判断，但是不同意夸张说法。总之，重点是记住。"
            "Hello, first explain the idea, second show detail. I agree, but I disagree. "
            "In conclusion, emphasize this point."
        )

        plan = server.build_motion_plan(script, 12, 30)
        repeat = server.build_motion_plan(script, 12, 30)

        self.assertEqual(plan["motionEvents"], repeat["motionEvents"])
        aroll_events = [
            event
            for event in plan["motionEvents"]
            if str(event.get("action", "")).startswith("Aroll_")
        ]
        actions = {event.get("action") for event in aroll_events}
        self.assertTrue(
            {
                "Aroll_Greeting_Wave",
                "Aroll_Count_One",
                "Aroll_Count_Two",
                "Aroll_Count_Three",
                "Aroll_OpenPalm_Explain",
                "Aroll_Pinch_Detail",
                "Aroll_Agree_Nod",
                "Aroll_Disagree_Shake",
                "Aroll_Emphasis_SoftFist",
            }.issubset(actions),
            actions,
        )
        semantics = {event.get("semantic") for event in aroll_events}
        self.assertTrue(
            {
                "greeting",
                "enumeration",
                "explanation",
                "detail",
                "agreement",
                "disagreement",
                "emphasis",
            }.issubset(semantics),
            semantics,
        )
        self.assertTrue(
            all(event.get("gestureGroup") in {"right_hand", "left_hand", "head", "body"} for event in aroll_events),
            aroll_events,
        )

        legacy_grouped_events = [
            event
            for event in plan["motionEvents"]
            if event.get("gestureGroup") and not str(event.get("action", "")).startswith("Aroll_")
        ]
        self.assertTrue(legacy_grouped_events)
        self.assert_no_grouped_motion_overlaps(plan["motionEvents"])

    def test_subtitle_builder_splits_script_over_duration(self) -> None:
        server = load_server()

        subtitle = server.build_subtitle_text("先输入口播。再让三维 IP 角色完成动作和表达。", 6)

        self.assertIn("00:00:00,000 -->", subtitle)
        self.assertIn("先输入口播。", subtitle)
        self.assertIn("再让三维 IP 角色完成动作和表达。", subtitle)

    def test_render_dry_run_writes_planning_artifacts(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")
            output = root / "out"

            result = server.render_talking_video(
                script="今天我们用波波介绍这个开源 AI 视频创作项目。",
                modelPath=str(model),
                outputDir=str(output),
                characterId="bobo",
                durationSec=5,
                fps=12,
                width=640,
                height=360,
                dryRun=True,
            )

            self.assertEqual(result["status"], "planned")
            self.assertTrue(pathlib.Path(result["motionPlanPath"]).exists())
            self.assertTrue(pathlib.Path(result["subtitlePath"]).exists())
            self.assertTrue(pathlib.Path(result["renderInputPath"]).exists())
            self.assertTrue(pathlib.Path(result["renderReportPath"]).exists())

    def test_render_rejects_non_glb_model(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "model.obj"
            path.write_text("o model\n", encoding="utf-8")

            with self.assertRaises(ValueError):
                server.render_talking_video(script="test", modelPath=str(path), outputDir=tmp, dryRun=True)

    def test_render_accepts_rigged_fbx_as_character_source(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "character.fbx"
            model.write_bytes(b"Kaydara FBX Binary placeholder")

            result = server.render_talking_video(
                script="用新的角色模型讲解这个观点。",
                modelPath=str(model),
                outputDir=str(root / "out"),
                durationSec=2,
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["modelPath"], str(model.resolve()))
            self.assertTrue(render_input["preserveExistingRig"])

    def test_render_defaults_to_source_face_and_auto_rig_outputs(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            result = server.render_talking_video(
                script="波波挥手并点头。",
                modelPath=str(model),
                outputDir=str(root / "out"),
                durationSec=2,
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["faceScreenMode"], "source")
            self.assertEqual(render_input["rigMode"], "auto")
            self.assertTrue(render_input["preserveExistingRig"])
            self.assertTrue(render_input["enhanceExistingRig"])
            self.assertTrue(render_input["elbowRig"])
            self.assertEqual(render_input["mouthMode"], "auto")
            self.assertEqual(render_input["facialDetailMode"], "rich")
            self.assertLess(render_input["backgroundBrightness"], 1.0)
            self.assertTrue(render_input["preserveSourceMaterials"])
            self.assertTrue(result["riggedBlendPath"].endswith("rigged_avatar.blend"))
            self.assertTrue(result["riggedGlbPath"].endswith("rigged_avatar.glb"))

    def test_render_input_preserves_static_background_for_compositing(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            background = root / "background.png"
            model.write_bytes(b"glTF placeholder")
            background.write_bytes(b"PNG placeholder")

            result = server.render_talking_video(
                script="固定背景口播。",
                modelPath=str(model),
                backgroundPath=str(background),
                outputDir=str(root / "out"),
                durationSec=2,
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["backgroundPath"], str(background.resolve()))
            self.assertEqual(render_input["backgroundMode"], "static_plate")

    def test_render_scene_mode_resolves_blend_and_takes_precedence_over_plate(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "character.glb"
            scene = root / "editorial-studio.blend"
            background = root / "fallback.png"
            model.write_bytes(b"glTF placeholder")
            scene.write_bytes(b"BLENDER placeholder")
            background.write_bytes(b"PNG placeholder")

            result = server.render_talking_video(
                script="今天分析一个值得关注的变化。",
                modelPath=str(model),
                sceneBlendPath=str(scene),
                backgroundPath=str(background),
                outputDir=str(root / "out"),
                durationSec=8,
                fps=30,
                cameraPreset="auto",
                lightingPreset="editorial_soft",
                renderEngine="BLENDER_EEVEE_NEXT",
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["sceneBlendPath"], str(scene.resolve()))
            self.assertEqual(render_input["backgroundMode"], "blender_scene")
            self.assertEqual(render_input["cameraPreset"], "auto")
            self.assertEqual(render_input["lightingPreset"], "editorial_soft")
            self.assertEqual(render_input["renderEngine"], "BLENDER_EEVEE_NEXT")
            self.assertGreater(len(render_input["cameraPlan"]), 1)
            self.assertEqual(result["sceneBlendPath"], str(scene.resolve()))

    def test_render_scene_mode_rejects_non_blend_scene(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "character.glb"
            scene = root / "studio.glb"
            model.write_bytes(b"glTF placeholder")
            scene.write_bytes(b"glTF placeholder")

            with self.assertRaisesRegex(ValueError, "sceneBlendPath must point to a Blender .blend file"):
                server.render_talking_video(
                    script="场景测试。",
                    modelPath=str(model),
                    sceneBlendPath=str(scene),
                    outputDir=str(root / "out"),
                    dryRun=True,
                )

    def test_render_uses_character_profile_defaults(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "main.glb"
            background = root / "editorial.png"
            profile = root / "character-profile.json"
            model.write_bytes(b"glTF placeholder")
            background.write_bytes(b"PNG placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "characterId": "main_ip_sloth",
                        "model": {
                            "path": "main.glb",
                            "rigMode": "auto",
                            "preserveExistingRig": True,
                            "enhanceExistingRig": True,
                        },
                        "facial": {
                            "mouthMode": "independent_visemes",
                            "mouthHeightRatio": 0.805,
                            "mouthScale": 0.72,
                            "mouthStyle": "organic_dark",
                            "facialDetailMode": "rich",
                            "topologyMode": "source_retopology",
                        },
                        "render": {
                            "backgroundPath": "editorial.png",
                            "backgroundBrightness": 0.76,
                            "qualityPreset": "production_2k",
                            "renderDetailMode": "publish",
                            "resolution": {"width": 2560, "height": 1440},
                        },
                        "voice": {
                            "provider": "kokoro",
                            "voiceId": "zf_xiaobei",
                            "language": "zh",
                            "speed": 0.94,
                        },
                    }
                ),
                encoding="utf-8",
            )

            result = server.render_talking_video(
                script="观点分享。",
                modelPath="",
                characterProfilePath=str(profile),
                outputDir=str(root / "out"),
                durationSec=2,
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["characterId"], "main_ip_sloth")
            self.assertEqual(render_input["modelPath"], str(model.resolve()))
            self.assertEqual(render_input["backgroundPath"], str(background.resolve()))
            self.assertEqual(render_input["mouthMode"], "independent_visemes")
            self.assertEqual(render_input["mouthHeightRatio"], 0.805)
            self.assertEqual(render_input["mouthScale"], 0.72)
            self.assertEqual(render_input["mouthStyle"], "organic_dark")
            self.assertTrue(render_input["enhanceExistingRig"])
            self.assertEqual(render_input["facialDetailMode"], "rich")
            self.assertEqual(render_input["facialTopologyMode"], "source_retopology")
            self.assertEqual(render_input["backgroundBrightness"], 0.76)
            self.assertEqual(render_input["resolution"], {"width": 2560, "height": 1440})
            self.assertEqual(render_input["qualityPreset"], "production_2k")
            self.assertEqual(render_input["renderDetailMode"], "publish")
            self.assertEqual(render_input["eeveeSamples"], 64)
            self.assertEqual(render_input["voice"]["provider"], "kokoro")
            self.assertEqual(render_input["voice"]["voiceId"], "zf_xiaobei")
            self.assertEqual(render_input["voice"]["language"], "zh")
            self.assertEqual(render_input["voice"]["speed"], 0.94)

    def test_default_render_resolution_is_qhd_2k(self) -> None:
        server = load_server()

        self.assertEqual(server.DEFAULT_WIDTH, 2560)
        self.assertEqual(server.DEFAULT_HEIGHT, 1440)

    def test_motion_plan_adds_natural_face_beats_and_fallback_gestures(self) -> None:
        server = load_server()

        plan = server.build_motion_plan(
            "今天和你分享一个观察。它会影响我们接下来的选择。最后给出我的建议。",
            duration_sec=10,
            fps=30,
            motion_style="expressive",
        )
        motions = [event["motion"] for event in plan["motionEvents"]]

        self.assertIn("micro_gaze", motions)
        self.assertIn("brow_beat", motions)
        self.assertTrue({"present", "emphasis"}.intersection(motions))
        blink_times = [event["timeSec"] for event in plan["motionEvents"] if event["motion"] == "blink"]
        intervals = [round(right - left, 2) for left, right in zip(blink_times, blink_times[1:])]
        self.assertGreaterEqual(len(set(intervals)), 2)

    def test_compose_command_overlays_rgba_avatar_on_static_background(self) -> None:
        server = load_server()
        command = server._build_compose_video_args(
            frames_dir=pathlib.Path("/tmp/avatar_frames"),
            audio_path="/tmp/narration.m4a",
            output_path=pathlib.Path("/tmp/ip_layer.mp4"),
            duration_sec=3,
            fps=30,
            background_path="/tmp/studio.png",
            width=1920,
            height=1080,
        )

        joined = " ".join(command)
        self.assertIn("-loop 1", joined)
        self.assertIn("[bg][avatar]overlay", joined)
        self.assertIn("scale=1920:1080", joined)
        self.assertIn("colorchannelmixer", joined)
        self.assertIn("-crf 16", joined)
        self.assertIn("-preset slow", joined)
        self.assertIn("-profile:v high", joined)
        self.assertIn("-r 30", joined)
        self.assertIn("-fps_mode cfr", joined)
        self.assertIn("loudnorm=I=-16:TP=-1.5:LRA=7", joined)
        self.assertIn("-b:a 128k", joined)
        self.assertIn("-ar 48000", joined)

    def test_audio_engine_uses_pinned_character_voice(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            engine = root / "audio.mjs"
            engine.write_text("// fake audio engine", encoding="utf-8")

            def fake_run(args, timeout=600):
                request_path = pathlib.Path(args[args.index("--request") + 1])
                meta_path = pathlib.Path(args[args.index("--out") + 1])
                request = json.loads(request_path.read_text(encoding="utf-8"))
                self.assertEqual(request["provider"], "kokoro")
                self.assertEqual(request["voice"], "zf_xiaobei")
                self.assertEqual(request["lang"], "zh")
                self.assertEqual(request["speed"], 0.94)
                audio = root / "assets" / "voice" / "narration.wav"
                audio.parent.mkdir(parents=True, exist_ok=True)
                audio.write_bytes(b"RIFF fake wav")
                meta_path.write_text(
                    json.dumps(
                        {
                            "tts_provider": "kokoro",
                            "voice_id": "zf_xiaobei",
                            "voices": [
                                {
                                    "id": "narration",
                                    "path": "assets/voice/narration.wav",
                                    "duration_s": 2.1,
                                    "words": [],
                                }
                            ],
                        }
                    ),
                    encoding="utf-8",
                )
                return mock.Mock(returncode=0, stdout="", stderr="")

            with mock.patch.object(server, "find_audio_engine", return_value=str(engine)), mock.patch.object(
                server, "_run", side_effect=fake_run
            ):
                audio_path, source, metadata = server.ensure_audio(
                    "今天分享一个重要观点。",
                    root,
                    2.1,
                    voice_provider="kokoro",
                    voice_id="zf_xiaobei",
                    voice_language="zh",
                    voice_speed=0.94,
                )

            self.assertEqual(pathlib.Path(audio_path), (root / "assets" / "voice" / "narration.wav").resolve())
            self.assertEqual(source, "hyperframes_kokoro")
            self.assertEqual(metadata["voiceId"], "zf_xiaobei")
            self.assertFalse(metadata["humanVoiceProvider"])

    def test_apple_character_voice_is_pinned_and_mastered_for_production(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            commands = []

            def fake_run(args, timeout=600):
                commands.append(args)
                output = pathlib.Path(args[args.index("-o") + 1]) if args[0].endswith("say") else pathlib.Path(args[-1])
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_bytes(b"audio")
                return mock.Mock(returncode=0, stdout="", stderr="")

            with mock.patch.object(server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"), mock.patch.object(
                server.shutil,
                "which",
                side_effect=lambda name: "/usr/bin/say" if name == "say" else None,
            ), mock.patch.object(server, "_run", side_effect=fake_run):
                audio_path, source, metadata = server.ensure_audio(
                    "今天分享一个重要观点。",
                    root,
                    2.0,
                    voice_provider="apple",
                    voice_id="Eddy (中文（中国大陆）)",
                    voice_language="zh-CN",
                    voice_speed=0.94,
                )

            self.assertEqual(source, "apple_neural_voice")
            self.assertEqual(metadata["voiceId"], "Eddy (中文（中国大陆）)")
            self.assertTrue(metadata["productionReady"])
            self.assertEqual(pathlib.Path(audio_path), root / "narration_master.m4a")
            self.assertIn("-v", commands[0])
            self.assertIn("Eddy (中文（中国大陆）)", commands[0])
            mastering = " ".join(commands[1])
            self.assertIn("highpass", mastering)
            self.assertIn("acompressor", mastering)
            self.assertIn("loudnorm", mastering)

    def test_resolves_mixamo_style_humanoid_bones(self) -> None:
        semantics = load_rig_semantics()

        mapping = semantics.resolve_bone_roles(
            [
                "mixamorig:Hips",
                "mixamorig:Spine2",
                "mixamorig:Head",
                "mixamorig:LeftArm",
                "mixamorig:LeftForeArm",
                "mixamorig:LeftHand",
                "mixamorig:RightArm",
                "mixamorig:RightForeArm",
                "mixamorig:RightHand",
                "mixamorig:LeftUpLeg",
                "mixamorig:RightUpLeg",
            ]
        )

        self.assertEqual(mapping["head"], "mixamorig:Head")
        self.assertEqual(mapping["upper_arm_l"], "mixamorig:LeftArm")
        self.assertEqual(mapping["forearm_r"], "mixamorig:RightForeArm")
        self.assertEqual(mapping["leg_l"], "mixamorig:LeftUpLeg")
        self.assertTrue(semantics.has_presenter_controls(mapping))

    def test_generated_rig_semantics_preserve_full_articulation_roles(self) -> None:
        semantics = load_rig_semantics()

        names = [
            "Root",
            "Body",
            "Spine",
            "Chest",
            "Neck",
            "Head",
            "Jaw",
            "Shoulder.L",
            "UpperArm.L",
            "ForeArm.L",
            "Hand.L",
            "Finger_01.L",
            "Finger_02.L",
            "Finger_03.L",
            "Shoulder.R",
            "UpperArm.R",
            "ForeArm.R",
            "Hand.R",
            "Finger_01.R",
            "Finger_02.R",
            "Finger_03.R",
            "Thigh.L",
            "Shin.L",
            "Foot.L",
            "Thigh.R",
            "Shin.R",
            "Foot.R",
        ]

        mapping = semantics.resolve_bone_roles(names)

        expected = {
            "root",
            "body",
            "spine",
            "chest",
            "neck",
            "head",
            "jaw",
            "shoulder_l",
            "upper_arm_l",
            "forearm_l",
            "hand_l",
            "finger_1_l",
            "finger_2_l",
            "finger_3_l",
            "shoulder_r",
            "upper_arm_r",
            "forearm_r",
            "hand_r",
            "finger_1_r",
            "finger_2_r",
            "finger_3_r",
            "leg_l",
            "shin_l",
            "foot_l",
            "leg_r",
            "shin_r",
            "foot_r",
        }
        self.assertTrue(expected.issubset(mapping), sorted(expected.difference(mapping)))
        self.assertEqual(mapping["body"], "Body")
        self.assertEqual(mapping["chest"], "Chest")
        self.assertEqual(mapping["finger_2_r"], "Finger_02.R")

    def test_resolves_three_segment_three_digit_hand_roles(self) -> None:
        semantics = load_rig_semantics()
        names = [
            f"Finger_{digit:02d}_{segment}.{side}"
            for side in ("L", "R")
            for digit in (1, 2, 3)
            for segment in ("Proximal", "Middle", "Distal")
        ]

        mapping = semantics.resolve_bone_roles(names)

        for side in ("l", "r"):
            for digit in (1, 2, 3):
                assert mapping[f"finger_{digit}_{side}"] == f"Finger_{digit:02d}_Proximal.{side.upper()}"
                assert mapping[f"finger_{digit}_mid_{side}"] == f"Finger_{digit:02d}_Middle.{side.upper()}"
                assert mapping[f"finger_{digit}_tip_{side}"] == f"Finger_{digit:02d}_Distal.{side.upper()}"

    def test_validate_character_asset_accepts_rig_and_required_visemes(self) -> None:
        server = load_server()
        semantics = load_rig_semantics()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "character.gltf"
            profile = root / "character-profile.json"
            bone_names = [
                "mixamorig:Hips",
                "mixamorig:Spine2",
                "mixamorig:Head",
                "mixamorig:LeftArm",
                "mixamorig:LeftForeArm",
                "mixamorig:LeftHand",
                "mixamorig:RightArm",
                "mixamorig:RightForeArm",
                "mixamorig:RightHand",
                "mixamorig:LeftUpLeg",
                "mixamorig:RightUpLeg",
            ]
            visemes = [
                "Mouth_Rest",
                "Mouth_A",
                "Mouth_E",
                "Mouth_O",
                "Mouth_U",
                "Mouth_MBP",
                "Mouth_Smile",
                "Mouth_Frown",
                "Mouth_Surprise",
            ]
            model.write_text(
                json.dumps(
                    {
                        "asset": {"version": "2.0"},
                        "nodes": [{"name": name} for name in bone_names],
                        "skins": [{"joints": list(range(len(bone_names)))}],
                        "meshes": [{"name": "Face", "extras": {"targetNames": visemes}, "primitives": [{}]}],
                        "animations": [{"name": "Idle"}],
                    }
                ),
                encoding="utf-8",
            )
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "characterId": "main_ip_sloth",
                        "model": {
                            "path": "character.gltf",
                            "requiredBoneRoles": [
                                "root",
                                "body",
                                "head",
                                "upper_arm_l",
                                "forearm_l",
                                "hand_l",
                                "upper_arm_r",
                                "forearm_r",
                                "hand_r",
                                "leg_l",
                                "leg_r",
                            ],
                        },
                        "facial": {"requiredShapeKeys": visemes},
                        "turnaround": {},
                        "render": {},
                    }
                ),
                encoding="utf-8",
            )

            result = server.validate_character_asset(characterProfilePath=str(profile))

            self.assertTrue(result["success"])
            self.assertTrue(result["readyForTalkingVideo"])
            self.assertEqual(result["missingBoneRoles"], [])
            self.assertEqual(result["missingShapeKeys"], [])


if __name__ == "__main__":
    unittest.main()
