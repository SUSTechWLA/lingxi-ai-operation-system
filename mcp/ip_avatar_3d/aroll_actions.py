"""Shared semantic hand poses for avatar actions and procedural timelines."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Mapping


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

AROLL_ACTIONS: tuple[str, ...] = (
    "Aroll_Seated_Idle",
    "Aroll_Idle_Listening",
    "Aroll_Greeting_Wave",
    "Aroll_OpenPalm_Explain",
    "Aroll_Explain_Left",
    "Aroll_Explain_Right",
    "Aroll_Count_One",
    "Aroll_Count_Two",
    "Aroll_Count_Three",
    "Aroll_Point_Left",
    "Aroll_Point_Right",
    "Aroll_Pinch_Detail",
    "Aroll_Emphasis_SoftFist",
    "Aroll_Think",
    "Aroll_Agree_Nod",
    "Aroll_Disagree_Shake",
    "Aroll_Transition_Reset",
)


def presentation_pose(mode: str, source_rig: bool) -> dict[str, dict[str, tuple[float, float, float]]]:
    selected = str(mode or "standing").strip().lower()
    if selected == "standing":
        return {}
    if selected != "seated":
        raise ValueError(f"unsupported presentation mode: {mode}")
    if source_rig:
        leg_l = (0.0, 0.0, 1.35)
        leg_r = (0.05, 0.197, -1.30)
        shin_l = (0.0, 0.0, -1.55)
        shin_r = (-0.047, -0.0595, 1.52)
        foot_l = (0.0, 0.0, -0.229)
        foot_r = (-0.07175, -0.2125, 0.146)
    else:
        leg_l = leg_r = (1.02, 0.0, 0.0)
        shin_l = shin_r = (-1.16, 0.0, 0.0)
        foot_l = foot_r = (0.18, 0.0, 0.0)
    return {
        "root": {"location": (0.0, 0.12, -0.301)},
        "body": {"rotation": (0.08, 0.0, 0.0)},
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
    return {name: specs[name] for name in AROLL_ACTIONS}
