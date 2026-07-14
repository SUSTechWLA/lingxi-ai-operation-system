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
