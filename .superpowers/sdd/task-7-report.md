# Task 7 Report: Stable A-roll Character Masters

## Outcome

Task 7 adds a one-time master preparation path and a fail-closed master loading
path for talking-video renders. The main-IP profile now declares the stable
`models/main-ip-aroll-master.blend` asset, `aroll_close` quality tier, complete
three-segment finger semantics, and the Task 6 `squint_only` facial truth.

The implementation does not silently fall back to source refinement during a
normal render. If the configured master is missing, rendering stops before
audio or Blender and points the caller to `prepare_character_master`. If a
configured file exists but lacks the named collection, version metadata, or
exactly one Armature, Blender raises a specific validation error.

## TDD Evidence

The first Task 7 server run executed 38 tests and failed on the intentionally
missing contract:

- missing `model.masterBlendPath` in the real profile;
- missing `prepare_character_master`;
- missing master fields in `render_input.json`;
- no non-Blend master rejection;
- missing `master_asset.py` helper module.

A separate fail-closed test then demonstrated the previous source fallback: a
configured missing master reached Blender and failed later because no rig
outputs were produced. The implementation now raises `FileNotFoundError` with
`prepare_character_master` before audio generation or Blender execution.

## Implementation

### Master collection

`mcp/ip_avatar_3d/master_asset.py` defines:

- `MASTER_COLLECTION = "IP_Character_Master"`;
- `ip_aroll_master_version = 1` metadata;
- save-time duplicate Armature and duplicate collection rejection;
- append-time file, collection, version, duplicate collection, and exactly-one
  Armature validation;
- append of all source Actions together with the master collection.

The save path moves the complete character hierarchy into the versioned master
collection before saving the Blend. Materials, Shape Keys, object hierarchy,
Armature data, and fake-user Actions remain Blender datablock dependencies of
the saved master.

### Preparation tool

`prepare_character_master` resolves profile assets relative to the profile
directory and uses `sourceModel`, then `model.sourcePath`, then `model.path`.
It writes deterministic sibling paths derived from the configured master name:

- `main-ip-aroll-master.blend`;
- `main-ip-aroll-rigged.glb`;
- `main-ip-aroll-rig-report.json`;
- `main-ip-aroll-qa-input.json`.

The QA input is also the Blender asset-only request. Dry-run writes this input
without requiring Blender. A real preparation imports and refines the source
once, builds the action library, saves the versioned collection, exports GLB,
and emits the rig report.

### Render loading

`render_talking_video` writes the absolute profile-resolved
`masterBlendPath` into `render_input.json`. When that file exists it sets
`useMasterAsset=true`, disables existing-rig enhancement, and changes facial
topology work to `source_only`.

The Blender renderer opens the authored studio first and then appends the
validated master. It skips FBX/GLB import, hand refinement, source face
retopology, and render-detail mutation. It reuses the existing Armature,
Mouth Shape Keys, face topology objects, materials, and Actions before applying
the current motion plan and authored-scene placement.

## Profile Contract

`ip形象/main_ip/character-profile.json` now contains:

- `model.masterBlendPath = "models/main-ip-aroll-master.blend"`;
- `model.qualityTier = "aroll_close"`;
- `model.fingerTopology = "three_digits_three_segments"`;
- all six `finger_*_mid_*` required bone roles;
- `facial.blinkCapability = "squint_only"`.

No `Eye_Blink.*`, `Face_Blink`, or full-blink capability is claimed.

## Verification

- `python3 mcp/ip_avatar_3d/test_server.py`: 40 tests passed, with the
  Blender-only integration test skipped under system Python.
- Blender-focused master integration: 1 test passed. It saved a real versioned
  Blend, reset Blender, appended the collection, retained exactly one Armature,
  and loaded a fake-user A-roll Action.
- `test_blender_scene_contract.py`: 5 tests passed on Blender 5.1.2.
- `test_blender_character_rig.py`: all direct-runner tests passed on Blender
  5.1.2, including Task 6 squint-only and source-PBR fail-closed coverage.
- Python compilation passed for `master_asset.py`, `server.py`, and
  `test_server.py`.
- A non-dry-run preparation against the real main-IP FBX completed in a
  temporary directory and verified all four deterministic output files.

## Residual

`ip形象/main_ip/models/main-ip-aroll-master.blend` is not created or
committed by this source-code task. Until an approved persistent master is
prepared at that path, a real talking-video render from the updated profile
will fail closed as designed. The temporary verification master was deleted
after the test. A full encoded talking-video render from the persistent master
remains an operational acceptance step after that asset is approved.
