"""Shared semantic hand poses for avatar actions and procedural timelines."""

from __future__ import annotations

from dataclasses import dataclass
from types import MappingProxyType
from typing import Any, Literal, Mapping, Sequence


@dataclass(frozen=True)
class DigitPose:
    """A curl/splay/opposition pose expressed in the avatar's semantic digit axes."""

    proximal: float
    middle: float
    distal: float
    splay: float = 0.0
    opposition: float = 0.0


OPEN = DigitPose(0.015, 0.010, 0.005)
RELAXED = DigitPose(0.10, 0.12, 0.06)
FIST = DigitPose(0.28, 0.32, 0.20)


def _digits(*poses: DigitPose) -> dict[int, DigitPose]:
    return {digit: pose for digit, pose in enumerate(poses, start=1)}


HAND_POSES: Mapping[str, Mapping[int, DigitPose]] = {
    "open_hand": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.14),
        OPEN,
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=-0.14),
    ),
    "relaxed_hand": _digits(
        DigitPose(RELAXED.proximal, RELAXED.middle, RELAXED.distal, splay=0.035),
        RELAXED,
        DigitPose(RELAXED.proximal, RELAXED.middle, RELAXED.distal, splay=-0.035),
    ),
    "soft_curl": _digits(
        DigitPose(0.22, 0.28, 0.18, splay=0.035),
        DigitPose(0.20, 0.26, 0.17),
        DigitPose(0.24, 0.30, 0.19, splay=-0.035),
    ),
    "fist": _digits(
        FIST,
        FIST,
        DigitPose(FIST.proximal, FIST.middle, FIST.distal, splay=-0.035, opposition=0.11),
    ),
    "pinch": _digits(
        DigitPose(0.24, 0.30, 0.20, splay=0.05),
        DigitPose(0.14, 0.18, 0.12),
        DigitPose(0.27, 0.33, 0.21, splay=-0.06, opposition=0.10),
    ),
    "count_one": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.10),
        DigitPose(0.28, 0.33, 0.21),
        DigitPose(0.29, 0.34, 0.22),
    ),
    "count_two": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.05),
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=-0.05),
        DigitPose(0.29, 0.34, 0.22),
    ),
    "count_three": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.12),
        OPEN,
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=-0.12),
    ),
    "point": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.10),
        DigitPose(0.28, 0.33, 0.21),
        DigitPose(0.29, 0.34, 0.22),
    ),
    "finger_roll": _digits(
        DigitPose(0.29, 0.34, 0.22, splay=0.08),
        DigitPose(0.29, 0.34, 0.22),
        DigitPose(0.29, 0.34, 0.22, splay=-0.08),
    ),
}

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
        _meta("Aroll_Idle_Listening", "either", "either", 2.0, ("body", "head"), ("idle", "listening"), loopable=True),
        _meta("Aroll_Greeting_Wave", "either", "either", 1.8, ("arm_r", "hand_r"), ("greeting", "wave")),
        _meta("Aroll_OpenPalm_Explain", "either", "either", 1.8, ("arms", "hands"), ("explain", "present")),
        _meta("Aroll_Explain_Left", "either", "either", 1.6, ("arm_l", "hand_l"), ("explain", "left")),
        _meta("Aroll_Explain_Right", "either", "either", 1.6, ("arm_r", "hand_r"), ("explain", "right")),
        _meta("Aroll_Count_One", "either", "either", 1.6, ("arm_r", "hand_r"), ("count", "one")),
        _meta("Aroll_Count_Two", "either", "either", 1.6, ("arm_r", "hand_r"), ("count", "two")),
        _meta("Aroll_Count_Three", "either", "either", 1.6, ("arm_r", "hand_r"), ("count", "three")),
        _meta("Aroll_Point_Left", "either", "either", 1.5, ("arm_l", "hand_l"), ("point", "left")),
        _meta("Aroll_Point_Right", "either", "either", 1.5, ("arm_r", "hand_r"), ("point", "right")),
        _meta("Aroll_Pinch_Detail", "either", "either", 1.5, ("arm_r", "hand_r"), ("detail",)),
        _meta("Aroll_Emphasis_SoftFist", "either", "either", 1.5, ("body", "arm_r", "hand_r"), ("emphasis",)),
        _meta("Aroll_Think", "either", "either", 1.5, ("head", "arm_r", "hand_r"), ("thinking",)),
        _meta("Aroll_Agree_Nod", "either", "either", 1.2, ("head",), ("agreement", "nod")),
        _meta("Aroll_Disagree_Shake", "either", "either", 1.2, ("head",), ("disagreement", "shake")),
        _meta("Aroll_Transition_Reset", "either", "either", 1.0, ("body", "arms", "hands"), ("reset",)),
    )
})

AROLL_ACTIONS: tuple[str, ...] = tuple(ACTION_CATALOG)


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


def hand_pose(name: str) -> Mapping[int, DigitPose]:
    return HAND_POSES[name]


def finger_roll_pose(
    selected: int, *, unselected_pose: str = "open_hand"
) -> dict[int, DigitPose]:
    if selected not in (1, 2, 3):
        raise ValueError(f"finger-roll digit must be 1, 2, or 3; got {selected!r}")
    rest_pose = HAND_POSES[unselected_pose]
    roll_pose = HAND_POSES["finger_roll"]
    return {
        digit: roll_pose[digit] if digit == selected else rest_pose[digit]
        for digit in (1, 2, 3)
    }


def blend_digit_pose(base: DigitPose, target: DigitPose, amount: float) -> DigitPose:
    amount = max(0.0, min(1.0, float(amount)))
    return DigitPose(
        proximal=base.proximal + (target.proximal - base.proximal) * amount,
        middle=base.middle + (target.middle - base.middle) * amount,
        distal=base.distal + (target.distal - base.distal) * amount,
        splay=base.splay + (target.splay - base.splay) * amount,
        opposition=base.opposition + (target.opposition - base.opposition) * amount,
    )


def blend_hand_pose(
    base: Mapping[int, DigitPose], target: Mapping[int, DigitPose], amount: float
) -> dict[int, DigitPose]:
    return {digit: blend_digit_pose(base[digit], target[digit], amount) for digit in (1, 2, 3)}


def chain_eulers(side: str, pose: DigitPose) -> dict[str, tuple[float, float, float]]:
    """Map a semantic digit pose onto the source humanoid's local finger axes."""
    sign = 1.0 if str(side).lower().startswith("r") else -1.0
    return {
        "proximal": (pose.splay, pose.opposition, sign * pose.proximal),
        "middle": (pose.splay * 0.22, 0.0, sign * pose.middle),
        "distal": (pose.splay * 0.08, 0.0, sign * pose.distal),
    }


def chain_roles(side: str, digit: int) -> dict[str, str]:
    return {
        "proximal": f"finger_{digit}_{side}",
        "middle": f"finger_{digit}_mid_{side}",
        "distal": f"finger_{digit}_tip_{side}",
    }


def has_three_segment_chain(bone_map: Mapping[str, str], side: str, digit: int) -> bool:
    roles = chain_roles(side, digit)
    names = [bone_map.get(role) for role in roles.values()]
    return all(names) and len(set(names)) == 3


def has_legacy_two_segment_chain(bone_map: Mapping[str, str], side: str, digit: int) -> bool:
    roles = chain_roles(side, digit)
    proximal = bone_map.get(roles["proximal"])
    middle = bone_map.get(roles["middle"])
    distal = bone_map.get(roles["distal"])
    return bool(proximal and distal and (not middle or middle in {proximal, distal}))


def articulated_pose(curl: float, splay: float = 0.0, opposition: float = 0.0) -> DigitPose:
    """Preserve a semantic proximal-only cue while explicitly keying a full chain."""
    curl = max(0.0, min(0.46, float(curl)))
    return DigitPose(
        proximal=curl,
        middle=min(0.54, curl * 1.25),
        distal=min(0.38, curl * 0.84),
        splay=float(splay),
        opposition=float(opposition),
    )


def hand_pose_eulers(
    side: str, poses: Mapping[int, DigitPose], bone_map: Mapping[str, str]
) -> dict[str, tuple[float, float, float]]:
    """Return explicit local Euler values for every available three-segment digit role."""
    rotations: dict[str, tuple[float, float, float]] = {}
    for digit in (1, 2, 3):
        if digit not in poses or not has_three_segment_chain(bone_map, side, digit):
            continue
        roles = chain_roles(side, digit)
        for segment, euler in chain_eulers(side, poses[digit]).items():
            rotations[roles[segment]] = euler
    return rotations


ActionPose = dict[str, Any]
ActionSpec = list[tuple[int, ActionPose]]


def _action_frames(fps: int) -> tuple[int, int, int, int, int]:
    fps = max(8, int(fps or 30))
    anticipation = max(2, int(round(fps * 0.40)))
    hold_in = max(anticipation + 1, int(round(fps * 0.80)))
    hold_out = max(hold_in + 1, int(round(fps * 1.55)))
    end = max(hold_out + 1, int(round(fps * 2.00)))
    return 1, anticipation, hold_in, hold_out, end


def _pose(*parts: Mapping[str, Any], left_hand: str = "", right_hand: str = "") -> ActionPose:
    merged: ActionPose = {}
    for part in parts:
        merged.update(part)
    if left_hand:
        merged["__digit_pose_l"] = left_hand
    if right_hand:
        merged["__digit_pose_r"] = right_hand
    return merged


def _five_phase(
    frames: tuple[int, int, int, int, int],
    anticipation_pose: Mapping[str, Any],
    hold_pose: Mapping[str, Any],
    *,
    release_pose: Mapping[str, Any] | None = None,
) -> ActionSpec:
    start, anticipation, hold_in, hold_out, end = frames
    return [
        (start, {}),
        (anticipation, dict(anticipation_pose)),
        (hold_in, dict(hold_pose)),
        (hold_out, dict(hold_pose if release_pose is None else release_pose)),
        (end, {}),
    ]


def blend_action_pose(base: Mapping[str, Any], target: Mapping[str, Any], amount: float) -> ActionPose:
    """Interpolate semantic location/rotation components between two poses."""
    amount = max(0.0, min(1.0, float(amount)))
    blended: ActionPose = {}
    for bone in set(base) | set(target):
        if bone in {"__digit_pose_l", "__digit_pose_r"}:
            preferred = base.get(bone) if amount < 0.5 else target.get(bone)
            fallback = target.get(bone) if amount < 0.5 else base.get(bone)
            source = preferred if preferred is not None else fallback
            if source is not None:
                blended[bone] = source
            continue
        base_bone = base.get(bone) or {}
        target_bone = target.get(bone) or {}
        components: dict[str, tuple[float, float, float]] = {}
        for component in set(base_bone) | set(target_bone):
            if component not in {"location", "rotation"}:
                continue
            start = tuple(base_bone.get(component, (0.0, 0.0, 0.0)))
            end = tuple(target_bone.get(component, (0.0, 0.0, 0.0)))
            components[component] = tuple(
                start[index] + (end[index] - start[index]) * amount for index in range(3)
            )
        if components:
            blended[bone] = components
    return blended


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


def build_aroll_action_specs(source_rig: bool, fps: int) -> dict[str, ActionSpec]:
    """Build close-shot A-roll action specs from shared semantic hand poses."""
    frames = _action_frames(fps)
    _, anticipation, hold_in, hold_out, end = frames

    if source_rig:
        right_anticipation = {
            "upper_arm_r": (-0.04, -0.02, -0.96),
            "forearm_r": (-0.36, -0.04, 0.44),
            "hand_r": (0.04, -0.12, 0.08),
        }
        right_chest = {
            "upper_arm_r": (-0.06, -0.02, -0.72),
            "forearm_r": (-0.72, -0.06, 0.82),
            "hand_r": (0.10, -0.28, 0.18),
        }
        left_anticipation = {
            "upper_arm_l": (-0.04, 0.02, 0.96),
            "forearm_l": (-0.36, 0.04, -0.44),
            "hand_l": (0.04, 0.12, -0.08),
        }
        left_chest = {
            "upper_arm_l": (-0.06, 0.02, 0.72),
            "forearm_l": (-0.72, 0.06, -0.82),
            "hand_l": (0.10, 0.28, -0.18),
        }
        right_point = {
            "upper_arm_r": (-0.05, -0.02, -0.58),
            "forearm_r": (-0.48, -0.08, 0.60),
            "hand_r": (0.04, -0.20, 0.12),
        }
        left_point = {
            "upper_arm_l": (-0.05, 0.02, 0.58),
            "forearm_l": (-0.48, 0.08, -0.60),
            "hand_l": (0.04, 0.20, -0.12),
        }
        open_explain = {
            **left_chest,
            **right_chest,
            "hand_l": (0.08, 0.24, -0.12),
            "hand_r": (0.08, -0.24, 0.12),
        }
        open_explain_anticipation = {**left_anticipation, **right_anticipation}
        wave_raise = {
            "upper_arm_r": (-0.08, 0.03, -0.34),
            "forearm_r": (-0.25, 0.02, 1.04),
            "hand_r": (0.035, -1.10, 0.055),
        }
        wave_return = {
            "upper_arm_r": (-0.08, 0.03, -0.34),
            "forearm_r": (-0.25, 0.02, 1.04),
            "hand_r": (-0.035, -1.10, -0.055),
        }
        think_pose = {
            "upper_arm_r": (-0.09, -0.03, -0.74),
            "forearm_r": (-0.56, -0.08, 0.90),
            "hand_r": (0.13, -0.18, 0.12),
            "head": (-0.08, 0.10, 0.02),
        }
        nod_anticipation = {"head": (0.0, 0.0, -0.03)}
        nod_pose = {"head": (0.0, 0.0, 0.14)}
        shake_left = {"head": (0.0, -0.15, 0.0)}
        shake_right = {"head": (0.0, 0.15, 0.0)}
        idle_anticipation = {"body": (0.0, 0.0, -0.012), "head": (0.0, -0.012, -0.006)}
        idle_pose = {"body": (0.0, 0.0, 0.014), "head": (0.0, 0.010, 0.010)}
    else:
        right_anticipation = {
            "upper_arm_r": (-0.42, -0.08, 0.08),
            "forearm_r": (0.40, -0.14, 0.04),
            "hand_r": (0.04, -0.12, 0.06),
        }
        right_chest = {
            "upper_arm_r": (-0.24, -0.10, 0.14),
            "forearm_r": (0.72, -0.22, 0.08),
            "hand_r": (0.08, -0.16, 0.10),
        }
        left_anticipation = {
            "upper_arm_l": (-0.42, 0.08, -0.08),
            "forearm_l": (0.40, 0.14, -0.04),
            "hand_l": (0.04, 0.12, -0.06),
        }
        left_chest = {
            "upper_arm_l": (-0.24, 0.10, -0.14),
            "forearm_l": (0.72, 0.22, -0.08),
            "hand_l": (0.08, 0.16, -0.10),
        }
        right_point = {
            "upper_arm_r": (-0.20, -0.12, 0.16),
            "forearm_r": (0.46, -0.18, 0.06),
            "hand_r": (0.04, -0.16, 0.08),
        }
        left_point = {
            "upper_arm_l": (-0.20, 0.12, -0.16),
            "forearm_l": (0.46, 0.18, -0.06),
            "hand_l": (0.04, 0.16, -0.08),
        }
        open_explain = {
            **left_chest,
            **right_chest,
            "hand_l": (0.06, 0.22, -0.10),
            "hand_r": (0.06, -0.22, 0.10),
        }
        open_explain_anticipation = {**left_anticipation, **right_anticipation}
        wave_raise = {
            "shoulder_r": (0.04, -0.06, 0.10),
            "upper_arm_r": (0.02, 0.20, 0.32),
            "forearm_r": (0.82, 0.04, 0.02),
            "hand_r": (0.035, -1.10, 0.055),
        }
        wave_return = {
            "shoulder_r": (0.04, -0.06, 0.10),
            "upper_arm_r": (0.02, 0.20, 0.32),
            "forearm_r": (0.82, 0.04, 0.02),
            "hand_r": (-0.035, -1.10, -0.055),
        }
        think_pose = {
            "upper_arm_r": (-0.18, -0.14, 0.13),
            "forearm_r": (0.84, -0.22, 0.08),
            "hand_r": (0.13, -0.16, 0.10),
            "head": (0.02, 0.10, -0.08),
        }
        nod_anticipation = {"head": (-0.02, 0.0, 0.0)}
        nod_pose = {"head": (0.16, 0.0, 0.0)}
        shake_left = {"head": (0.0, -0.16, 0.0)}
        shake_right = {"head": (0.0, 0.16, 0.0)}
        idle_anticipation = {"body": (0.0, 0.0, -0.012), "head": (-0.008, 0.0, 0.010)}
        idle_pose = {"body": (0.0, 0.0, 0.014), "head": (0.014, 0.0, -0.010)}

    specs: dict[str, ActionSpec] = {
        "Aroll_Seated_Idle": [
            (frame, presentation_pose("seated", source_rig)) for frame in frames
        ],
        "Aroll_Idle_Listening": _five_phase(frames, idle_anticipation, idle_pose),
        "Aroll_Greeting_Wave": [
            (1, {}),
            (anticipation, _pose(wave_raise, right_hand="open_hand")),
            (hold_in, _pose(wave_return, right_hand="open_hand")),
            (hold_out, _pose(wave_raise, right_hand="open_hand")),
            (end, {}),
        ],
        "Aroll_OpenPalm_Explain": _five_phase(
            frames,
            _pose(open_explain_anticipation, left_hand="relaxed_hand", right_hand="relaxed_hand"),
            _pose(open_explain, left_hand="open_hand", right_hand="open_hand"),
        ),
        "Aroll_Explain_Left": _five_phase(
            frames,
            _pose(left_anticipation, left_hand="relaxed_hand"),
            _pose(left_chest, left_hand="open_hand"),
        ),
        "Aroll_Explain_Right": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="open_hand"),
        ),
        "Aroll_Count_One": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="count_one"),
        ),
        "Aroll_Count_Two": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="count_two"),
        ),
        "Aroll_Count_Three": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="count_three"),
        ),
        "Aroll_Point_Left": _five_phase(
            frames,
            _pose(left_anticipation, left_hand="relaxed_hand"),
            _pose(left_point, left_hand="point"),
        ),
        "Aroll_Point_Right": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_point, right_hand="point"),
        ),
        "Aroll_Pinch_Detail": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="pinch"),
        ),
        "Aroll_Emphasis_SoftFist": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="soft_curl"),
            _pose({**right_chest, "body": (0.0, 0.0, 0.026)}, right_hand="soft_curl"),
        ),
        "Aroll_Think": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(think_pose, right_hand="soft_curl"),
        ),
        "Aroll_Agree_Nod": [
            (1, {}),
            (anticipation, nod_anticipation),
            (hold_in, nod_pose),
            (hold_out, nod_anticipation),
            (end, {}),
        ],
        "Aroll_Disagree_Shake": [
            (1, {}),
            (anticipation, shake_left),
            (hold_in, shake_right),
            (hold_out, shake_left),
            (end, {}),
        ],
        "Aroll_Transition_Reset": [
            (1, {}),
            (anticipation, {}),
            (hold_in, {}),
            (hold_out, {}),
            (end, {}),
        ],
    }
    specs.update({
        "Aroll_Standing_Idle": _five_phase(frames, idle_anticipation, idle_pose),
        "Aroll_Welcome_OpenArms": _five_phase(
            frames,
            _pose(open_explain_anticipation, left_hand="relaxed_hand", right_hand="relaxed_hand"),
            _pose(open_explain, left_hand="open_hand", right_hand="open_hand"),
        ),
        "Aroll_Question_PalmUp": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="open_hand"),
        ),
        "Aroll_Compare_TwoSides": _five_phase(
            frames,
            _pose(open_explain_anticipation, left_hand="relaxed_hand", right_hand="relaxed_hand"),
            _pose(open_explain, left_hand="open_hand", right_hand="open_hand"),
        ),
        "Aroll_KeyPoint_OneFinger": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_point, right_hand="count_one"),
        ),
        "Aroll_List_Three": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="count_three"),
        ),
        "Aroll_Caution_Stop": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            _pose(right_chest, right_hand="open_hand"),
        ),
        "Aroll_Quote_Frame": _five_phase(
            frames,
            _pose(open_explain_anticipation, left_hand="relaxed_hand", right_hand="relaxed_hand"),
            _pose(open_explain, left_hand="open_hand", right_hand="open_hand"),
        ),
        "Aroll_Conclusion_HandsTogether": _five_phase(
            frames,
            _pose(open_explain_anticipation, left_hand="relaxed_hand", right_hand="relaxed_hand"),
            _pose(open_explain, left_hand="soft_curl", right_hand="soft_curl"),
        ),
        "Aroll_Seated_Explain": [
            (frame, {**presentation_pose("seated", source_rig), **pose})
            for frame, pose in _five_phase(
                frames,
                _pose(right_anticipation, right_hand="relaxed_hand"),
                _pose(right_chest, right_hand="open_hand"),
            )
        ],
        "Aroll_Seated_OpenPalm": [
            (frame, {**presentation_pose("seated", source_rig), **pose})
            for frame, pose in _five_phase(
                frames,
                _pose(right_anticipation, right_hand="relaxed_hand"),
                _pose(right_chest, right_hand="open_hand"),
            )
        ],
        "Aroll_Seated_LeanIn": [
            (frame, {**presentation_pose("seated", source_rig), **pose})
            for frame, pose in _five_phase(
                frames,
                _pose({"body": (0.0, 0.0, 0.02)}, right_hand="relaxed_hand"),
                _pose({"body": (0.08, 0.0, 0.0)}, right_hand="soft_curl"),
            )
        ],
    })
    specs.update(build_transition_specs(source_rig, fps))
    return {name: specs[name] for name in AROLL_ACTIONS}
