# Task 6 Report: Close-Shot Face and Source PBR Refinement

## Outcome

Task 6 finishes with a production-safe, fail-closed squint capability rather
than a full blink. The original integrated source face from
`ip形象/main_ip/turnaround/带骨骼3d模型.fbx` remains the visible face and is
checked against `ip形象/main_ip/turnaround/front.png`. No visible eye/lip
overlay, image card, face-texture replacement, generated eyelid material, or
generated eyelid geometry is present.

The source does not contain independent lid strips. Two generated full-lid
implementations passed structural checks but failed original-size visual QA
with ocular-texture streaking, collapsed triangles, white lid blocks, open
corners, and torn perimeters. Those implementations are not used. The final
asset reports:

- `trueEyelidTopology=false`
- `blinkCapability='squint_only'`
- `Eye_Squint.L` and `Eye_Squint.R` only
- no `Eye_Blink.L/R` and no `Face_Blink`

Legacy full-blink metadata is rejected with a clear `rebuild master from source
FBX for blinkCapability=squint_only` error.
`facialTopologyMode=volumetric`, `true_geometry`, and `lips_eyelids` are also
rejected before face setup; the production source asset supports
`source_retopology` with `blinkCapability=squint_only` only.

## RED/GREEN Evidence

Initial Task 6 RED:

`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py`

- The first source-retopology contract failed because true eyelid topology,
  contact, UV, and PBR evidence did not exist.
- Strengthened tests subsequently exposed full-XYZ contact, fixed-boundary,
  source-link material-role, modified-region UV/custom-data, GLB extras/reuse,
  and runner-exit gaps.

Visual RED and fail-closed decision:

- Generated-lid reviews failed twice at original `1024x1024` size. The first
  collapse had `752/1110` faces below 1% area, `112` zero-area faces, `777`
  aspect ratios above 100, `134` edges above 2x stretch, and `169` bridge
  streaks. Later strips preserved the ocular core but remained visibly white,
  striped, jagged, and torn.
- The full-lid branch was abandoned as required; failed `full_blink.png`
  evidence was removed and is not a deliverable.

Squint fallback RED/GREEN:

- RED: `RuntimeError: squint-only fallback found an incomplete L skin ring:
  upper=6, lower=59` exposed the asymmetric `center_z` split.
- GREEN: candidates are now sorted by normalized Z and use balanced top/bottom
  samples.
- RED: the first balanced geometric annulus rendered lower-face/mouth-corner
  pulls. A structural test then rejected selected vertices outside two mesh
  edges of the ocular core.
- GREEN: each side now uses exactly 12 upper and 12 lower zero-eye-weight
  vertices from the first two topological skin rings, bounded to normalized
  radius `1.30`, with maximum closure reduced to `0.12`.
- RED: GLB reimport produced 35 moved vertices from 32 stored source indices
  because glTF split vertices at loop/UV boundaries. Source-index cardinality
  was therefore not a valid serialization assertion.
- GREEN: fresh FBX builds retain exact per-index UV checks. GLB displacements
  must map to stored skin-loop UVs within `5e-5`, remain finite, and retain zero
  Eye weight; ocular metadata remains preserved and imported ocular UVs remain
  finite without requiring unstable vertex/loop multiplicity.
- RED: reuse accepted deleted center metadata, ocular-core Shape Key edits while
  stored displacement remained zero, injected `Eye.L` skin weight, and a stale
  PBR claim after its linked base-color node was removed.
- GREEN: reuse now recomputes core and non-skin displacement from `Basis` and
  `Eye_Squint.L/R`, verifies opposite-side isolation, live `Eye.L/R` weights,
  finite active-skin UVs, signed nonzero closure bounded by stored closure
  `<=0.12`, and canonical source or packed PBR links/color spaces.
- RED: `facialTopologyMode=volumetric` generated upper/lower lid objects and an
  eyelid material for the source asset.
- GREEN: generated-lid modes fail at `setup_face` entry, before any lid object or
  material can be created; the abandoned integrated `Eye_Blink` branch was
  removed from `add_rich_source_face_shapes`.

Direct-runner RED reliability:

`env IP_AVATAR_FORCE_TEST_FAILURE=1 /Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py`

- Blender `5.1.2`, exit `1`.
- Output: `FAIL deliberate_direct_runner_failure` and
  `FAILED 1 direct-runner test(s)`.

Final character GREEN:

`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py`

- Blender `5.1.2`, exit `0`.
- PASS all `20` direct-runner tests, including fresh source build, independent
  L/R squint, no-visible-overlay checks, mouth bounds, linked source PBR roles,
  live-data fail-closed reuse, generated-lid mode rejection, GLB extras, and live
  GLB export/reimport.

Final scene GREEN:

`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_scene_contract.py`

- Blender `5.1.2`, exit `0`.
- PASS all `5` scene-contract tests.

## Structural Evidence

Squint and ocular isolation:

- Source face vertex count: `6416`; eye subdivision/new lid vertices: `0`.
- Original ocular core: `63` vertices per side, all unchanged by both squint
  Shape Keys (`max displacement = 0.0`).
- Active source-skin ring: `12` upper + `12` lower vertices per side.
- Active skin vertices have zero `Eye.L/R` weight and lie within two topology
  edges of the ocular core. Maximum non-skin displacement is `0.0`.
- L/R active sets are disjoint and the opposite-side Shape Key displacement is
  zero. Eye bones continue to provide gaze control.
- No new lid faces exist, so the failed full-lid triangle-collapse class is
  absent rather than hidden by a weaker bound.

UV, custom data, reuse, and export:

- Original UV layer names and relevant color/custom-attribute signatures are
  stored and checked. A 64-vertex untouched UV guard remains exact on the fresh
  build.
- Exact ocular and squint-skin per-loop UV evidence is stored before Shape Keys.
  No UV values are authored or modified by the fallback; source and imported
  evidence is checked for finite values.
- Original deform assignments are preserved; no generated lid vertices or
  weights exist.
- Supported glTF exporters receive `export_extras=True`. Reimport must retain
  squint capability, per-side core/skin metadata, boundary/UV/deform evidence,
  and tuned PBR role metadata.
- Existing rich/source-retopology assets are reusable only when every current
  Task 6 squint property validates and the live shapes, weights, and UVs agree.
  Missing metadata, mutated ocular/skin data, and legacy full-blink mode all fail
  closed with a source-FBX rebuild error.

Mouth restraint:

- Head-width lateral limit: `0.0022401830`.
- Maximum corner shifts: `Mouth_Smile 0.0011520907`, `Mouth_E 0.0017921478`,
  and `Mouth_MBP 0.0007554740`.
- `Mouth_A` vertical gap: `0.0738434792`; `Mouth_MBP` gap: `0.0420825481`.
- The close-shot smile remains restrained and closed while jaw opening and oral
  cavity visibility remain available.

Source PBR:

- The original linked 4096x4096 maps remain wired as
  `Image Texture - Base Color`, `Image Texture - Metallic`,
  `Image Texture - Normal`, and `Image Texture - Roughness`.
- Roles resolve from Principled/Normal Map links first. Filename fallback only
  confirms unambiguous missing roles and rejects duplicates.
- Base color is `sRGB`; metallic, normal, and roughness are `Non-Color`.
- Tangent normal strength is `0.34`, specular IOR level is `0.28`, and source
  roughness is remapped to `0.38..0.76`.
- No global `BUMP` or `TEX_NOISE` node is added. Packed GLB material graphs keep
  tuned metadata only when the live base-color, packed metallic/roughness, and
  tangent-normal graph has canonical links and color spaces. Disconnected or
  stale PBR claims fail closed.

## Visual Evidence

Fresh-FBX close-shot renders, regenerated after the final topological-ring fix:

- Neutral: `/Users/wanglian/Projects/tangying-ai-operation-system/outputs/task6_face_review/neutral.png`
- Restrained smile: `/Users/wanglian/Projects/tangying-ai-operation-system/outputs/task6_face_review/smile.png`
- Full-strength squint: `/Users/wanglian/Projects/tangying-ai-operation-system/outputs/task6_face_review/squint.png`

All were inspected at their original `1024x1024` size. Exposure remains `-0.35`
and was not used as a fix. The neutral and smile preserve the source texture,
soft mapped surface response, open eyes, and narrow closed smile without a
toothy/dark open-mouth seam. The final squint keeps both eyes visible and has no
triangular smear, eye-texture streak, white block, torn perimeter, corner hole,
or lower-face pull. Its deliberately restrained deformation is measurable
against neutral (`SSIM 0.994454`, `PSNR 34.794216 dB`).

## Residual Issue

This source asset does not provide production-safe automated full-blink
topology. Full blink is intentionally unavailable, not simulated or mislabeled;
the delivered capability is an open-eye, source-skin squint. A future true blink
requires authored integrated lid topology or a source model with separable lid
rings.

Blender `5.1.2` also emits expected `Material.use_nodes` deprecation warnings
for Blender 6.0; they do not fail either required suite.
