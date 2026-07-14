#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import hashlib
import io
import json
import os
import pathlib
import sys
import tempfile
import types
import unittest
import wave
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


def load_master_asset():
    path = pathlib.Path(__file__).with_name("master_asset.py")
    spec = importlib.util.spec_from_file_location("ip_avatar_3d_master_asset", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


def load_aroll_actions():
    path = pathlib.Path(__file__).with_name("aroll_actions.py")
    spec = importlib.util.spec_from_file_location("ip_avatar_3d_aroll_actions", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def load_blender_renderer():
    path = pathlib.Path(__file__).with_name("blender_renderer.py")
    spec = importlib.util.spec_from_file_location("ip_avatar_3d_blender_renderer", path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


def test_wav_bytes(*, sample_rate: int = 48000, channels: int = 1) -> bytes:
    buffer = io.BytesIO()
    with wave.open(buffer, "wb") as wav_file:
        wav_file.setnchannels(channels)
        wav_file.setsampwidth(2)
        wav_file.setframerate(sample_rate)
        wav_file.writeframes(b"\x00\x00" * channels * 480)
    return buffer.getvalue()


class IPAvatar3DMCPTests(unittest.TestCase):
    def test_close_shot_folded_hand_poses_use_restrained_joint_curls(self) -> None:
        actions = load_aroll_actions()

        for pose_name in ("fist", "pinch", "count_one", "count_two", "finger_roll"):
            for pose in actions.hand_pose(pose_name).values():
                self.assertLessEqual(pose.proximal, 0.30, pose_name)
                self.assertLessEqual(pose.middle, 0.34, pose_name)
                self.assertLessEqual(pose.distal, 0.22, pose_name)
        for pose in actions.hand_pose("fist").values():
            self.assertGreaterEqual(
                pose.proximal + pose.middle + pose.distal,
                0.70,
            )

    def write_voice_audition_profile(
        self,
        root: pathlib.Path,
        candidates: list[str] | None = None,
    ) -> pathlib.Path:
        profile = root / "character-profile.json"
        profile.write_text(
            json.dumps(
                {
                    "schemaVersion": "tangying-ip-character/v1",
                    "characterId": "test_character",
                    "voice": {
                        "renderMode": "production",
                        "provider": "heygen",
                        "voiceId": "",
                        "fallbackPolicy": "error",
                        "productionVoiceCandidates": candidates
                        if candidates is not None
                        else ["heygen_voice_1", "heygen_voice_2", "heygen_voice_3"],
                        "language": "zh-CN",
                        "speed": 0.94,
                        "speakingRate": 185,
                    },
                }
            ),
            encoding="utf-8",
        )
        return profile

    def successful_voice_audition_fakes(self):
        synthesis_calls = []
        ffmpeg_commands = []

        def fake_ensure_audio(
            requested_script,
            candidate_dir,
            duration_sec,
            audio_path="",
            voice_name="",
            speaking_rate=190,
            **kwargs,
        ):
            synthesis_calls.append(
                {
                    "script": requested_script,
                    "durationSec": duration_sec,
                    "speakingRate": speaking_rate,
                    **kwargs,
                }
            )
            audio = pathlib.Path(candidate_dir) / "narration.wav"
            audio.parent.mkdir(parents=True, exist_ok=True)
            audio.write_bytes(b"RIFF synthesized audio")
            return str(audio), "hyperframes_heygen", {
                "tts_provider": "heygen",
                "voice_id": kwargs["voice_id"],
                "language": kwargs["voice_language"],
                "speed": kwargs["voice_speed"],
                "productionReady": True,
            }

        def fake_run(args, timeout=600):
            ffmpeg_commands.append(args)
            if args[-1] == "-":
                return mock.Mock(
                    returncode=0,
                    stdout="",
                    stderr=(
                        '[Parsed_loudnorm_0] {\n'
                        '  "input_i" : "-16.0",\n'
                        '  "input_tp" : "-1.6",\n'
                        '  "input_lra" : "2.1"\n'
                        "}"
                    ),
                )
            pathlib.Path(args[-1]).write_bytes(b"RIFF normalized audio")
            return mock.Mock(returncode=0, stdout="", stderr="")

        return synthesis_calls, ffmpeg_commands, fake_ensure_audio, fake_run

    def run_successful_voice_auditions(
        self,
        server,
        *,
        script: str,
        profile: pathlib.Path,
        output_dir: pathlib.Path,
    ):
        synthesis_calls, ffmpeg_commands, fake_ensure_audio, fake_run = (
            self.successful_voice_audition_fakes()
        )
        with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
            server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
        ), mock.patch.object(
            server.shutil, "which", side_effect=lambda name: "/usr/bin/node" if name == "node" else None
        ), mock.patch.object(server, "ensure_audio", side_effect=fake_ensure_audio), mock.patch.object(
            server, "_run", side_effect=fake_run
        ):
            result = server.generate_voice_auditions(
                script=script,
                characterProfilePath=str(profile),
                outputDir=str(output_dir),
            )
        return result, synthesis_calls, ffmpeg_commands

    def plan_voice_auditions(
        self,
        server,
        *,
        script: str,
        profile: pathlib.Path,
        output_dir: pathlib.Path,
    ):
        with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
            server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
        ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"):
            return server.generate_voice_auditions(
                script=script,
                characterProfilePath=str(profile),
                outputDir=str(output_dir),
                dryRun=True,
            )

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

    def test_presentation_mode_is_written_to_render_input_and_dry_run_outputs(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "avatar.glb"
            model.write_bytes(b"glTF placeholder")

            result = server.render_talking_video(
                script="坐姿口播测试。",
                modelPath=str(model),
                outputDir=str(root / "out"),
                presentationMode="seated",
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            report = json.loads(pathlib.Path(result["renderReportPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["presentationMode"], "seated")
            self.assertEqual(report["presentationMode"], "seated")
            self.assertEqual(result["presentationMode"], "seated")

    def test_auto_presentation_mode_defaults_to_standing(self) -> None:
        server = load_server()

        self.assertEqual(server.resolve_presentation_mode("auto"), "standing")

    def test_profile_presentation_mode_is_used_only_for_auto_request(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "avatar.glb"
            profile = root / "character-profile.json"
            model.write_bytes(b"glTF placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "model": {"path": "avatar.glb"},
                        "render": {"presentationMode": "seated"},
                    }
                ),
                encoding="utf-8",
            )

            auto_result = server.render_talking_video(
                script="从 profile 解析坐姿。",
                characterProfilePath=str(profile),
                outputDir=str(root / "auto"),
                dryRun=True,
            )
            explicit_result = server.render_talking_video(
                script="显式请求站姿。",
                characterProfilePath=str(profile),
                outputDir=str(root / "explicit"),
                presentationMode="standing",
                dryRun=True,
            )

            self.assertEqual(auto_result["presentationMode"], "seated")
            self.assertEqual(explicit_result["presentationMode"], "standing")

    def test_invalid_presentation_mode_fails_before_audio(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "avatar.glb"
            model.write_bytes(b"glTF placeholder")

            with mock.patch.object(server, "ensure_audio") as ensure_audio, self.assertRaisesRegex(
                ValueError, "presentationMode"
            ):
                server.render_talking_video(
                    script="非法模式不应开始音频合成。",
                    modelPath=str(model),
                    outputDir=str(root / "out"),
                    presentationMode="crouching",
                    voiceProvider="apple",
                    voiceId="Eddy (中文（中国大陆）)",
                )

            ensure_audio.assert_not_called()

    def test_omitted_render_mode_is_safely_treated_as_preview(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            result = server.render_talking_video(
                script="预览模式口播。",
                modelPath=str(model),
                outputDir=str(root / "out"),
                voiceProvider="apple",
                voiceId="Eddy (中文（中国大陆）)",
                dryRun=True,
            )

            self.assertEqual(result["voicePolicy"]["renderMode"], "preview")
            self.assertEqual(result["voicePolicy"]["policyStatus"], "ready")
            self.assertEqual(result["voicePolicy"]["requestedProvider"], "apple")
            self.assertNotIn("provider", result["voicePolicy"])
            self.assertFalse(result["voicePolicy"]["productionReady"])

    def test_explicit_production_rejects_preview_voice_before_audio(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            with mock.patch.object(server, "ensure_audio") as ensure_audio:
                with self.assertRaises(server.ProductionVoiceUnavailable):
                    server.render_talking_video(
                        script="生产模式不能使用预览声音。",
                        modelPath=str(model),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="apple",
                        voiceId="Eddy (中文（中国大陆）)",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

            ensure_audio.assert_not_called()

    def test_production_does_not_treat_legacy_voice_name_as_pinned_id(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            with mock.patch.object(server, "ensure_audio") as ensure_audio:
                with self.assertRaisesRegex(server.ProductionVoiceUnavailable, "voice ID"):
                    server.render_talking_video(
                        script="生产模式必须显式固定音色 ID。",
                        modelPath=str(model),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceName="legacy-name",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

            ensure_audio.assert_not_called()

    def test_production_rejects_synthesis_that_changes_pinned_voice(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")
            audio = root / "narration.wav"
            audio.write_bytes(b"RIFF verified audio")
            mismatched_audio = (
                str(audio),
                "hyperframes_apple",
                {
                    "provider": "apple",
                    "voiceId": "Eddy (中文（中国大陆）)",
                    "tts_provider": "apple",
                    "voice_id": "Eddy (中文（中国大陆）)",
                    "language": "zh",
                    "speed": 1.0,
                    "productionReady": False,
                },
            )

            with mock.patch.object(server, "ensure_audio", return_value=mismatched_audio):
                with self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable,
                    "pinned provider and voice ID",
                ):
                    server.render_talking_video(
                        script="后端不能替换已固定的生产音色。",
                        modelPath=str(model),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceId="dMkR1XwIkarpNqWUJLnX",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

    def test_production_rejects_backend_missing_voice_provenance(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            audio = root / "narration.wav"
            model.write_bytes(b"glTF placeholder")
            audio.write_bytes(b"RIFF verified audio")
            missing_provenance = (
                str(audio),
                "hyperframes_unknown",
                {
                    "provider": "",
                    "voiceId": "",
                    "language": "zh",
                    "speed": 1.0,
                    "productionReady": False,
                },
            )

            with mock.patch.object(server, "ensure_audio", return_value=missing_provenance):
                with self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable,
                    "explicit tts_provider and voice_id provenance",
                ):
                    server.render_talking_video(
                        script="生产后端必须返回来源。",
                        modelPath=str(model),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceId="dMkR1XwIkarpNqWUJLnX",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

    def test_production_wraps_explicit_provider_backend_failure(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            with mock.patch.object(
                server,
                "ensure_audio",
                side_effect=RuntimeError("heygen backend unavailable"),
            ):
                with self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable,
                    "production voice synthesis failed",
                ) as raised:
                    server.render_talking_video(
                        script="生产后端失败必须封闭。",
                        modelPath=str(model),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceId="dMkR1XwIkarpNqWUJLnX",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

            self.assertIsInstance(raised.exception.__cause__, RuntimeError)

    def test_production_rejects_unverified_uploaded_audio_before_synthesis(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            uploaded = root / "uploaded.wav"
            model.write_bytes(b"glTF placeholder")
            uploaded.write_bytes(b"RIFF uploaded audio")

            with mock.patch.object(server, "ensure_audio") as ensure_audio:
                with self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable,
                    "uploaded audio provenance is unverified",
                ):
                    server.render_talking_video(
                        script="生产上传音频缺少可信来源。",
                        modelPath=str(model),
                        audioPath=str(uploaded),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceId="dMkR1XwIkarpNqWUJLnX",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

            ensure_audio.assert_not_called()

    def test_production_rejects_missing_synthesized_audio_path(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")
            missing_audio = (
                str(root / "missing.wav"),
                "hyperframes_heygen",
                {
                    "provider": "heygen",
                    "voiceId": "dMkR1XwIkarpNqWUJLnX",
                    "tts_provider": "heygen",
                    "voice_id": "dMkR1XwIkarpNqWUJLnX",
                },
            )

            with mock.patch.object(server, "ensure_audio", return_value=missing_audio):
                with self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable,
                    "audio path",
                ):
                    server.render_talking_video(
                        script="生产音频文件必须存在。",
                        modelPath=str(model),
                        outputDir=str(root / "out"),
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceId="dMkR1XwIkarpNqWUJLnX",
                        fallbackPolicy="error",
                        dryRun=False,
                    )

    def test_production_accepts_exact_backend_provenance_and_audio_path(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            audio = root / "narration.wav"
            model.write_bytes(b"glTF placeholder")
            audio.write_bytes(b"RIFF verified audio")
            voice_id = "dMkR1XwIkarpNqWUJLnX"
            captured_render_input = {}

            def fake_blender_run(args, timeout=600):
                render_input = json.loads(pathlib.Path(args[-1]).read_text(encoding="utf-8"))
                captured_render_input.update(render_input)
                for key in ("riggedBlendPath", "riggedGlbPath", "rigReportPath"):
                    path = pathlib.Path(render_input[key])
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(b"render output")
                return mock.Mock(returncode=0, stdout="", stderr="")

            def fake_compose(_frames, _audio, output_path, *_args, **_kwargs):
                pathlib.Path(output_path).write_bytes(b"video")

            def fake_preview(_video_path, preview_path):
                pathlib.Path(preview_path).write_bytes(b"preview")

            exact_audio = (
                str(audio),
                "hyperframes_heygen",
                {
                    "provider": "heygen",
                    "voiceId": voice_id,
                    "tts_provider": "heygen",
                    "voice_id": voice_id,
                    "language": "zh",
                    "speed": 1.0,
                    "humanVoiceProvider": True,
                    "productionReady": False,
                },
            )
            with mock.patch.object(server, "ensure_audio", return_value=exact_audio), mock.patch.object(
                server, "audio_duration_sec", return_value=2.25
            ), mock.patch.object(
                server, "find_blender", return_value="/usr/bin/blender"
            ), mock.patch.object(server, "_run", side_effect=fake_blender_run), mock.patch.object(
                server, "_compose_video", side_effect=fake_compose
            ), mock.patch.object(server, "_extract_preview", side_effect=fake_preview), mock.patch.object(
                server, "_probe_video", return_value={"durationSec": 2.0}
            ), mock.patch.object(
                server,
                "_measure_voice_audition_loudness",
                return_value={
                    "integratedLufs": -16.1,
                    "truePeakDbtp": -2.0,
                    "loudnessRangeLu": 2.0,
                },
            ):
                result = server.render_talking_video(
                    script="验证生产音色成功路径。",
                    modelPath=str(model),
                    outputDir=str(root / "out"),
                    durationSec=7,
                    renderMode="production",
                    voiceProvider="heygen",
                    voiceId=voice_id,
                    fallbackPolicy="error",
                    presentationMode="seated",
                    dryRun=False,
                )

            self.assertTrue(result["success"])
            self.assertEqual(result["voice"]["provider"], "heygen")
            self.assertEqual(result["voice"]["voiceId"], voice_id)
            self.assertEqual(result["voice"]["requestedProvider"], "heygen")
            self.assertEqual(result["voice"]["requestedVoiceId"], voice_id)
            self.assertTrue(result["voice"]["productionReady"])
            self.assertEqual(result["durationSec"], 2.25)
            self.assertEqual(captured_render_input["durationSec"], 2.25)
            self.assertEqual(captured_render_input["motionPlan"]["durationSec"], 2.25)
            self.assertEqual(captured_render_input["presentationMode"], "seated")
            report = json.loads((root / "out" / "render_report.json").read_text(encoding="utf-8"))
            self.assertEqual(report["voice"]["provider"], "heygen")
            self.assertEqual(report["qa"]["finalAudioLoudness"]["truePeakDbtp"], -2.0)
            self.assertEqual(report["presentationMode"], "seated")
            self.assertEqual(result["presentationMode"], "seated")

    def test_production_removes_encoded_video_when_final_audio_gate_fails(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            audio = root / "narration.wav"
            output_dir = root / "out"
            model.write_bytes(b"glTF placeholder")
            audio.write_bytes(b"RIFF verified audio")
            voice_id = "dMkR1XwIkarpNqWUJLnX"

            def fake_blender_run(args, timeout=600):
                render_input = json.loads(pathlib.Path(args[-1]).read_text(encoding="utf-8"))
                for key in ("riggedBlendPath", "riggedGlbPath", "rigReportPath"):
                    path = pathlib.Path(render_input[key])
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(b"render output")
                return mock.Mock(returncode=0, stdout="", stderr="")

            def fake_compose(_frames, _audio, output_path, *_args, **_kwargs):
                pathlib.Path(output_path).write_bytes(b"encoded video with invalid audio")

            exact_audio = (
                str(audio),
                "hyperframes_heygen",
                {
                    "provider": "heygen",
                    "voiceId": voice_id,
                    "tts_provider": "heygen",
                    "voice_id": voice_id,
                    "language": "zh",
                    "speed": 1.0,
                    "humanVoiceProvider": True,
                    "productionReady": False,
                },
            )
            with mock.patch.object(server, "ensure_audio", return_value=exact_audio), mock.patch.object(
                server, "audio_duration_sec", return_value=2.25
            ), mock.patch.object(
                server, "find_blender", return_value="/usr/bin/blender"
            ), mock.patch.object(server, "_run", side_effect=fake_blender_run), mock.patch.object(
                server, "_compose_video", side_effect=fake_compose
            ), mock.patch.object(
                server,
                "_measure_voice_audition_loudness",
                return_value={
                    "integratedLufs": -16.1,
                    "truePeakDbtp": -1.4,
                    "loudnessRangeLu": 2.0,
                },
            ):
                with self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable,
                    "final encoded audio",
                ):
                    server.render_talking_video(
                        script="验证最终音频门禁失败路径。",
                        modelPath=str(model),
                        outputDir=str(output_dir),
                        durationSec=7,
                        renderMode="production",
                        voiceProvider="heygen",
                        voiceId=voice_id,
                        fallbackPolicy="error",
                        dryRun=False,
                    )

            self.assertFalse((output_dir / "ip_layer.mp4").exists())
            self.assertFalse((output_dir / "render_report.json").exists())

    def test_preview_uploaded_audio_succeeds_with_unverified_provenance(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            uploaded = root / "uploaded.wav"
            model.write_bytes(b"glTF placeholder")
            uploaded.write_bytes(b"RIFF uploaded audio")

            def fake_blender_run(args, timeout=600):
                render_input = json.loads(pathlib.Path(args[-1]).read_text(encoding="utf-8"))
                for key in ("riggedBlendPath", "riggedGlbPath", "rigReportPath"):
                    path = pathlib.Path(render_input[key])
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(b"render output")
                return mock.Mock(returncode=0, stdout="", stderr="")

            def fake_compose(_frames, _audio, output_path, *_args, **_kwargs):
                pathlib.Path(output_path).write_bytes(b"video")

            with mock.patch.object(server, "audio_duration_sec", return_value=2.0), mock.patch.object(
                server, "find_blender", return_value="/usr/bin/blender"
            ), mock.patch.object(server, "_run", side_effect=fake_blender_run), mock.patch.object(
                server, "_compose_video", side_effect=fake_compose
            ), mock.patch.object(server, "_extract_preview"), mock.patch.object(
                server, "_probe_video", return_value={"durationSec": 2.0}
            ):
                result = server.render_talking_video(
                    script="预览上传音频。",
                    modelPath=str(model),
                    audioPath=str(uploaded),
                    outputDir=str(root / "out"),
                    renderMode="preview",
                    dryRun=False,
                )

            self.assertTrue(result["success"])
            self.assertEqual(result["audioSource"], "uploaded_audio")
            self.assertEqual(result["voice"]["provider"], "uploaded")
            self.assertEqual(result["voice"]["voiceId"], "")
            self.assertEqual(result["voice"]["requestedProvider"], "auto")
            self.assertEqual(result["voice"]["requestedVoiceId"], "")
            self.assertFalse(result["voice"]["humanVoiceProvider"])
            self.assertFalse(result["voice"]["productionReady"])

    def test_production_dry_run_surfaces_ready_policy_without_synthesis(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            with mock.patch.object(server, "ensure_audio") as ensure_audio:
                result = server.render_talking_video(
                    script="生产音色干跑。",
                    modelPath=str(model),
                    outputDir=str(root / "out"),
                    renderMode="production",
                    voiceProvider="heygen",
                    voiceId="dMkR1XwIkarpNqWUJLnX",
                    fallbackPolicy="error",
                    dryRun=True,
                )

            ensure_audio.assert_not_called()
            self.assertEqual(
                result["voicePolicy"],
                {
                    "renderMode": "production",
                    "requestedProvider": "heygen",
                    "requestedVoiceId": "dMkR1XwIkarpNqWUJLnX",
                    "language": "zh",
                    "speed": 1.0,
                    "fallbackPolicy": "error",
                    "productionReady": False,
                    "allowPreviewFallback": False,
                    "policyStatus": "ready",
                },
            )

    def test_production_dry_run_surfaces_blocked_policy_without_downgrade(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "bobo.glb"
            model.write_bytes(b"glTF placeholder")

            with mock.patch.object(server, "ensure_audio") as ensure_audio:
                result = server.render_talking_video(
                    script="待定生产音色干跑。",
                    modelPath=str(model),
                    outputDir=str(root / "out"),
                    renderMode="production",
                    voiceProvider="heygen",
                    voiceId="",
                    fallbackPolicy="error",
                    dryRun=True,
                )

            ensure_audio.assert_not_called()
            policy = result["voicePolicy"]
            self.assertEqual(policy["renderMode"], "production")
            self.assertEqual(policy["requestedProvider"], "heygen")
            self.assertEqual(policy["requestedVoiceId"], "")
            self.assertEqual(policy["fallbackPolicy"], "error")
            self.assertEqual(policy["policyStatus"], "blocked")
            self.assertFalse(policy["productionReady"])
            self.assertFalse(policy["allowPreviewFallback"])
            self.assertIn("voice ID", policy["policyError"])

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
            self.assertEqual(render_input["voice"]["requestedProvider"], "kokoro")
            self.assertEqual(render_input["voice"]["requestedVoiceId"], "zf_xiaobei")
            self.assertEqual(render_input["voice"]["language"], "zh")
            self.assertEqual(render_input["voice"]["speed"], 0.94)

    def test_main_ip_profile_declares_stable_aroll_master_contract(self) -> None:
        profile_path = pathlib.Path(__file__).resolve().parents[2] / "ip形象" / "main_ip" / "character-profile.json"
        profile = json.loads(profile_path.read_text(encoding="utf-8"))
        model = profile["model"]

        self.assertEqual(model["masterBlendPath"], "models/main-ip-aroll-master.blend")
        self.assertEqual(model["qualityTier"], "aroll_close")
        self.assertEqual(model["fingerTopology"], "three_digits_three_segments")
        expected_middle_roles = {
            f"finger_{digit}_mid_{side}"
            for side in ("l", "r")
            for digit in (1, 2, 3)
        }
        self.assertTrue(expected_middle_roles.issubset(model["requiredBoneRoles"]))
        self.assertEqual(profile["facial"]["blinkCapability"], "squint_only")
        self.assertFalse(
            any("Blink" in name for name in profile["facial"]["requiredShapeKeys"]),
            profile["facial"]["requiredShapeKeys"],
        )

    def test_main_ip_profile_pins_verified_local_production_voice(self) -> None:
        profile_path = pathlib.Path(__file__).resolve().parents[2] / "ip形象" / "main_ip" / "character-profile.json"
        profile = json.loads(profile_path.read_text(encoding="utf-8"))
        voice = profile["voice"]

        self.assertEqual(voice["renderMode"], "production")
        self.assertEqual(voice["provider"], "gpt_sovits_local")
        self.assertEqual(voice["voiceId"], "main_ip_warm_knowledge_host_v1")
        self.assertEqual(voice["fallbackPolicy"], "error")
        self.assertEqual(
            voice["preview"],
            {
                "provider": "apple",
                "voiceId": "Eddy (中文（中国大陆）)",
            },
        )
        self.assertEqual(
            voice["gptSovitsLocal"],
            {
                "endpoint": "http://127.0.0.1:9880",
                "allowRemoteEndpoint": False,
                "referenceSource": {
                    "path": "../ip音频.wav",
                    "sha256": "0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9",
                    "clipStartSec": 1.248396,
                    "clipDurationSec": 8.542,
                    "sampleRateHz": 48000,
                    "channels": 1,
                    "sampleFormat": "pcm_s16le",
                },
                "referenceAudioPath": "voice/reference/main_ip_voice_ref_v1.wav",
                "expectedReferenceAudioSha256": "0119b8a407ed1be5731db699d6166b7e4c86de540435c69bf4fb5229415382d8",
                "promptText": "唐影是一款面向创作者的AI视频生产系统，将选题、脚本、分镜、素材、审核与成片串成可追踪、可修改的自动化工作流。",
                "promptTextVerified": True,
                "promptLanguage": "zh",
                "textLanguage": "zh",
                "gptWeightsPath": "~/.local/share/tangying-aios/GPT-SoVITS/GPT_SoVITS/pretrained_models/s1v3.ckpt",
                "expectedGptWeightsSha256": "87133414860ea14ff6620c483a3db5ed07b44be42e2c3fcdad65523a729a745a",
                "sovitsWeightsPath": "~/.local/share/tangying-aios/GPT-SoVITS/GPT_SoVITS/pretrained_models/v2Pro/s2Gv2ProPlus.pth",
                "expectedSovitsWeightsSha256": "d42a22bbbf65fb2bbdd45ad6a66841156977db45c7aabe0a6992ff378d9c7d3b",
                "modelVersion": "v2ProPlus",
                "seed": 20260714,
                "timeoutSec": 120,
                "settings": {},
            },
        )
        self.assertNotIn("productionVoiceCandidates", voice)

    def test_main_ip_local_voice_health_resolves_pinned_bundle_without_synthesis(self) -> None:
        server = load_server()
        profile_path = pathlib.Path(__file__).resolve().parents[2] / "ip形象" / "main_ip" / "character-profile.json"
        provenance = {
            "provider": "gpt_sovits_local",
            "voiceId": "main_ip_warm_knowledge_host_v1",
            "bundleReady": True,
            "productionReady": False,
        }

        with mock.patch.object(server, "GPTSoVITSClient", create=True) as client_class:
            client_class.return_value.preflight.return_value = provenance
            result = server.check_gpt_sovits_voice(characterProfilePath=str(profile_path))

        self.assertEqual(result["status"], "ready")
        self.assertTrue(result["success"])
        self.assertEqual(result["provider"], "gpt_sovits_local")
        self.assertEqual(result["voice"], provenance)
        preflight = client_class.return_value.preflight.call_args.kwargs
        self.assertEqual(preflight["voice_id"], "main_ip_warm_knowledge_host_v1")
        self.assertEqual(preflight["seed"], 20260714)
        self.assertEqual(preflight["model_version"], "v2ProPlus")
        self.assertEqual(
            preflight["ref_audio_path"],
            str((profile_path.parent / "voice/reference/main_ip_voice_ref_v1.wav").resolve()),
        )
        self.assertEqual(
            preflight["gpt_weights_path"],
            str(
                pathlib.Path(
                    "~/.local/share/tangying-aios/GPT-SoVITS/GPT_SoVITS/pretrained_models/s1v3.ckpt"
                ).expanduser().resolve()
            ),
        )
        self.assertNotIn("promptText", result)

    def test_local_voice_health_reports_verified_bundle_without_synthesis(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            reference = root / "reference.wav"
            gpt_weights = root / "voice.ckpt"
            sovits_weights = root / "voice.pth"
            profile = root / "character-profile.json"
            for path, content in (
                (reference, b"RIFF reference"),
                (gpt_weights, b"gpt"),
                (sovits_weights, b"sovits"),
            ):
                path.write_bytes(content)
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "voice": {
                            "provider": "gpt_sovits_local",
                            "voiceId": "main-ip-gpt-sovits-v1",
                            "fallbackPolicy": "error",
                            "gptSovitsLocal": {
                                "endpoint": "http://127.0.0.1:9880",
                                "referenceAudioPath": "reference.wav",
                                "promptText": "人工核对的参考原文。",
                                "promptTextVerified": True,
                                "promptLanguage": "zh",
                                "gptWeightsPath": "voice.ckpt",
                                "sovitsWeightsPath": "voice.pth",
                                "modelVersion": "gpt-sovits-v2-main-ip-2026-07",
                            },
                        },
                    }
                ),
                encoding="utf-8",
            )
            provenance = {
                "provider": "gpt_sovits_local",
                "voiceId": "main-ip-gpt-sovits-v1",
                "referenceAudioSha256": "1" * 64,
                "gptWeightsSha256": "2" * 64,
                "sovitsWeightsSha256": "3" * 64,
                "endpoint": "http://127.0.0.1:9880",
                "modelIdentifier": "GPT-SoVITS/api_v2",
                "modelVersion": "gpt-sovits-v2-main-ip-2026-07",
                "seed": 24680,
                "settings": {},
                "bundleReady": True,
                "productionReady": False,
            }

            with mock.patch.object(server, "GPTSoVITSClient", create=True) as client_class:
                client_class.return_value.preflight.return_value = provenance
                result = server.check_gpt_sovits_voice(characterProfilePath=str(profile))

            self.assertEqual(result["status"], "ready")
            self.assertTrue(result["success"])
            self.assertEqual(result["voice"], provenance)
            preflight = client_class.return_value.preflight.call_args.kwargs
            self.assertEqual(preflight["ref_audio_path"], str(reference.resolve()))
            self.assertEqual(preflight["gpt_weights_path"], str(gpt_weights.resolve()))
            self.assertEqual(preflight["sovits_weights_path"], str(sovits_weights.resolve()))
            self.assertNotIn("promptText", result)

    def test_profile_preview_mode_uses_nested_preview_voice(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "main.glb"
            profile = root / "character-profile.json"
            model.write_bytes(b"glTF placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "model": {"path": "main.glb"},
                        "render": {},
                        "voice": {
                            "renderMode": "production",
                            "provider": "heygen",
                            "voiceId": "",
                            "fallbackPolicy": "error",
                            "preview": {
                                "provider": "apple",
                                "voiceId": "Eddy (中文（中国大陆）)",
                            },
                        },
                    }
                ),
                encoding="utf-8",
            )

            result = server.render_talking_video(
                script="显式预览。",
                characterProfilePath=str(profile),
                outputDir=str(root / "out"),
                renderMode="preview",
                dryRun=True,
            )

            policy = result["voicePolicy"]
            self.assertEqual(policy["renderMode"], "preview")
            self.assertEqual(policy["requestedProvider"], "apple")
            self.assertEqual(policy["requestedVoiceId"], "Eddy (中文（中国大陆）)")
            self.assertFalse(policy["productionReady"])

    def test_render_resolves_profile_local_voice_paths_before_ensure_audio(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "main.glb"
            reference = root / "reference.wav"
            gpt_weights = root / "voice.ckpt"
            sovits_weights = root / "voice.pth"
            profile = root / "character-profile.json"
            model.write_bytes(b"glTF placeholder")
            reference.write_bytes(b"RIFF reference")
            gpt_weights.write_bytes(b"gpt")
            sovits_weights.write_bytes(b"sovits")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "model": {"path": "main.glb"},
                        "render": {},
                        "voice": {
                            "renderMode": "production",
                            "provider": "gpt_sovits_local",
                            "voiceId": "main-ip-gpt-sovits-v1",
                            "fallbackPolicy": "error",
                            "language": "zh-CN",
                            "speed": 0.94,
                            "gptSovitsLocal": {
                                "endpoint": "http://127.0.0.1:9880",
                                "referenceAudioPath": "reference.wav",
                                "expectedReferenceAudioSha256": "1" * 64,
                                "promptText": "人工核对的参考原文。",
                                "promptTextVerified": True,
                                "promptLanguage": "zh",
                                "gptWeightsPath": "$GPT_SOVITS_HOME/voice.ckpt",
                                "expectedGptWeightsSha256": "2" * 64,
                                "sovitsWeightsPath": "$GPT_SOVITS_HOME/voice.pth",
                                "expectedSovitsWeightsSha256": "3" * 64,
                                "modelVersion": "gpt-sovits-v2-main-ip-2026-07",
                                "seed": 24680,
                            },
                        },
                    }
                ),
                encoding="utf-8",
            )

            with mock.patch.dict(os.environ, {"GPT_SOVITS_HOME": str(root)}), mock.patch.object(
                server,
                "ensure_audio",
                side_effect=RuntimeError("stop after config capture"),
            ) as ensure_audio_mock, self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "production voice synthesis failed"
            ):
                server.render_talking_video(
                    script="验证本地配置解析。",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "out"),
                    durationSec=2,
                    dryRun=False,
                )

            config = ensure_audio_mock.call_args.kwargs["voice_config"]
            self.assertEqual(config["referenceAudioPath"], str(reference.resolve()))
            self.assertEqual(config["gptWeightsPath"], str(gpt_weights.resolve()))
            self.assertEqual(config["sovitsWeightsPath"], str(sovits_weights.resolve()))
            self.assertEqual(config["promptText"], "人工核对的参考原文。")
            self.assertTrue(config["promptTextVerified"])

    def test_render_profile_uses_existing_master_without_source_refinement(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "models" / "source.fbx"
            master = root / "models" / "approved-master.blend"
            profile = root / "character-profile.json"
            model.parent.mkdir(parents=True)
            model.write_bytes(b"Kaydara FBX Binary placeholder")
            master.write_bytes(b"BLENDER placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "characterId": "main_ip_sloth",
                        "model": {
                            "path": "models/source.fbx",
                            "masterBlendPath": "models/approved-master.blend",
                            "enhanceExistingRig": True,
                        },
                        "facial": {
                            "mouthMode": "source_mesh_visemes",
                            "facialDetailMode": "rich",
                            "topologyMode": "source_retopology",
                            "blinkCapability": "squint_only",
                        },
                        "render": {},
                        "voice": {},
                    }
                ),
                encoding="utf-8",
            )

            result = server.render_talking_video(
                script="稳定主角色干跑计划。",
                characterProfilePath=str(profile),
                outputDir=str(root / "out"),
                durationSec=2,
                dryRun=True,
            )

            render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(render_input["masterBlendPath"], str(master.resolve()))
            self.assertTrue(render_input["useMasterAsset"])
            self.assertFalse(render_input["enhanceExistingRig"])
            self.assertEqual(render_input["facialTopologyMode"], "source_only")
            self.assertEqual(result["masterBlendPath"], str(master.resolve()))

    def test_render_rejects_configured_non_blend_master(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            model = root / "source.fbx"
            invalid_master = root / "master.glb"
            profile = root / "character-profile.json"
            model.write_bytes(b"Kaydara FBX Binary placeholder")
            invalid_master.write_bytes(b"glTF placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "model": {"path": "source.fbx", "masterBlendPath": "master.glb"},
                    }
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(ValueError, "masterBlendPath must point to a Blender .blend file"):
                server.render_talking_video(
                    script="invalid master",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "out"),
                    dryRun=True,
                )

    def test_render_requires_preparation_when_configured_master_is_missing(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source.fbx"
            profile = root / "character-profile.json"
            source.write_bytes(b"Kaydara FBX Binary placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "model": {
                            "path": "source.fbx",
                            "masterBlendPath": "missing-master.blend",
                        },
                    }
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(FileNotFoundError, "prepare_character_master"):
                server.render_talking_video(
                    script="must not fall back",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "out"),
                    dryRun=False,
                )

    def test_prepare_character_master_dry_run_writes_deterministic_qa_input(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source.fbx"
            profile = root / "character-profile.json"
            output = root / "models"
            source.write_bytes(b"Kaydara FBX Binary placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "characterId": "main_ip_sloth",
                        "model": {
                            "sourcePath": "source.fbx",
                            "masterBlendPath": "models/main-ip-aroll-master.blend",
                            "qualityTier": "aroll_close",
                            "rigMode": "auto",
                            "preserveExistingRig": True,
                            "enhanceExistingRig": True,
                        },
                        "facial": {
                            "mouthMode": "source_mesh_visemes",
                            "facialDetailMode": "rich",
                            "topologyMode": "source_retopology",
                            "blinkCapability": "squint_only",
                        },
                        "render": {"targetCharacterHeight": 2.55},
                    }
                ),
                encoding="utf-8",
            )

            result = server.prepare_character_master(
                sourceModel="",
                characterProfilePath=str(profile),
                outputDir=str(output),
                qualityTier="aroll_close",
                dryRun=True,
            )

            self.assertEqual(result["status"], "planned")
            self.assertEqual(pathlib.Path(result["masterBlendPath"]), output.resolve() / "main-ip-aroll-master.blend")
            self.assertEqual(pathlib.Path(result["exportGlbPath"]), output.resolve() / "main-ip-aroll-rigged.glb")
            self.assertEqual(pathlib.Path(result["rigReportPath"]), output.resolve() / "main-ip-aroll-rig-report.json")
            self.assertEqual(pathlib.Path(result["qaInputPath"]), output.resolve() / "main-ip-aroll-qa-input.json")
            qa_input = json.loads(pathlib.Path(result["qaInputPath"]).read_text(encoding="utf-8"))
            self.assertEqual(qa_input["sourceModel"], str(source.resolve()))
            self.assertEqual(qa_input["qualityTier"], "aroll_close")
            self.assertTrue(qa_input["assetOnly"])
            self.assertTrue(qa_input["prepareMaster"])
            self.assertEqual(qa_input["masterCollection"], "IP_Character_Master")

    def test_prepare_character_master_reports_canonical_quality_tier_spelling(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source.fbx"
            profile = root / "character-profile.json"
            source.write_bytes(b"Kaydara FBX Binary placeholder")
            profile.write_text(
                json.dumps(
                    {
                        "schemaVersion": "tangying-ip-character/v1",
                        "model": {
                            "sourcePath": "source.fbx",
                            "masterBlendPath": "master.blend",
                            "qualityTier": "aroll_close",
                        },
                    }
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(ValueError, "qualityTier must be aroll_close"):
                server.prepare_character_master(
                    sourceModel="",
                    characterProfilePath=str(profile),
                    qualityTier="preview",
                    dryRun=True,
                )

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

    def test_compose_command_preserves_headroom_for_mastered_audio(self) -> None:
        server = load_server()
        command = server._build_compose_video_args(
            frames_dir=pathlib.Path("/tmp/avatar_frames"),
            audio_path="/tmp/narration_master.wav",
            output_path=pathlib.Path("/tmp/ip_layer.mp4"),
            duration_sec=3,
            fps=30,
            audio_mastered=True,
        )

        joined = " ".join(command)
        self.assertIn(
            "volume=0.2dB,alimiter=limit=0.75:attack=5:release=50:level=false",
            joined,
        )
        self.assertNotIn("loudnorm=", joined)

    def test_final_production_audio_gate_rejects_encoded_peak_overshoot(self) -> None:
        server = load_server()
        with mock.patch.object(
            server,
            "_measure_voice_audition_loudness",
            return_value={
                "integratedLufs": -16.1,
                "truePeakDbtp": -1.4,
                "loudnessRangeLu": 2.0,
            },
        ):
            with self.assertRaisesRegex(server.ProductionVoiceUnavailable, "final encoded audio"):
                server._validate_final_production_audio(pathlib.Path("/tmp/final.mp4"))

    def test_generate_voice_auditions_returns_blind_candidates_and_private_manifest(self) -> None:
        server = load_server()
        script = "今天我们不追热点，只讲清楚一个真正重要的变化。"
        candidate_ids = ["heygen_voice_1", "heygen_voice_2", "heygen_voice_3"]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root, candidate_ids)
            output_dir = root / "auditions"

            result, synthesis_calls, ffmpeg_commands = self.run_successful_voice_auditions(
                server,
                script=script,
                profile=profile,
                output_dir=output_dir,
            )

            set_id = result["setId"]
            self.assertEqual(len(set_id), 64)
            self.assertTrue(all(char in "0123456789abcdef" for char in set_id))
            set_dir = output_dir.resolve() / set_id
            expected_public = [
                {"label": "A", "path": str(set_dir / "audition_A.wav")},
                {"label": "B", "path": str(set_dir / "audition_B.wav")},
                {"label": "C", "path": str(set_dir / "audition_C.wav")},
            ]
            self.assertEqual(result["status"], "ready")
            self.assertFalse(result["reusedExisting"])
            self.assertEqual(result["candidates"], expected_public)
            self.assertEqual(result["publicCandidates"], expected_public)
            self.assertTrue(result["requiresUserSelection"])
            self.assertNotIn("voiceId", json.dumps(result))
            for candidate_id in candidate_ids:
                self.assertNotIn(candidate_id, json.dumps(result))
            self.assertTrue(all(pathlib.Path(item["path"]).is_file() for item in expected_public))

            self.assertEqual(
                [call["voice_id"] for call in synthesis_calls],
                candidate_ids,
            )
            self.assertEqual({call["script"] for call in synthesis_calls}, {script})
            self.assertEqual({call["voice_provider"] for call in synthesis_calls}, {"heygen"})
            self.assertEqual({call["voice_language"] for call in synthesis_calls}, {"zh-CN"})
            self.assertEqual({call["voice_speed"] for call in synthesis_calls}, {0.94})
            self.assertEqual({call["speakingRate"] for call in synthesis_calls}, {185})
            mastering_commands = [args for args in ffmpeg_commands if args[-1] != "-"]
            self.assertEqual(len(mastering_commands), 3)
            self.assertTrue(
                all("loudnorm=I=-16:TP=-1.5:LRA=7" in " ".join(args) for args in mastering_commands)
            )

            manifest_path = set_dir / "manifest.json"
            manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
            self.assertEqual(manifest_path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(manifest["schemaVersion"], "ip-avatar-voice-auditions/v1")
            self.assertEqual(manifest["setId"], set_id)
            self.assertEqual(manifest["script"], script)
            self.assertEqual(manifest["provider"], "heygen")
            self.assertEqual(manifest["settings"]["language"], "zh-CN")
            self.assertEqual(manifest["settings"]["speed"], 0.94)
            self.assertEqual(manifest["settings"]["speakingRate"], 185)
            self.assertEqual(
                [item["providerVoiceId"] for item in manifest["candidates"]],
                candidate_ids,
            )
            self.assertEqual(
                [item["synthesisProvenance"]["voiceId"] for item in manifest["candidates"]],
                candidate_ids,
            )
            self.assertTrue(
                all(item["synthesisProvenance"]["ttsProvider"] == "heygen" for item in manifest["candidates"])
            )
            self.assertTrue(all(item["contentSha256"] for item in manifest["candidates"]))
            self.assertEqual(sorted(path.resolve() for path in output_dir.iterdir()), [set_dir])

    def test_generate_voice_auditions_dry_run_reports_blocked_without_audio(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)
            output_dir = root / "auditions"

            with mock.patch.object(server, "find_audio_engine", return_value=""), mock.patch.object(
                server, "ensure_audio"
            ) as ensure_audio_mock:
                result = server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(output_dir),
                    dryRun=True,
                )

            self.assertEqual(result["status"], "blocked")
            self.assertFalse(result["success"])
            self.assertTrue(result["dryRun"])
            self.assertTrue(result["requiresUserSelection"])
            self.assertEqual([item["label"] for item in result["candidates"]], ["A", "B", "C"])
            self.assertFalse(output_dir.exists())
            ensure_audio_mock.assert_not_called()

    def test_generate_voice_auditions_reuses_valid_existing_set_without_synthesis(self) -> None:
        server = load_server()
        script = "固定试音文案。"
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)
            output_dir = root / "auditions"
            first, _calls, _commands = self.run_successful_voice_auditions(
                server,
                script=script,
                profile=profile,
                output_dir=output_dir,
            )
            manifest_path = output_dir.resolve() / first["setId"] / "manifest.json"
            manifest_before = manifest_path.read_bytes()

            with mock.patch.object(server, "find_audio_engine", return_value=""), mock.patch.object(
                server, "ensure_audio"
            ) as ensure_audio_mock:
                second = server.generate_voice_auditions(
                    script=script,
                    characterProfilePath=str(profile),
                    outputDir=str(output_dir),
                )

            self.assertTrue(second["reusedExisting"])
            self.assertEqual(second["setId"], first["setId"])
            self.assertEqual(second["candidates"], first["candidates"])
            self.assertEqual(manifest_path.read_bytes(), manifest_before)
            ensure_audio_mock.assert_not_called()

    def test_generate_voice_auditions_rejects_invalid_existing_target_without_overwrite(self) -> None:
        server = load_server()
        script = "固定试音文案。"
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)
            output_dir = root / "auditions"
            plan = self.plan_voice_auditions(
                server,
                script=script,
                profile=profile,
                output_dir=output_dir,
            )
            target_dir = output_dir.resolve() / plan["setId"]
            target_dir.mkdir(parents=True)
            manifest_path = target_dir / "manifest.json"
            manifest_path.write_bytes(b"existing invalid manifest")

            with mock.patch.object(server, "ensure_audio") as ensure_audio_mock, self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "collision"
            ):
                server.generate_voice_auditions(
                    script=script,
                    characterProfilePath=str(profile),
                    outputDir=str(output_dir),
                )

            self.assertEqual(manifest_path.read_bytes(), b"existing invalid manifest")
            ensure_audio_mock.assert_not_called()

    def test_generate_voice_auditions_rejects_concurrent_set_lock(self) -> None:
        server = load_server()
        script = "固定试音文案。"
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)
            output_dir = root / "auditions"
            plan = self.plan_voice_auditions(
                server,
                script=script,
                profile=profile,
                output_dir=output_dir,
            )
            output_dir.mkdir(parents=True)
            lock_path = output_dir.resolve() / f".voice-auditions-{plan['setId']}.lock"
            lock_path.write_text("other-call", encoding="utf-8")

            with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                server, "ensure_audio"
            ) as ensure_audio_mock, self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "locked by another call"
            ):
                server.generate_voice_auditions(
                    script=script,
                    characterProfilePath=str(profile),
                    outputDir=str(output_dir),
                )

            self.assertEqual(lock_path.read_text(encoding="utf-8"), "other-call")
            self.assertFalse((output_dir / plan["setId"]).exists())
            ensure_audio_mock.assert_not_called()

    def test_failed_rerun_preserves_existing_valid_voice_audition_set(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)
            output_dir = root / "auditions"
            first, _calls, _commands = self.run_successful_voice_auditions(
                server,
                script="第一版固定试音文案。",
                profile=profile,
                output_dir=output_dir,
            )
            first_dir = output_dir.resolve() / first["setId"]
            first_snapshot = {
                path.name: path.read_bytes()
                for path in first_dir.iterdir()
            }
            failed_plan = self.plan_voice_auditions(
                server,
                script="第二版固定试音文案。",
                profile=profile,
                output_dir=output_dir,
            )

            with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                server, "ensure_audio", side_effect=RuntimeError("provider unavailable")
            ), self.assertRaisesRegex(server.ProductionVoiceUnavailable, "production synthesis failed"):
                server.generate_voice_auditions(
                    script="第二版固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(output_dir),
                )

            self.assertEqual(
                {path.name: path.read_bytes() for path in first_dir.iterdir()},
                first_snapshot,
            )
            self.assertFalse((output_dir.resolve() / failed_plan["setId"]).exists())
            self.assertFalse(
                (output_dir.resolve() / f".voice-auditions-{failed_plan['setId']}.lock").exists()
            )
            self.assertEqual(list(output_dir.glob(f".{failed_plan['setId']}.staging-*")), [])

    def test_generate_voice_auditions_rejects_invalid_candidate_count(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root, ["heygen_voice_1", "heygen_voice_2"])

            with self.assertRaisesRegex(ValueError, "exactly 3 unique nonempty"):
                server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "auditions"),
                )

    def test_generate_voice_auditions_fails_when_production_provider_is_unavailable(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)

            with mock.patch.object(server, "find_audio_engine", return_value=""), mock.patch.object(
                server, "ensure_audio"
            ) as ensure_audio_mock, self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "HeyGen production provider is unavailable"
            ):
                server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "auditions"),
                )

            ensure_audio_mock.assert_not_called()

    def test_generate_voice_auditions_fails_when_synthesis_provenance_is_missing(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)

            def fake_ensure_audio(_script, candidate_dir, _duration, **_kwargs):
                audio = pathlib.Path(candidate_dir) / "narration.wav"
                audio.parent.mkdir(parents=True, exist_ok=True)
                audio.write_bytes(b"RIFF audio")
                return str(audio), "hyperframes_unknown", {"tts_provider": "", "voice_id": ""}

            with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                server, "ensure_audio", side_effect=fake_ensure_audio
            ), self.assertRaisesRegex(server.ProductionVoiceUnavailable, "missing synthesis provenance"):
                server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "auditions"),
                )

    def test_generate_voice_auditions_fails_when_synthesis_provenance_mismatches(self) -> None:
        server = load_server()
        mismatches = [
            {"tts_provider": "elevenlabs", "voice_id": "heygen_voice_1"},
            {"tts_provider": "heygen", "voice_id": "different_voice"},
        ]
        for provenance in mismatches:
            with self.subTest(provenance=provenance), tempfile.TemporaryDirectory() as tmp:
                root = pathlib.Path(tmp)
                profile = self.write_voice_audition_profile(root)

                def fake_ensure_audio(_script, candidate_dir, _duration, **_kwargs):
                    audio = pathlib.Path(candidate_dir) / "narration.wav"
                    audio.parent.mkdir(parents=True, exist_ok=True)
                    audio.write_bytes(b"RIFF audio")
                    return str(audio), "hyperframes_mismatch", provenance

                with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                    server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
                ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                    server, "ensure_audio", side_effect=fake_ensure_audio
                ), self.assertRaisesRegex(server.ProductionVoiceUnavailable, "synthesis provenance mismatch"):
                    server.generate_voice_auditions(
                        script="固定试音文案。",
                        characterProfilePath=str(profile),
                        outputDir=str(root / "auditions"),
                    )

    def test_generate_voice_auditions_fails_when_synthesis_output_is_missing(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)

            def fake_ensure_audio(_script, candidate_dir, _duration, **kwargs):
                return str(pathlib.Path(candidate_dir) / "missing.wav"), "hyperframes_heygen", {
                    "tts_provider": "heygen",
                    "voice_id": kwargs["voice_id"],
                }

            with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                server, "ensure_audio", side_effect=fake_ensure_audio
            ), self.assertRaisesRegex(server.ProductionVoiceUnavailable, "synthesis output is missing"):
                server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "auditions"),
                )

    def test_generate_voice_auditions_fails_closed_when_normalization_fails(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)
            output_dir = root / "auditions"
            plan = self.plan_voice_auditions(
                server,
                script="固定试音文案。",
                profile=profile,
                output_dir=output_dir,
            )

            def fake_ensure_audio(_script, candidate_dir, _duration, **kwargs):
                audio = pathlib.Path(candidate_dir) / "narration.wav"
                audio.parent.mkdir(parents=True, exist_ok=True)
                audio.write_bytes(b"RIFF audio")
                return str(audio), "hyperframes_heygen", {
                    "tts_provider": "heygen",
                    "voice_id": kwargs["voice_id"],
                }

            with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                server, "ensure_audio", side_effect=fake_ensure_audio
            ), mock.patch.object(server, "_run", side_effect=RuntimeError("ffmpeg failed")), self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "normalization failed for candidate A"
            ):
                server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(output_dir),
                )

            self.assertFalse((output_dir.resolve() / plan["setId"]).exists())
            self.assertFalse(
                (output_dir.resolve() / f".voice-auditions-{plan['setId']}.lock").exists()
            )
            self.assertEqual(list(output_dir.glob(f".{plan['setId']}.staging-*")), [])

    def test_generate_voice_auditions_fails_when_loudness_analysis_fails(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            profile = self.write_voice_audition_profile(root)

            def fake_ensure_audio(_script, candidate_dir, _duration, **kwargs):
                audio = pathlib.Path(candidate_dir) / "narration.wav"
                audio.parent.mkdir(parents=True, exist_ok=True)
                audio.write_bytes(b"RIFF audio")
                return str(audio), "hyperframes_heygen", {
                    "tts_provider": "heygen",
                    "voice_id": kwargs["voice_id"],
                }

            run_count = 0

            def fake_run(args, timeout=600):
                nonlocal run_count
                run_count += 1
                if run_count == 1:
                    pathlib.Path(args[-1]).write_bytes(b"RIFF normalized")
                    return mock.Mock(returncode=0, stdout="", stderr="")
                raise RuntimeError("loudness analysis failed")

            with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                server, "ensure_audio", side_effect=fake_ensure_audio
            ), mock.patch.object(server, "_run", side_effect=fake_run), self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "loudness verification failed for candidate A"
            ):
                server.generate_voice_auditions(
                    script="固定试音文案。",
                    characterProfilePath=str(profile),
                    outputDir=str(root / "auditions"),
                )

    def test_generate_voice_auditions_fails_when_measured_loudness_is_outside_target(self) -> None:
        server = load_server()
        measurements = [
            {"input_i": "-14.9", "input_tp": "-1.6", "input_lra": "2.1"},
            {"input_i": "-16.0", "input_tp": "-1.4", "input_lra": "2.1"},
        ]
        for measurement in measurements:
            with self.subTest(measurement=measurement), tempfile.TemporaryDirectory() as tmp:
                root = pathlib.Path(tmp)
                profile = self.write_voice_audition_profile(root)

                def fake_ensure_audio(_script, candidate_dir, _duration, **kwargs):
                    audio = pathlib.Path(candidate_dir) / "narration.wav"
                    audio.parent.mkdir(parents=True, exist_ok=True)
                    audio.write_bytes(b"RIFF audio")
                    return str(audio), "hyperframes_heygen", {
                        "tts_provider": "heygen",
                        "voice_id": kwargs["voice_id"],
                    }

                def fake_run(args, timeout=600):
                    if args[-1] == "-":
                        return mock.Mock(returncode=0, stdout="", stderr=json.dumps(measurement))
                    pathlib.Path(args[-1]).write_bytes(b"RIFF normalized")
                    return mock.Mock(returncode=0, stdout="", stderr="")

                with mock.patch.object(server, "find_audio_engine", return_value="/tmp/audio.mjs"), mock.patch.object(
                    server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
                ), mock.patch.object(server.shutil, "which", return_value="/usr/bin/node"), mock.patch.object(
                    server, "ensure_audio", side_effect=fake_ensure_audio
                ), mock.patch.object(server, "_run", side_effect=fake_run), self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable, "outside loudness target"
                ):
                    server.generate_voice_auditions(
                        script="固定试音文案。",
                        characterProfilePath=str(profile),
                        outputDir=str(root / "auditions"),
                    )

    def test_local_gpt_sovits_ensure_audio_routes_directly_with_provenance(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            reference = root / "reference.wav"
            gpt_weights = root / "voice.ckpt"
            sovits_weights = root / "voice.pth"
            generated = root / "narration_gpt_sovits.wav"
            reference.write_bytes(b"RIFF reference")
            gpt_weights.write_bytes(b"gpt")
            sovits_weights.write_bytes(b"sovits")
            generated.write_bytes(test_wav_bytes())
            raw_generated_hash = hashlib.sha256(generated.read_bytes()).hexdigest()
            client_result = {
                "audioPath": str(generated),
                "provider": "gpt_sovits_local",
                "voiceId": "main-ip-gpt-sovits-v1",
                "tts_provider": "gpt_sovits_local",
                "voice_id": "main-ip-gpt-sovits-v1",
                "referenceAudioSha256": "1" * 64,
                "gptWeightsSha256": "2" * 64,
                "sovitsWeightsSha256": "3" * 64,
                "generatedFileSha256": raw_generated_hash,
                "endpoint": "http://127.0.0.1:9880",
                "modelIdentifier": "GPT-SoVITS/api_v2",
                "modelVersion": "gpt-sovits-v2-main-ip-2026-07",
                "seed": 24680,
                "settings": {"seed": 24680},
                "productionReady": True,
            }
            config = {
                "endpoint": "http://127.0.0.1:9880",
                "allowRemoteEndpoint": False,
                "referenceAudioPath": str(reference),
                "expectedReferenceAudioSha256": "1" * 64,
                "promptText": "人工核对的参考原文。",
                "promptTextVerified": True,
                "promptLanguage": "zh",
                "gptWeightsPath": str(gpt_weights),
                "expectedGptWeightsSha256": "2" * 64,
                "sovitsWeightsPath": str(sovits_weights),
                "expectedSovitsWeightsSha256": "3" * 64,
                "modelVersion": "gpt-sovits-v2-main-ip-2026-07",
                "seed": 24680,
                "timeoutSec": 120,
                "settings": {"top_k": 12},
            }

            commands = []

            def fake_run(args, timeout=600):
                commands.append(args)
                if args[-1] == "-":
                    null_pass = sum(1 for command in commands if command[-1] == "-")
                    if null_pass == 1:
                        return mock.Mock(
                            returncode=0,
                            stdout="",
                            stderr=(
                                '[Parsed_loudnorm_0] {\n'
                                '  "input_i" : "-24.8",\n'
                                '  "input_tp" : "-7.4",\n'
                                '  "input_lra" : "3.8",\n'
                                '  "input_thresh" : "-35.0",\n'
                                '  "output_i" : "-16.2",\n'
                                '  "output_tp" : "-1.5",\n'
                                '  "output_lra" : "3.1",\n'
                                '  "output_thresh" : "-26.5",\n'
                                '  "normalization_type" : "dynamic",\n'
                                '  "target_offset" : "0.2"\n'
                                "}"
                            ),
                        )
                    return mock.Mock(
                        returncode=0,
                        stdout="",
                        stderr=(
                            '[Parsed_loudnorm_0] {\n'
                            '  "input_i" : "-16.1",\n'
                            '  "input_tp" : "-1.7",\n'
                            '  "input_lra" : "3.2"\n'
                            "}"
                        ),
                    )
                pathlib.Path(args[-1]).write_bytes(test_wav_bytes())
                return mock.Mock(returncode=0, stdout="", stderr="")

            with mock.patch.object(server, "GPTSoVITSClient", create=True) as client_class, mock.patch.object(
                server, "find_audio_engine"
            ) as shared_engine, mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server, "_run", side_effect=fake_run):
                client_class.return_value.synthesize.return_value = client_result
                audio_path, source, metadata = server.ensure_audio(
                    "今天分享一个本地声音测试。",
                    root,
                    2.0,
                    speaking_rate=185,
                    voice_provider="gpt_sovits_local",
                    voice_id="main-ip-gpt-sovits-v1",
                    voice_language="zh-CN",
                    voice_speed=0.94,
                    voice_config=config,
                )

            client_class.assert_called_once_with(
                endpoint="http://127.0.0.1:9880",
                timeout_sec=120.0,
                allow_remote=False,
            )
            synthesize_kwargs = client_class.return_value.synthesize.call_args.kwargs
            self.assertEqual(synthesize_kwargs["text_lang"], "zh")
            self.assertEqual(synthesize_kwargs["ref_audio_path"], str(reference))
            self.assertEqual(synthesize_kwargs["prompt_text"], "人工核对的参考原文。")
            self.assertTrue(synthesize_kwargs["prompt_text_verified"])
            self.assertEqual(synthesize_kwargs["gpt_weights_path"], str(gpt_weights))
            self.assertEqual(synthesize_kwargs["sovits_weights_path"], str(sovits_weights))
            self.assertEqual(synthesize_kwargs["voice_id"], "main-ip-gpt-sovits-v1")
            self.assertEqual(synthesize_kwargs["seed"], 24680)
            self.assertEqual(synthesize_kwargs["settings"], {"top_k": 12, "speed_factor": 0.94})
            mastered = (root / "narration_gpt_sovits_master.wav").resolve()
            self.assertEqual(pathlib.Path(audio_path), mastered)
            self.assertEqual(source, "gpt_sovits_local")
            self.assertEqual(metadata["referenceAudioSha256"], "1" * 64)
            self.assertEqual(metadata["generatedFileSha256"], raw_generated_hash)
            self.assertEqual(metadata["rawGeneratedFileSha256"], raw_generated_hash)
            self.assertEqual(
                metadata["masteredFileSha256"],
                hashlib.sha256(mastered.read_bytes()).hexdigest(),
            )
            self.assertEqual(
                metadata["loudnessMeasurement"],
                {
                    "integratedLufs": -16.1,
                    "truePeakDbtp": -1.7,
                    "loudnessRangeLu": 3.2,
                },
            )
            self.assertEqual(len(commands), 3)
            analysis = commands[0]
            analysis_filter = analysis[analysis.index("-af") + 1]
            self.assertLess(
                analysis_filter.index("aresample=48000"),
                analysis_filter.index("lowpass=f=18000"),
            )
            self.assertIn("print_format=json", analysis_filter)
            mastering = commands[1]
            mastering_filter = mastering[mastering.index("-af") + 1]
            self.assertIn("highpass=f=55", mastering_filter)
            self.assertIn("lowpass=f=18000", mastering_filter)
            self.assertIn("acompressor", mastering_filter)
            self.assertIn("threshold=-20dB", mastering_filter)
            self.assertIn("ratio=2.5", mastering_filter)
            self.assertIn("loudnorm=I=-16:TP=-1.5:LRA=7", mastering_filter)
            self.assertIn("measured_I=-24.8", mastering_filter)
            self.assertIn("measured_TP=-7.4", mastering_filter)
            self.assertIn("measured_LRA=3.8", mastering_filter)
            self.assertIn("measured_thresh=-35.0", mastering_filter)
            self.assertIn("offset=0.2", mastering_filter)
            self.assertIn("linear=true", mastering_filter)
            self.assertEqual(mastering[mastering.index("-ar") + 1], "48000")
            self.assertEqual(mastering[mastering.index("-ac") + 1], "1")
            self.assertEqual(mastering[mastering.index("-c:a") + 1], "pcm_s16le")
            self.assertTrue(metadata["humanVoiceProvider"])
            self.assertTrue(metadata["productionReady"])
            shared_engine.assert_not_called()

    def test_local_gpt_sovits_mastering_fails_closed_outside_loudness_target(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            generated = root / "narration_gpt_sovits.wav"
            generated.write_bytes(test_wav_bytes())
            raw_generated_hash = hashlib.sha256(generated.read_bytes()).hexdigest()
            client_result = {
                "audioPath": str(generated),
                "tts_provider": "gpt_sovits_local",
                "voice_id": "main-ip-gpt-sovits-v1",
                "referenceAudioSha256": "1" * 64,
                "gptWeightsSha256": "2" * 64,
                "sovitsWeightsSha256": "3" * 64,
                "generatedFileSha256": raw_generated_hash,
                "endpoint": "http://127.0.0.1:9880",
                "modelIdentifier": "GPT-SoVITS/api_v2",
                "modelVersion": "v2ProPlus",
                "seed": 20260714,
                "settings": {"seed": 20260714},
                "productionReady": True,
            }

            def fake_run(args, timeout=600):
                if args[-1] == "-":
                    analysis_filter = args[args.index("-af") + 1]
                    if "aresample=48000" in analysis_filter:
                        return mock.Mock(
                            returncode=0,
                            stdout="",
                            stderr=(
                                '[Parsed_loudnorm_0] {\n'
                                '  "input_i" : "-24.8",\n'
                                '  "input_tp" : "-7.4",\n'
                                '  "input_lra" : "3.8",\n'
                                '  "input_thresh" : "-35.0",\n'
                                '  "target_offset" : "0.2"\n'
                                "}"
                            ),
                        )
                    return mock.Mock(
                        returncode=0,
                        stdout="",
                        stderr=(
                            '[Parsed_loudnorm_0] {\n'
                            '  "input_i" : "-15.2",\n'
                            '  "input_tp" : "-1.4",\n'
                            '  "input_lra" : "3.2"\n'
                            "}"
                        ),
                    )
                pathlib.Path(args[-1]).write_bytes(test_wav_bytes())
                return mock.Mock(returncode=0, stdout="", stderr="")

            with mock.patch.object(server, "GPTSoVITSClient", create=True) as client_class, mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(server, "_run", side_effect=fake_run), self.assertRaisesRegex(
                server.ProductionVoiceUnavailable, "outside loudness target"
            ):
                client_class.return_value.synthesize.return_value = client_result
                server.ensure_audio(
                    "验证母带响度门槛。",
                    root,
                    2.0,
                    voice_provider="gpt_sovits_local",
                    voice_id="main-ip-gpt-sovits-v1",
                    voice_config={
                        "promptText": "人工核对的参考原文。",
                        "promptTextVerified": True,
                    },
                )

            self.assertFalse((root / "narration_gpt_sovits_master.wav").exists())

    def test_explicit_local_gpt_sovits_failure_never_uses_shared_or_preview_fallback(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            config = {
                "endpoint": "http://127.0.0.1:9880",
                "referenceAudioPath": str(root / "reference.wav"),
                "promptText": "人工核对的参考原文。",
                "promptTextVerified": True,
                "promptLanguage": "zh",
                "gptWeightsPath": str(root / "voice.ckpt"),
                "sovitsWeightsPath": str(root / "voice.pth"),
                "modelVersion": "gpt-sovits-v2-main-ip-2026-07",
            }

            with mock.patch.object(server, "GPTSoVITSClient", create=True) as client_class, mock.patch.object(
                server, "find_audio_engine"
            ) as shared_engine, mock.patch.object(server, "find_ffmpeg") as ffmpeg, mock.patch.object(
                server.shutil, "which"
            ) as which, self.assertRaisesRegex(RuntimeError, "local backend unavailable"):
                client_class.return_value.synthesize.side_effect = RuntimeError("local backend unavailable")
                server.ensure_audio(
                    "本地后端失败必须封闭。",
                    root,
                    2.0,
                    voice_provider="gpt_sovits_local",
                    voice_id="main-ip-gpt-sovits-v1",
                    voice_config=config,
                )

            shared_engine.assert_not_called()
            ffmpeg.assert_not_called()
            which.assert_not_called()

    def test_local_gpt_sovits_rejects_unready_or_incomplete_adapter_provenance(self) -> None:
        server = load_server()
        invalid_results = [
            {
                "audioPath": "/tmp/generated.wav",
                "tts_provider": "gpt_sovits_local",
                "voice_id": "main-ip-gpt-sovits-v1",
                "productionReady": False,
            },
            {
                "audioPath": "/tmp/generated.wav",
                "tts_provider": "gpt_sovits_local",
                "voice_id": "different-bundle",
                "referenceAudioSha256": "1" * 64,
                "gptWeightsSha256": "2" * 64,
                "sovitsWeightsSha256": "3" * 64,
                "generatedFileSha256": "4" * 64,
                "endpoint": "http://127.0.0.1:9880",
                "modelIdentifier": "GPT-SoVITS/api_v2",
                "modelVersion": "v2",
                "seed": 24680,
                "settings": {},
                "productionReady": True,
            },
        ]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            config = {
                "endpoint": "http://127.0.0.1:9880",
                "promptText": "人工核对的参考原文。",
                "promptTextVerified": True,
            }
            for result in invalid_results:
                with self.subTest(result=result), mock.patch.object(
                    server, "GPTSoVITSClient", create=True
                ) as client_class, self.assertRaisesRegex(
                    server.ProductionVoiceUnavailable, "provenance is incomplete or mismatched"
                ):
                    client_class.return_value.synthesize.return_value = result
                    server.ensure_audio(
                        "验证本地来源。",
                        root,
                        2.0,
                        voice_provider="gpt_sovits_local",
                        voice_id="main-ip-gpt-sovits-v1",
                        voice_config=config,
                    )

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
            self.assertEqual(metadata["tts_provider"], "kokoro")
            self.assertEqual(metadata["voice_id"], "zf_xiaobei")
            self.assertFalse(metadata["humanVoiceProvider"])

    def test_audio_engine_does_not_substitute_requested_voice_for_missing_provenance(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            engine = root / "audio.mjs"
            engine.write_text("// fake audio engine", encoding="utf-8")

            def fake_run(args, timeout=600):
                meta_path = pathlib.Path(args[args.index("--out") + 1])
                audio = root / "assets" / "voice" / "narration.wav"
                audio.parent.mkdir(parents=True, exist_ok=True)
                audio.write_bytes(b"RIFF fake wav")
                meta_path.write_text(
                    json.dumps(
                        {
                            "voices": [
                                {
                                    "id": "narration",
                                    "path": "assets/voice/narration.wav",
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
                _audio_path, source, metadata = server.ensure_audio(
                    "生产来源不能靠请求填充。",
                    root,
                    2.0,
                    voice_provider="heygen",
                    voice_id="dMkR1XwIkarpNqWUJLnX",
                )

            self.assertEqual(source, "hyperframes_unknown")
            self.assertEqual(metadata["provider"], "")
            self.assertEqual(metadata["voiceId"], "")
            self.assertEqual(metadata["tts_provider"], "")
            self.assertEqual(metadata["voice_id"], "")
            self.assertFalse(metadata["humanVoiceProvider"])
            self.assertFalse(metadata["productionReady"])

    def test_uploaded_audio_metadata_is_unverified(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            uploaded = root / "uploaded.wav"
            uploaded.write_bytes(b"RIFF uploaded audio")

            audio_path, source, metadata = server.ensure_audio(
                "",
                root,
                2.0,
                audio_path=str(uploaded),
            )

            self.assertEqual(pathlib.Path(audio_path), uploaded.resolve())
            self.assertEqual(source, "uploaded_audio")
            self.assertEqual(metadata["provider"], "uploaded")
            self.assertEqual(metadata["voiceId"], "")
            self.assertFalse(metadata["humanVoiceProvider"])
            self.assertFalse(metadata["productionReady"])

    def test_local_preview_does_not_report_requested_voice_as_actual_provenance(self) -> None:
        server = load_server()
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)

            def fake_run(args, timeout=600):
                output = pathlib.Path(args[args.index("-o") + 1]) if args[0].endswith("say") else pathlib.Path(args[-1])
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_bytes(b"audio")
                return mock.Mock(returncode=0, stdout="", stderr="")

            with mock.patch.object(server, "find_audio_engine", return_value=""), mock.patch.object(
                server, "find_ffmpeg", return_value="/usr/local/bin/ffmpeg"
            ), mock.patch.object(
                server.shutil,
                "which",
                side_effect=lambda name: "/usr/bin/say" if name == "say" else None,
            ), mock.patch.object(server, "_run", side_effect=fake_run):
                _audio_path, source, metadata = server.ensure_audio(
                    "预览本地音色。",
                    root,
                    2.0,
                    voice_name="Eddy (中文（中国大陆）)",
                    voice_provider="auto",
                )

            self.assertEqual(source, "local_say_preview")
            self.assertEqual(metadata["provider"], "macos_say")
            self.assertEqual(metadata["voiceId"], "")
            self.assertFalse(metadata["productionReady"])

    def test_apple_character_voice_remains_preview_only_after_mastering(self) -> None:
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
            self.assertFalse(metadata["humanVoiceProvider"])
            self.assertFalse(metadata["productionReady"])
            self.assertNotIn("naturalVoiceProvider", metadata)
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


class MasterAssetHelperTests(unittest.TestCase):
    @staticmethod
    def collection(*objects, version=1):
        collection = {"ip_aroll_master_version": version}
        collection = types.SimpleNamespace(
            name="IP_Character_Master",
            all_objects=list(objects),
            get=collection.get,
        )
        return collection

    def test_master_validation_rejects_missing_version(self) -> None:
        master = load_master_asset()
        armature = types.SimpleNamespace(name="Rig", type="ARMATURE")
        collection = types.SimpleNamespace(
            name=master.MASTER_COLLECTION,
            all_objects=[armature],
            get=lambda _key, default=None: default,
        )

        with self.assertRaisesRegex(RuntimeError, "missing ip_aroll_master_version"):
            master.validate_master_collection(collection, pathlib.Path("missing-version.blend"))

    def test_master_validation_rejects_non_integer_versions(self) -> None:
        master = load_master_asset()
        armature = types.SimpleNamespace(name="Rig", type="ARMATURE")

        for invalid_version in (1.5, True, "1"):
            with self.subTest(version=invalid_version), self.assertRaisesRegex(
                RuntimeError, "invalid ip_aroll_master_version"
            ):
                master.validate_master_collection(
                    self.collection(armature, version=invalid_version),
                    pathlib.Path("invalid-version.blend"),
                )

    def test_master_validation_rejects_missing_or_duplicate_armature(self) -> None:
        master = load_master_asset()
        mesh = types.SimpleNamespace(name="Body", type="MESH")
        first = types.SimpleNamespace(name="RigA", type="ARMATURE")
        second = types.SimpleNamespace(name="RigB", type="ARMATURE")

        with self.assertRaisesRegex(RuntimeError, "exactly one Armature"):
            master.validate_master_collection(self.collection(mesh), pathlib.Path("missing-rig.blend"))
        with self.assertRaisesRegex(RuntimeError, "duplicate Armatures"):
            master.save_master_collection(
                character_objects=[mesh, first, second],
                armature=first,
                output_path=pathlib.Path("duplicate-rig.blend"),
            )

    def test_append_master_collection_loads_actions_and_links_valid_collection(self) -> None:
        master = load_master_asset()
        armature = types.SimpleNamespace(name="Rig", type="ARMATURE")
        body = types.SimpleNamespace(name="Body", type="MESH")
        loaded_collection = self.collection(body, armature)
        target = types.SimpleNamespace(collections=[], actions=[])
        requested = {}

        class LibraryContext:
            def __enter__(self):
                source = types.SimpleNamespace(
                    collections=[master.MASTER_COLLECTION],
                    actions=["Aroll_Idle", "Aroll_Greeting_Wave"],
                )
                return source, target

            def __exit__(self, exc_type, exc, traceback):
                requested["collections"] = list(target.collections)
                requested["actions"] = list(target.actions)
                target.collections = [loaded_collection]
                return False

        class Children:
            def __init__(self):
                self.linked = []

            def link(self, collection):
                self.linked.append(collection)

        fake_bpy = types.SimpleNamespace(
            data=types.SimpleNamespace(
                collections={},
                libraries=types.SimpleNamespace(load=lambda *_args, **_kwargs: LibraryContext()),
            )
        )
        scene_collection = types.SimpleNamespace(children=Children())

        with tempfile.TemporaryDirectory() as tmp:
            master_path = pathlib.Path(tmp) / "approved.blend"
            master_path.write_bytes(b"BLENDER placeholder")
            with mock.patch.object(master, "bpy", fake_bpy):
                objects = master.append_master_collection(master_path, scene_collection)

        self.assertEqual(requested["collections"], [master.MASTER_COLLECTION])
        self.assertEqual(requested["actions"], ["Aroll_Idle", "Aroll_Greeting_Wave"])
        self.assertEqual(objects, [body, armature])
        self.assertEqual(scene_collection.children.linked, [loaded_collection])

    def test_append_master_collection_rejects_missing_named_collection(self) -> None:
        master = load_master_asset()
        target = types.SimpleNamespace(collections=[], actions=[])

        class LibraryContext:
            def __enter__(self):
                return types.SimpleNamespace(collections=["Other"], actions=[]), target

            def __exit__(self, exc_type, exc, traceback):
                return False

        fake_bpy = types.SimpleNamespace(
            data=types.SimpleNamespace(
                collections={},
                libraries=types.SimpleNamespace(load=lambda *_args, **_kwargs: LibraryContext()),
            )
        )
        scene_collection = types.SimpleNamespace(children=types.SimpleNamespace(link=lambda _collection: None))

        with tempfile.TemporaryDirectory() as tmp:
            master_path = pathlib.Path(tmp) / "invalid.blend"
            master_path.write_bytes(b"BLENDER placeholder")
            with mock.patch.object(master, "bpy", fake_bpy), self.assertRaisesRegex(
                RuntimeError, "missing IP_Character_Master"
            ):
                master.append_master_collection(master_path, scene_collection)


class BlenderMasterAssetIntegrationTests(unittest.TestCase):
    def test_blender_saves_and_appends_versioned_master_with_actions(self) -> None:
        try:
            import bpy
        except ImportError:
            self.skipTest("requires Blender bpy")

        master = load_master_asset()
        with tempfile.TemporaryDirectory() as tmp:
            master_path = pathlib.Path(tmp) / "integration-master.blend"
            bpy.ops.wm.read_factory_settings(use_empty=True)
            bpy.ops.mesh.primitive_cube_add()
            body = bpy.context.object
            body.name = "IP_Test_Body"
            armature_data = bpy.data.armatures.new("IP_Test_Rig_Data")
            armature = bpy.data.objects.new("IP_Test_Rig", armature_data)
            bpy.context.scene.collection.objects.link(armature)
            action = bpy.data.actions.new("Aroll_Test_Action")
            action.use_fake_user = True

            saved = master.save_master_collection(
                character_objects=[body],
                armature=armature,
                output_path=master_path,
            )

            self.assertTrue(master_path.is_file())
            self.assertEqual(saved["collection"], master.MASTER_COLLECTION)
            self.assertEqual(saved["version"], master.MASTER_VERSION)
            bpy.ops.wm.read_factory_settings(use_empty=True)

            objects = master.append_master_collection(master_path, bpy.context.scene.collection)

            self.assertEqual([obj.name for obj in objects if obj.type == "ARMATURE"], ["IP_Test_Rig"])
            self.assertIn("IP_Test_Body", {obj.name for obj in objects})
            self.assertIsNotNone(bpy.data.actions.get("Aroll_Test_Action"))
            collection = bpy.data.collections.get(master.MASTER_COLLECTION)
            self.assertEqual(collection[master.MASTER_VERSION_PROPERTY], master.MASTER_VERSION)

    def test_full_master_authored_studio_path_preserves_canonical_assets(self) -> None:
        try:
            import bpy
        except ImportError:
            self.skipTest("requires Blender bpy")

        master = load_master_asset()
        renderer = load_blender_renderer()

        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            master_path = root / "full-path-master.blend"
            studio_path = root / "authored-studio.blend"

            bpy.ops.wm.read_factory_settings(use_empty=True)
            bpy.ops.mesh.primitive_cube_add(size=2.0)
            mouth = bpy.context.object
            mouth.name = "IP_Test_Character"
            material = bpy.data.materials.new("IP_Test_Character_Material")
            material.use_nodes = True
            mouth.data.materials.append(material)
            for shape_name in (
                "Basis",
                "Mouth_Rest",
                "Mouth_A",
                "Mouth_E",
                "Mouth_O",
                "Mouth_U",
                "Mouth_MBP",
                "Mouth_Smile",
                "Mouth_Frown",
                "Mouth_Surprise",
                "Eye_Squint.L",
                "Eye_Squint.R",
            ):
                mouth.shape_key_add(name=shape_name)

            armature_data = bpy.data.armatures.new("IP_Test_Master_Rig_Data")
            armature = bpy.data.objects.new("IP_Test_Master_Rig", armature_data)
            bpy.context.scene.collection.objects.link(armature)
            bpy.ops.object.select_all(action="DESELECT")
            armature.select_set(True)
            bpy.context.view_layer.objects.active = armature
            bpy.ops.object.mode_set(mode="EDIT")
            root_bone = armature.data.edit_bones.new("Root")
            root_bone.head = (0.0, 0.0, -1.0)
            root_bone.tail = (0.0, 0.0, 1.0)
            bpy.ops.object.mode_set(mode="OBJECT")
            armature_modifier = mouth.modifiers.new("IP_Test_Armature", "ARMATURE")
            armature_modifier.object = armature
            root_group = mouth.vertex_groups.new(name="Root")
            root_group.add(range(len(mouth.data.vertices)), 1.0, "REPLACE")

            armature.animation_data_create()
            talk_action = bpy.data.actions.new("Talk_Loop")
            talk_action.use_fake_user = True
            armature.animation_data.action = talk_action
            armature.pose.bones["Root"].rotation_mode = "XYZ"
            armature.pose.bones["Root"].rotation_euler.z = 0.1
            armature.pose.bones["Root"].keyframe_insert(data_path="rotation_euler", frame=1)

            mouth.data.shape_keys.animation_data_create()
            mouth_action = bpy.data.actions.new("Mouth_Viseme_Timeline")
            mouth_action.use_fake_user = True
            mouth.data.shape_keys.animation_data.action = mouth_action
            mouth.data.shape_keys.key_blocks["Mouth_A"].value = 0.5
            mouth.data.shape_keys.key_blocks["Mouth_A"].keyframe_insert(data_path="value", frame=1)

            for action_name in ("Aroll_Greeting_Wave", "Gesture_Wave", "Expression_Happy"):
                action = bpy.data.actions.new(action_name)
                action.use_fake_user = True

            character_objects = [mouth]
            shape_key_names = {key.name for key in mouth.data.shape_keys.key_blocks}
            material_names = {material.name}
            canonical_actions = {action.name for action in bpy.data.actions}
            master.save_master_collection(
                character_objects=character_objects,
                armature=armature,
                output_path=master_path,
            )

            bpy.ops.wm.read_factory_settings(use_empty=True)
            camera_data = bpy.data.cameras.new("Camera_Medium_Data")
            camera = bpy.data.objects.new("Camera_Medium", camera_data)
            bpy.context.scene.collection.objects.link(camera)
            camera.location = (0.0, -6.0, 1.5)
            bpy.context.scene.camera = camera
            light_data = bpy.data.lights.new("Key_Light_Data", type="AREA")
            light_data.energy = 500.0
            key_light = bpy.data.objects.new("Key_Light", light_data)
            key_light["ip_light_role"] = "key"
            key_light["ip_base_energy"] = 500.0
            bpy.context.scene.collection.objects.link(key_light)
            spawn = bpy.data.objects.new("IP_Character_Spawn", None)
            spawn["target_height"] = 2.55
            bpy.context.scene.collection.objects.link(spawn)
            focus = bpy.data.objects.new("IP_Focus_Head", None)
            bpy.context.scene.collection.objects.link(focus)
            bpy.ops.wm.save_as_mainfile(filepath=str(studio_path))

            render_input = {
                "assetOnly": True,
                "useMasterAsset": True,
                "masterBlendPath": str(master_path),
                "modelPath": "",
                "sceneBlendPath": str(studio_path),
                "backgroundPath": "",
                "backgroundMode": "blender_scene",
                "durationSec": 1.0,
                "fps": 30,
                "resolution": {"width": 640, "height": 360},
                "transparent": False,
                "cameraPreset": "medium",
                "cameraPlan": [{"frame": 1, "camera": "Camera_Medium"}],
                "lightingPreset": "editorial_soft",
                "renderEngine": "BLENDER_EEVEE_NEXT",
                "eeveeSamples": 16,
                "cyclesSamples": 16,
                "targetCharacterHeight": 2.55,
                "faceScreenMode": "source",
                "rigMode": "auto",
                "preserveExistingRig": True,
                "enhanceExistingRig": False,
                "mouthMode": "source_mesh_visemes",
                "facialDetailMode": "rich",
                "facialTopologyMode": "source_only",
                "characterId": "main_ip_sloth",
                "motionPlan": {
                    "durationSec": 1.0,
                    "motionEvents": [],
                    "lipSync": [{"timeSec": 0.0, "viseme": "a", "open": 0.5}],
                },
                "riggedBlendPath": str(root / "runtime.blend"),
                "riggedGlbPath": str(root / "runtime.glb"),
                "rigReportPath": str(root / "runtime-report.json"),
            }
            captured = {}

            def capture_assets(_data, objects, _assets, runtime_armature, face, *_args, **_kwargs):
                captured["objects"] = list(objects)
                captured["armature"] = runtime_armature
                captured["face"] = dict(face)

            with mock.patch.object(renderer, "read_input", return_value=render_input), mock.patch.object(
                renderer, "save_rigged_assets", side_effect=capture_assets
            ):
                renderer.main()

            action_names = {action.name for action in bpy.data.actions}
            duplicate_actions = sorted(
                name
                for name in action_names
                for canonical in canonical_actions
                if name.startswith(f"{canonical}.") and name[len(canonical) + 1 :].isdigit()
            )
            self.assertEqual(duplicate_actions, [])
            self.assertTrue(canonical_actions.issubset(action_names))
            self.assertEqual(
                [obj.name for obj in bpy.context.scene.objects if obj.type == "ARMATURE"],
                [captured["armature"].name],
            )
            runtime_mouth = captured["face"]["mouth"]
            self.assertEqual({key.name for key in runtime_mouth.data.shape_keys.key_blocks}, shape_key_names)
            self.assertEqual(
                {
                    material.name
                    for obj in captured["objects"]
                    for material in obj.data.materials
                    if material
                },
                material_names,
            )
            self.assertIsNotNone(bpy.data.objects.get("IP_Character_Spawn"))


if __name__ == "__main__":
    unittest_args = [sys.argv[0]]
    if "--" in sys.argv:
        unittest_args.extend(sys.argv[sys.argv.index("--") + 1 :])
    else:
        unittest_args.extend(sys.argv[1:])
    unittest.main(argv=unittest_args)
