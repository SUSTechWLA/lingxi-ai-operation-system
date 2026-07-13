#!/usr/bin/env python3
"""Bone-name semantics shared by tests and the Blender renderer."""

from __future__ import annotations

import re
from collections.abc import Iterable


ROLE_ALIASES: dict[str, tuple[str, ...]] = {
    "root": ("root", "hips", "pelvis", "cog", "center"),
    "body": ("body", "torso", "hips", "pelvis", "upperchest", "chest", "spine2", "spine1", "spine"),
    "spine": ("spine", "spine1", "spine01", "lowerchest"),
    "chest": ("chest", "upperchest", "spine2", "spine02"),
    "neck": ("neck", "neck1", "neck01"),
    "head": ("head", "headbone"),
    "jaw": ("jaw", "lowerjaw", "mandible"),
    "eye_l": ("eyel", "lefteye", "eyeleft"),
    "eye_r": ("eyer", "righteye", "eyeright"),
    "tongue_1": ("tongue01", "tongue1", "tonguebase"),
    "tongue_2": ("tongue02", "tongue2", "tonguemid"),
    "tongue_3": ("tongue03", "tongue3", "tonguetip"),
    "shoulder_l": ("leftshoulder", "leftclavicle", "shoulderl", "claviclel", "lshoulder"),
    "upper_arm_l": ("leftupperarm", "leftarm", "upperarml", "arml", "lupperarm"),
    "forearm_l": ("leftforearm", "leftlowerarm", "forearml", "lowerarml", "lforearm"),
    "hand_l": ("lefthand", "handl", "lhand"),
    "finger_1_l": ("finger01l", "finger01proximall", "finger1l", "leftfinger01", "leftfinger1", "lfinger01"),
    "finger_1_tip_l": ("finger01distall", "finger1distall", "leftfinger01distal", "leftfinger1distal"),
    "finger_2_l": ("finger02l", "finger02proximall", "finger2l", "leftfinger02", "leftfinger2", "lfinger02"),
    "finger_2_tip_l": ("finger02distall", "finger2distall", "leftfinger02distal", "leftfinger2distal"),
    "finger_3_l": ("finger03l", "finger03proximall", "finger3l", "leftfinger03", "leftfinger3", "lfinger03"),
    "finger_3_tip_l": ("finger03distall", "finger3distall", "leftfinger03distal", "leftfinger3distal"),
    "shoulder_r": ("rightshoulder", "rightclavicle", "shoulderr", "clavicler", "rshoulder"),
    "upper_arm_r": ("rightupperarm", "rightarm", "upperarmr", "armr", "rupperarm"),
    "forearm_r": ("rightforearm", "rightlowerarm", "forearmr", "lowerarmr", "rforearm"),
    "hand_r": ("righthand", "handr", "rhand"),
    "finger_1_r": ("finger01r", "finger01proximalr", "finger1r", "rightfinger01", "rightfinger1", "rfinger01"),
    "finger_1_tip_r": ("finger01distalr", "finger1distalr", "rightfinger01distal", "rightfinger1distal"),
    "finger_2_r": ("finger02r", "finger02proximalr", "finger2r", "rightfinger02", "rightfinger2", "rfinger02"),
    "finger_2_tip_r": ("finger02distalr", "finger2distalr", "rightfinger02distal", "rightfinger2distal"),
    "finger_3_r": ("finger03r", "finger03proximalr", "finger3r", "rightfinger03", "rightfinger3", "rfinger03"),
    "finger_3_tip_r": ("finger03distalr", "finger3distalr", "rightfinger03distal", "rightfinger3distal"),
    "leg_l": ("leftupleg", "leftthigh", "uplegl", "thighl", "leftleg", "legl", "lthigh"),
    "shin_l": ("leftleg", "leftlowerleg", "leftshin", "leftcalf", "shinl", "lowerlegl"),
    "foot_l": ("leftfoot", "footl", "lfoot"),
    "leg_r": ("rightupleg", "rightthigh", "uplegr", "thighr", "rightleg", "legr", "rthigh"),
    "shin_r": ("rightleg", "rightlowerleg", "rightshin", "rightcalf", "shinr", "lowerlegr"),
    "foot_r": ("rightfoot", "footr", "rfoot"),
}

for side, side_words in (("l", ("left", "l")), ("r", ("right", "r"))):
    for digit in (1, 2, 3):
        ROLE_ALIASES[f"finger_{digit}_mid_{side}"] = (
            f"finger{digit}middle{side}",
            f"finger{digit:02d}middle{side}",
            f"finger_{digit:02d}_middle.{side}",
            *(f"{word}finger{digit}middle" for word in side_words),
        )

COMMON_PREFIXES = ("mixamorig", "armature", "skeleton", "rig", "jnt", "bone")


def normalize_bone_name(name: str) -> str:
    return re.sub(r"[^a-z0-9]+", "", str(name).lower())


def _variants(name: str) -> tuple[str, ...]:
    normalized = normalize_bone_name(name)
    variants = [normalized]
    for prefix in COMMON_PREFIXES:
        if normalized.startswith(prefix) and len(normalized) > len(prefix):
            variants.append(normalized[len(prefix) :])
    return tuple(dict.fromkeys(variants))


def _wrong_side(role: str, candidate: str) -> bool:
    if role.endswith("_l"):
        return "right" in candidate or (candidate.endswith("r") and "left" not in candidate)
    if role.endswith("_r"):
        return "left" in candidate or (candidate.endswith("l") and "right" not in candidate)
    return False


def _match_score(role: str, name: str) -> int:
    best = -1
    for candidate in _variants(name):
        if _wrong_side(role, candidate):
            continue
        for alias_index, alias in enumerate(ROLE_ALIASES[role]):
            if candidate == alias:
                score = 1000 - alias_index
            elif candidate.endswith(alias):
                score = 800 - alias_index
            elif alias in candidate:
                score = 500 - alias_index
            else:
                continue
            best = max(best, score)
    return best


def resolve_bone_roles(bone_names: Iterable[str]) -> dict[str, str]:
    names = [str(name) for name in bone_names]
    resolved: dict[str, str] = {}
    used: set[str] = set()
    for role in ROLE_ALIASES:
        candidates = sorted(
            ((_match_score(role, name), name) for name in names if name not in used),
            key=lambda item: item[0],
            reverse=True,
        )
        if candidates and candidates[0][0] >= 0:
            resolved[role] = candidates[0][1]
            used.add(candidates[0][1])
    return resolved


def has_presenter_controls(mapping: dict[str, str]) -> bool:
    arm_roles = sum(role in mapping for role in ("upper_arm_l", "forearm_l", "upper_arm_r", "forearm_r"))
    leg_roles = sum(role in mapping for role in ("leg_l", "leg_r"))
    return "head" in mapping and arm_roles >= 2 and arm_roles + leg_roles >= 4
