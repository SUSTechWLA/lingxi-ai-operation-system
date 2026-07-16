# Sloth Warm Studio Integration Design

## Context

The production sloth character and the warm studio were completed on separate
branches. The current character branch contains the newer A-roll master, hand
and facial controls, 1080p production settings, and the pinned local
GPT-SoVITS voice. `codex/warm-sloth-studio` contains a packed empty Blender
room with authored cameras, lights, furniture, props, and character markers,
but it predates those character and voice changes.

This integration starts from the current character pipeline and selectively
imports studio-specific assets. It does not merge the older character profile,
voice policy, or render defaults from the studio branch.

## Goals

- Place the production sloth inside the real Blender studio so character and
  room share lighting, shadows, depth of field, and camera perspective.
- Support standing and seated A-roll as stable reusable production modes.
- Preserve facial animation, visemes, wrists, and independent finger controls
  in both modes.
- Make the character the brightest and clearest subject while retaining a warm,
  detailed knowledge-sharing environment.
- Deliver separate 1080p standing and seated demos plus a combined review reel.
- Expose the mode through the existing `ip_avatar_3d` MCP flow.

## Non-Goals

- Do not create dynamic or generated backgrounds.
- Do not make standing-to-sitting transitions part of the production path.
- Do not replace the current character model, rig, voice bundle, or viseme set.
- Do not restore the studio branch's older 2K or HeyGen defaults.
- Do not add B-roll generation to this integration.

## Branch And Asset Strategy

Work happens on `codex/sloth-warm-studio-integration`, based on the completed
character pipeline. Import only these studio-owned artifacts and their tests:

- packed warm studio `.blend`, preview, and brand image;
- warm studio builder, contract, validator, and QA renderer;
- warm studio documentation and studio-specific tests.

Shared renderer and profile files remain based on the current character branch
and are modified only for the dual-mode contract. The canonical character
master remains the single character source of truth.

## Scene Contract

The packed studio remains an environment asset without embedded character
geometry. It gains explicit mode markers:

- `IP_Standing_Spawn`
- `IP_Standing_Focus_Head`
- `IP_Seated_Spawn`
- `IP_Seated_Focus_Head`
- `IP_Seat_Target`
- `IP_Foot_Target.L`
- `IP_Foot_Target.R`

The existing `IP_Character_Spawn` and `IP_Focus_Head` remain aliases for the
standing mode so older callers continue to work.

Mode-specific cameras are semantic aliases or authored duplicates of the
studio camera set:

- `Camera_Standing_Wide`
- `Camera_Standing_Medium`
- `Camera_Standing_ThreeQuarter`
- `Camera_Seated_Wide`
- `Camera_Seated_Medium`
- `Camera_Seated_ThreeQuarter`

No production camera may intersect furniture, plants, walls, or the character.
The desk remains foreground context rather than an occluding wall across the
character's hands.

## Standing Mode

Standing A-roll places the character behind the desk on the studio center axis.
The default medium frame shows the head, torso, elbows, wrists, and hand
gestures. The wide frame establishes the room and shows the full standing pose
where the desk does not obscure it. The three-quarter frame supports restrained
editorial variation without breaking eye contact.

Standing animation uses the existing talking, nod, emphasis, explain, wave,
micro-gaze, wrist, and finger actions. Feet remain planted during speech; body
sway is limited and must not move hands through the desktop.

## Seated Mode

Seated A-roll uses a dedicated `Aroll_Seated_Idle` base action rather than
scaling or lowering the standing character. The pose bends hips and knees,
places the pelvis over the chair seat, aligns feet with the floor targets, and
keeps the spine upright with a small natural forward lean.

Upper-body speech gestures are layered over the seated base. Head, jaw,
visemes, brows, eye direction, wrists, and independent fingers retain their
existing controls. Seated hand motions use a smaller vertical range and remain
above or beside the desk edge. The chair, legs, feet, desk, and hands must not
visibly intersect in the medium or three-quarter cameras.

Standing-to-seated transition animation is intentionally excluded. Callers
select a mode per shot, which is more reliable for automated production.

## Lighting And Color

The current studio preview is too evenly bright for A-roll. The integrated
profile uses a subject-first warm editorial treatment:

- reduce world and window contribution so the room sits approximately 1 to
  1.5 stops below the character's face;
- use a broad 4300-4600 K camera-left key aimed at the head and upper torso;
- use a restrained neutral fill from camera-right;
- use a 3000-3300 K back/rim light to separate brown fur from wood slats;
- retain 2700 K practical lamps as visible warm accents;
- lower wall, curtain, and desktop highlight intensity before changing
  character materials;
- use AgX with a moderately contrasty look and evidence-based exposure.

Character fur, cream cardigan, mustard hoodie, and eye highlights must retain
texture and color separation. Background practicals may glow but must not clip
into featureless white shapes. The room stays warm without becoming an orange
or beige monochrome image.

## Camera And Render Profiles

The production default remains Eevee Next at 1920x1080, 30 fps, CFR H.264 with
AAC audio. Cycles is used for still comparisons and final look validation, not
for full demo animation unless explicitly requested.

Both modes use restrained depth of field. The eyes and mouth remain sharp,
hands remain acceptably sharp across normal gestures, and the studio background
retains enough detail to communicate a real room.

## MCP Interface

`render_talking_video` accepts an optional presentation mode:

```json
{
  "presentationMode": "standing"
}
```

Allowed values are `standing`, `seated`, and `auto`. `auto` defaults to
standing unless the motion plan explicitly requests a seated delivery. The
resolved mode is written to render input, render report, and returned metadata.

The character profile points to the warm studio as the production scene while
retaining the current 1080p render profile and `gpt_sovits_local` voice. A
caller may still override the scene or presentation mode for a specific shot.

## Data Flow

1. Resolve the current character profile, canonical master, warm studio, voice,
   and requested presentation mode.
2. Open the packed studio and validate its cameras, lights, and mode markers.
3. Import or link the canonical character master once.
4. Scale and place the character at the selected spawn marker.
5. Apply the standing or seated base action, then layer speech, face, gaze,
   hand, and finger animation.
6. Aim the selected camera and subject lights at the selected head target.
7. Render character and studio together in Blender.
8. Compose the validated narration audio and run existing video/audio QA gates.
9. Persist mode, scene, lighting, camera, asset hashes, and QA results.

## Demo Deliverables

- `outputs/Sloth_WarmStudio_Standing_Demo_1080p.mp4`
- `outputs/Sloth_WarmStudio_Seated_Demo_1080p.mp4`
- `outputs/Sloth_WarmStudio_DualMode_Reel_1080p.mp4`
- `outputs/Sloth_WarmStudio_Standing_ContactSheet.png`
- `outputs/Sloth_WarmStudio_Seated_ContactSheet.png`
- `outputs/Sloth_WarmStudio_Lighting_Comparison.png`
- `outputs/Sloth_WarmStudio_Integration_Report.json`

Each individual demo is approximately 20-30 seconds and uses the permanent IP
voice. The combined reel labels the two modes and is a review artifact, not a
separate production mode.

## Validation

Automated validation must cover:

- required scene markers, cameras, lights, and packed resources;
- exact `presentationMode` validation and backward-compatible standing default;
- stable seated action and preserved upper-body/facial channels;
- character-to-desk, character-to-chair, hand-to-desk, and foot-to-floor
  clearance samples across the rendered actions;
- 1920x1080, 30 fps CFR, expected duration, no adjacent exact duplicate frames;
- final encoded audio within the existing loudness and true-peak gates;
- persisted scene, character, voice, mode, and output provenance.

Visual review must include standing and seated medium, wide, and three-quarter
frames. Accept only when the face is the exposure priority, fur and clothing are
not clipped, background lighting is coherent, feet are supported, seated weight
looks plausible, and no visible geometry intersection or deformation spike is
present.

## Failure Handling

- Missing studio or character assets fail before Blender rendering.
- Missing mode markers, cameras, seated action, or required bone roles fail
  closed for the requested mode.
- Collision or floor-clearance failures block publishable output and retain QA
  evidence for diagnosis.
- Final audio-gate failure removes the encoded video before returning an error.
- Demo rendering writes to staging paths and publishes files only after all
  required QA checks pass.

## Acceptance

The integration is complete when both standalone demos and the combined reel
are rendered at 1080p, the character visibly belongs to the studio lighting,
standing and seated poses remain natural through speech gestures, the permanent
IP voice passes audio QA, all automated tests pass, and the integration report
contains reproducible asset and render provenance.
