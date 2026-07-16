#!/usr/bin/env python3
"""Shared Blender-independent contract for the warm studio scene."""

from __future__ import annotations

from pathlib import Path
from types import MappingProxyType
from typing import Tuple


ROOM_SIZE = (6.2, 5.8, 3.4)
DESK_SIZE = (2.7, 0.95, 0.92)
WINDOW_OPENING = (2.6, 2.75)
DOOR_OPENING = (1.0, 2.4)
TARGET_CHARACTER_HEIGHT = 2.55
DEFAULT_RENDER_RESOLUTION = (1920, 1080)
TRANSITION_FOCUS_LOCATION = (0.0, 0.38, 1.48)

PRESENTATION_MODES = ("standing", "seated")

MODE_MARKER_SPECS = MappingProxyType({
    "standing": MappingProxyType({
        "spawn": (0.0, 0.30, 0.0),
        "focus": (0.0, 0.30, 1.93),
        "seat": (0.0, 1.35, 0.56),
        "knee_l": (-0.20, 0.04, 0.54),
        "knee_r": (0.20, 0.04, 0.54),
        "foot_l": (-0.22, 0.00, 0.0),
        "foot_r": (0.22, 0.00, 0.0),
    }),
    "seated": MappingProxyType({
        "spawn": (0.0, 0.43, 0.0),
        "focus": (0.0, 0.43, 1.58),
        "seat": (0.0, 1.35, 0.56),
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

HERO_CHAIR_LOCATION = (0.0, 1.35, 0.0)
HERO_CHAIR_SEAT_SIZE = (1.06, 0.58, 0.12)
HERO_CHAIR_BACK_SIZE = (1.03, 0.10, 0.54)
HERO_CHAIR_ARM_X = 0.56
HERO_CHAIR_FOOT_X = 0.415

MODE_CAMERA_SPECS = MappingProxyType({
    "standing": MappingProxyType({
        "wide": ("Camera_Standing_Wide", (0.0, -2.74, 1.67), 24.0),
        "medium": ("Camera_Standing_Medium", (0.0, -2.15, 1.78), 50.0),
        "close": ("Camera_Standing_Close", (0.0, -1.82, 1.92), 70.0),
        "three_quarter": ("Camera_Standing_ThreeQuarter", (-2.25, -1.65, 1.82), 50.0),
        "transition": ("Camera_Standing_Transition", (-1.58, -2.42, 1.48), 20.0),
    }),
    "seated": MappingProxyType({
        "wide": ("Camera_Seated_Wide", (0.0, -2.74, 1.55), 24.0),
        "medium": ("Camera_Seated_Medium", (0.0, -2.15, 1.60), 50.0),
        "close": ("Camera_Seated_Close", (0.0, -1.82, 1.66), 70.0),
        "three_quarter": ("Camera_Seated_ThreeQuarter", (-2.20, -1.60, 1.62), 50.0),
        "transition": ("Camera_Seated_Transition", (-1.58, -2.42, 1.48), 20.0),
    }),
})

SUBJECT_LIGHT_PROFILE = MappingProxyType({
    "name": "bright_subject_first_v5",
    "worldStrength": 0.1,
    "keyTemperatureK": 6500,
    "keyEnergy": 1100.0,
    "keyTarget": (0.0, 0.365, 1.9),
    "keySpotSizeDegrees": 55.0,
    "keySpotBlend": 0.65,
    "keySoftRadius": 0.55,
    "fillEnergy": 110.0,
    "frontFillEnergy": 180.0,
    "frontFillTemperatureK": 5600,
    "frontFillTarget": (0.0, 0.365, 1.2),
    "rimEnergy": 180.0,
    "rimSpotSizeDegrees": 28.0,
    "rimSpotBlend": 0.65,
    "rimSoftRadius": 0.5,
    "windowEnergy": 50.0,
    "practicalWallEnergy": 28.0,
    "practicalShelfEnergy": 30.0,
    "downlightEnergy": 5.0,
    "rimTemperatureK": 3200,
    "practicalTemperatureK": 2700,
    "backgroundStopsBelowFace": 1.75,
    "backgroundStopsRange": (1.2, 2.2),
    "authoredExposure": -2.45,
    "cyclesFinalExposure": -3.7,
    "cyclesKeyEnergyMultiplier": 3.5,
    "whiteBalanceTemperatureK": 4500.0,
    "whiteBalanceTint": 10.0,
    "brightNeutralRedBlueRatio": (0.95, 1.22),
    "brightNeutralRedGreenRatio": (0.95, 1.14),
})

REQUIRED_COLLECTIONS = (
    "STUDIO_ARCHITECTURE",
    "STUDIO_FURNITURE",
    "STUDIO_PROPS",
    "STUDIO_VEGETATION",
    "STUDIO_BRAND",
    "STUDIO_LIGHTS",
    "STUDIO_CAMERAS",
    "STUDIO_MARKERS",
    "QA_ONLY",
)

REQUIRED_OBJECTS = (
    "SET_MASTER",
    "IP_Character_Spawn",
    "IP_Focus_Head",
    "IP_Focus_Desk",
    "IP_Focus_Shelf",
    "Camera_Wide",
    "Camera_Medium",
    "Camera_Close",
)

CAMERA_SPECS = MappingProxyType(
    {
        "Camera_Wide": MappingProxyType(
            {
                "location": (0.0, -2.74, 1.67),
                "target": (0.0, 0.45, 0.85),
                "lens": 18.0,
                "focus": "IP_Focus_Head",
            }
        ),
        "Camera_Medium": MappingProxyType(
            {
                "location": (0.0, -2.15, 1.78),
                "target": (0.0, 0.30, 1.85),
                "lens": 50.0,
                "focus": "IP_Focus_Head",
            }
        ),
        "Camera_Close": MappingProxyType(
            {
                "location": (0.08, -1.82, 1.92),
                "target": (0.0, 0.30, 1.93),
                "lens": 70.0,
                "focus": "IP_Focus_Head",
            }
        ),
        "Camera_ThreeQuarter_Left": MappingProxyType(
            {
                "location": (-2.25, -1.65, 1.82),
                "target": (0.0, 0.30, 1.72),
                "lens": 50.0,
                "focus": "IP_Focus_Head",
            }
        ),
        "Camera_ThreeQuarter_Right": MappingProxyType(
            {
                "location": (2.25, -1.65, 1.82),
                "target": (0.0, 0.30, 1.72),
                "lens": 50.0,
                "focus": "IP_Focus_Head",
            }
        ),
        "Camera_Desk_Detail": MappingProxyType(
            {
                "location": (1.95, -2.07, 1.68),
                "target": (0.55, -0.25, 0.98),
                "lens": 85.0,
                "focus": "IP_Focus_Desk",
            }
        ),
        "Camera_Shelf_Detail": MappingProxyType(
            {
                "location": (1.35, -0.35, 1.95),
                "target": (1.45, 2.50, 1.95),
                "lens": 85.0,
                "focus": "IP_Focus_Shelf",
            }
        ),
    }
)

PLANT_LINK_KEY = "WarmStudio_FloorPlant_LinkedData"


def repo_root() -> Path:
    """Return the repository root containing this contract module."""

    return Path(__file__).resolve().parents[2]


def floor_plant_transforms() -> Tuple[
    Tuple[float, float, float, float],
    Tuple[float, float, float, float],
]:
    """Return mirrored location and Z-rotation transforms for the floor plants."""

    return (
        (-2.15, 0.65, 0.0, 0.18),
        (2.15, 0.65, 0.0, -0.18),
    )
