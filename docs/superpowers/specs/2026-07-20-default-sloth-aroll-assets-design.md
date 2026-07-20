# Default Sloth A-roll Assets Design

Date: 2026-07-20

Status: approved (approach A)

## Goal

Publish the currently approved sloth character and warm studio as the bundled,
portable default A-roll identity for the Tangying agent system. A caller that
does not choose another character profile must receive this identity. The
default presentation is a standing front talking shot, while full-body, seated,
and face-close-up presets remain available from the same character asset.

## Constraints

- Preserve the validated Blender source files and all existing release assets.
- Do not depend on a developer worktree or an absolute local path at runtime.
- Keep exactly one formal character and one formal armature in the character
  master.
- Keep the studio template free of a formal character so the renderer cannot
  import a duplicate.
- Preserve the character's rig, actions, shape keys, drivers, UVs, materials,
  groom data, and vertex order.
- Use versioned, immutable Blender artifacts and a manifest containing hashes.
- Build from the latest remote `develop_go/release` commit rather than merging
  the unrelated local `release` history.

## Selected Architecture

The bundled identity remains a pair of Blender assets because the existing
`ip_avatar_3d` provider already opens a scene template and imports a character
master at the studio spawn marker.

1. `main-ip-aroll-master-20260720.blend` is the character-only source of truth.
2. `warm-sloth-studio-20260720.blend` is the studio-only source of truth.
3. `default-aroll-assets.json` records schema version, logical asset IDs,
   relative paths, SHA-256 hashes, validation evidence, and source provenance.
4. `character-profile.json` points to the versioned pair and supplies the
   default camera, presentation, render, and voice settings.
5. `_default_character_profile_path()` remains the runtime entry point. Its
   repository-relative bundled profile is therefore the default whenever the
   request omits `characterProfilePath`.

The stable profile is the indirection layer. A future model upgrade publishes a
new immutable pair and changes the profile and manifest; it does not overwrite
the previous pair.

## Asset Publication

The latest approved integrated Blender scene is treated as immutable source
evidence. Publication scripts are saved under `ip形象/main_ip/scripts/` before
execution.

The exporter creates two new files without changing the approved source:

- Character master: retain `COL_CHR_SLOTH_FINAL` and its dependent armature,
  meshes, materials, node groups, images, hair curves, shape keys, actions,
  constraints, and drivers; omit studio cameras, lights, architecture, and set
  dressing.
- Studio template: retain the warm studio scene, world, cameras, lights, set
  dressing, `IP_Character_Spawn`, and `IP_Character_Focus`; omit
  `COL_CHR_SLOTH_FINAL` and other formal character objects.

After export, each file is reopened independently and audited. A publication is
accepted only if reopening succeeds and all structural assertions pass.

## Runtime Defaults

The default profile uses:

- camera preset: `front_talking`
- presentation mode: `standing`
- render preset: `production_1080p`
- frame rate: 30 fps
- production voice provider: the existing local GPT-SoVITS configuration

Callers may override the presentation or camera preset to request full-body,
seated, or face-close-up coverage. These are camera/presentation choices only;
they never select a different character mesh, rig, material system, groom, or
expression system.

## Validation and Failure Handling

Static validation checks:

- every profile and manifest path is repository-relative and exists;
- every published artifact matches its recorded SHA-256 hash;
- the character master has one formal collection and one formal armature;
- the studio contains required spawn/focus markers and no formal character;
- the profile resolves to the published pair when no explicit profile is given;
- a dry-run render plan imports the character once and exposes the approved
  default and optional camera/presentation presets.

Blender validation checks:

- the master reopens with rig, actions, shape keys, drivers, UV layers,
  materials, groom data, vertex counts, and vertex-order fingerprints intact;
- the studio reopens with cameras, lights, world, markers, and render settings;
- a smoke render completes using the bundled default profile without missing
  datablocks, duplicate characters, or import errors.

If any assertion fails, publication stops. The profile is not changed to the
new pair, and the existing release assets remain the active fallback.

## Verification Outputs

The change produces:

- the two immutable Blender assets;
- `ip形象/main_ip/manifests/default-aroll-assets.json`;
- machine-readable character and studio audit reports;
- an A-roll default-resolution test and manifest-integrity test;
- a smoke-render report and preview frame;
- release documentation identifying the profile as the system A-roll default.

## Non-goals

- No new character modeling, rigging, grooming, or look-development work.
- No camera-specific character proportion changes.
- No external asset downloads or 3D generation services.
- No deletion of earlier release assets or validated checkpoints.

