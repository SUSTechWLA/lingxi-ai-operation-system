# Warm Sloth Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and render-verify a detailed, empty, 360-degree warm sloth talking-head studio in Blender 5.1.2 that matches `工作室设计版.png` and preserves the existing A-roll scene contract.

**Architecture:** A pure-Python contract module owns dimensions, required names, camera specifications, and symmetry rules. A Blender builder consumes that contract through staged architecture, furniture, prop, vegetation, brand, lighting, and camera functions; a separate validator and QA renderer produce machine-readable checks and render evidence. The new scene is additive and never modifies the character profile or existing dark editorial studio.

**Tech Stack:** Python 3.11, Blender 5.1.2 Python 3.13 / `bpy`, Eevee Next, Cycles, FFmpeg, `unittest`, OpenAI image generation for the brand icon source.

---

## File Map

- Create `mcp/ip_avatar_3d/warm_studio_contract.py`: Blender-independent scene dimensions, names, camera specs, plant symmetry, and output paths.
- Create `mcp/ip_avatar_3d/test_warm_studio_contract.py`: fast unit tests for the pure contract.
- Create `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`: staged Blender scene construction and command-line entry point.
- Modify `mcp/ip_avatar_3d/test_blender_scene_contract.py`: Blender-side geometry, material, symmetry, camera, and lighting contract tests.
- Create `mcp/ip_avatar_3d/validate_warm_studio.py`: Blender-side structural/material/resource validator that writes JSON.
- Create `mcp/ip_avatar_3d/render_warm_studio_qa.py`: deterministic multi-camera Eevee/Cycles still renderer.
- Create `ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png`: generated scene-specific brand icon source.
- Create `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`: packed reusable empty scene.
- Create `ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png`: hero preview.
- Create `outputs/warm_sloth_studio_v1/qa/`: validation JSON, clay render, camera stills, engine comparison, and contact sheet.
- Modify `mcp/ip_avatar_3d/README.md`: document the new optional scene without changing the active character profile.

## Fixed Commands

Use these exact environment values throughout:

```bash
export BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
export STUDIO_BLEND="$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend"
export STUDIO_PREVIEW="$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png"
export STUDIO_QA="$PWD/outputs/warm_sloth_studio_v1/qa"
```

### Task 1: Add the Blender-Independent Studio Contract

**Files:**
- Create: `mcp/ip_avatar_3d/warm_studio_contract.py`
- Create: `mcp/ip_avatar_3d/test_warm_studio_contract.py`

- [ ] **Step 1: Write failing contract tests**

Create tests that lock the approved room, desk, opening, camera, marker, and mirrored-plant values:

```python
#!/usr/bin/env python3
from __future__ import annotations

import unittest

import warm_studio_contract as contract


class WarmStudioContractTests(unittest.TestCase):
    def test_approved_metric_dimensions_are_stable(self) -> None:
        self.assertEqual(contract.ROOM_SIZE, (6.2, 5.8, 3.4))
        self.assertEqual(contract.DESK_SIZE, (2.7, 0.95, 0.92))
        self.assertEqual(contract.WINDOW_OPENING, (2.6, 2.75))
        self.assertEqual(contract.DOOR_OPENING, (1.0, 2.4))
        self.assertEqual(contract.TARGET_CHARACTER_HEIGHT, 2.55)

    def test_standard_aroll_names_are_present(self) -> None:
        self.assertTrue({"Camera_Wide", "Camera_Medium", "Camera_Close"}.issubset(contract.CAMERA_SPECS))
        self.assertTrue({"IP_Character_Spawn", "IP_Focus_Head"}.issubset(contract.REQUIRED_OBJECTS))
        self.assertIn("STUDIO_VEGETATION", contract.REQUIRED_COLLECTIONS)

    def test_floor_plants_are_strictly_mirrored(self) -> None:
        left, right = contract.floor_plant_transforms()
        self.assertEqual(left[0], -right[0])
        self.assertEqual(left[1:3], right[1:3])
        self.assertEqual(left[3], -right[3])
        self.assertEqual(contract.PLANT_LINK_KEY, "WarmStudio_FloorPlant_LinkedData")

    def test_camera_focal_lengths_match_approved_design(self) -> None:
        self.assertEqual(contract.CAMERA_SPECS["Camera_Wide"]["lens"], 32.0)
        self.assertEqual(contract.CAMERA_SPECS["Camera_Medium"]["lens"], 50.0)
        self.assertEqual(contract.CAMERA_SPECS["Camera_Close"]["lens"], 70.0)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the tests and verify the missing-module failure**

Run:

```bash
PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest mcp/ip_avatar_3d/test_warm_studio_contract.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'warm_studio_contract'`.

- [ ] **Step 3: Implement the contract module**

Define immutable constants and a deterministic mirrored transform helper:

```python
#!/usr/bin/env python3
from __future__ import annotations

from pathlib import Path

ROOM_SIZE = (6.2, 5.8, 3.4)
DESK_SIZE = (2.7, 0.95, 0.92)
WINDOW_OPENING = (2.6, 2.75)
DOOR_OPENING = (1.0, 2.4)
TARGET_CHARACTER_HEIGHT = 2.55
PLANT_LINK_KEY = "WarmStudio_FloorPlant_LinkedData"

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

CAMERA_SPECS = {
    "Camera_Wide": {"location": (0.0, -2.55, 1.62), "target": (0.0, 0.35, 1.48), "lens": 32.0, "focus": "IP_Focus_Head"},
    "Camera_Medium": {"location": (0.0, -2.15, 1.78), "target": (0.0, 0.30, 1.85), "lens": 50.0, "focus": "IP_Focus_Head"},
    "Camera_Close": {"location": (0.08, -1.82, 1.92), "target": (0.0, 0.30, 1.93), "lens": 70.0, "focus": "IP_Focus_Head"},
    "Camera_ThreeQuarter_Left": {"location": (-2.25, -1.65, 1.82), "target": (0.0, 0.30, 1.72), "lens": 50.0, "focus": "IP_Focus_Head"},
    "Camera_ThreeQuarter_Right": {"location": (2.25, -1.65, 1.82), "target": (0.0, 0.30, 1.72), "lens": 50.0, "focus": "IP_Focus_Head"},
    "Camera_Desk_Detail": {"location": (1.55, -1.55, 1.48), "target": (0.55, -0.25, 0.98), "lens": 85.0, "focus": "IP_Focus_Desk"},
    "Camera_Shelf_Detail": {"location": (1.35, -0.35, 1.95), "target": (1.45, 2.50, 1.95), "lens": 85.0, "focus": "IP_Focus_Shelf"},
}

def floor_plant_transforms() -> tuple[tuple[float, float, float, float], tuple[float, float, float, float]]:
    return ((-2.35, 0.55, 0.0, 0.18), (2.35, 0.55, 0.0, -0.18))

def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]
```

- [ ] **Step 4: Run the pure tests and verify they pass**

Run the Step 2 command again. Expected: 4 tests PASS.

- [ ] **Step 5: Commit the contract only**

```bash
git add mcp/ip_avatar_3d/warm_studio_contract.py mcp/ip_avatar_3d/test_warm_studio_contract.py
git commit -m "test: define warm studio contract" --only -- mcp/ip_avatar_3d/warm_studio_contract.py mcp/ip_avatar_3d/test_warm_studio_contract.py
```

### Task 2: Build the Scene Foundation and Complete Architecture

**Files:**
- Create: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`

- [ ] **Step 1: Add a failing Blender architecture test**

Add `import warm_sloth_studio_builder` and this test to `test_blender_scene_contract.py`:

```python
def test_warm_studio_architecture_is_a_complete_metric_room() -> None:
    context = warm_sloth_studio_builder.create_scene_context()
    warm_sloth_studio_builder.build_architecture(context)

    assert bpy.context.scene.unit_settings.system == "METRIC"
    assert tuple(round(value, 3) for value in bpy.data.objects["Studio_InteriorVolume"].dimensions) == (6.2, 5.8, 3.4)
    for name in (
        "Wall_Back",
        "Wall_Front_Left",
        "Wall_Front_Right",
        "Wall_Left_WindowLower",
        "Wall_Left_WindowUpper",
        "Wall_Right_DoorRear",
        "Wall_Right_DoorFront",
        "Studio_Floor",
        "Studio_Ceiling",
        "Window_Frame",
        "Door_Leaf",
    ):
        assert bpy.data.objects.get(name) is not None, name
    assert round(bpy.data.objects["Wall_Back"].dimensions.x, 3) == 6.2
    assert bpy.data.objects["Window_Frame"]["clear_width"] == 2.6
    assert bpy.data.objects["Door_Leaf"]["clear_height"] == 2.4
```

Append the test to the manual `tests` list at the bottom of the file.

- [ ] **Step 2: Run the Blender test and verify the missing-builder failure**

```bash
$BLENDER_BIN --background --factory-startup --python mcp/ip_avatar_3d/test_blender_scene_contract.py
```

Expected: FAIL while importing `warm_sloth_studio_builder`.

- [ ] **Step 3: Implement foundation helpers and staged context**

Create `warm_sloth_studio_builder.py` with these public boundaries:

```python
@dataclass(frozen=True)
class StudioContext:
    scene: bpy.types.Scene
    master: bpy.types.Object
    collections: dict[str, bpy.types.Collection]
    materials: dict[str, bpy.types.Material]

def create_scene_context() -> StudioContext:
    bpy.ops.wm.read_factory_settings(use_empty=True)
    scene = bpy.context.scene
    scene.name = "Scene_Warm_Sloth_Studio"
    scene.unit_settings.system = "METRIC"
    scene.unit_settings.length_unit = "METERS"
    scene["ip_scene_contract"] = "tangying-warm-sloth-studio/v1"
    scene["ip_authored_exposure"] = 0.0
    collections = {name: ensure_collection(name) for name in contract.REQUIRED_COLLECTIONS}
    master = bpy.data.objects.new("SET_MASTER", None)
    collections["STUDIO_MARKERS"].objects.link(master)
    master["room_size"] = json.dumps(contract.ROOM_SIZE)
    return StudioContext(scene, master, collections, build_material_library())
```

Implement reusable `ensure_collection`, `move_to_collection`, `add_box`, `add_cylinder`, `add_curve_profile`, `add_bevel`, `parent_to_master`, and `look_at` helpers. Every mesh helper applies scale after dimensions are assigned and parents the object to `SET_MASTER` while preserving world transform.

At this stage `build_material_library()` must return at least `Wall_WarmPlaster`, `Floor_LightOak`, `Trim_WarmWhite`, `Glass_Window`, `Metal_DarkBrown`, `Curtain_Sheer`, and `Curtain_Outer` using simple Principled node graphs. Task 3 enriches those same named materials without changing their callers.

- [ ] **Step 4: Implement the complete room shell**

`build_architecture(context)` must create:

```python
def build_architecture(ctx: StudioContext) -> None:
    width, depth, height = contract.ROOM_SIZE
    add_reference_volume("Studio_InteriorVolume", (0, 0, height / 2), contract.ROOM_SIZE, ctx)
    add_box("Studio_Floor", (0, 0, -0.06), (width + 0.36, depth + 0.36, 0.12), "Floor_Oak", ctx, "STUDIO_ARCHITECTURE")
    add_box("Studio_Ceiling", (0, 0, height + 0.06), (width + 0.36, depth + 0.36, 0.12), "Wall_Plaster", ctx, "STUDIO_ARCHITECTURE")
    build_back_wall(ctx)
    build_front_wall(ctx)
    build_left_window_wall(ctx, clear_width=2.6, clear_height=2.75)
    build_right_door_wall(ctx, clear_width=1.0, clear_height=2.4)
    build_baseboards_and_crown(ctx)
    build_floorboards(ctx, board_width=0.18, gap=0.004)
    build_window_frames_and_curtains(ctx)
    build_door_leaf_and_hardware(ctx)
```

Model wall segments around the openings instead of leaving a flat wall behind them. Mark `Studio_InteriorVolume.hide_render = True` and put it in `QA_ONLY`.

- [ ] **Step 5: Run the Blender contract test**

Run the Step 2 command. Expected: architecture test PASS and all pre-existing scene-contract tests PASS.

- [ ] **Step 6: Commit the architecture stage**

```bash
git add mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
git commit -m "feat: build complete warm studio room" --only -- mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
```

### Task 3: Add Detailed Materials, Furniture, and Set Dressing

**Files:**
- Modify: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`

- [ ] **Step 1: Add failing furniture and material assertions**

```python
def test_warm_studio_furniture_matches_reference_layout() -> None:
    context = warm_sloth_studio_builder.create_scene_context()
    warm_sloth_studio_builder.build_architecture(context)
    objects = warm_sloth_studio_builder.build_furniture(context)
    warm_sloth_studio_builder.build_set_dressing(context)
    desk = bpy.data.objects["Desk_Top"]
    assert tuple(round(value, 3) for value in objects["desk_dimensions"]) == (2.7, 0.95, 0.92)
    assert abs(desk.location.x) < 1e-6
    assert bpy.data.objects.get("Chair_Main") is not None
    assert bpy.data.objects["Chair_Main"].location.y > desk.location.y
    assert bpy.data.objects.get("Cabinet_Left") is not None
    assert bpy.data.objects.get("SlatWall_Back_Right") is not None
    assert len([obj for obj in bpy.data.objects if obj.name.startswith("Slat_Back_")]) >= 24
    assert len([obj for obj in bpy.data.objects if obj.name.startswith("Shelf_Right_")]) == 3
    assert len([obj for obj in bpy.data.objects if obj.name.startswith("Cabinet_Drawer_")]) == 6
    for material in ("Desk_WarmOak", "Slat_Walnut", "Floor_LightOak", "Wall_WarmPlaster", "Metal_DarkBrown", "Rug_Jute"):
        assert bpy.data.materials.get(material) is not None, material
```

- [ ] **Step 2: Verify the furniture test fails**

Run the Blender command from Task 2. Expected: FAIL on the missing furniture functions or objects.

- [ ] **Step 3: Implement the procedural material library**

Implement `build_material_library()` with Principled materials and reusable node groups:

```python
def build_material_library() -> dict[str, bpy.types.Material]:
    return {
        "Desk_WarmOak": wood_material("Desk_WarmOak", (0.39, 0.20, 0.09, 1), roughness=0.42, scale=4.2),
        "Slat_Walnut": wood_material("Slat_Walnut", (0.18, 0.075, 0.032, 1), roughness=0.47, scale=5.6),
        "Floor_LightOak": wood_material("Floor_LightOak", (0.55, 0.31, 0.15, 1), roughness=0.50, scale=3.4),
        "Wall_WarmPlaster": plaster_material("Wall_WarmPlaster", (0.72, 0.61, 0.47, 1), roughness=0.72),
        "Curtain_Sheer": fabric_material("Curtain_Sheer", (0.86, 0.81, 0.72, 1), roughness=0.78, transmission=0.22),
        "Curtain_Outer": fabric_material("Curtain_Outer", (0.73, 0.65, 0.55, 1), roughness=0.86, transmission=0.0),
        "Metal_DarkBrown": principled_material("Metal_DarkBrown", (0.045, 0.028, 0.020, 1), roughness=0.31, metallic=0.76),
        "Ceramic_Sand": ceramic_material("Ceramic_Sand", (0.58, 0.47, 0.34, 1), roughness=0.58),
        "Rug_Jute": woven_material("Rug_Jute", (0.52, 0.38, 0.22, 1), roughness=0.88),
    }
```

Use Generated/Object coordinates and Mapping nodes so the material does not depend on external image textures. Store `grain_axis` as a custom property on wood objects and rotate mapping per object.

- [ ] **Step 4: Build the reference furniture and props**

Add these staged functions:

```python
def build_furniture(ctx: StudioContext) -> dict[str, object]:
    desk = build_main_desk(ctx, size=contract.DESK_SIZE, location=(0.0, -0.45, 0.0))
    build_hidden_hero_chair(ctx, location=(0.0, 0.40, 0.0))
    build_left_six_drawer_cabinet(ctx, location=(-1.72, 2.50, 0.0))
    build_right_slat_wall(ctx, x_range=(0.35, 2.88), back_y=2.82, slat_pitch=0.095)
    build_three_floating_shelves(ctx, center=(1.66, 2.62, 1.75))
    build_oval_rug(ctx, center=(0.0, -0.10, 0.012), radii=(1.78, 1.20))
    return {"desk_dimensions": contract.DESK_SIZE, "desk": desk}

def build_set_dressing(ctx: StudioContext) -> None:
    build_tabletop_props(ctx)
    build_cabinet_props(ctx)
    build_shelf_prop_clusters(ctx)
    build_right_wall_portrait(ctx)
```

Books use at least five linked source meshes with controlled height and cover-color variation. Lamps include shade, base, stem, visible bulb diffuser, and separate emissive material. The rug gets a beveled ellipse mesh and sparse curve fibers only around its edge.

- [ ] **Step 5: Run the Blender contract test**

Expected: all existing and new tests PASS.

- [ ] **Step 6: Commit the material and furniture stage**

```bash
git add mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
git commit -m "feat: furnish warm sloth studio" --only -- mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
```

### Task 4: Create the Scene-Specific Brand Artwork and Symmetric Plants

**Files:**
- Create: `ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png`
- Modify: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`

- [ ] **Step 1: Generate the icon source with the image generation skill**

Use `frontend/public/躺营ai视频创作助手.png` as the referenced image and generate one 2048×2048 transparent PNG with this exact art direction:

```text
Create a clean single-color brand icon derived from the supplied Tangying AI sloth app icon. Keep the recognizable calm sloth face, closed eyes, small smile, and a subtle play-button / speech-bubble motif. Use one deep warm-brown color only, no gradients, no shadows, no background, no border, no Chinese or English text. Make it suitable for a framed Scandinavian warm-wood studio poster and easy to read from a distance. Centered, symmetric, bold simple shapes, transparent background.
```

Save the accepted output as `ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png`. Inspect it before use and reject outputs with text, a non-transparent background, asymmetric facial marks, or thin details that disappear in the hero view.

- [ ] **Step 2: Add failing plant-link and brand-copy tests**

Add `import warm_studio_contract` beside the warm-studio builder import before adding this test.

```python
def test_warm_studio_brand_and_floor_plants_are_production_assets() -> None:
    context = warm_sloth_studio_builder.create_scene_context()
    warm_sloth_studio_builder.build_architecture(context)
    warm_sloth_studio_builder.build_furniture(context)
    warm_sloth_studio_builder.build_set_dressing(context)
    warm_sloth_studio_builder.build_symmetric_floor_plants(context)
    warm_sloth_studio_builder.build_brand_art(
        context,
        warm_studio_contract.repo_root() / "ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png",
    )
    left = bpy.data.objects["FloorPlant_Left"]
    right = bpy.data.objects["FloorPlant_Right"]
    assert left.data is right.data
    assert round(left.location.x, 4) == -round(right.location.x, 4)
    assert round(left.location.y, 4) == round(right.location.y, 4)
    assert round(left.rotation_euler.z, 4) == -round(right.rotation_euler.z, 4)
    assert tuple(round(value, 4) for value in left.dimensions) == tuple(round(value, 4) for value in right.dimensions)
    assert bpy.data.objects["Brand_Copy_Line1"].data.body == "Slow Down."
    assert bpy.data.objects["Brand_Copy_Line2"].data.body == "Think Better."
    icon_image = bpy.data.images.get("warm-sloth-brand-icon.png")
    assert icon_image is not None
    assert icon_image.packed_file is not None
```

- [ ] **Step 3: Run the Blender test and verify it fails**

Run the standard Blender contract command. Expected: FAIL on missing floor plants or brand objects.

- [ ] **Step 4: Implement linked mirrored vegetation**

Create one detailed broad-leaf plant source from curved stems and 20-28 instanced leaf meshes. Link the same mesh/object data into left and right plant assemblies:

```python
def build_symmetric_floor_plants(ctx: StudioContext) -> tuple[bpy.types.Object, bpy.types.Object]:
    source = build_floor_plant_source(ctx)
    left_spec, right_spec = contract.floor_plant_transforms()
    left = linked_plant_instance("FloorPlant_Left", source, left_spec, ctx)
    right = linked_plant_instance("FloorPlant_Right", source, right_spec, ctx)
    left["symmetry_partner"] = right.name
    right["symmetry_partner"] = left.name
    left["linked_data_key"] = right["linked_data_key"] = contract.PLANT_LINK_KEY
    return left, right
```

Create the pots as separate linked ceramic objects. Their dimensions and vertical positions must be identical.

- [ ] **Step 5: Implement packed brand artwork**

`build_brand_art(ctx, icon_path)` must load the PNG with `check_existing=True`, call `image.pack()`, create an alpha-aware Principled image material, and place the icon on a thin plane above the poster paper. Create `Brand_Copy_Line1` and `Brand_Copy_Line2` as Blender text objects using the built-in font, center alignment, dark-brown material, and separate transform controls. Add paper, thin dark frame, back panel, and lightly rough glass as separate objects.

- [ ] **Step 6: Run the Blender tests**

Expected: plant linking, symmetry, exact English copy, and packed image assertions PASS.

- [ ] **Step 7: Commit generated art and scene assets**

```bash
git add ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
git commit -m "feat: add warm studio brand and vegetation" --only -- ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
```

### Task 5: Add Character Markers, Cameras, and Warm Daylight Lighting

**Files:**
- Modify: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`

- [ ] **Step 1: Add failing marker, camera, and light tests**

```python
def test_warm_studio_cameras_lights_and_markers_follow_contract() -> None:
    result = warm_sloth_studio_builder.build_scene(include_brand=False, include_lighting=True)
    spawn = bpy.data.objects["IP_Character_Spawn"]
    assert spawn["target_height"] == 2.55
    assert spawn["contract"] == "character_spawn"
    for name, spec in warm_studio_contract.CAMERA_SPECS.items():
        camera = bpy.data.objects[name]
        assert camera.type == "CAMERA"
        assert camera.data.lens == spec["lens"]
        assert camera.data.dof.focus_object.name == spec["focus"]
    roles = {obj.get("ip_light_role") for obj in bpy.data.objects if obj.type == "LIGHT"}
    assert {"window", "key", "fill", "practical", "downlight"}.issubset(roles)
    assert bpy.context.scene.render.resolution_x == 2560
    assert bpy.context.scene.render.resolution_y == 1440
    assert bpy.context.scene.view_settings.view_transform == "AgX"
    assert result["wide"].name == "Camera_Wide"
```

Add `import warm_studio_contract` beside the existing warm-studio builder import in the Blender test file.

- [ ] **Step 2: Verify the new test fails**

Run the standard Blender contract command. Expected: FAIL on missing markers, cameras, or lights.

- [ ] **Step 3: Implement markers and cameras from the pure contract**

Create `IP_Character_Spawn` at `(0.0, 0.30, 0.0)`, `IP_Focus_Head` at `(0.0, 0.30, 1.93)`, `IP_Focus_Desk` at `(0.55, -0.25, 0.98)`, and `IP_Focus_Shelf` at `(1.45, 2.50, 1.95)`. Use `contract.CAMERA_SPECS` to build all seven cameras. Set sensor width to 36 mm, clip range to `0.03-100 m`, nine aperture blades, and f-stops of 5.6 for wide, 5.0 for medium, 4.8 for close/three-quarter, and 4.0 for details.

After all staged builders exist, add the final orchestration boundary:

```python
def build_scene(
    *,
    include_brand: bool = True,
    include_lighting: bool = True,
    brand_icon_path: Path | None = None,
) -> dict[str, object]:
    ctx = create_scene_context()
    build_architecture(ctx)
    furniture = build_furniture(ctx)
    build_set_dressing(ctx)
    build_symmetric_floor_plants(ctx)
    if include_brand:
        icon_path = brand_icon_path or contract.repo_root() / "ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png"
        build_brand_art(ctx, icon_path)
    cameras = build_markers_and_cameras(ctx)
    if include_lighting:
        build_lighting(ctx)
    return {"context": ctx, **furniture, **cameras}
```

`build_markers_and_cameras()` returns keys `wide`, `medium`, `close`, `three_quarter_left`, `three_quarter_right`, `desk_detail`, and `shelf_detail`, each mapped to its authored camera object.

- [ ] **Step 4: Implement the approved warm lighting rig**

Add authored lights with base-energy metadata:

```python
LIGHT_SPECS = (
    ("Window_Softbox", "AREA", (-3.00, -0.10, 2.35), 720.0, (1.0, 0.84, 0.66), "window"),
    ("Studio_Key", "AREA", (-2.35, -2.10, 2.85), 520.0, (1.0, 0.88, 0.73), "key"),
    ("Studio_Fill", "AREA", (2.55, -1.60, 2.35), 145.0, (0.80, 0.88, 1.0), "fill"),
    ("Practical_Wall", "POINT", (-1.70, 2.36, 2.78), 42.0, (1.0, 0.49, 0.20), "practical"),
    ("Practical_Shelf", "POINT", (2.18, 2.28, 2.02), 48.0, (1.0, 0.49, 0.20), "practical"),
)
```

Add three 3000 K recessed point/area lights as `downlight`. Store `ip_base_energy` and `ip_light_role` on every light. Only window/key/downlights cast full shadows; fill and small practical lights use restrained shadows for Eevee performance.

- [ ] **Step 5: Configure render settings**

Default to Eevee Next, 2560×1440, 30 fps, PNG RGBA, opaque film, AgX with Medium High Contrast, and exposure 0.0. Create a low-strength neutral world and set `scene.camera = Camera_Wide` before saving.

- [ ] **Step 6: Run Blender contract tests and commit**

Expected: all scene-contract tests PASS.

```bash
git add mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
git commit -m "feat: light and frame warm studio" --only -- mcp/ip_avatar_3d/warm_sloth_studio_builder.py mcp/ip_avatar_3d/test_blender_scene_contract.py
```

### Task 6: Add Structural Validation and Multi-Camera QA Rendering

**Files:**
- Create: `mcp/ip_avatar_3d/validate_warm_studio.py`
- Create: `mcp/ip_avatar_3d/render_warm_studio_qa.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`

- [ ] **Step 1: Add a failing validator test**

```python
def test_warm_studio_validator_detects_contract_damage() -> None:
    warm_sloth_studio_builder.build_scene(include_brand=False, include_lighting=True)
    clean = validate_warm_studio.validate_scene(require_packed_brand=False)
    assert clean["errors"] == []
    bpy.data.objects.remove(bpy.data.objects["Camera_Close"], do_unlink=True)
    damaged = validate_warm_studio.validate_scene(require_packed_brand=False)
    assert any("Camera_Close" in item for item in damaged["errors"])
```

Import `validate_warm_studio` and append the test to the manual test list.

- [ ] **Step 2: Run the test and verify the missing-validator failure**

Expected: FAIL importing `validate_warm_studio`.

- [ ] **Step 3: Implement the JSON validator**

`validate_scene(require_packed_brand=True)` returns:

```python
{
    "schemaVersion": "tangying-warm-studio-qa/v1",
    "blenderVersion": bpy.app.version_string,
    "errors": [],
    "warnings": [],
    "counts": {"objects": 0, "meshes": 0, "materials": 0, "lights": 0, "cameras": 0},
    "requiredCollections": {},
    "requiredObjects": {},
    "plantSymmetry": {},
    "packedImages": [],
    "negativeScaleObjects": [],
    "floatingFurniture": [],
}
```

Check required collections/objects, absence of character mesh/armature collections, exact camera focal lengths, packed brand image, negative scale, floor-plant linked data and mirrored transforms, material presence, furniture floor contact within 2 cm, and image file resolution. The CLI accepts output JSON after `--`, writes it with `ensure_ascii=False, indent=2`, prints `WARM_STUDIO_VALIDATION=<path>`, and exits non-zero when `errors` is non-empty.

- [ ] **Step 4: Implement deterministic QA still rendering**

The QA renderer accepts `blend_path`, `output_dir`, required engine, and an optional comma-separated camera-name filter after `--`. It opens the `.blend`, sets 960×540 for camera coverage stills, and renders:

```python
CAMERA_OUTPUTS = {
    "Camera_Wide": "01-wide.png",
    "Camera_Medium": "02-medium.png",
    "Camera_Close": "03-close.png",
    "Camera_ThreeQuarter_Left": "04-three-quarter-left.png",
    "Camera_ThreeQuarter_Right": "05-three-quarter-right.png",
    "Camera_Desk_Detail": "06-desk-detail.png",
    "Camera_Shelf_Detail": "07-shelf-detail.png",
}
```

Add a temporary clay material override and render `08-wide-clay.png`; restore the original material override afterward. Accept engine values `eevee` and `cycles`. Use 32 Eevee temporal samples and 64 Cycles samples for QA. In Blender 5.1.2 set `scene.cycles.use_denoising = True`; guard the assignment with `hasattr(scene, "cycles")` so the script remains import-safe.

- [ ] **Step 5: Run Blender tests and commit**

Expected: all Blender scene-contract tests PASS.

```bash
git add mcp/ip_avatar_3d/validate_warm_studio.py mcp/ip_avatar_3d/render_warm_studio_qa.py mcp/ip_avatar_3d/test_blender_scene_contract.py
git commit -m "test: validate and render warm studio" --only -- mcp/ip_avatar_3d/validate_warm_studio.py mcp/ip_avatar_3d/render_warm_studio_qa.py mcp/ip_avatar_3d/test_blender_scene_contract.py
```

### Task 7: Build the Packed Blend and Produce Render Evidence

**Files:**
- Create: `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`
- Create: `ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png`
- Create: `outputs/warm_sloth_studio_v1/qa/validation.json`
- Create: `outputs/warm_sloth_studio_v1/qa/eevee/*.png`
- Create: `outputs/warm_sloth_studio_v1/qa/cycles/*.png`
- Create: `outputs/warm_sloth_studio_v1/qa/contact-sheet.png`

- [ ] **Step 1: Complete the builder command-line entry point**

`main()` resolves optional blend, preview, and icon paths after `--`; defaults to the approved deliverable paths. It calls `build_scene`, packs all resources, renders a 960×540 wide preview, restores 2560×1440 render settings, makes `Camera_Wide` active, saves the `.blend`, and prints:

```text
WARM_STUDIO_BLEND=<absolute path>
WARM_STUDIO_PREVIEW=<absolute path>
WARM_STUDIO_BRAND_ICON=<absolute path>
```

- [ ] **Step 2: Generate the complete scene**

```bash
mkdir -p "$STUDIO_QA"
$BLENDER_BIN --background --factory-startup --python mcp/ip_avatar_3d/warm_sloth_studio_builder.py -- "$STUDIO_BLEND" "$STUDIO_PREVIEW" "$PWD/ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png"
```

Expected: exit 0, preview exists, and `.blend` exists.

- [ ] **Step 3: Run the packed-resource validator**

```bash
$BLENDER_BIN --background "$STUDIO_BLEND" --python mcp/ip_avatar_3d/validate_warm_studio.py -- "$STUDIO_QA/validation.json"
python3 -c 'import json; p=json.load(open("outputs/warm_sloth_studio_v1/qa/validation.json")); assert not p["errors"], p["errors"]; print(p["counts"])'
```

Expected: validator exit 0 and empty `errors`.

- [ ] **Step 4: Render the Eevee camera set**

```bash
$BLENDER_BIN --background "$STUDIO_BLEND" --python mcp/ip_avatar_3d/render_warm_studio_qa.py -- "$STUDIO_BLEND" "$STUDIO_QA/eevee" eevee
```

Expected: eight PNGs exist and none are blank.

- [ ] **Step 5: Inspect the first contact sheet and correct visual defects**

Create a sheet:

```bash
ffmpeg -y -pattern_type glob -i "$STUDIO_QA/eevee/*.png" -vf "scale=640:-1,tile=3x3:padding=12:margin=12:color=white" -frames:v 1 "$STUDIO_QA/contact-sheet.png"
```

Inspect `contact-sheet.png` and correct only evidence-backed defects: camera clipping, non-symmetric plants, floating props, unreadable brand art, blown lamp shades, empty wall areas, broken grain direction, overly dark corners, or hero-layout mismatch. Rebuild and rerender after each correction.

- [ ] **Step 6: Render a Cycles hero comparison**

```bash
$BLENDER_BIN --background "$STUDIO_BLEND" --python mcp/ip_avatar_3d/render_warm_studio_qa.py -- "$STUDIO_BLEND" "$STUDIO_QA/cycles" cycles Camera_Wide,Camera_Desk_Detail,Camera_Shelf_Detail
```

Expected: three explicitly labeled Cycles stills exist for the hero, desk detail, and shelf detail cameras. Do not relabel Eevee evidence as Cycles.

- [ ] **Step 7: Verify media and file size**

```bash
file "$STUDIO_BLEND" "$STUDIO_PREVIEW" "$STUDIO_QA/contact-sheet.png"
du -h "$STUDIO_BLEND"
ffprobe -v error -show_entries stream=width,height,pix_fmt -of json "$STUDIO_PREVIEW"
```

Expected: preview is 960×540, QA stills are readable PNGs, and the `.blend` opens without external resources. Record the actual `.blend` size in the validation report rather than enforcing an arbitrary size limit.

- [ ] **Step 8: Commit the final scene and evidence**

```bash
git add ip形象/main_ip/scenes/warm-sloth-studio-v1.blend ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png outputs/warm_sloth_studio_v1/qa
git commit -m "feat: deliver warm sloth studio scene" --only -- ip形象/main_ip/scenes/warm-sloth-studio-v1.blend ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png outputs/warm_sloth_studio_v1/qa
```

### Task 8: Document Usage and Run the Final Verification Gate

**Files:**
- Modify: `mcp/ip_avatar_3d/README.md`

- [ ] **Step 1: Document the new optional scene**

Add a warm-studio section that states:

```markdown
### Warm Sloth Studio

`ip形象/main_ip/scenes/warm-sloth-studio-v1.blend` is the warm domestic talking-head alternative to the dark editorial studio. It is an empty, packed, 360-degree room with `IP_Character_Spawn`, `IP_Focus_Head`, `Camera_Wide`, `Camera_Medium`, and `Camera_Close`. Use it by passing its absolute path as `sceneBlendPath`. The active character profile is intentionally unchanged.
```

- [ ] **Step 2: Run fast Python verification**

```bash
PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest mcp/ip_avatar_3d/test_warm_studio_contract.py -v
python3 -m unittest mcp/ip_avatar_3d/test_server.py -v
```

Expected: both suites PASS.

- [ ] **Step 3: Run Blender verification**

```bash
$BLENDER_BIN --background --factory-startup --python mcp/ip_avatar_3d/test_blender_scene_contract.py
$BLENDER_BIN --background "$STUDIO_BLEND" --python mcp/ip_avatar_3d/validate_warm_studio.py -- "$STUDIO_QA/validation-final.json"
```

Expected: all tests print PASS and final validation contains no errors.

- [ ] **Step 4: Perform final visual checks**

Inspect the hero preview and contact sheet at original size. Confirm:

- hero layout matches `工作室设计版.png`;
- both large plants are visibly symmetrical;
- left cabinet/brand zone and right slat/shelf zone retain the reference relationship;
- every saved camera is inside the complete room and does not clip geometry;
- desk, cabinet, shelves, books, lamps, rug, and plants are grounded;
- exact English brand copy is readable;
- Eevee and Cycles preserve the same material identity;
- no character mesh or armature exists in the delivered scene.

- [ ] **Step 5: Commit documentation**

```bash
git add mcp/ip_avatar_3d/README.md
git commit -m "docs: document warm sloth studio" --only -- mcp/ip_avatar_3d/README.md
```

- [ ] **Step 6: Record final repository state**

```bash
git status --short
git log --oneline -8
```

Report unrelated pre-existing changes without modifying, staging, or committing them.

## Completion Evidence

Do not claim completion until all of the following exist and were inspected in the current run:

- `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`
- `ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png`
- `outputs/warm_sloth_studio_v1/qa/validation-final.json` with no errors
- `outputs/warm_sloth_studio_v1/qa/contact-sheet.png`
- at least one Cycles hero/detail render clearly labeled as Cycles
- passing pure-Python contract tests
- passing Blender scene-contract tests
