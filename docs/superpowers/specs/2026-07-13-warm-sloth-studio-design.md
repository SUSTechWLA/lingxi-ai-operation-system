# Warm Sloth Studio Design

## Context

The repository already contains a reusable dark editorial Blender studio and an A-roll import contract for the main sloth character. The new request is different: create a detailed, warm, domestic talking-head studio that closely follows `ip形象/main_ip/scenes/references/工作室设计版.png` and can be reused as a complete filming environment.

The character is being produced independently. This project therefore delivers an empty studio scene with stable spawn and focus markers, not a bundled character.

## Goals

- Reconstruct the layout, composition, colors, furniture, and practical lighting shown in `ip形象/main_ip/scenes/references/工作室设计版.png`.
- Deliver a complete 360-degree room that supports the reference hero angle, side angles, reverse angles, and restrained camera moves without revealing missing geometry.
- Keep the room detailed enough for 1920x1080 production framing and 4K close shots of the desk, shelves, lamps, books, ceramics, plants, curtains, and branded artwork.
- Preserve Blender 5.1.2 compatibility and support both Eevee preview renders and Cycles final renders.
- Integrate with the existing A-roll scene contract through `IP_Character_Spawn`, `IP_Focus_Head`, `Camera_Wide`, `Camera_Medium`, and `Camera_Close`.
- Keep the studio modular, packed, non-destructive, and independent of the character asset and the existing dark editorial studio.

## Non-Goals

- Do not create, modify, or embed the sloth character.
- Do not overwrite `ip形象/main_ip/scenes/editorial-news-studio.blend`.
- Do not change the active character profile while another agent is producing the character.
- Do not depend on paid asset libraries or external texture paths.
- Do not reproduce the source reference as a flat background plate.
- Do not build an architectural construction drawing or physically certified interior plan.

## Chosen Approach

Use a hybrid of reference-camera matching and a modular complete room.

The hero camera and all visible object relationships are matched first. The remainder of the room is then completed using consistent dimensions, wall construction, materials, and prop density. This preserves the reference image while avoiding the limitations of a camera-only set.

The room, furniture, props, vegetation, brand art, lights, cameras, and markers are separate collections. Repeated elements use linked data or instances to limit file size.

## Reference Fidelity Rules

The reference board controls the following decisions:

- Main desk centered on the room and hero-camera axis.
- Chair behind the desk, retained in the empty set but hidden by the desk in the hero view.
- Left side of the back wall: plaster wall, six-drawer cabinet, brand artwork, wall lamp, globe, books, a small plant, and a task lamp.
- Right side of the back wall: vertical walnut slats, three floating shelves, books, small frames, pottery, plants, a globe, and a warm table lamp.
- Left wall: large floor-height window with sheer and outer curtains.
- Right wall: vertical framed artwork in the middle and a door in the camera-side/front section.
- Main desk: warm solid wood top and drawer body with dark rectangular steel legs.
- Oval jute rug beneath the desk and chair.
- Warm oak floor, warm off-white plaster walls, cream textiles, dark brown metal, muted ceramics, and abundant natural foliage.
- Three recessed ceiling lights plus visible practical lights.

The left and right floor plants are a strict compositional pair. They use the same linked plant asset, mirrored locations, equal distance from the center axis, equal canopy width and height, and matching planters. Their rotations may be mirrored, but their visual weight must remain equal.

## Room Scale and Layout

Blender uses metric units.

- Interior room: 6.2 m wide, 5.8 m deep, 3.4 m clear height.
- Main desk: 2.7 m wide, 0.95 m deep, 0.92 m high.
- Window clear opening: 2.6 m wide and 2.75 m high, centered on the left wall.
- Door clear opening: 1.0 m wide and 2.4 m high, placed in the front section of the right wall.
- Rug: oval, sized to extend clearly beyond the desk legs and chair zone.
- Character scale anchor: 2.55 m, matching the current A-roll pipeline contract.

`SET_MASTER` is the scene-wide root and supports uniform rescaling. Architectural thickness, bevel sizes, prop locations, camera clipping, light sizes, and focus targets must remain coherent under the chosen scale.

The desk, chair, spawn point, focus point, rug, and hero camera share the same center axis. A clear walking zone remains behind the chair and around both desk sides. The front wall is complete, while the hero camera remains inside the room near its center.

## Architecture

The architecture is real geometry, not single-sided image planes.

- Four walls with thickness.
- Full floor and ceiling.
- Window and door openings with frames, sills, trim, and recess depth.
- Door leaf, handle, hinges, and swing clearance.
- Baseboards and restrained crown/ceiling trim.
- Individually modeled floorboards or instanced board modules with visible seams.
- Walnut slat modules with real depth, gaps, backing panel, and softened edges.
- Curtain rail or concealed track, sheer curtain, and thicker outer drape.
- Simple exterior light blocker beyond the window so reverse angles do not expose an empty world.

The room should feel intimate and domestic. It must not read as an oversized news studio.

## Furniture and Props

### Main Desk

The desk is a near-camera hero asset.

- Solid wood top with believable thickness, softened edges, end grain, and grain direction aligned to the boards.
- Two broad drawer fronts with dark handles.
- Dark brown rectangular steel frame and feet.
- Separate tabletop props: ceramic plant pot, branded mug, three books, notebook, and pen.

### Left Cabinet Zone

- Six-drawer wood cabinet with dark handles and small feet.
- Globe, vertical and stacked books, small ceramic planter, compact task lamp, and a woven basket.
- Brand artwork centered above the cabinet.
- Warm wall lamp centered over the artwork.

### Right Slat and Shelf Zone

- Full-height walnut slat feature wall.
- Three floating wood shelves with correct attachment depth and bevels.
- Books with controlled color variation, small frames, pottery, plants, one globe, and a warm-shaded table lamp.
- Props form balanced clusters rather than evenly spaced filler.

### Soft Furnishings and Vegetation

- The chair is complete but visually hidden by the desk in the hero angle.
- The jute rug uses a detailed PBR surface plus sparse silhouette fibers, avoiding a heavy full-fiber simulation.
- Curtains use frozen cloth-like folds and remain editable through their source/control geometry.
- The two large floor plants are linked mirrored instances.
- Small desk and shelf plants remain separate assets.

## Materials

Materials use Principled BSDF-compatible node graphs that render consistently in Eevee and Cycles.

- Main desk oak: warm medium brown, visible directional grain, medium roughness.
- Walnut slats and shelves: darker brown with restrained grain and real groove shadows.
- Floor oak: lighter, warmer, and less saturated than the desk.
- Wall plaster: warm off-white with subtle low-frequency bump.
- Curtains: cream sheer fabric plus a denser warm-beige outer fabric.
- Ceramics: beige and muted sand colors with micro-roughness variation.
- Metals: dark brown-black powder coating, moderate roughness, restrained highlights.
- Lamps: cream textile or frosted shade with warm emissive interior.
- Rug: natural jute color with woven bump and irregular edge fibers.
- Plants: physically plausible green variation without neon saturation.

Real bevels or bevel modifiers remain visible on close assets. Bump and normal detail must supplement, not replace, important silhouettes.

## Brand Artwork

The brand artwork keeps the English copy:

```text
Slow Down.
Think Better.
```

The icon is not copied from the generic sloth mark in the studio reference. It is newly derived from `frontend/public/躺营ai视频创作助手.png`:

- simplified sloth face or resting sloth silhouette;
- closed-eye, calm expression;
- subtle play or conversation motif;
- single dark-brown mark suitable for a warm interior;
- no Chinese text inside the framed artwork.

The generated icon is treated as source art, simplified for clean rendering, and packed into the `.blend`. The English copy uses Blender text objects so it remains exact and editable. Icon, text, paper, frame, and glass are separate objects.

## Lighting

The default lighting represents soft warm daytime, matching the reference.

- Large soft window source from camera-left, 4800 K.
- Broad soft key from camera-left/front to keep the future character readable.
- Low-intensity neutral fill from camera-right.
- Warm practical table lamp on the right shelf, 2700 K.
- Warm practical wall lamp over the brand artwork.
- Three restrained recessed ceiling lights.
- Low-strength neutral world illumination.

Practical shades visibly glow but do not provide all scene illumination. Lights use stored base energy and role metadata so the existing lighting-preset system can scale them safely.

Color management uses AgX with a moderately contrasty look. Exposure is set from rendered evidence rather than assumed values. Highlights in lamp shades, curtains, and cream walls must retain detail.

## Cameras and Focus Targets

The scene contains:

- `Camera_Wide`: 18 mm hero room view at `(0.0, -2.74, 1.67)`, aimed at `(0.0, 0.45, 0.85)`.
- `Camera_Medium`: 50 mm talking-head view.
- `Camera_Close`: 70 mm close talking-head view.
- `Camera_ThreeQuarter_Left`: 50 mm.
- `Camera_ThreeQuarter_Right`: 50 mm.
- `Camera_Desk_Detail`: 85 mm, f/6.3 tabletop close-up from `(1.95, -2.07, 1.68)`, with `IP_Focus_Desk` on the mug at `(0.80, -0.50, 1.005)`.
- `Camera_Shelf_Detail`: 85 mm shelf and lamp close-up.

Character-facing cameras focus on `IP_Focus_Head`. Detail cameras use separate desk and shelf focus markers. Depth of field remains restrained so the environment is recognizable and compositing is stable.

The hero camera matches the reference before secondary cameras are tuned. No saved camera may intersect walls, furniture, or foliage.

The 18 mm wide framing and revised desk-detail focus are evidence-driven production revisions made after inspecting the rendered camera set. They supersede the initial 32 mm planning assumption while preserving the approved layout and camera roles.

## Blender Scene Structure

The `.blend` contains these top-level collections:

- `STUDIO_ARCHITECTURE`
- `STUDIO_FURNITURE`
- `STUDIO_PROPS`
- `STUDIO_VEGETATION`
- `STUDIO_BRAND`
- `STUDIO_LIGHTS`
- `STUDIO_CAMERAS`
- `STUDIO_MARKERS`
- `QA_ONLY`

`QA_ONLY` contains a scale proxy and diagnostic objects. It is disabled for final viewport display and rendering in the delivered scene.

Contract objects include:

- `SET_MASTER`
- `IP_Character_Spawn`
- `IP_Focus_Head`
- `IP_Focus_Desk`
- `IP_Focus_Shelf`
- `Camera_Wide`
- `Camera_Medium`
- `Camera_Close`

All production objects use clear semantic names. Mirrored plants and repeated books/slats use linked data or collection instances. Transforms are normalized unless an unapplied modifier requires otherwise.

## Render Configuration

- Blender version: 5.1.2.
- Default interactive engine: Eevee Next.
- Final engine: Cycles when selected by the caller.
- Cycles final exposure profile: `ip_cycles_final_exposure=-0.8`, applied by the QA renderer while the saved Eevee exposure remains `0.0`.
- Default resolution: 1920x1080 (1080p production output).
- Frame rate metadata: 30 fps.
- Color management: AgX.
- Textures and generated brand artwork: packed into the `.blend`.
- No paid add-ons or runtime asset downloads.

The scene remains a single source of truth. The existing renderer may override the engine and quality preset without replacing the world, lights, cameras, or materials.

## Deliverables

- Builder source: `mcp/ip_avatar_3d/warm_sloth_studio_builder.py`.
- Studio file: `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`.
- Hero preview: `ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png`.
- Packed brand source asset under `ip形象/main_ip/scenes/assets/`.
- QA report and render evidence under `outputs/warm_sloth_studio_v1/qa/`.

The builder creates a new output and never overwrites the dark editorial studio. The active character profile is not changed in this task.

## Build and Data Flow

1. Generate and save the simplified brand icon source.
2. Start a clean Blender scene and create the collection contract.
3. Build metric architecture and openings.
4. Build furniture and reusable prop assets.
5. Create materials and apply correct grain/fabric direction.
6. Create the linked plant pair and remaining vegetation.
7. Create brand artwork from the icon and exact Blender text.
8. Add lights, cameras, focus targets, and metadata.
9. Validate the scene contract and packed resources.
10. Save the `.blend` and render the QA set.

The output scene is empty of character geometry. A renderer or artist later imports a character, scales it to the spawn contract, and updates `IP_Focus_Head` to the final head position.

## Failure Handling

- The build operates on a new scene and output path; it does not mutate the only copy of an existing production asset.
- Missing generated artwork blocks final packaging rather than leaving an external missing texture.
- If a font cannot be resolved, the exact English copy falls back to Blender's built-in font.
- Missing texture and unpacked-resource checks run before saving the deliverable.
- Contract validation fails on missing markers, standard cameras, collections, or metadata.
- Geometry validation reports negative scale, unintentional non-manifold architecture, inverted normals, and floating furniture.
- A render failure preserves the `.blend` and reports the failed camera and engine instead of deleting intermediate work.
- Cycles unavailability falls back only for preview evidence; it does not relabel an Eevee render as Cycles evidence.

## Verification

### Structural Checks

- Open the result in Blender 5.1.2 without missing-resource warnings.
- Confirm all required collections, cameras, lights, and markers exist.
- Confirm the scene contains no character meshes or armatures.
- Confirm all packed image resources resolve after moving the `.blend` to a clean temporary directory.
- Confirm the left and right floor plants share linked source data and have mirrored transforms.

### Render Evidence

- Empty hero-room render.
- Wide, medium, close, left 3/4, right 3/4, desk detail, and shelf detail stills.
- Clay hero render to validate geometry and contact shadows.
- Eevee/Cycles hero comparison.
- Contact sheet containing all approved views.

### Visual Acceptance

- The hero view clearly matches the reference layout and warm color relationships.
- Both floor plants have equal visual weight and symmetrical placement.
- Desk, rug, furniture, and props visibly contact the floor or their supporting surfaces.
- The right wall door and artwork, left window, and completed front wall support non-hero angles.
- Curtains, rug, wood grain, lamps, books, ceramics, and branded art hold up in 2K close views.
- Practical lights retain highlight detail and do not clip large areas of the scene.
- No camera intersects walls, leaves, furniture, or props.
- Eevee preview and Cycles final retain the same composition and material identity.

## Acceptance Criteria

The studio is complete when:

1. The required `.blend`, builder, preview, packed brand source, QA report, and contact sheet exist.
2. The scene opens cleanly in Blender 5.1.2 with no missing external resources.
3. The hero render is recognizably derived from `ip形象/main_ip/scenes/references/工作室设计版.png`.
4. The room remains complete and believable from every saved camera.
5. The linked plant pair is symmetrical.
6. Standard A-roll cameras and character markers satisfy the current scene contract.
7. The deliverable contains no character asset and does not modify the active character profile or dark editorial studio.
8. Render-based QA finds no blocking geometry, lighting, composition, or packaging defect.
