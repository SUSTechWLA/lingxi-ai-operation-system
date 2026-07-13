"""Shared semantic hand poses for avatar actions and procedural timelines."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Mapping


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
FIST = DigitPose(0.38, 0.48, 0.32)


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
        DigitPose(0.30, 0.40, 0.28, splay=0.06),
        DigitPose(0.18, 0.24, 0.16),
        DigitPose(0.34, 0.42, 0.30, splay=-0.08, opposition=0.16),
    ),
    "count_one": _digits(
        OPEN,
        DigitPose(0.34, 0.43, 0.29),
        DigitPose(0.36, 0.45, 0.31),
    ),
    "count_two": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.05),
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=-0.05),
        DigitPose(0.36, 0.45, 0.31),
    ),
    "count_three": _digits(
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=0.12),
        OPEN,
        DigitPose(OPEN.proximal, OPEN.middle, OPEN.distal, splay=-0.12),
    ),
    "point": _digits(
        OPEN,
        DigitPose(0.34, 0.43, 0.29),
        DigitPose(0.36, 0.45, 0.31),
    ),
    "finger_roll": _digits(
        DigitPose(0.36, 0.45, 0.31, splay=0.08),
        DigitPose(0.36, 0.45, 0.31),
        DigitPose(0.36, 0.45, 0.31, splay=-0.08),
    ),
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
