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
        DigitPose(0.10, 0.10, 0.06, splay=0.12),
        DigitPose(0.10, 0.10, 0.06),
        DigitPose(0.10, 0.10, 0.06, splay=-0.12),
    ),
    "point": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.10),
        DigitPose(0.28, 0.33, 0.21),
        DigitPose(0.29, 0.34, 0.22),
    ),
    "stop": _digits(
        DigitPose(0.04, 0.03, 0.02, splay=0.11, opposition=0.12),
        DigitPose(0.05, 0.04, 0.03, opposition=0.12),
        DigitPose(0.06, 0.05, 0.04, splay=-0.11, opposition=0.12),
    ),
    "finger_roll": _digits(
        DigitPose(0.29, 0.34, 0.22, splay=0.08),
        DigitPose(0.29, 0.34, 0.22),
        DigitPose(0.29, 0.34, 0.22, splay=-0.08),
    ),
}

PoseState = Literal["standing", "seated", "either"]

SOURCE_RIGHT_FOOT_PROFILE_DELTA = (0.0, 0.10, 0.04)
SOURCE_STAND_TO_SIT_RIGHT_FOOT_DELTA = (-0.04, 0.12, 0.06)


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
        if not source_rig:
            return {}
        return {
            "foot_l": {"rotation": (0.108573, 0.0047755, 0.0)},
            "foot_r": {"rotation": (0.1542314, -0.0882571, -0.0093923)},
        }
    if selected != "seated":
        raise ValueError(f"unsupported presentation mode: {mode}")
    if source_rig:
        leg_l = (0.0, 0.0, 1.35)
        leg_r = (0.05, 0.197, -1.30)
        shin_l = (0.0, 0.0, -1.55)
        shin_r = (-0.047, -0.0595, 1.52)
        foot_l = (-0.1024819, -0.2659181, -0.2740981)
        foot_r = (-0.1510481, 0.1414181, 0.2348769)
    else:
        leg_l = leg_r = (1.02, 0.0, 0.0)
        shin_l = shin_r = (-1.16, 0.0, 0.0)
        foot_l = foot_r = (0.18, 0.0, 0.0)
    return {
        "root": {"location": (0.0, 0.12, -0.3005)},
        "body": {"rotation": (0.08, 0.0, 0.0)},
        "leg_l": {"rotation": leg_l},
        "shin_l": {"rotation": shin_l},
        "foot_l": {"rotation": foot_l},
        "leg_r": {"rotation": leg_r},
        "shin_r": {"rotation": shin_r},
        "foot_r": {"rotation": foot_r},
    }


def _offset_rotation(
    rotation: tuple[float, float, float],
    delta: tuple[float, float, float],
) -> tuple[float, float, float]:
    return tuple(rotation[index] + delta[index] for index in range(3))


def _offset_pose_rotation(
    pose: ActionPose,
    role: str,
    delta: tuple[float, float, float],
) -> ActionPose:
    adjusted: ActionPose = {
        name: dict(transform) if isinstance(transform, dict) else transform
        for name, transform in pose.items()
    }
    transform = adjusted.get(role)
    if isinstance(transform, dict) and "rotation" in transform:
        transform["rotation"] = _offset_rotation(transform["rotation"], delta)
    return adjusted


def transition_presentation_pose(
    mode: str,
    source_rig: bool,
) -> dict[str, dict[str, tuple[float, float, float]]]:
    pose = presentation_pose(mode, source_rig)
    if not source_rig:
        return pose
    return _offset_pose_rotation(
        pose,
        "foot_r",
        SOURCE_RIGHT_FOOT_PROFILE_DELTA,
    )


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


_ARM_POSE_ROLES = {
    "shoulder_l", "shoulder_r", "upper_arm_l", "upper_arm_r",
    "forearm_l", "forearm_r", "hand_l", "hand_r",
}


def _seated_action_pose(
    pose: Mapping[str, Any],
    source_rig: bool,
    *,
    arm_scale: float = 0.82,
) -> ActionPose:
    """Layer an upper-body cue over the exact seated contact pose."""
    seated = presentation_pose("seated", source_rig)
    merged: ActionPose = {
        role: dict(transform) for role, transform in seated.items()
    }
    for role, value in pose.items():
        if role in {"__digit_pose_l", "__digit_pose_r"}:
            merged[role] = value
            continue
        if role in _ARM_POSE_ROLES and isinstance(value, (tuple, list)):
            merged[role] = tuple(float(component) * arm_scale for component in value)
            continue
        if role in {"body", "root"}:
            transform = (
                {"rotation": tuple(value)}
                if isinstance(value, (tuple, list))
                else dict(value)
            )
            base = merged.setdefault(role, {})
            for component, delta in transform.items():
                origin = tuple(base.get(component, (0.0, 0.0, 0.0)))
                base[component] = tuple(
                    float(origin[index]) + float(delta[index]) for index in range(3)
                )
            continue
        merged[role] = dict(value) if isinstance(value, dict) else value
    return merged


def _seated_action_spec(
    frames: tuple[int, int, int, int, int],
    source_rig: bool,
    anticipation_pose: Mapping[str, Any],
    hold_pose: Mapping[str, Any],
    *,
    release_pose: Mapping[str, Any] | None = None,
) -> ActionSpec:
    return [
        (frame, _seated_action_pose(pose, source_rig))
        for frame, pose in _five_phase(
            frames,
            anticipation_pose,
            hold_pose,
            release_pose=release_pose,
        )
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
    standing = presentation_pose("standing", source_rig)
    stable_seated = transition_presentation_pose("seated", source_rig)
    stable_standing = transition_presentation_pose("standing", source_rig)
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
    if source_rig:
        lower_roles = ("leg_l", "shin_l", "foot_l", "leg_r", "shin_r", "foot_r")
        source_contact_data = (
            (1, (0.0, 0.0, 0.0), (0.0, 0.0, 0.0), (0.0, 0.0, 0.0), (0.108573, 0.0047755, 0.0), (0.0, 0.0, 0.0), (0.0, 0.0, 0.0), (0.1542314, -0.0882571, -0.0093923)),
            (5, (0.0, 0.0, 0.0), (0.0, 0.0805481, 0.0006699), (0.0, 0.0822144, -0.0006699), (0.061607, 0.1318455, 0.0206927), (0.0, 0.0073205, 0.0), (0.0, 0.0, 0.0006699), (0.0783336, -0.0511086, -0.0068054)),
            (9, (0.0, 0.0, 0.0), (0.0, 0.1562244, 0.0013397), (0.0, 0.1644289, -0.0013397), (0.014641, 0.2583629, 0.0252803), (0.0, 0.014641, 0.0), (0.0, 0.0, 0.0013397), (0.0024359, -0.001841, 0.0006533)),
            (13, (0.0, 0.0123077, -0.0144142), (0.0013627, 0.1577983, -0.001597), (-0.0013627, 0.1626539, -0.0137075), (-0.0176904, 0.2305333, 0.0942217), (0.0, -0.0124827, 0.0), (-0.0004122, 0.0040927, 0.011596), (0.0076935, 0.0669189, -0.0520457)),
            (17, (0.0, 0.0246154, -0.0308284), (0.0027255, 0.1618082, -0.0045337), (-0.0027255, 0.160879, -0.0236395), (-0.0500217, 0.1998249, 0.163163), (0.0, -0.0396064, 0.0), (-0.0008244, 0.0081854, 0.0174234), (0.013394, 0.1266049, -0.0958868)),
            (21, (0.0, 0.0369231, -0.0452426), (0.0040882, 0.1609462, -0.0074703), (-0.0040882, 0.159104, -0.0384432), (-0.0823531, 0.1719953, 0.2321044), (0.0, -0.0667301, 0.0), (-0.0012366, 0.0122781, 0.0296726), (0.0255164, 0.1851527, -0.1441569)),
            (22, (0.0, 0.04, -0.0487328), (0.0044289, 0.1613397, -0.0082045), (-0.0044289, 0.1586603, -0.0415352), (-0.0904359, 0.16, 0.2493397), (0.0, -0.073511, 0.0), (-0.0013397, 0.0133013, 0.0346726), (0.0274398, 0.2033744, -0.1562244)),
            (25, (0.0, 0.0477333, -0.0649824), (0.0036908, 0.2386682, 0.1538208), (0.0043618, 0.1617145, -0.1918914), (-0.0064332, 0.1286032, 0.3322142), (0.006, 0.0279358, -0.156), (-0.0067564, 0.0175745, 0.1704837), (-0.0170122, 0.3675613, -0.1466817)),
            (27, (0.0, 0.0528889, -0.0760541), (0.0031987, 0.0402321, 0.3058377), (0.0773953, 0.1416005, -0.3201056), (-0.0206498, 0.1630577, 0.2655416), (0.01, 0.0538859, -0.26), (-0.0103676, 0.059668, 0.2032726), (-0.0070475, 0.2713526, -0.1086046)),
            (29, (0.0, 0.0580444, -0.0871258), (0.0027066, -0.158204, 0.4578546), (0.1504287, 0.1214864, -0.4483197), (-0.0948664, 0.1375121, 0.198869), (0.014, 0.079836, -0.364), (-0.0139787, 0.1017614, 0.2360615), (0.0029173, 0.1751439, -0.0705275)),
            (33, (0.0, 0.0683556, -0.1104156), (0.0017224, -0.106731, 0.5858883), (-0.0232281, 0.2701895, -0.5343673), (0.0572594, 0.0389796, 0.1471804), (0.022, 0.012093, -0.5675711), (-0.0187651, 0.0432161, 0.4425695), (-0.0880821, 0.2454688, -0.0085388)),
            (37, (0.0, 0.0786667, -0.1379179), (0.0007381, -0.0173189, 0.8019221), (-0.0007381, 0.0308639, -0.806959), (-0.0297136, -0.0022459, -0.0017155), (0.03, 0.1776975, -0.78), (-0.0239944, 0.0361113, 0.7606774), (0.011485, 0.1206225, 0.070552)),
            (40, (0.0, 0.0864, -0.1578784), (0.0, -0.0426314, 0.9639474), (0.0, 0.1061905, -0.9919877), (0.0, -0.0403388, -0.203227), (0.036, 0.05384, -0.936), (-0.03384, 0.0699429, 0.9639584), (0.008294, 0.0375661, 0.1502429)),
            (41, (0.0, 0.08976, -0.1734051), (0.0, -0.0383683, 1.0021751), (0.0, 0.0848718, -1.0461632), (-0.0012481, -0.0673545, -0.1967551), (0.037266, 0.06829, -0.9741077), (-0.03529, 0.0708562, 1.0194112), (0.0033794, 0.0640978, 0.1452382)),
            (45, (0.0, 0.1032, -0.2295986), (0.0, -0.0213157, 1.1550859), (0.0, 0.014851, -1.2628653), (-0.0062407, -0.1754176, -0.2053005), (0.0423301, 0.1260899, -1.1265384), (-0.0410898, 0.1002512, 1.2509661), (-0.0162789, 0.1051478, 0.1210662)),
            (47, (0.0, 0.10992, -0.2614294), (0.0, -0.0127894, 1.2315413), (0.0, -0.0057399, -1.3702199), (-0.0097336, -0.229449, -0.2428044), (0.0448622, 0.1549898, -1.2027538), (-0.0439898, 0.1391328, 1.3679615), (-0.0261081, 0.10743, 0.1353788)),
            (49, (0.0, 0.11664, -0.2932602), (0.0, -0.0042631, 1.3079967), (0.0, -0.0263307, -1.4775744), (-0.0132264, -0.2834805, -0.2903083), (0.0473943, 0.1838897, -1.2789692), (-0.0468897, 0.1780144, 1.4849568), (-0.0359373, 0.1097123, 0.1496914)),
            (50, (0.0, 0.12, -0.3116458), (0.0, 0.0, 1.3462244), (0.0, -0.0407846, -1.5337429), (-0.0124815, -0.3104963, -0.3031827), (0.0486603, 0.1983397, -1.3170769), (-0.0483397, 0.1970258, 1.5428455), (-0.0408519, 0.0964276, 0.1563629)),
            (53, (0.0, 0.12, -0.3091602), (0.0080526, 0.0, 1.3478425), (0.0, -0.016717, -1.5302218), (-0.0615415, -0.3072201, -0.3280686), (0.0492345, 0.1977655, -1.3141871), (-0.0477655, 0.1317321, 1.5399194), (-0.081214, 0.153146, 0.1900898)),
            (57, (0.0, 0.12, -0.3005), (0.0, 0.0, 1.35), (0.0, 0.0, -1.55), (-0.1024819, -0.2659181, -0.2740981), (0.05, 0.197, -1.3), (-0.047, -0.0595, 1.52), (-0.1510481, 0.1414181, 0.2348769)),
        )

        def lower_pose(item: tuple[Any, ...]) -> ActionPose:
            return {
                "root": {"location": item[1]},
                **{
                    role: {"rotation": item[index + 2]}
                    for index, role in enumerate(lower_roles)
                },
            }

        contact_spec = [(item[0], lower_pose(item)) for item in source_contact_data]

        def sample_spec(spec: ActionSpec, source_frame: float) -> ActionPose:
            before = max((item for item in spec if item[0] <= source_frame), default=spec[0], key=lambda item: item[0])
            after = min((item for item in spec if item[0] >= source_frame), default=spec[-1], key=lambda item: item[0])
            if before[0] == after[0]:
                return dict(before[1])
            return blend_action_pose(
                before[1],
                after[1],
                (source_frame - before[0]) / (after[0] - before[0]),
            )

        stand_upper_spec = [
            (stand_to_sit[0], standing),
            (stand_to_sit[1], prep),
            (stand_to_sit[2], load),
            (stand_to_sit[3], blend_action_pose({}, seated, 0.72)),
            (stand_to_sit[4], seated),
            (stand_to_sit[5], seated),
        ]
        sit_upper_spec = [
            (sit_to_stand[0], seated),
            (sit_to_stand[1], blend_action_pose(seated, load, 0.34)),
            (sit_to_stand[2], blend_action_pose(seated, load, 0.72)),
            (sit_to_stand[3], blend_action_pose(seated, {}, 0.68)),
            (sit_to_stand[4], prep),
            (sit_to_stand[5], standing),
        ]
        sit_frames = sorted(set(range(1, sit_to_stand[-1] + 1, 4)) | set(sit_to_stand))
        stand_spec = [
            (frame, _pose(sample_spec(stand_upper_spec, frame), lower))
            for frame, lower in contact_spec
        ]
        sit_spec = [
            (
                frame,
                _pose(
                    sample_spec(sit_upper_spec, frame),
                    sample_spec(
                        contact_spec,
                        stand_to_sit[-1]
                        - (frame - 1) / (sit_to_stand[-1] - 1) * (stand_to_sit[-1] - 1),
                    ),
                ),
            )
            for frame in sit_frames
        ]
        stand_spec = [
            (
                frame,
                _offset_pose_rotation(
                    pose, "foot_r", SOURCE_STAND_TO_SIT_RIGHT_FOOT_DELTA
                ),
            )
            for frame, pose in stand_spec
        ]
        sit_spec = [
            (
                frame,
                _offset_pose_rotation(
                    pose, "foot_r", SOURCE_RIGHT_FOOT_PROFILE_DELTA
                ),
            )
            for frame, pose in sit_spec
        ]
        stand_spec[0] = (
            stand_to_sit[0],
            _offset_pose_rotation(
                standing, "foot_r", SOURCE_STAND_TO_SIT_RIGHT_FOOT_DELTA
            ),
        )
        stand_spec[-1] = (stand_to_sit[-1], stable_seated)
        sit_spec[0] = (sit_to_stand[0], stable_seated)
        sit_spec[-1] = (sit_to_stand[-1], stable_standing)
        return {
            "Aroll_Transition_StandToSit": stand_spec,
            "Aroll_Transition_SitToStand": sit_spec,
        }
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
            (sit_to_stand[5], standing),
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
    if source_rig:
        welcome = _pose(
            {
                "body": (0.0, 0.0, 0.018),
                "upper_arm_l": (-0.04, 0.02, 0.42),
                "forearm_l": (-0.30, 0.08, -0.10),
                "hand_l": (0.05, 0.30, -0.10),
                "upper_arm_r": (-0.04, -0.02, -0.42),
                "forearm_r": (-0.30, -0.08, 0.10),
                "hand_r": (0.05, -0.30, 0.10),
            },
            left_hand="open_hand",
            right_hand="open_hand",
        )
        question = _pose(
            {
                "head": (0.0, -0.05, -0.02),
                "upper_arm_r": (-0.06, -0.02, -0.70),
                "forearm_r": (-0.66, -0.06, 0.72),
                "hand_r": (0.12, -0.42, 0.14),
            },
            right_hand="open_hand",
        )
        caution = _pose(
            {
                "body": (0.0, 0.0, -0.012),
                "upper_arm_r": (-0.02, -0.02, -0.48),
                "forearm_r": (-0.28, -0.04, 0.70),
                "hand_r": (0.02, -1.02, 0.02),
            },
            right_hand="stop",
        )
        quote_frame = _pose(
            {
                "upper_arm_l": (-0.05, 0.02, 0.62),
                "forearm_l": (-0.64, 0.08, -0.50),
                "hand_l": (0.08, 0.18, -0.18),
                "upper_arm_r": (-0.05, -0.02, -0.62),
                "forearm_r": (-0.64, -0.08, 0.50),
                "hand_r": (0.08, -0.18, 0.18),
            },
            left_hand="count_two",
            right_hand="count_two",
        )
        hands_together = _pose(
            {
                "body": (0.0, 0.0, 0.015),
                "upper_arm_l": (-0.05, 0.02, 0.72),
                "forearm_l": (-0.76, 0.06, -0.70),
                "hand_l": (0.08, 0.34, -0.12),
                "upper_arm_r": (-0.05, -0.02, -0.72),
                "forearm_r": (-0.76, -0.06, 0.70),
                "hand_r": (0.08, -0.34, 0.12),
            },
            left_hand="relaxed_hand",
            right_hand="relaxed_hand",
        )
    else:
        welcome = _pose(open_explain, left_hand="open_hand", right_hand="open_hand")
        question = _pose(right_chest, right_hand="open_hand")
        caution = _pose(
            {**right_point, "hand_r": (0.02, -1.02, 0.02)},
            right_hand="stop",
        )
        quote_frame = _pose(open_explain, left_hand="count_two", right_hand="count_two")
        hands_together = _pose(
            open_explain,
            left_hand="relaxed_hand",
            right_hand="relaxed_hand",
        )

    compare_left = _pose(left_chest, left_hand="open_hand")
    compare_right = _pose(right_chest, right_hand="open_hand")
    key_point = _pose(right_chest, right_hand="count_one")
    list_three = _pose(right_chest, right_hand="count_three")
    rich_anticipation = _pose(
        open_explain_anticipation,
        left_hand="relaxed_hand",
        right_hand="relaxed_hand",
    )
    standing = presentation_pose("standing", source_rig)
    welcome_spec = _five_phase(frames, rich_anticipation, welcome)
    welcome_spec[0] = (frames[0], standing)
    welcome_spec[-1] = (frames[-1], standing)
    conclusion_spec = _five_phase(frames, rich_anticipation, hands_together)
    conclusion_spec[0] = (frames[0], standing)
    conclusion_spec[-1] = (frames[-1], standing)
    compare_right_frame = max(hold_in + 1, int(round(max(8, fps) * 1.28)))
    compare_spec = [
        (frames[0], {}),
        (anticipation, _pose(left_anticipation, left_hand="relaxed_hand")),
        (hold_in, compare_left),
        (compare_right_frame, compare_right),
        (hold_out, _pose(right_anticipation, right_hand="relaxed_hand")),
        (end, {}),
    ]
    lean_anticipation = _pose(
        open_explain_anticipation,
        {"body": (0.025, 0.0, 0.0), "root": {"location": (0.0, 0.012, 0.0)}},
        left_hand="relaxed_hand",
        right_hand="relaxed_hand",
    )
    lean_hold = _pose(
        open_explain,
        {"body": (0.10, 0.0, 0.0), "root": {"location": (0.0, 0.035, 0.0)}},
        left_hand="relaxed_hand",
        right_hand="relaxed_hand",
    )
    lean_release = _pose(
        open_explain_anticipation,
        {"body": (0.045, 0.0, 0.0), "root": {"location": (0.0, 0.015, 0.0)}},
        left_hand="relaxed_hand",
        right_hand="relaxed_hand",
    )

    specs.update({
        "Aroll_Standing_Idle": _five_phase(frames, idle_anticipation, idle_pose),
        "Aroll_Welcome_OpenArms": welcome_spec,
        "Aroll_Question_PalmUp": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            question,
        ),
        "Aroll_Compare_TwoSides": compare_spec,
        "Aroll_KeyPoint_OneFinger": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            key_point,
        ),
        "Aroll_List_Three": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            list_three,
        ),
        "Aroll_Caution_Stop": _five_phase(
            frames,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            caution,
        ),
        "Aroll_Quote_Frame": _five_phase(frames, rich_anticipation, quote_frame),
        "Aroll_Conclusion_HandsTogether": conclusion_spec,
        "Aroll_Seated_Explain": _seated_action_spec(
            frames,
            source_rig,
            rich_anticipation,
            _pose(open_explain, left_hand="open_hand", right_hand="open_hand"),
        ),
        "Aroll_Seated_OpenPalm": _seated_action_spec(
            frames,
            source_rig,
            _pose(right_anticipation, right_hand="relaxed_hand"),
            question,
        ),
        "Aroll_Seated_LeanIn": _seated_action_spec(
            frames,
            source_rig,
            lean_anticipation,
            lean_hold,
            release_pose=lean_release,
        ),
    })
    specs.update(build_transition_specs(source_rig, fps))
    return {name: specs[name] for name in AROLL_ACTIONS}
