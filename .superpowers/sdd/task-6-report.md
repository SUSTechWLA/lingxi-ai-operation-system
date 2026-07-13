# Task 6 Report: Close-Shot Face and Source PBR Refinement

## Summary

Refined the original integrated source face from
`ip形象/main_ip/turnaround/带骨骼3d模型.fbx` without adding visible eyelid/lip
overlays or replacing the source texture. The fresh-FBX path now subdivides the
resolved eye regions before Basis creation, creates independent paired L/R blink
contact, restrains mouth-corner travel, and tunes the existing linked 4K PBR maps.

Owned files changed:
- `mcp/ip_avatar_3d/blender_renderer.py`
- `mcp/ip_avatar_3d/test_blender_character_rig.py`
- `.superpowers/sdd/task-6-report.md`

## TDD Evidence

Initial RED:
`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py`

Result:
- The direct runner reached the new source-retopology test and failed at
  `face_mesh["true_eyelid_topology"]` with the expected `KeyError`.
- The renderer had no integrated eyelid refinement metadata or contact topology.
- Blender returned process exit `0` despite the script traceback; the traceback,
  not the process code alone, was used as RED evidence.

Visual-audit RED:
- The first broad geometric eye classifier passed structural assertions but a
  close render dragged unweighted muzzle/forehead vertices into the blink.
- A new assertion requiring every active lid vertex to retain at least `0.012`
  weight in its resolved `Eye.L`/`Eye.R` group failed with
  `RuntimeError: Vertex not in group`.

Correctness-review RED:
- Strengthened tests changed blink contact from projected XZ proximity to stored
  full-XYZ pairs and added boundary-coordinate, UV-loop, and link-priority PBR
  checks.
- The direct runner failed with the expected
  `AttributeError: _resolve_source_pbr_texture_roles` before renderer support was
  added.

Final GREEN:
`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py`

Result:
- Exit `0` under Blender `5.1.2`.
- PASS all 17 direct-runner character tests.
- Includes fresh FBX refinement, no-visible-overlay checks, independent blink
  pairs, fixed boundary coordinates, UV guards, mouth limits, linked PBR roles,
  animation, publish detail, and exported packed-metallic/roughness GLB
  compatibility.

Final GREEN:
`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_scene_contract.py`

Result:
- Exit `0` under Blender `5.1.2`.
- PASS all 5 scene-contract tests.

## Structural Evidence

Integrated eye topology:
- Final source-face vertices: `6926`.
- Local eye subdivision added vertices: `510`.
- Per side: `257` weighted region vertices, `95` upper vertices, `162` lower
  vertices, and `36` fixed original boundary vertices.
- Per side: `75` deterministic upper/lower contact pairs, or `150` real contact
  samples for `eyelid_loop_vertex_count_*`.
- Maximum full 3D world-space contact-pair distance at blink: `0.0` for L and R;
  allowed bound: `2.55 * 0.002 = 0.0051`.
- L/R Shape Keys remain isolated.
- Subdivision excludes edges touching the precomputed eye-region boundary.
- Stored pre-subdivision boundary coordinates match the final Basis coordinates.
- `64` untouched original vertices guard the active UV layer; their per-vertex UV
  loop multisets match before and after subdivision.
- Topology mode is explicitly `integrated_source_face`, not detached eyelid
  geometry.

Mouth restraint:
- Head-width lateral bound: `0.0022401830`.
- `Mouth_Smile` maximum corner shift: `0.0011520907`.
- `Mouth_E` maximum corner shift: `0.0017921478`.
- `Mouth_MBP` maximum corner shift: `0.0007554740`.
- `Mouth_A` vertical gap: `0.0738434792`; `Mouth_MBP` gap: `0.0420825481`, so
  speech opening and oral interior visibility remain available.
- No image-card mouth, replacement texture, visible lip overlay, or material
  mouth-mask driver was added to the integrated path.

Source PBR:
- Actual FBX material inspection found localized source node names and four linked
  4096x4096 images: base color, metallic, normal, and roughness.
- Roles are resolved from existing Principled/Normal Map socket links first.
  Filename fallback ignores unknown or ambiguous packed textures and cannot
  overwrite duplicate role candidates.
- Resolved nodes are temporarily renamed before canonical exact names are
  assigned, avoiding Blender `.001` collision suffixes.
- Color spaces: base color `sRGB`; metallic, normal, and roughness `Non-Color`.
- Normal strength: `0.34`; Principled specular IOR level: `0.28`.
- Roughness map range: `0.38..0.76`.
- No source-material `BUMP` or `TEX_NOISE` node was added.
- Existing exported GLB packed metallic/roughness material remains supported and
  is not destructively rewritten as four distinct maps.

## Visual Audit

Compared `outputs/main_ip_hand_interaction_2k/frames/frame_0045.png` with
`ip形象/main_ip/turnaround/front.png`, then rendered close neutral, smile, and
blink checks from the fresh FBX.

- Exposure stayed at `-0.35`; no exposure increase was used as a material fix.
- Neutral and smile keep a closed, restrained seam without the prior visible
  tooth row.
- Smile corner travel is visibly reduced and remains close to the reference's
  narrow smile.
- The corrected blink no longer drags broad muzzle slabs or creates detached lid
  objects. Paired integrated source vertices meet exactly in 3D.
- Existing mapped normal/roughness detail produces a softer face/fur response
  while retaining the original eye, face, and fabric textures.

## Residual Notes

- A narrow source-texture crease remains visible at full blink because closure is
  built from the integrated textured source face; it is not hidden with an
  overlay or replacement texture. Structural contact is exact.
- Blender `5.1.2` emits deprecation warnings that `Material.use_nodes` is expected
  to be removed in Blender 6.0. They do not affect the two required suites.
