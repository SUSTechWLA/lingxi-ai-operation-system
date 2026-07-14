# Sloth A-roll Transition And Action Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the warm-studio sloth into a stateful A-roll performer that can sit down and stand up on camera, use a richer reusable gesture library, speak with readable source-geometry visemes, and publish corrected 1080p demos without the current seated-leg artifact.

**Architecture:** Keep semantic poses and state transitions in `aroll_actions.py`, expose a pure action catalog and sequence resolver through `server.py`, and sample the resolved events inside the existing Blender timeline renderer. Re-author the seated studio contract around explicit seat, knee, and foot targets, preserve the original facial mesh, and extend the fail-closed warm-studio demo publisher with transition, action-pack, geometry, and viseme QA evidence.

**Tech Stack:** Python 3.11, Blender 5.1.2 Python API, Eevee Next, FastMCP, FFmpeg/FFprobe, local GPT-SoVITS, unittest, Go local-MCP bridge tests.

## Global Constraints

- Work only on `codex/sloth-warm-studio-integration` in `/Users/wanglian/.config/superpowers/worktrees/tangying-ai-operation-system/sloth-warm-studio-integration`.
- Preserve the canonical source humanoid rig, original mouth/brow/eyelid/eye geometry, source materials, wrists, and independent three-segment fingers.
- Do not add a mouth card, replacement face texture, dynamic background, locomotion, running, jumping, or stunt animation.
- Production animation remains 1920x1080, 30 fps CFR, Eevee Next, H.264 plus AAC 48 kHz mono.
- Render exactly one canonical Blender validation demo, 15-30 seconds long with a target near 24 seconds; derive review aliases or trims with FFmpeg instead of rendering multiple full performances.
- Production demos use `gpt_sovits_local` with `main_ip_warm_knowledge_host_v1`; no TTS fallback is allowed.
- Standing and seated remain valid initial presentation modes; state-changing actions may transition between them inside one shot.
- Generated `.blend`, `.glb`, images, reports, audio, and videos remain untracked output artifacts unless an existing tracked studio asset must be rebuilt.
- Publish final demo files only after Python, Blender, geometry, media, mouth, collision, and encoded-audio gates pass.
- Commit after every task and do not stage unrelated untracked model, voice, backup, or output files.

---

### Task 1: Define The Stateful A-roll Action Contract

**Files:**
- Create: `mcp/ip_avatar_3d/test_aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/aroll_actions.py`

**Interfaces:**
- Consumes: existing semantic pose dictionaries, hand poses, and `presentation_pose(mode, source_rig)`.
- Produces: `ActionMetadata`, `ACTION_CATALOG`, `action_catalog_payload()`, `resolve_action_sequence()`, `build_action_events()`, and state-aware action specs.

- [ ] **Step 1: Write failing catalog and state-resolution tests**

```python
import unittest

import aroll_actions


class ArollActionContractTests(unittest.TestCase):
    def test_catalog_has_unique_stateful_actions(self) -> None:
        payload = aroll_actions.action_catalog_payload()
        names = [item["name"] for item in payload["actions"]]
        self.assertEqual(len(names), len(set(names)))
        self.assertTrue(
            {
                "Aroll_Standing_Idle",
                "Aroll_Seated_Idle",
                "Aroll_Transition_StandToSit",
                "Aroll_Transition_SitToStand",
                "Aroll_Welcome_OpenArms",
                "Aroll_Question_PalmUp",
                "Aroll_Compare_TwoSides",
                "Aroll_KeyPoint_OneFinger",
                "Aroll_List_Three",
                "Aroll_Caution_Stop",
                "Aroll_Quote_Frame",
                "Aroll_Conclusion_HandsTogether",
                "Aroll_Seated_Explain",
                "Aroll_Seated_OpenPalm",
                "Aroll_Seated_LeanIn",
            }.issubset(names)
        )
        for item in payload["actions"]:
            self.assertIn(item["startState"], {"standing", "seated", "either"})
            self.assertIn(item["endState"], {"standing", "seated", "either"})
            self.assertGreater(item["durationSec"], 0.0)
            self.assertTrue(item["channels"])

    def test_sequence_inserts_required_transitions(self) -> None:
        resolved = aroll_actions.resolve_action_sequence(
            ["Aroll_Welcome_OpenArms", "Aroll_Seated_Explain", "Aroll_Conclusion_HandsTogether"],
            initial_state="standing",
        )
        self.assertEqual(
            resolved,
            [
                "Aroll_Welcome_OpenArms",
                "Aroll_Transition_StandToSit",
                "Aroll_Seated_Explain",
                "Aroll_Transition_SitToStand",
                "Aroll_Conclusion_HandsTogether",
            ],
        )

    def test_invalid_action_and_state_fail_closed(self) -> None:
        with self.assertRaisesRegex(ValueError, "unknown A-roll action"):
            aroll_actions.resolve_action_sequence(["Aroll_NotReal"], "standing")
        with self.assertRaisesRegex(ValueError, "initial state"):
            aroll_actions.resolve_action_sequence(["Aroll_Standing_Idle"], "crouching")

    def test_transition_events_are_contiguous_and_end_in_expected_state(self) -> None:
        events = aroll_actions.build_action_events(
            ["Aroll_Transition_StandToSit", "Aroll_Seated_Explain", "Aroll_Transition_SitToStand"],
            initial_state="standing",
            start_time_sec=1.0,
            spacing_sec=0.12,
        )
        self.assertEqual(events[0]["startState"], "standing")
        self.assertEqual(events[0]["endState"], "seated")
        self.assertEqual(events[-1]["endState"], "standing")
        self.assertEqual([event["motion"] for event in events], ["avatar_action"] * 3)
        self.assertTrue(all(events[i]["timeSec"] < events[i + 1]["timeSec"] for i in range(2)))


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the tests to verify RED**

Run:

```bash
python3 mcp/ip_avatar_3d/test_aroll_actions.py -v
```

Expected: fail because `action_catalog_payload`, `resolve_action_sequence`, and `build_action_events` do not exist.

- [ ] **Step 3: Add immutable metadata and state resolution**

Add to `aroll_actions.py`:

```python
from dataclasses import asdict, dataclass
from types import MappingProxyType
from typing import Any, Literal, Mapping, Sequence

PoseState = Literal["standing", "seated", "either"]


@dataclass(frozen=True)
class ActionMetadata:
    name: str
    start_state: PoseState
    end_state: PoseState
    duration_sec: float
    loopable: bool
    layerable: bool
    channels: tuple[str, ...]
    intents: tuple[str, ...]

    def payload(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "startState": self.start_state,
            "endState": self.end_state,
            "durationSec": self.duration_sec,
            "loopable": self.loopable,
            "layerable": self.layerable,
            "channels": list(self.channels),
            "intents": list(self.intents),
        }


def _meta(
    name: str,
    start: PoseState,
    end: PoseState,
    duration: float,
    channels: tuple[str, ...],
    intents: tuple[str, ...],
    *,
    loopable: bool = False,
    layerable: bool = True,
) -> ActionMetadata:
    return ActionMetadata(name, start, end, duration, loopable, layerable, channels, intents)


ACTION_CATALOG: Mapping[str, ActionMetadata] = MappingProxyType({
    item.name: item
    for item in (
        _meta("Aroll_Standing_Idle", "standing", "standing", 2.0, ("body", "head"), ("idle",), loopable=True),
        _meta("Aroll_Seated_Idle", "seated", "seated", 2.0, ("body", "head", "legs"), ("idle",), loopable=True),
        _meta("Aroll_Transition_StandToSit", "standing", "seated", 1.9, ("root", "body", "legs", "arms", "hands"), ("sit",), layerable=False),
        _meta("Aroll_Transition_SitToStand", "seated", "standing", 1.8, ("root", "body", "legs", "arms", "hands"), ("stand",), layerable=False),
        _meta("Aroll_Welcome_OpenArms", "standing", "standing", 1.8, ("body", "arms", "hands"), ("welcome", "opening")),
        _meta("Aroll_Question_PalmUp", "either", "either", 1.6, ("head", "arm_r", "hand_r"), ("question",)),
        _meta("Aroll_Compare_TwoSides", "either", "either", 2.0, ("head", "arms", "hands"), ("compare",)),
        _meta("Aroll_KeyPoint_OneFinger", "either", "either", 1.6, ("head", "arm_r", "hand_r"), ("key_point", "first")),
        _meta("Aroll_List_Three", "either", "either", 2.0, ("arm_r", "hand_r"), ("list", "three")),
        _meta("Aroll_Caution_Stop", "either", "either", 1.5, ("body", "arm_r", "hand_r"), ("warning", "stop")),
        _meta("Aroll_Quote_Frame", "either", "either", 1.8, ("arms", "hands"), ("quote", "concept")),
        _meta("Aroll_Conclusion_HandsTogether", "standing", "standing", 1.8, ("body", "arms", "hands"), ("conclusion", "closing")),
        _meta("Aroll_Seated_Explain", "seated", "seated", 1.8, ("body", "arms", "hands"), ("explain",)),
        _meta("Aroll_Seated_OpenPalm", "seated", "seated", 1.6, ("arm_r", "hand_r"), ("present",)),
        _meta("Aroll_Seated_LeanIn", "seated", "seated", 1.5, ("body", "head", "arms"), ("emphasis", "confide")),
    )
})


def action_catalog_payload() -> dict[str, Any]:
    return {
        "schemaVersion": "tangying-ip-aroll-action-catalog/v1",
        "actions": [ACTION_CATALOG[name].payload() for name in ACTION_CATALOG],
    }


def _required_transition(current: str, required: str) -> str:
    if current == "standing" and required == "seated":
        return "Aroll_Transition_StandToSit"
    if current == "seated" and required == "standing":
        return "Aroll_Transition_SitToStand"
    raise ValueError(f"cannot transition from {current!r} to {required!r}")


def resolve_action_sequence(action_names: Sequence[str], initial_state: str) -> list[str]:
    state = str(initial_state).strip().lower()
    if state not in {"standing", "seated"}:
        raise ValueError("initial state must be standing or seated")
    resolved: list[str] = []
    for name in action_names:
        metadata = ACTION_CATALOG.get(str(name))
        if metadata is None:
            raise ValueError(f"unknown A-roll action: {name}")
        if metadata.start_state != "either" and metadata.start_state != state:
            transition = _required_transition(state, metadata.start_state)
            resolved.append(transition)
            state = ACTION_CATALOG[transition].end_state
        resolved.append(metadata.name)
        if metadata.end_state != "either":
            state = metadata.end_state
    return resolved


def build_action_events(
    action_names: Sequence[str],
    initial_state: str,
    *,
    start_time_sec: float = 0.0,
    spacing_sec: float = 0.12,
) -> list[dict[str, Any]]:
    state = str(initial_state).strip().lower()
    cursor = max(0.0, float(start_time_sec))
    events: list[dict[str, Any]] = []
    for name in resolve_action_sequence(action_names, state):
        metadata = ACTION_CATALOG[name]
        start_state = state if metadata.start_state == "either" else metadata.start_state
        end_state = start_state if metadata.end_state == "either" else metadata.end_state
        events.append({
            "timeSec": round(cursor, 3),
            "motion": "avatar_action",
            "action": name,
            "duration": metadata.duration_sec,
            "strength": 1.0,
            "startState": start_state,
            "endState": end_state,
            "gestureGroups": list(metadata.channels),
        })
        cursor += metadata.duration_sec + max(0.0, float(spacing_sec))
        state = end_state
    return events
```

The immutable catalog must include every existing name in the current
`AROLL_ACTIONS` tuple as well as the new names. Use `either -> either` for
upper-body-only actions, `seated -> seated` for `Aroll_Seated_Idle`, and
`either -> either` for `Aroll_Transition_Reset`. Preserve the existing
durations and semantic intent names. After construction, define
`AROLL_ACTIONS = tuple(ACTION_CATALOG)` so no callable action can exist without
state metadata. Update `build_aroll_action_specs()` so every catalog action has
a concrete five-phase pose spec and both transition actions end in their target
stable pose rather than `{}`.

- [ ] **Step 4: Add explicit mirrored seated and transition key poses**

Replace the asymmetric source-rig seated values with mirrored semantic values and add transition specs:

```python
def presentation_pose(mode: str, source_rig: bool) -> dict[str, dict[str, tuple[float, float, float]]]:
    selected = str(mode or "standing").strip().lower()
    if selected == "standing":
        return {}
    if selected != "seated":
        raise ValueError(f"unsupported presentation mode: {mode}")
    if source_rig:
        leg_l = (0.02, -0.08, 1.16)
        leg_r = (0.02, 0.08, -1.16)
        shin_l = (-0.02, 0.04, -1.38)
        shin_r = (-0.02, -0.04, 1.38)
        foot_l = (0.0, 0.06, -0.18)
        foot_r = (0.0, -0.06, 0.18)
    else:
        leg_l = leg_r = (0.98, 0.0, 0.0)
        shin_l = shin_r = (-1.12, 0.0, 0.0)
        foot_l = foot_r = (0.16, 0.0, 0.0)
    return {
        "root": {"location": (0.0, 0.10, -0.286)},
        "body": {"rotation": (0.065, 0.0, 0.0)},
        "leg_l": {"rotation": leg_l},
        "shin_l": {"rotation": shin_l},
        "foot_l": {"rotation": foot_l},
        "leg_r": {"rotation": leg_r},
        "shin_r": {"rotation": shin_r},
        "foot_r": {"rotation": foot_r},
    }


def build_transition_specs(source_rig: bool, fps: int) -> dict[str, ActionSpec]:
    seated = presentation_pose("seated", source_rig)
    stand_to_sit = (
        1,
        max(2, round(fps * 0.30)),
        max(3, round(fps * 0.75)),
        max(4, round(fps * 1.32)),
        max(5, round(fps * 1.68)),
        max(6, round(fps * 1.90)),
    )
    sit_to_stand = (
        1,
        max(2, round(fps * 0.28)),
        max(3, round(fps * 0.70)),
        max(4, round(fps * 1.20)),
        max(5, round(fps * 1.55)),
        max(6, round(fps * 1.80)),
    )
    prep = _pose({"body": {"rotation": (0.10, 0.0, 0.0)}}, left_hand="relaxed_hand", right_hand="relaxed_hand")
    load = _pose({"root": {"location": (0.0, 0.04, -0.05)}, "body": {"rotation": (0.14, 0.0, 0.0)}}, left_hand="relaxed_hand", right_hand="relaxed_hand")
    return {
        "Aroll_Transition_StandToSit": [
            (stand_to_sit[0], {}),
            (stand_to_sit[1], prep),
            (stand_to_sit[2], load),
            (stand_to_sit[3], blend_action_pose({}, seated, 0.72)),
            (stand_to_sit[4], blend_action_pose({}, seated, 1.0)),
            (stand_to_sit[5], seated),
        ],
        "Aroll_Transition_SitToStand": [
            (sit_to_stand[0], seated),
            (sit_to_stand[1], blend_action_pose(seated, load, 0.34)),
            (sit_to_stand[2], blend_action_pose(seated, load, 0.72)),
            (sit_to_stand[3], blend_action_pose(seated, {}, 0.68)),
            (sit_to_stand[4], prep),
            (sit_to_stand[5], {}),
        ],
    }
```

Implement public `blend_action_pose()` as component-wise interpolation over
`location` and `rotation`; missing target values resolve to zero. Preserve
`__digit_pose_l` and `__digit_pose_r` from the nearest phase. This is the same
helper consumed by the Blender runtime in Task 4.

- [ ] **Step 5: Run the pure action tests**

Run:

```bash
python3 mcp/ip_avatar_3d/test_aroll_actions.py -v
python3 -m unittest mcp.ip_avatar_3d.test_server -v
```

Expected: the new action tests pass and existing motion-plan tests remain green.

- [ ] **Step 6: Commit**

```bash
git add mcp/ip_avatar_3d/aroll_actions.py mcp/ip_avatar_3d/test_aroll_actions.py
git commit -m "feat: define stateful sloth a-roll actions"
```

---

### Task 2: Expose The Action Catalog And Stateful Planner Through MCP

**Files:**
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `local-backend/internal/localmcp/client_test.go`
- Modify: `mcp/ip_avatar_3d/README.md`
- Modify: `docs/mcp-providers.md`

**Interfaces:**
- Consumes: `aroll_actions.action_catalog_payload()` and `aroll_actions.build_action_events()`.
- Produces: MCP tool `list_aroll_actions()`, optional `actionSequence` on `plan_motion()` and `render_talking_video()`, and persisted `initialPoseState` plus `resolvedActionSequence` metadata.

- [ ] **Step 1: Write failing MCP tests**

Add to `test_server.py`:

```python
def test_list_aroll_actions_exposes_stateful_catalog(self) -> None:
    server = load_server()
    payload = server.list_aroll_actions()
    names = {item["name"] for item in payload["actions"]}
    self.assertIn("Aroll_Transition_StandToSit", names)
    self.assertIn("Aroll_Transition_SitToStand", names)
    self.assertEqual(
        next(item for item in payload["actions"] if item["name"] == "Aroll_Seated_Explain")["startState"],
        "seated",
    )

def test_motion_plan_inserts_transitions_for_requested_actions(self) -> None:
    server = load_server()
    plan = server.build_motion_plan(
        "先打招呼，然后坐下来解释，最后站起来总结。",
        14.0,
        30,
        "expressive",
        presentation_mode="standing",
        action_sequence=[
            "Aroll_Welcome_OpenArms",
            "Aroll_Seated_Explain",
            "Aroll_Conclusion_HandsTogether",
        ],
    )
    self.assertEqual(
        plan["resolvedActionSequence"],
        [
            "Aroll_Welcome_OpenArms",
            "Aroll_Transition_StandToSit",
            "Aroll_Seated_Explain",
            "Aroll_Transition_SitToStand",
            "Aroll_Conclusion_HandsTogether",
        ],
    )
    self.assertEqual(plan["initialPoseState"], "standing")
    transitions = [event for event in plan["motionEvents"] if event.get("action", "").startswith("Aroll_Transition_")]
    self.assertEqual([event["endState"] for event in transitions], ["seated", "standing"])

def test_render_dry_run_persists_requested_action_sequence(self) -> None:
    server = load_server()
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        model = root / "avatar.glb"
        model.write_bytes(b"glTF placeholder")
        result = server.render_talking_video(
            script="坐下来解释，然后站起来总结。",
            modelPath=str(model),
            outputDir=str(root / "out"),
            presentationMode="standing",
            actionSequence=["Aroll_Seated_Explain", "Aroll_Conclusion_HandsTogether"],
            dryRun=True,
        )
        render_input = json.loads(pathlib.Path(result["renderInputPath"]).read_text())
        self.assertEqual(render_input["motionPlan"]["initialPoseState"], "standing")
        self.assertEqual(
            render_input["motionPlan"]["resolvedActionSequence"],
            [
                "Aroll_Transition_StandToSit",
                "Aroll_Seated_Explain",
                "Aroll_Transition_SitToStand",
                "Aroll_Conclusion_HandsTogether",
            ],
        )
```

- [ ] **Step 2: Run RED**

Run:

```bash
python3 -m unittest \
  mcp.ip_avatar_3d.test_server.IPAvatar3DMCPTests.test_list_aroll_actions_exposes_stateful_catalog \
  mcp.ip_avatar_3d.test_server.IPAvatar3DMCPTests.test_motion_plan_inserts_transitions_for_requested_actions \
  mcp.ip_avatar_3d.test_server.IPAvatar3DMCPTests.test_render_dry_run_persists_requested_action_sequence -v
```

Expected: missing tool and arguments.

- [ ] **Step 3: Add the MCP catalog and planner inputs**

Add `import aroll_actions`, then expose:

```python
@mcp.tool()
def list_aroll_actions() -> dict[str, Any]:
    """Return reusable standing, seated, and transition actions with pose-state metadata."""
    return aroll_actions.action_catalog_payload()
```

Change the pure planner signature and merge stateful events after deterministic baseline events:

```python
def build_motion_plan(
    script: str,
    duration_sec: float,
    fps: int = DEFAULT_FPS,
    motion_style: str = "expressive",
    *,
    presentation_mode: str = "standing",
    action_sequence: list[str] | None = None,
) -> dict[str, Any]:
    initial_state = resolve_presentation_mode(presentation_mode)
    requested = [str(name) for name in (action_sequence or []) if str(name).strip()]
    action_events = aroll_actions.build_action_events(
        requested,
        initial_state,
        start_time_sec=0.35,
        spacing_sec=0.12,
    )
    resolved_names = [str(event["action"]) for event in action_events]
    required_duration = max(
        (float(event["timeSec"]) + float(event["duration"]) + 0.35 for event in action_events),
        default=0.0,
    )
    if required_duration > duration_sec + 1e-6:
        raise ValueError(
            f"actionSequence requires {required_duration:.2f}s but durationSec is {duration_sec:.2f}s"
        )
    events.extend(action_events)
    events = _finalize_motion_events(events, duration_sec)
    return {
        "schemaVersion": "ip-avatar-3d-motion-plan/v2",
        "durationSec": round(duration_sec, 3),
        "fps": fps,
        "motionStyle": motion_style,
        "initialPoseState": initial_state,
        "resolvedActionSequence": resolved_names,
        "motionEvents": events,
        "lipSync": lip_sync,
        "notes": ["Stateful A-roll actions are deterministic and locally rendered."],
    }
```

Update `plan_motion()` and `render_talking_video()` with:

```python
actionSequence: list[str] | None = None
```

Pass `presentation_mode=presentation_mode` and `action_sequence=actionSequence` into every `build_motion_plan()` call. Persist the resolved sequence in render input, dry-run report, final report, and returned metadata.

- [ ] **Step 4: Prevent state transitions from being shifted by hand-only overlap logic**

Extend `_finalize_motion_events()` so `avatar_action` events retain their authored time and reserve all declared groups:

```python
if item.get("motion") == "avatar_action":
    groups = [
        group
        for group in item.get("gestureGroups", [])
        if group in {"root", "body", "legs", "arms", "hands", "head", "arm_l", "arm_r", "hand_l", "hand_r"}
    ]
    start = max(0.0, float(item.get("timeSec") or 0.0))
    duration = max(0.1, float(item.get("duration") or 0.1))
    if start + duration > duration_sec:
        raise ValueError(f"A-roll action {item.get('action')} exceeds durationSec")
    item["timeSec"] = round(start, 3)
    item["duration"] = round(duration, 3)
    resolved.append(item)
    continue
```

Keep legacy body/hand/head event coalescing unchanged.

Also extend the accepted camera roles used by the new demo jobs:

```python
CAMERA_PRESETS = {"auto", "wide", "medium", "close", "three_quarter", "transition"}
```

Map `three_quarter` to `Camera_ThreeQuarter` and `transition` to
`Camera_Transition` in `build_camera_plan()`. In
`blender_renderer.configure_camera_plan()`, add both aliases and resolve them
against mode camera roles:

```python
aliases.update({
    "three_quarter": "three_quarter",
    "transition": "transition",
    "Camera_ThreeQuarter": "three_quarter",
    "Camera_Transition": "transition",
})
```

Add a dry-run server test proving `cameraPreset="transition"` is accepted and
persists a `Camera_Transition` camera-plan entry. The packed studio objects that
fulfil this alias are added in Task 3.

- [ ] **Step 5: Cover MCP bridge discovery**

Add a Go test that discovers `ip_avatar_3d.list_aroll_actions`, calls it through `CallTool`, and asserts the JSON result contains `Aroll_Transition_StandToSit`. The transport remains generic `LOCAL_MCP_TOOL_CALL`; do not add a new local command.

Run:

```bash
go test ./local-backend/internal/localmcp -run 'Test.*Aroll.*' -v
```

Expected: pass.

- [ ] **Step 6: Document the callable contract**

Add `list_aroll_actions` to both tool tables. Add this production example to `mcp/ip_avatar_3d/README.md`:

```json
{
  "script": "先站着开场，然后坐下解释，最后站起来总结。",
  "characterProfilePath": "/absolute/path/to/ip形象/main_ip/character-profile.json",
  "presentationMode": "standing",
  "actionSequence": [
    "Aroll_Welcome_OpenArms",
    "Aroll_Seated_Explain",
    "Aroll_Conclusion_HandsTogether"
  ]
}
```

- [ ] **Step 7: Run tests and commit**

```bash
python3 -m unittest mcp.ip_avatar_3d.test_server -v
go test ./local-backend/internal/localmcp -v
git add mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py \
  mcp/ip_avatar_3d/blender_renderer.py \
  local-backend/internal/localmcp/client_test.go \
  mcp/ip_avatar_3d/README.md docs/mcp-providers.md
git commit -m "feat: expose stateful a-roll actions through mcp"
```

---

### Task 3: Re-author The Seated Studio Contract And Visible Stool

**Files:**
- Modify: `mcp/ip_avatar_3d/warm_studio_contract.py`
- Modify: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`
- Modify: `mcp/ip_avatar_3d/test_warm_studio_contract.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`
- Regenerate tracked: `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`
- Regenerate tracked: `ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png`

**Interfaces:**
- Consumes: the authored desk, `Chair_Main`, existing mode markers, and mode camera builder.
- Produces: explicit seated knee markers, a transition focus marker/camera, and a partially visible hero stool/chair silhouette.

- [ ] **Step 1: Write failing studio contract tests**

```python
def test_seated_contract_has_knees_and_transition_camera(self) -> None:
    seated = contract.MODE_MARKER_SPECS["seated"]
    self.assertEqual(set(seated), {"spawn", "focus", "seat", "knee_l", "knee_r", "foot_l", "foot_r"})
    self.assertIn("transition", contract.MODE_CAMERA_SPECS["standing"])
    self.assertIn("transition", contract.MODE_CAMERA_SPECS["seated"])
    self.assertEqual(
        contract.MODE_CAMERA_SPECS["standing"]["transition"][0],
        "Camera_Standing_Transition",
    )

def test_stool_visibility_contract_is_not_full_occlusion(self) -> None:
    self.assertEqual(contract.SEAT_VISIBILITY_PROFILE["strategy"], "partial_profile")
    self.assertGreaterEqual(contract.SEAT_VISIBILITY_PROFILE["minimumVisibleFraction"], 0.08)
    self.assertLessEqual(contract.SEAT_VISIBILITY_PROFILE["maximumVisibleFraction"], 0.28)
```

Add Blender assertions that these objects exist and carry the expected custom properties:

```python
for name in (
    "IP_Seated_Knee_Target.L",
    "IP_Seated_Knee_Target.R",
    "IP_Transition_Focus",
    "Camera_Standing_Transition",
    "Camera_Seated_Transition",
):
    assert bpy.data.objects.get(name) is not None, name
assert bpy.data.objects["Chair_Main"]["hero_visibility_strategy"] == "partial_profile"
```

- [ ] **Step 2: Run RED**

```bash
python3 mcp/ip_avatar_3d/test_warm_studio_contract.py -v
```

Expected: missing knee markers, transition cameras, and seat profile.

- [ ] **Step 3: Extend the immutable studio contract**

Use these calibrated marker and camera targets:

```python
MODE_MARKER_SPECS = MappingProxyType({
    "standing": MappingProxyType({
        "spawn": (0.0, 0.30, 0.0),
        "focus": (0.0, 0.30, 1.93),
        "seat": (0.0, 0.53, 0.56),
        "knee_l": (-0.20, 0.04, 0.54),
        "knee_r": (0.20, 0.04, 0.54),
        "foot_l": (-0.22, 0.00, 0.0),
        "foot_r": (0.22, 0.00, 0.0),
    }),
    "seated": MappingProxyType({
        "spawn": (0.0, 0.43, 0.0),
        "focus": (0.0, 0.43, 1.58),
        "seat": (0.0, 0.53, 0.56),
        "knee_l": (-0.20, -0.01, 0.54),
        "knee_r": (0.20, -0.01, 0.54),
        "foot_l": (-0.22, -0.03, 0.0),
        "foot_r": (0.22, -0.03, 0.0),
    }),
})

SEAT_VISIBILITY_PROFILE = MappingProxyType({
    "strategy": "partial_profile",
    "minimumVisibleFraction": 0.08,
    "maximumVisibleFraction": 0.28,
})
```

Add `transition` to both mode camera specs:

```python
"transition": ("Camera_Standing_Transition", (-1.58, -2.42, 1.48), 42.0)
```

and:

```python
"transition": ("Camera_Seated_Transition", (-1.58, -2.42, 1.48), 42.0)
```

Both transition cameras focus on `IP_Transition_Focus` at `(0.0, 0.38, 1.66)`.

- [ ] **Step 4: Build visible stool geometry and markers**

Rename `build_hidden_hero_chair()` to `build_hero_stool_chair()`, keep `Chair_Main` for compatibility, and set:

```python
root["hero_visibility_strategy"] = contract.SEAT_VISIBILITY_PROFILE["strategy"]
root["minimum_visible_fraction"] = contract.SEAT_VISIBILITY_PROFILE["minimumVisibleFraction"]
root["maximum_visible_fraction"] = contract.SEAT_VISIBILITY_PROFILE["maximumVisibleFraction"]
```

Move the seat center to `(0.0, 0.53, 0.50)`, reduce the back height to `0.18`, and retain the warm fabric/oak materials. Add both knee markers and `IP_Transition_Focus` to `MARKER_SPECS`. Update `_add_mode_camera()` to accept an explicit focus name so transition cameras use `IP_Transition_Focus` and other cameras use the current mode head marker.

- [ ] **Step 5: Rebuild and validate the packed studio**

```bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/warm_sloth_studio_builder.py -- \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend" \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png" \
  "$PWD/ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png"
"$BLENDER_BIN" --background \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend" \
  --python mcp/ip_avatar_3d/test_blender_scene_contract.py
```

Expected: all markers/cameras exist, the stool is render-enabled, and no camera intersects furniture.

- [ ] **Step 6: Commit**

```bash
git add mcp/ip_avatar_3d/warm_studio_contract.py \
  mcp/ip_avatar_3d/warm_sloth_studio_builder.py \
  mcp/ip_avatar_3d/test_warm_studio_contract.py \
  mcp/ip_avatar_3d/test_blender_scene_contract.py \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png
git commit -m "fix: reauthor seated studio framing and stool"
```

---

### Task 4: Animate Continuous Stand And Sit Transitions In Blender

**Files:**
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/test_blender_character_rig.py`

**Interfaces:**
- Consumes: stateful `motionEvents`, transition specs, `IP_Seat_Target`, knee targets, and foot targets.
- Produces: `build_pose_state_timeline()`, `sample_aroll_action_pose()`, state-aware `animate()`, foot-lock evidence, seat-contact evidence, and continuous action keyframes.

- [ ] **Step 1: Write failing Blender transition tests**

Add a test that loads the enhanced FBX character, creates source-mouth visemes, and animates this plan:

```python
plan = {
    "durationSec": 7.0,
    "initialPoseState": "standing",
    "resolvedActionSequence": [
        "Aroll_Transition_StandToSit",
        "Aroll_Seated_Explain",
        "Aroll_Transition_SitToStand",
    ],
    "motionEvents": [
        {"timeSec": 0.6, "motion": "avatar_action", "action": "Aroll_Transition_StandToSit", "duration": 1.9, "strength": 1.0, "startState": "standing", "endState": "seated"},
        {"timeSec": 2.7, "motion": "avatar_action", "action": "Aroll_Seated_Explain", "duration": 1.8, "strength": 0.8, "startState": "seated", "endState": "seated"},
        {"timeSec": 4.7, "motion": "avatar_action", "action": "Aroll_Transition_SitToStand", "duration": 1.8, "strength": 1.0, "startState": "seated", "endState": "standing"},
    ],
    "lipSync": [{"timeSec": 0.0, "viseme": "a", "open": 0.85}],
}
```

Sample frames `(1, 18, 36, 57, 75, 105, 141, 168, 198, 210)` and assert:

```python
assert max(foot_drift_l) < 0.025
assert max(foot_drift_r) < 0.025
assert min(knee_separation) > dimensions["width"] * 0.055
assert min(seat_clearance_during_contact) > -0.018
assert max(seat_clearance_after_contact) < 0.035
assert max(root_frame_delta) < dimensions["height"] * 0.055
assert final_state_metrics["pelvisHeight"] > seated_metrics["pelvisHeight"] + dimensions["height"] * 0.10
```

Also assert no lower-body mesh sample has a central forward spike more than `0.045 m` beyond both neighboring silhouette columns.

- [ ] **Step 2: Run the Blender test to verify RED**

```bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: the current renderer ignores `avatar_action` and the transition assertions fail.

- [ ] **Step 3: Add action-pose interpolation**

Add these pure helpers to `blender_renderer.py`:

```python
def build_pose_state_timeline(plan: dict[str, Any]) -> list[dict[str, Any]]:
    state = str(plan.get("initialPoseState") or "standing")
    timeline = [{"timeSec": 0.0, "state": state}]
    for event in sorted(plan.get("motionEvents") or [], key=lambda item: float(item.get("timeSec") or 0.0)):
        if event.get("motion") != "avatar_action":
            continue
        if str(event.get("startState") or state) != state:
            raise RuntimeError(
                f"A-roll action {event.get('action')} requires {event.get('startState')} but timeline is {state}"
            )
        state = str(event.get("endState") or state)
        timeline.append({
            "timeSec": float(event.get("timeSec") or 0.0) + float(event.get("duration") or 0.0),
            "state": state,
        })
    return timeline


def sample_aroll_action_pose(
    action_name: str,
    elapsed_sec: float,
    duration_sec: float,
    *,
    source_rig: bool,
    fps: int,
) -> dict[str, Any]:
    specs = aroll_actions.build_aroll_action_specs(source_rig, fps)
    spec = specs[action_name]
    source_duration = max(frame for frame, _ in spec) / float(fps)
    source_time = max(0.0, min(source_duration, elapsed_sec / max(duration_sec, 1e-6) * source_duration))
    source_frame = source_time * fps
    before = max((item for item in spec if item[0] <= source_frame), default=spec[0], key=lambda item: item[0])
    after = min((item for item in spec if item[0] >= source_frame), default=spec[-1], key=lambda item: item[0])
    if before[0] == after[0]:
        return dict(before[1])
    amount = (source_frame - before[0]) / (after[0] - before[0])
    return aroll_actions.blend_action_pose(before[1], after[1], amount)
```

- [ ] **Step 4: Make `animate()` state-aware**

At each sampled frame:

1. Resolve the stable pose state from `build_pose_state_timeline()`.
2. Apply `presentation_pose(state, source_rig)` as the lower-body base.
3. For each active `avatar_action`, sample its pose and apply numeric location/rotation channels.
4. Apply requested semantic hand poses through `apply_hand_pose()`.
5. Suppress legacy `happy_bounce`, `leg_step`, and `weight_shift` while either transition is active.

Use this event selector:

```python
def active_avatar_actions(events: list[dict[str, Any]], t: float) -> list[tuple[dict[str, Any], float]]:
    active: list[tuple[dict[str, Any], float]] = []
    for event in events:
        if event.get("motion") != "avatar_action":
            continue
        start = float(event.get("timeSec") or 0.0)
        duration = max(0.001, float(event.get("duration") or 0.001))
        if start <= t <= start + duration:
            active.append((event, t - start))
    return active
```

Convert source-rig root world offsets with the existing `source_root_location_from_world()` helper. Use `set_action_interpolation()` so generated curves remain `BEZIER` with `AUTO_CLAMPED` handles.

- [ ] **Step 5: Add foot lock and seat-contact correction**

During transition events, compute world-space ankle and seat deltas after the semantic pose is applied. Correct only root X/Y drift and the final pelvis Z contact:

```python
def apply_transition_contact_correction(
    armature: bpy.types.Object,
    pose,
    bone_map: dict[str, str],
    mode_objects: dict[str, Any] | None,
    phase: float,
    target_state: str,
) -> dict[str, float]:
    if not mode_objects:
        return {"footDriftL": 0.0, "footDriftR": 0.0, "seatClearance": 0.0}
    root = pose[bone_map["root"]]
    foot_deltas = []
    for role, target_key in (("foot_l", "foot_l"), ("foot_r", "foot_r")):
        foot = pose[bone_map[role]]
        foot_world = armature.matrix_world @ foot.matrix.translation
        target = mode_objects[target_key].matrix_world.translation
        foot_deltas.append(target - foot_world)
    correction = (foot_deltas[0] + foot_deltas[1]) * 0.5
    lock_weight = math.sin(max(0.0, min(1.0, phase)) * math.pi) ** 2
    root.location.x += correction.x * lock_weight
    root.location.y += correction.y * lock_weight
    seat = mode_objects["seat"].matrix_world.translation
    pelvis_world = armature.matrix_world @ pose[bone_map.get("cog", bone_map["root"])].matrix.translation
    seat_clearance = pelvis_world.z - seat.z
    if target_state == "seated" and phase > 0.70:
        root.location.z -= max(-0.018, min(0.035, seat_clearance - 0.012)) * ((phase - 0.70) / 0.30)
    return {
        "footDriftL": foot_deltas[0].length,
        "footDriftR": foot_deltas[1].length,
        "seatClearance": seat_clearance,
    }
```

Pass `mode_objects` into `animate()` from `main()`. Aggregate maximum drift and seat-contact values into the rig report.

- [ ] **Step 6: Run Blender transition and regression tests**

```bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
python3 -m unittest mcp.ip_avatar_3d.test_server -v
```

Expected: transition metrics pass, existing wave/finger/face/action tests stay green.

- [ ] **Step 7: Commit**

```bash
git add mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "feat: animate continuous sloth stand and sit transitions"
```

---

### Task 5: Build The Rich Standing And Seated Gesture Library

**Files:**
- Modify: `mcp/ip_avatar_3d/aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/test_aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/test_blender_character_rig.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`

**Interfaces:**
- Consumes: stateful metadata, semantic arm roles, wrist controls, and independent hand poses.
- Produces: concrete action specs for all catalog actions and keyword-to-action planning for normal narration.

- [ ] **Step 1: Write failing action-detail tests**

In `test_aroll_actions.py`, assert every catalog action has at least five phases, its final transition pose matches the required state, and every hand-oriented action declares a semantic digit pose at its readable hold.

```python
def test_catalog_actions_have_complete_pose_specs(self) -> None:
    specs = aroll_actions.build_aroll_action_specs(source_rig=True, fps=30)
    for name, metadata in aroll_actions.ACTION_CATALOG.items():
        self.assertIn(name, specs)
        self.assertGreaterEqual(len(specs[name]), 5)
        frames = [frame for frame, _ in specs[name]]
        self.assertEqual(frames, sorted(frames))
        self.assertEqual(len(frames), len(set(frames)))
    for name in (
        "Aroll_Welcome_OpenArms",
        "Aroll_Question_PalmUp",
        "Aroll_Compare_TwoSides",
        "Aroll_KeyPoint_OneFinger",
        "Aroll_List_Three",
        "Aroll_Caution_Stop",
        "Aroll_Quote_Frame",
        "Aroll_Conclusion_HandsTogether",
        "Aroll_Seated_Explain",
        "Aroll_Seated_OpenPalm",
    ):
        hold = specs[name][2][1]
        self.assertTrue("__digit_pose_l" in hold or "__digit_pose_r" in hold)
```

In Blender, load each action at its hold frame and assert:

- wrist rotation is greater than `0.10 rad` for palm-facing actions;
- open/point/count/stop digit poses differ by at least `0.08 rad` in one segment;
- left/right comparison poses are mirrored within `0.14 rad`;
- seated actions preserve the seated pelvis height within `0.035 m`;
- sampled hand/torso and hand/desk BVH distances remain positive.

- [ ] **Step 2: Run RED**

```bash
python3 mcp/ip_avatar_3d/test_aroll_actions.py -v
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: missing action specs and Blender actions.

- [ ] **Step 3: Add concrete semantic pose specs**

For the source rig, use these readable hold poses and mirrored counterparts:

```python
welcome = _pose(
    {"body": (0.0, 0.0, 0.018), "upper_arm_l": (-0.04, 0.02, 0.42), "forearm_l": (-0.30, 0.08, -0.10), "hand_l": (0.05, 0.30, -0.10),
     "upper_arm_r": (-0.04, -0.02, -0.42), "forearm_r": (-0.30, -0.08, 0.10), "hand_r": (0.05, -0.30, 0.10)},
    left_hand="open_hand", right_hand="open_hand",
)
question = _pose(
    {"head": (0.0, -0.05, -0.02), "upper_arm_r": (-0.06, -0.02, -0.70), "forearm_r": (-0.66, -0.06, 0.72), "hand_r": (0.12, -0.42, 0.14)},
    right_hand="open_hand",
)
compare_left = _pose(left_chest, left_hand="open_hand")
compare_right = _pose(right_chest, right_hand="open_hand")
key_point = _pose(right_chest, right_hand="count_one")
list_three = _pose(right_chest, right_hand="count_three")
caution = _pose(
    {"body": (0.0, 0.0, -0.012), "upper_arm_r": (-0.02, -0.02, -0.48), "forearm_r": (-0.28, -0.04, 0.70), "hand_r": (0.02, -1.02, 0.02)},
    right_hand="open_hand",
)
quote_frame = _pose(
    {"upper_arm_l": (-0.05, 0.02, 0.62), "forearm_l": (-0.64, 0.08, -0.50), "hand_l": (0.08, 0.18, -0.18),
     "upper_arm_r": (-0.05, -0.02, -0.62), "forearm_r": (-0.64, -0.08, 0.50), "hand_r": (0.08, -0.18, 0.18)},
    left_hand="count_two", right_hand="count_two",
)
hands_together = _pose(
    {"body": (0.0, 0.0, 0.015), "upper_arm_l": (-0.05, 0.02, 0.72), "forearm_l": (-0.76, 0.06, -0.70), "hand_l": (0.08, 0.34, -0.12),
     "upper_arm_r": (-0.05, -0.02, -0.72), "forearm_r": (-0.76, -0.06, 0.70), "hand_r": (0.08, -0.34, 0.12)},
    left_hand="relaxed_hand", right_hand="relaxed_hand",
)
```

Build each action with anticipation, hold-in, readable hold, release, and stable end. For seated actions, merge `presentation_pose("seated", source_rig)` into every phase and reduce arm amplitude to `0.82` of the standing equivalent. `Aroll_Seated_LeanIn` changes body pitch by at most `0.10 rad` and root forward offset by at most `0.035 m`.

- [ ] **Step 4: Bake all actions into the canonical Blend action library**

Keep the existing `build_action()` path and add every catalog spec returned by `build_aroll_action_specs()`:

```python
action_specs.update(aroll_actions.build_aroll_action_specs(source_rig, fps))
for name, keyframes in action_specs.items():
    created.append(build_action(name, keyframes))
```

Do not short-circuit when an action already exists. Clear and rebuild catalog actions so changed seated and transition values replace stale actions in the master:

```python
if existing and name in aroll_actions.ACTION_CATALOG:
    bpy.data.actions.remove(existing)
    existing = None
```

- [ ] **Step 5: Map common script intent to core actions**

Add deterministic keyword rules before generic legacy gestures:

```python
AROLL_INTENT_RULES = (
    (("大家好", "欢迎", "你好"), "Aroll_Welcome_OpenArms"),
    (("为什么", "问题", "想一想"), "Aroll_Question_PalmUp"),
    (("相比", "对比", "一方面", "另一方面"), "Aroll_Compare_TwoSides"),
    (("重点", "关键", "第一"), "Aroll_KeyPoint_OneFinger"),
    (("三点", "第三"), "Aroll_List_Three"),
    (("注意", "不要", "风险"), "Aroll_Caution_Stop"),
    (("所谓", "有人说", "引用"), "Aroll_Quote_Frame"),
    (("坐下来", "坐着", "深入聊", "详细解释"), "Aroll_Seated_Explain"),
    (("总结", "最后", "结论"), "Aroll_Conclusion_HandsTogether"),
)
```

When the script has no explicit `actionSequence`, choose no more than one catalog action per sentence and no more than one action every `2.2 s`. If a seated-only action is selected, rely on the state resolver to insert transitions.

- [ ] **Step 6: Run all action tests and commit**

```bash
python3 mcp/ip_avatar_3d/test_aroll_actions.py -v
python3 -m unittest mcp.ip_avatar_3d.test_server -v
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
git add mcp/ip_avatar_3d/aroll_actions.py mcp/ip_avatar_3d/blender_renderer.py \
  mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_aroll_actions.py \
  mcp/ip_avatar_3d/test_server.py mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "feat: add rich standing and seated a-roll gestures"
```

---

### Task 6: Strengthen Original-Mesh Visemes And Jaw Performance

**Files:**
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/test_blender_character_rig.py`

**Interfaces:**
- Consumes: source-mouth shape keys, jaw bone, deterministic Mandarin viseme timeline.
- Produces: `VISEME_RESPONSE`, stronger source-mesh shapes, two-to-three-frame smoothing, and mouth QA metrics.

- [ ] **Step 1: Write failing mouth range and timing tests**

Extend the existing source-mouth tests:

```python
gap_mbp = mouth_open_gap(face["mouth"], "Mouth_MBP")
gap_rest = mouth_open_gap(face["mouth"], "Mouth_Rest")
gap_a = mouth_open_gap(face["mouth"], "Mouth_A")
gap_o = mouth_open_gap(face["mouth"], "Mouth_O")
gap_u = mouth_open_gap(face["mouth"], "Mouth_U")
gap_surprise = mouth_open_gap(face["mouth"], "Mouth_Surprise")
assert gap_mbp <= gap_rest + dimensions["height"] * 0.0015
assert gap_a >= gap_mbp + dimensions["height"] * 0.0060
assert gap_o >= gap_mbp + dimensions["height"] * 0.0048
assert gap_u >= gap_mbp + dimensions["height"] * 0.0036
assert gap_surprise >= gap_a + dimensions["height"] * 0.0010
assert abs(mouth_width(face["mouth"], "Mouth_E") - mouth_width(face["mouth"], "Mouth_O")) >= dimensions["width"] * 0.012
```

Animate a sequence `MBP -> A -> O -> U -> Rest` and assert jaw maximum is between `0.20` and `0.25 rad`, MBP remains below `0.03 rad`, and no shape key changes from below `0.1` to above `0.9` in one sampled frame.

- [ ] **Step 2: Run RED**

```bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: current jaw maximum is approximately `0.14 rad` and open-shape gaps are too small.

- [ ] **Step 3: Strengthen source mesh deformation without replacement geometry**

In `create_source_mesh_visemes()`, use these vertical displacement multipliers for vertices already classified as upper or lower lip:

```python
SOURCE_VISEME_DISPLACEMENT = {
    "Mouth_A": {"upper": 0.0052, "lower": -0.0190, "width": 0.96},
    "Mouth_E": {"upper": 0.0028, "lower": -0.0095, "width": 1.10},
    "Mouth_O": {"upper": 0.0070, "lower": -0.0152, "width": 0.78},
    "Mouth_U": {"upper": 0.0048, "lower": -0.0110, "width": 0.68},
    "Mouth_MBP": {"upper": -0.0008, "lower": 0.0008, "width": 0.98},
    "Mouth_Surprise": {"upper": 0.0092, "lower": -0.0220, "width": 0.80},
}
```

Multiply upper/lower values by character height and preserve the current falloff mask so cheek and nose vertices remain stable. Clamp any vertex displacement to `0.028 * height` and retain the integrated oral interior.

- [ ] **Step 4: Use viseme-specific intensity and jaw response**

Add:

```python
VISEME_RESPONSE = {
    "Mouth_Rest": {"shapeFloor": 1.0, "shapeGain": 0.0, "jawGain": 0.00},
    "Mouth_MBP": {"shapeFloor": 1.0, "shapeGain": 0.0, "jawGain": 0.00},
    "Mouth_A": {"shapeFloor": 0.72, "shapeGain": 0.28, "jawGain": 0.24},
    "Mouth_E": {"shapeFloor": 0.68, "shapeGain": 0.25, "jawGain": 0.16},
    "Mouth_O": {"shapeFloor": 0.75, "shapeGain": 0.25, "jawGain": 0.22},
    "Mouth_U": {"shapeFloor": 0.72, "shapeGain": 0.24, "jawGain": 0.18},
}
```

In `animate()`:

```python
response = VISEME_RESPONSE[active_name]
shape_value = min(1.0, response["shapeFloor"] + mouth_open * response["shapeGain"])
active_key.value = shape_value
set_bone("jaw", rotation=(mouth_open * response["jawGain"], 0.0, 0.0))
```

- [ ] **Step 5: Smooth the deterministic lip timeline over 2-3 frames**

Replace the one-step 0.35/0.65 filter in `build_motion_plan()` with a bounded attack/release filter:

```python
target = open_value
previous = lip_sync[-1]["open"] if lip_sync else target
attack = 0.72 if target > previous else 0.58
smoothed = previous + (target - previous) * attack
if viseme == "mbp":
    smoothed = min(smoothed, 0.025)
elif viseme == "closed":
    smoothed = min(smoothed, 0.08)
lip_sync.append({"timeSec": round(t, 3), "viseme": viseme, "open": round(smoothed, 3)})
```

Keep samples at `max(1 / fps, 0.08)` and preserve exact viseme timestamps.

- [ ] **Step 6: Run mouth and full renderer tests, then commit**

```bash
python3 -m unittest mcp.ip_avatar_3d.test_server -v
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
git add mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/server.py \
  mcp/ip_avatar_3d/test_server.py mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "feat: strengthen source-mouth viseme performance"
```

---

### Task 7: Add Fail-Closed Transition, Silhouette, And Viseme QA

**Files:**
- Create: `mcp/ip_avatar_3d/aroll_performance_qa.py`
- Create: `mcp/ip_avatar_3d/test_aroll_performance_qa.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/render_warm_studio_demo.py`
- Modify: `mcp/ip_avatar_3d/test_warm_studio_demo.py`

**Interfaces:**
- Consumes: frame-sampled rig metrics, PNG frames, camera matrices, mouth gaps, and media probe data.
- Produces: `validate_transition_metrics()`, `detect_central_silhouette_spike()`, `validate_viseme_metrics()`, and report sections that block publication on failure.

- [ ] **Step 1: Write failing pure QA tests**

```python
import unittest

import arroll_performance_qa as qa


class ArollPerformanceQATests(unittest.TestCase):
    def test_valid_transition_metrics_pass(self) -> None:
        report = qa.validate_transition_metrics({
            "maxFootDriftL": 0.012,
            "maxFootDriftR": 0.014,
            "minKneeSeparation": 0.18,
            "minSeatClearance": -0.010,
            "maxSettledSeatClearance": 0.022,
            "maxRootFrameDelta": 0.052,
            "maxCentralSilhouetteSpike": 0.016,
            "seatVisibleFraction": 0.14,
        })
        self.assertTrue(report["success"])
        self.assertEqual(report["errors"], [])

    def test_pixel_line_geometry_and_hidden_stool_fail(self) -> None:
        report = qa.validate_transition_metrics({
            "maxFootDriftL": 0.010,
            "maxFootDriftR": 0.010,
            "minKneeSeparation": 0.015,
            "minSeatClearance": -0.010,
            "maxSettledSeatClearance": 0.020,
            "maxRootFrameDelta": 0.030,
            "maxCentralSilhouetteSpike": 0.081,
            "seatVisibleFraction": 0.0,
        })
        self.assertFalse(report["success"])
        self.assertIn("central silhouette spike", " ".join(report["errors"]))
        self.assertIn("seat visibility", " ".join(report["errors"]))

    def test_viseme_separation_gate(self) -> None:
        report = qa.validate_viseme_metrics({
            "mbpGap": 0.002,
            "restGap": 0.004,
            "aGap": 0.028,
            "eWidth": 0.110,
            "oGap": 0.022,
            "oWidth": 0.074,
            "uGap": 0.016,
            "surpriseGap": 0.034,
            "maxJawRadians": 0.232,
        }, character_height=2.55, character_width=1.42)
        self.assertTrue(report["success"])
```

- [ ] **Step 2: Run RED**

```bash
python3 mcp/ip_avatar_3d/test_aroll_performance_qa.py -v
```

Expected: module does not exist.

- [ ] **Step 3: Implement exact QA thresholds**

```python
TRANSITION_LIMITS = {
    "maxFootDrift": 0.025,
    "minKneeSeparation": 0.075,
    "minSeatClearance": -0.018,
    "maxSettledSeatClearance": 0.035,
    "maxRootFrameDelta": 0.075,
    "maxCentralSilhouetteSpike": 0.045,
    "minSeatVisibleFraction": 0.08,
    "maxSeatVisibleFraction": 0.28,
}


def validate_transition_metrics(metrics: dict[str, float]) -> dict[str, object]:
    errors: list[str] = []
    if max(float(metrics["maxFootDriftL"]), float(metrics["maxFootDriftR"])) > TRANSITION_LIMITS["maxFootDrift"]:
        errors.append("foot lock drift exceeds 0.025 m")
    if float(metrics["minKneeSeparation"]) < TRANSITION_LIMITS["minKneeSeparation"]:
        errors.append("knee separation is below 0.075 m")
    if float(metrics["minSeatClearance"]) < TRANSITION_LIMITS["minSeatClearance"]:
        errors.append("pelvis penetrates the seat")
    if float(metrics["maxSettledSeatClearance"]) > TRANSITION_LIMITS["maxSettledSeatClearance"]:
        errors.append("pelvis does not settle onto the seat")
    if float(metrics["maxRootFrameDelta"]) > TRANSITION_LIMITS["maxRootFrameDelta"]:
        errors.append("root motion has a visible frame discontinuity")
    if float(metrics["maxCentralSilhouetteSpike"]) > TRANSITION_LIMITS["maxCentralSilhouetteSpike"]:
        errors.append("central silhouette spike exceeds 0.045 m")
    visible = float(metrics["seatVisibleFraction"])
    if not TRANSITION_LIMITS["minSeatVisibleFraction"] <= visible <= TRANSITION_LIMITS["maxSeatVisibleFraction"]:
        errors.append("seat visibility is outside the approved range")
    return {"success": not errors, "errors": errors, "metrics": dict(metrics)}
```

Implement `validate_viseme_metrics()` using the exact gap, width, and jaw bounds from Task 6.

- [ ] **Step 4: Persist Blender frame-sampled evidence**

In the renderer, sample every two frames and store:

```python
rig_report["arollPerformanceQa"] = {
    "schemaVersion": "tangying-aroll-performance-qa/v1",
    "transition": transition_report,
    "visemes": viseme_report,
    "sampledFrames": sampled_frames,
    "stateTimeline": state_timeline,
}
```

Compute the central silhouette metric from lower-body vertices projected to the active camera. Bin projected X into 64 columns; compare the center four columns' closest-camera depth against the mean of columns immediately left and right. Compute `seatVisibleFraction` from the chair's render mask pixel count divided by the character-plus-chair foreground pixel count in transition camera QA frames.

- [ ] **Step 5: Block demo publication on failed A-roll performance QA**

In `_embedded_collision_report()`, require:

```python
performance = payload.get("arollPerformanceQa") or {}
transition = performance.get("transition") or {}
transition_status = str(transition.get("status") or "")
if transition_status not in {"passed", "not_applicable"}:
    raise DemoQAError(f"{mode} transition geometry QA did not pass")
if transition_status == "passed" and transition.get("success") is not True:
    raise DemoQAError(f"{mode} transition geometry QA did not pass")
if performance.get("visemes", {}).get("success") is not True:
    raise DemoQAError(f"{mode} viseme QA did not pass")
```

For standalone standing/seated renders without transitions, require viseme QA but mark transition QA as `not_applicable`; do not fabricate passing transition values.

- [ ] **Step 6: Run QA tests and commit**

```bash
python3 mcp/ip_avatar_3d/test_aroll_performance_qa.py -v
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py -v
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
git add mcp/ip_avatar_3d/aroll_performance_qa.py \
  mcp/ip_avatar_3d/test_aroll_performance_qa.py \
  mcp/ip_avatar_3d/blender_renderer.py \
  mcp/ip_avatar_3d/render_warm_studio_demo.py \
  mcp/ip_avatar_3d/test_warm_studio_demo.py
git commit -m "test: gate a-roll transitions and visemes"
```

---

### Task 8: Publish One Short Canonical Demo And Derived Review Clips

**Files:**
- Modify: `mcp/ip_avatar_3d/render_warm_studio_demo.py`
- Modify: `mcp/ip_avatar_3d/test_warm_studio_demo.py`
- Modify: `mcp/ip_avatar_3d/README.md`
- Generate untracked: `outputs/Sloth_WarmStudio_Seated_Demo_1080p.mp4`
- Generate untracked: `outputs/Sloth_WarmStudio_StandSit_Demo_1080p.mp4`
- Generate untracked: `outputs/Sloth_WarmStudio_DualMode_Reel_1080p.mp4`
- Generate untracked: `outputs/Sloth_WarmStudio_ActionPack_1080p.mp4`
- Generate untracked: `outputs/Sloth_WarmStudio_Transition_ContactSheet.png`
- Generate untracked: `outputs/Sloth_WarmStudio_Viseme_Comparison.png`
- Generate untracked: `outputs/Sloth_WarmStudio_ActionPack_Report.json`

**Interfaces:**
- Consumes: `server.render_talking_video`, the production character profile, verified warm-studio lighting evidence, the permanent IP voice, and performance QA reports.
- Produces: one 15-30 second canonical Blender render plus inexpensive FFmpeg aliases/trims and review evidence.

- [ ] **Step 1: Write failing demo-runner tests**

Mock the renderer and assert exactly one production call:

```python
self.assertEqual(len(calls), 1)
self.assertEqual(calls[0]["kind"], "standSitActionPack")
self.assertEqual(calls[0]["presentationMode"], "standing")
self.assertEqual(
    calls[0]["actionSequence"],
    [
        "Aroll_Welcome_OpenArms",
        "Aroll_KeyPoint_OneFinger",
        "Aroll_Transition_StandToSit",
        "Aroll_Seated_Explain",
        "Aroll_Question_PalmUp",
        "Aroll_Seated_LeanIn",
        "Aroll_Transition_SitToStand",
        "Aroll_Conclusion_HandsTogether",
    ],
)
self.assertEqual(calls[0]["cameraPreset"], "transition")
```

Assert `FINAL_FILENAMES` contains all seven deliverables, staging rollback removes partial files, the canonical render duration is between 15 and 30 seconds, and the final report includes `stateTimeline`, `actionCatalogVersion`, `transitionQa`, `visemeQa`, hashes, 1920x1080, 30 fps CFR, audio loudness, and voice provenance.

- [ ] **Step 2: Run RED**

```bash
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py -v
```

Expected: current runner renders standing and seated separately and concatenates them with a hard cut.

- [ ] **Step 3: Define the single production script and action sequence**

```python
DEMO_JOB = {
    "kind": "standSitActionPack",
    "presentationMode": "standing",
    "cameraPreset": "transition",
    "script": "大家好，我是小唐。先说结论：AI创作真正重要的是可控。我们坐下来拆开看，选题、脚本、画面和审核都要能修改和复用。最后总结，稳定流程才能带来稳定内容。",
    "actionSequence": [
        "Aroll_Welcome_OpenArms",
        "Aroll_KeyPoint_OneFinger",
        "Aroll_Transition_StandToSit",
        "Aroll_Seated_Explain",
        "Aroll_Question_PalmUp",
        "Aroll_Seated_LeanIn",
        "Aroll_Transition_SitToStand",
        "Aroll_Conclusion_HandsTogether",
    ],
    "minimumDurationSec": 15.0,
    "maximumDurationSec": 30.0,
}
```

- [ ] **Step 4: Replace the hard-cut reel with the continuous performance**

Render `DEMO_JOB` exactly once. Copy that validated canonical render to the staged
`Sloth_WarmStudio_StandSit_Demo_1080p.mp4`,
`Sloth_WarmStudio_DualMode_Reel_1080p.mp4`, and
`Sloth_WarmStudio_ActionPack_1080p.mp4` paths without re-encoding. Derive the
seated review clip with FFmpeg from the canonical timeline, keeping a 15-second
window that begins before the sit transition and includes the settled seated
performance. Do not launch another Blender render. Build a separate transition
contact sheet from 18 frames spanning `0.4 s` before the sit action through
`0.4 s` after the character stands again.

Create the viseme comparison with labeled stills captured at maximum `Mouth_Rest`, `Mouth_MBP`, `Mouth_A`, `Mouth_E`, `Mouth_O`, `Mouth_U`, and `Mouth_Surprise` activation from the production camera. Labels are review evidence outside the rendered character image and are not face overlays.

- [ ] **Step 5: Keep atomic publication and extend validation**

Before `publish_transaction()` require:

```python
for key in ("seated", "standSit", "reel", "actionPack"):
    probe = media_probe(staged_paths[key])
    if (probe.get("width"), probe.get("height")) != (1920, 1080):
        raise DemoQAError(f"{key} must be 1920x1080")
    if not math.isclose(float(probe.get("fps") or 0.0), 30.0, abs_tol=0.01):
        raise DemoQAError(f"{key} must be 30 fps")
    if probe.get("constantFrameRate") is False:
        raise DemoQAError(f"{key} must be CFR")
    if not 15.0 <= float(probe.get("durationSec") or 0.0) <= 30.0:
        raise DemoQAError(f"{key} must be between 15 and 30 seconds")
```

Require each production result to report `gpt_sovits_local`, voice ID `main_ip_warm_knowledge_host_v1`, `productionReady=True`, `-16.0 +/- 0.5 LUFS`, and true peak no higher than `-1.5 dBTP`.

- [ ] **Step 6: Run demo-runner tests and commit tracked code**

```bash
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py -v
git add mcp/ip_avatar_3d/render_warm_studio_demo.py \
  mcp/ip_avatar_3d/test_warm_studio_demo.py \
  mcp/ip_avatar_3d/README.md
git commit -m "feat: publish continuous warm-studio a-roll demos"
```

---

### Task 9: Run Full Verification And Render Production Evidence

**Files:**
- Modify generated if corrections are required: `mcp/ip_avatar_3d/aroll_actions.py`
- Modify generated if corrections are required: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify generated if corrections are required: `mcp/ip_avatar_3d/warm_studio_contract.py`
- Modify generated if corrections are required: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`
- Generate untracked: all Task 8 outputs

**Interfaces:**
- Consumes: all implementation tasks and verified local production assets.
- Produces: passing test suite, rendered videos, visual evidence, and final report.

- [ ] **Step 1: Run all pure Python and Go tests**

```bash
python3 mcp/ip_avatar_3d/test_aroll_actions.py -v
python3 mcp/ip_avatar_3d/test_aroll_performance_qa.py -v
python3 mcp/ip_avatar_3d/test_warm_studio_contract.py -v
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py -v
python3 -m unittest mcp.ip_avatar_3d.test_server -v
go test ./local-backend/internal/localmcp -v
```

Expected: all tests pass.

- [ ] **Step 2: Run Blender scene and character suites**

```bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend" \
  --python mcp/ip_avatar_3d/test_blender_scene_contract.py
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: scene, transition, finger, wrist, mouth, collision, and action-library tests pass.

- [ ] **Step 3: Verify the local production voice before rendering**

```bash
python3 mcp/ip_avatar_3d/server.py --help >/dev/null
python3 - <<'PY'
import json
import sys
sys.path.insert(0, "mcp/ip_avatar_3d")
import server
print(json.dumps(server.check_gpt_sovits_voice(
    characterProfilePath="ip形象/main_ip/character-profile.json"
), ensure_ascii=False, indent=2))
PY
```

Expected: provider `gpt_sovits_local`, voice ID `main_ip_warm_knowledge_host_v1`, and `productionReady=true`.

- [ ] **Step 4: Render all production demos**

```bash
python3 mcp/ip_avatar_3d/render_warm_studio_demo.py \
  --profile "$PWD/ip形象/main_ip/character-profile.json" \
  --output-dir "$PWD/outputs" \
  --lighting-evidence "$PWD/outputs/Sloth_WarmStudio_Lighting_Evidence.json"
```

Expected: the seven required artifacts are published atomically.

- [ ] **Step 5: Probe the rendered media**

```bash
for video in \
  outputs/Sloth_WarmStudio_Seated_Demo_1080p.mp4 \
  outputs/Sloth_WarmStudio_StandSit_Demo_1080p.mp4 \
  outputs/Sloth_WarmStudio_DualMode_Reel_1080p.mp4 \
  outputs/Sloth_WarmStudio_ActionPack_1080p.mp4; do
  ffprobe -v error \
    -show_entries stream=codec_type,codec_name,width,height,r_frame_rate,avg_frame_rate,sample_rate,channels \
    -show_entries format=duration \
    -of json "$video"
done
```

Expected: each video is 1920x1080, 30 fps CFR, H.264 plus 48 kHz mono AAC, with positive duration.

- [ ] **Step 6: Perform visual review at motion extrema**

Inspect `Sloth_WarmStudio_Transition_ContactSheet.png` and the full stand/sit video at normal speed and frame-by-frame. Reject and recalibrate if any of these are visible:

- the old central thigh/pixel line;
- foot sliding, knee collapse, leg stretching, or body/desk/chair intersection;
- unsupported floating before seat contact or standing lift;
- hidden stool, oversized stool, or a camera that loses hands/head;
- hand penetration, incorrect palm direction, locked wrists, or identical finger poses;
- mouth-card appearance, weak A/O/U separation, lip inversion, jaw penetration, or prolonged accidental closure;
- exposure mismatch, clipped fur/cardigan, background brighter than the face, temporal stutter, duplicate frames, or audio drift.

Record acceptance in `Sloth_WarmStudio_ActionPack_Report.json` with `visualReview.success=true`, reviewed filenames, frame numbers, and reviewer timestamp.

- [ ] **Step 7: Run final repository checks and commit any evidence-driven code corrections**

```bash
git diff --check
git status --short
git log --oneline --decorate -12
```

If visual review required tracked calibration changes, rerun Steps 1-6, then commit only those tracked source/test files:

```bash
git add mcp/ip_avatar_3d/aroll_actions.py \
  mcp/ip_avatar_3d/blender_renderer.py \
  mcp/ip_avatar_3d/warm_studio_contract.py \
  mcp/ip_avatar_3d/warm_sloth_studio_builder.py \
  mcp/ip_avatar_3d/test_aroll_actions.py \
  mcp/ip_avatar_3d/test_aroll_performance_qa.py \
  mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "fix: calibrate production sloth a-roll performance"
```

Expected: tracked worktree is clean; only intentional untracked model, voice, backup, and output artifacts remain.

---

## Completion Criteria

- The character performs standing speech, sits without a cut, speaks while seated, and stands without a cut.
- The old 16-second central thigh artifact is absent at normal playback and frame-by-frame review.
- Feet remain planted, knees remain separated, pelvis visibly contacts the stool, and the stool is partially readable.
- All catalog actions are present in the Blend action library and callable through the MCP.
- Script planning inserts required state transitions and rejects impossible or overlong sequences.
- Original source-mouth geometry clearly distinguishes Rest, MBP, A, E, O, U, and Surprise.
- Seated, stand/sit, dual-mode, and action-pack videos are 1920x1080, 30 fps CFR, use the permanent local IP voice, and pass audio gates.
- Pure Python, Blender, MCP bridge, geometry, collision, mouth, render, and media tests all pass.
- Final output reports include reproducible character, studio, voice, action, state timeline, and QA provenance.
