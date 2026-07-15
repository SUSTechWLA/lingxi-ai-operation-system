#!/usr/bin/env python3
"""Pure fail-closed QA helpers for A-roll transitions and visemes."""

from __future__ import annotations

import math
from collections.abc import Iterable, Mapping, Sequence
from typing import Any


SCHEMA_VERSION = "tangying-aroll-performance-qa/v1"

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

VISEME_LIMITS = {
    "maxMbpAboveRestHeightFraction": 0.0015,
    "minAAboveMbpHeightFraction": 0.0060,
    "minOAboveMbpHeightFraction": 0.0048,
    "minUAboveMbpHeightFraction": 0.0036,
    "minSurpriseAboveAHeightFraction": 0.0010,
    "minEOWidthSeparationWidthFraction": 0.012,
    "minJawRadians": 0.20,
    "maxJawRadians": 0.25,
    "maxMbpJawRadians": 0.03,
}

TRANSITION_METRIC_NAMES = (
    "maxFootDriftL",
    "maxFootDriftR",
    "minKneeSeparation",
    "minSeatClearance",
    "maxSettledSeatClearance",
    "maxRootFrameDelta",
    "maxCentralSilhouetteSpike",
    "seatVisibleFraction",
)

VISEME_METRIC_NAMES = (
    "mbpGap",
    "restGap",
    "aGap",
    "eWidth",
    "oGap",
    "oWidth",
    "uGap",
    "surpriseGap",
    "maxJawRadians",
    "mbpJawRadians",
)

PHYSICAL_TRANSITION_STATES = {
    "Aroll_Transition_StandToSit": ("standing", "seated"),
    "Aroll_Transition_SitToStand": ("seated", "standing"),
}


def _finite_float(value: Any, label: str, errors: list[str]) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        errors.append(f"{label} must be a finite number")
        return None
    parsed = float(value)
    if not math.isfinite(parsed):
        errors.append(f"{label} must be a finite number")
        return None
    return parsed


def _required_metrics(
    metrics: Mapping[str, Any] | Any,
    names: Sequence[str],
) -> tuple[dict[str, float], list[str]]:
    if not isinstance(metrics, Mapping):
        return {}, ["metrics must be an object"]
    parsed: dict[str, float] = {}
    errors: list[str] = []
    for name in names:
        if name not in metrics:
            errors.append(f"missing required metric: {name}")
            continue
        value = _finite_float(metrics.get(name), name, errors)
        if value is not None:
            parsed[name] = value
    return parsed, errors


def validate_transition_metrics(metrics: Mapping[str, Any] | Any) -> dict[str, object]:
    """Apply every approved transition threshold without raising on bad evidence."""

    parsed, errors = _required_metrics(metrics, TRANSITION_METRIC_NAMES)
    if not errors:
        if max(parsed["maxFootDriftL"], parsed["maxFootDriftR"]) > TRANSITION_LIMITS["maxFootDrift"]:
            errors.append("foot lock drift exceeds 0.025 m")
        if parsed["minKneeSeparation"] < TRANSITION_LIMITS["minKneeSeparation"]:
            errors.append("knee separation is below 0.075 m")
        if parsed["minSeatClearance"] < TRANSITION_LIMITS["minSeatClearance"]:
            errors.append("pelvis penetrates the seat")
        if parsed["maxSettledSeatClearance"] > TRANSITION_LIMITS["maxSettledSeatClearance"]:
            errors.append("pelvis does not settle onto the seat")
        if parsed["maxRootFrameDelta"] > TRANSITION_LIMITS["maxRootFrameDelta"]:
            errors.append("root motion has a visible frame discontinuity")
        if parsed["maxCentralSilhouetteSpike"] > TRANSITION_LIMITS["maxCentralSilhouetteSpike"]:
            errors.append("central silhouette spike exceeds 0.045 m")
        visible = parsed["seatVisibleFraction"]
        if not (
            TRANSITION_LIMITS["minSeatVisibleFraction"]
            <= visible
            <= TRANSITION_LIMITS["maxSeatVisibleFraction"]
        ):
            errors.append("seat visibility is outside the approved range")
    success = not errors
    return {
        "status": "passed" if success else "failed",
        "success": success,
        "errors": errors,
        "metrics": parsed,
        "limits": dict(TRANSITION_LIMITS),
    }


def validate_viseme_metrics(
    metrics: Mapping[str, Any] | Any,
    *,
    character_height: Any,
    character_width: Any,
) -> dict[str, object]:
    """Validate the exact source-mouth separation and jaw bounds from Task 6."""

    parsed, errors = _required_metrics(metrics, VISEME_METRIC_NAMES)
    height = _finite_float(character_height, "character_height", errors)
    width = _finite_float(character_width, "character_width", errors)
    if height is not None and height <= 0.0:
        errors.append("character_height must be greater than zero")
    if width is not None and width <= 0.0:
        errors.append("character_width must be greater than zero")

    for name in ("mbpGap", "restGap", "aGap", "eWidth", "oGap", "oWidth", "uGap", "surpriseGap"):
        if name in parsed and parsed[name] < 0.0:
            errors.append(f"{name} must not be negative")

    required_values_ready = (
        not any(name not in parsed for name in VISEME_METRIC_NAMES)
        and height is not None
        and height > 0.0
        and width is not None
        and width > 0.0
    )
    if required_values_ready:
        assert height is not None and width is not None
        if parsed["mbpGap"] > parsed["restGap"] + height * VISEME_LIMITS["maxMbpAboveRestHeightFraction"]:
            errors.append("MBP gap is not closed relative to rest")
        if parsed["aGap"] < parsed["mbpGap"] + height * VISEME_LIMITS["minAAboveMbpHeightFraction"]:
            errors.append("A gap separation is below the Task 6 bound")
        if parsed["oGap"] < parsed["mbpGap"] + height * VISEME_LIMITS["minOAboveMbpHeightFraction"]:
            errors.append("O gap separation is below the Task 6 bound")
        if parsed["uGap"] < parsed["mbpGap"] + height * VISEME_LIMITS["minUAboveMbpHeightFraction"]:
            errors.append("U gap separation is below the Task 6 bound")
        if parsed["surpriseGap"] < parsed["aGap"] + height * VISEME_LIMITS["minSurpriseAboveAHeightFraction"]:
            errors.append("surprise gap separation is below the Task 6 bound")
        if abs(parsed["eWidth"] - parsed["oWidth"]) < width * VISEME_LIMITS["minEOWidthSeparationWidthFraction"]:
            errors.append("E/O width separation is below the Task 6 bound")
        if not (
            VISEME_LIMITS["minJawRadians"]
            <= parsed["maxJawRadians"]
            <= VISEME_LIMITS["maxJawRadians"]
        ):
            errors.append("maximum jaw rotation is outside 0.20..0.25 rad")
        if parsed["mbpJawRadians"] > VISEME_LIMITS["maxMbpJawRadians"]:
            errors.append("MBP jaw rotation exceeds 0.03 rad")

    success = not errors
    return {
        "status": "passed" if success else "failed",
        "success": success,
        "errors": errors,
        "metrics": parsed,
        "characterDimensions": {
            "height": height,
            "width": width,
        },
        "limits": dict(VISEME_LIMITS),
    }


def not_applicable_transition_report() -> dict[str, object]:
    """Represent a standalone render without inventing transition measurements."""

    return {
        "status": "not_applicable",
        "success": None,
        "errors": [],
        "metrics": {},
        "limits": dict(TRANSITION_LIMITS),
        "evidence": {"reason": "render has no A-roll transition actions"},
    }


def physical_transition_events(events: Iterable[Any] | Any) -> list[Mapping[str, Any]]:
    """Return only correctly state-changing stand/sit transition events."""

    try:
        items = list(events)
    except TypeError:
        return []
    physical: list[Mapping[str, Any]] = []
    for event in items:
        if not isinstance(event, Mapping) or event.get("motion") != "avatar_action":
            continue
        if str(event.get("action") or "") not in PHYSICAL_TRANSITION_STATES:
            continue
        start_state = str(event.get("startState") or "")
        end_state = str(event.get("endState") or "")
        if start_state in {"standing", "seated"} and end_state in {
            "standing",
            "seated",
        } and start_state != end_state:
            physical.append(event)
    return physical


def summarize_jaw_samples(samples: Iterable[Mapping[str, Any]] | Any) -> dict[str, object]:
    """Summarize evaluated jaw rotations without consulting response constants."""

    errors: list[str] = []
    try:
        items = list(samples)
    except TypeError:
        items = []
        errors.append("jaw samples must be iterable")
    jaw_values: list[float] = []
    mbp_values: list[float] = []
    sampled_frames: list[int] = []
    for index, item in enumerate(items):
        if not isinstance(item, Mapping):
            errors.append(f"jaw sample {index} must be an object")
            continue
        frame = item.get("frame")
        if isinstance(frame, bool) or not isinstance(frame, int):
            errors.append(f"jaw sample {index} frame must be an integer")
        else:
            sampled_frames.append(frame)
        viseme = item.get("viseme")
        if not isinstance(viseme, str) or not viseme:
            errors.append(f"jaw sample {index} viseme must be a non-empty string")
        jaw = _finite_float(item.get("jawRadians"), f"jaw sample {index} jawRadians", errors)
        if jaw is None:
            continue
        if jaw < 0.0:
            errors.append(f"jaw sample {index} jawRadians must not be negative")
            continue
        jaw_values.append(jaw)
        if viseme == "mbp":
            mbp_values.append(jaw)
    if not jaw_values:
        errors.append("evaluated jaw timeline is empty")
    if not mbp_values:
        errors.append("evaluated jaw timeline has no MBP samples")
    return {
        "success": not errors,
        "errors": errors,
        "maxJawRadians": max(jaw_values) if jaw_values else None,
        "mbpJawRadians": max(mbp_values) if mbp_values else None,
        "sampleCount": len(jaw_values),
        "mbpSampleCount": len(mbp_values),
        "sampledFrames": sampled_frames,
    }


def detect_central_silhouette_spike(
    projected_vertices: Iterable[Sequence[Any]] | Any,
    *,
    column_count: int = 64,
    normalize_subject_x: bool = False,
) -> dict[str, object]:
    """Measure a continuous center-depth spike from normalized X/depth samples."""

    errors: list[str] = []
    if isinstance(column_count, bool) or column_count != 64:
        errors.append("central silhouette evidence must use exactly 64 columns")
    closest_depth: dict[int, float] = {}
    sample_count = 0
    parsed_samples: list[tuple[float, float]] = []
    subject_min_x = None
    subject_max_x = None
    if not errors:
        try:
            samples = list(projected_vertices)
        except TypeError:
            samples = []
            errors.append("projected vertices must be iterable")
        for index, sample in enumerate(samples):
            if not isinstance(sample, Sequence) or len(sample) < 2:
                errors.append(f"projected vertex {index} must contain X and depth")
                continue
            item_errors: list[str] = []
            x = _finite_float(sample[0], f"projected vertex {index} X", item_errors)
            depth = _finite_float(sample[1], f"projected vertex {index} depth", item_errors)
            errors.extend(item_errors)
            if x is None or depth is None:
                continue
            if depth <= 0.0 or (not normalize_subject_x and not 0.0 <= x <= 1.0):
                continue
            parsed_samples.append((x, depth))

        if normalize_subject_x and parsed_samples:
            subject_min_x = min(item[0] for item in parsed_samples)
            subject_max_x = max(item[0] for item in parsed_samples)
            subject_span = subject_max_x - subject_min_x
            if subject_span <= 1e-9:
                errors.append("central silhouette subject X span must be greater than zero")
            else:
                parsed_samples = [
                    ((x - subject_min_x) / subject_span, depth)
                    for x, depth in parsed_samples
                ]
        elif parsed_samples:
            subject_min_x = min(item[0] for item in parsed_samples)
            subject_max_x = max(item[0] for item in parsed_samples)

        for x, depth in parsed_samples:
            if not 0.0 <= x <= 1.0:
                continue
            column = min(column_count - 1, int(x * column_count))
            closest_depth[column] = min(depth, closest_depth.get(column, depth))
            sample_count += 1

    center_columns = [30, 31, 32, 33]
    flank_columns = [29, 34]
    required_columns = center_columns + flank_columns
    missing = [column for column in required_columns if column not in closest_depth]
    if missing:
        errors.append(f"central silhouette evidence is missing columns: {missing}")

    spike = None
    center_depth = None
    center_conservative_depth = None
    flank_mean = None
    flank_nearest = None
    if not errors:
        center_depth = min(closest_depth[column] for column in center_columns)
        center_conservative_depth = max(
            closest_depth[column] for column in center_columns
        )
        flank_mean = sum(closest_depth[column] for column in flank_columns) / len(flank_columns)
        flank_nearest = min(closest_depth[column] for column in flank_columns)
        spike = max(0.0, flank_nearest - center_conservative_depth)
    return {
        "success": not errors,
        "errors": errors,
        "columnCount": column_count,
        "subjectXNormalized": bool(normalize_subject_x),
        "subjectMinX": subject_min_x,
        "subjectMaxX": subject_max_x,
        "sampleCount": sample_count,
        "centerColumns": center_columns,
        "flankColumns": flank_columns,
        "centerClosestDepth": center_depth,
        "centerConservativeDepth": center_conservative_depth,
        "flankMeanClosestDepth": flank_mean,
        "flankNearestDepth": flank_nearest,
        "spikeMeters": spike,
        "closestDepthByColumn": {
            str(column): closest_depth[column]
            for column in sorted(closest_depth)
        },
    }


def _parse_triangle(
    triangle: Any,
    label: str,
    index: int,
    errors: list[str],
) -> tuple[tuple[float, float, float], ...] | None:
    if not isinstance(triangle, Sequence) or len(triangle) != 3:
        errors.append(f"{label} triangle {index} must have exactly three vertices")
        return None
    parsed = []
    for vertex_index, vertex in enumerate(triangle):
        if not isinstance(vertex, Sequence) or len(vertex) < 3:
            errors.append(
                f"{label} triangle {index} vertex {vertex_index} must contain X, Y, and depth"
            )
            return None
        values: list[float] = []
        for axis, value in zip(("X", "Y", "depth"), vertex[:3]):
            parsed_value = _finite_float(
                value,
                f"{label} triangle {index} vertex {vertex_index} {axis}",
                errors,
            )
            if parsed_value is None:
                return None
            values.append(parsed_value)
        parsed.append(tuple(values))
        if values[2] <= 0.0:
            errors.append(
                f"{label} triangle {index} vertex {vertex_index} depth must be greater than zero"
            )
            return None
    return tuple(parsed)


def rasterize_seat_visibility(
    character_triangles: Iterable[Sequence[Sequence[Any]]] | Any,
    seat_triangles: Iterable[Sequence[Sequence[Any]]] | Any,
    *,
    width: int = 96,
    height: int = 54,
) -> dict[str, object]:
    """Rasterize a deterministic character/chair depth mask and count visible seat pixels."""

    errors: list[str] = []
    if (
        isinstance(width, bool)
        or isinstance(height, bool)
        or not isinstance(width, int)
        or not isinstance(height, int)
        or width <= 0
        or height <= 0
    ):
        return {
            "success": False,
            "errors": ["mask width and height must be positive integers"],
            "maskWidth": width,
            "maskHeight": height,
            "foregroundPixelCount": 0,
            "seatVisiblePixelCount": 0,
            "seatVisibleFraction": None,
            "minimumVisibleDepthMeters": None,
        }

    triangle_sets: list[tuple[str, list[tuple[tuple[float, float, float], ...]]]] = []
    for label, source in (("character", character_triangles), ("seat", seat_triangles)):
        try:
            raw_triangles = list(source)
        except TypeError:
            raw_triangles = []
            errors.append(f"{label} triangles must be iterable")
        parsed_triangles = []
        for index, triangle in enumerate(raw_triangles):
            parsed = _parse_triangle(triangle, label, index, errors)
            if parsed is not None:
                parsed_triangles.append(parsed)
        if not parsed_triangles:
            errors.append(f"{label} mask has no triangles")
        triangle_sets.append((label, parsed_triangles))

    if errors:
        return {
            "success": False,
            "errors": errors,
            "maskWidth": width,
            "maskHeight": height,
            "foregroundPixelCount": 0,
            "seatVisiblePixelCount": 0,
            "seatVisibleFraction": None,
            "minimumVisibleDepthMeters": None,
        }

    depths = [math.inf] * (width * height)
    labels = [""] * (width * height)
    epsilon = 1e-12
    for label, triangles in triangle_sets:
        for triangle in triangles:
            (x0, y0, z0), (x1, y1, z1), (x2, y2, z2) = triangle
            denominator = (y1 - y2) * (x0 - x2) + (x2 - x1) * (y0 - y2)
            if abs(denominator) <= epsilon:
                continue
            min_x = max(0, math.floor(min(x0, x1, x2) * width))
            max_x = min(width - 1, math.ceil(max(x0, x1, x2) * width) - 1)
            min_y = max(0, math.floor(min(y0, y1, y2) * height))
            max_y = min(height - 1, math.ceil(max(y0, y1, y2) * height) - 1)
            if min_x > max_x or min_y > max_y:
                continue
            for pixel_y in range(min_y, max_y + 1):
                sample_y = (pixel_y + 0.5) / height
                for pixel_x in range(min_x, max_x + 1):
                    sample_x = (pixel_x + 0.5) / width
                    a = (
                        (y1 - y2) * (sample_x - x2)
                        + (x2 - x1) * (sample_y - y2)
                    ) / denominator
                    b = (
                        (y2 - y0) * (sample_x - x2)
                        + (x0 - x2) * (sample_y - y2)
                    ) / denominator
                    c = 1.0 - a - b
                    if min(a, b, c) < -1e-10:
                        continue
                    reciprocal_depth = a / z0 + b / z1 + c / z2
                    if not math.isfinite(reciprocal_depth) or reciprocal_depth <= 0.0:
                        errors.append("perspective depth interpolation is invalid")
                        continue
                    depth = 1.0 / reciprocal_depth
                    offset = pixel_y * width + pixel_x
                    if depth < depths[offset]:
                        depths[offset] = depth
                        labels[offset] = label

    foreground = sum(bool(label) for label in labels)
    visible_seat = labels.count("seat")
    if foreground <= 0:
        errors.append("character-plus-chair foreground mask is empty")
    fraction = visible_seat / foreground if foreground else None
    finite_depths = [depth for depth in depths if math.isfinite(depth)]
    return {
        "success": not errors,
        "errors": errors,
        "maskWidth": width,
        "maskHeight": height,
        "foregroundPixelCount": foreground,
        "seatVisiblePixelCount": visible_seat,
        "seatVisibleFraction": fraction,
        "minimumVisibleDepthMeters": min(finite_depths) if finite_depths else None,
    }
