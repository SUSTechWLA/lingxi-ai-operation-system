# Main IP High-Quality A-roll Master Design

## Goal

Build one curated, reusable master asset for the sloth IP that can sustain presenter medium-close shots, clear hand gestures, expressive talking animation, and a fixed human-like Mandarin voice. Tangying AIOS should turn a script plus a selected scene into a consistent 2K A-roll video without rebuilding or visually drifting the character on every render.

## Confirmed Direction

- Camera quality target: presenter medium-close shot. Hands may approach the face and camera, but extreme hand macro shots are out of scope.
- Character treatment: targeted local retopology and rig refinement, not a full-body rebuild.
- Voice persona: warm Mandarin male, perceived age 28-35, mid-low register, natural knowledge-creator delivery, slight smile, no announcer or childish tone.
- Identity constraint: preserve the silhouette, proportions, clothing, colors, face markings, hairstyle, and overall appearance of `ip形象/main_ip/turnaround/front.png`.

## Current Audit

The current delivery asset is functional but does not meet the selected quality target:

- The Armature contains 46 bones and the hand augmentation contains 12 finger bones: three visible digits per hand, two segments per digit.
- Each digit can be evaluated independently, but current fingertip displacement is too small to read clearly in a presenter shot.
- `Gesture_OpenHand`, `Gesture_Fist`, `Gesture_Count_One`, `Gesture_Count_Two`, `Gesture_Pinch`, and `Gesture_FingerWave` produce different bone values, but their rendered silhouettes remain too similar.
- The conservative curl limit and weak distal-joint contribution prevent a complete fist or convincing pinch.
- Shared palm weights are too broad around the digit roots, so stronger rotation would currently deform the palm as a soft blob.
- The source mouth now has integrated lip boundaries and an oral interior, but the mouth corners need restrained corrective shapes.
- `trueLipTopology` is ready; `trueEyelidTopology` is not.
- Apple `Eddy` is the current production profile voice. Mastering improves loudness and tone but cannot remove its synthetic delivery.

## Asset Architecture

The main IP must be refined once and stored as a stable master. Rendering a new script must not rerun destructive or heuristic retopology.

```text
Source FBX + 4K textures
        |
        v
Curated Main-IP Master Blend
  - refined integrated mesh
  - final Armature and controls
  - hand and face corrective shapes
  - reusable A-roll Actions
  - approved materials
        |
        v
Tangying IP Avatar MCP
  - validate master capabilities
  - understand script beats
  - select and blend gestures
  - apply visemes and expressions
  - bind authored studio cameras/lights
  - synthesize pinned production voice
        |
        v
2K 30fps A-roll video + reusable GLB export
```

The character profile gains a stable `model.masterBlendPath`. When present, Blender appends the named master character collection into the authored studio. The existing FBX/GLB enhancement path remains a fallback for onboarding a new IP, but it must not silently replace the curated main-IP master.

## Hand Geometry And Rig

### Geometry

- Preserve the three-digit sloth hand design shown in the reference.
- Refine only the integrated hand regions of the source character mesh; do not add visible replacement hands or seams at the wrist.
- Add controlled edge density around each digit root and two internal knuckles.
- Add two support loops around each bending zone and retain the authored fingertip silhouette.
- Smooth palm and digit surfaces locally while pinning the wrist boundary and preserving hand volume.
- Keep the existing source texture coordinates wherever possible. New local vertices interpolate existing UVs and weights.

### Bones And Controls

Upgrade each visible digit from two to three deform segments:

```text
Finger_01_Proximal.L/R
  -> Finger_01_Middle.L/R
    -> Finger_01_Distal.L/R
Finger_02_Proximal.L/R
  -> Finger_02_Middle.L/R
    -> Finger_02_Distal.L/R
Finger_03_Proximal.L/R
  -> Finger_03_Middle.L/R
    -> Finger_03_Distal.L/R
```

This yields 18 finger deform bones total. Semantic roles expose proximal, middle, and distal controls for every digit and side. The lower digit receives a small opposition range so pinch and grip poses can close toward the palm rather than only curl parallel to the other digits.

### Weighting

- Recalculate digit membership after local hand refinement.
- Use a narrow palm-to-proximal blend, a centered proximal-to-middle blend, and a centered middle-to-distal blend.
- Keep at most four influences per vertex and normalize every weighted vertex.
- Preserve volume on the Armature modifier.
- Prevent one digit from materially moving the neighboring fingertip region.
- Add restrained corrective Shape Keys for open hand, closed fist, pinch, and the strongest pointing poses when bone deformation alone cannot preserve the palm.

### Hand Controls

Expose deterministic custom properties or controller channels:

- `curl_1`, `curl_2`, `curl_3`
- `splay_1`, `splay_2`, `splay_3`
- `opposition_3`
- `hand_open`
- `hand_fist`
- `hand_relax`

Every individual curl channel must be independently keyable. Aggregate controls drive the same channels through drivers or generated keyframes; they must not hide per-digit access.

## Face And Surface Detail

### Face

- Preserve the source face texture and real lip geometry.
- Refine the existing mouth boundary weights and add restrained mouth-corner correctives for `Smile`, `Frown`, `E`, and `MBP`.
- Keep jaw rotation for large opening and Shape Keys for lip articulation.
- Add true local eyelid loops and left/right blink Shape Keys that close over the eye without scaling the entire eye region.
- Keep existing eye bones for gaze and use Shape Keys only for eyelid contact and eye-wide poses.
- Avoid visible overlay geometry, replacement mouth cards, or texture patches.

### Materials

- Reuse the existing 4K source textures and color palette.
- Add low-strength material-specific micro-normal and roughness variation for fur, knit fabric, hoodie, trousers, eyes, and nose.
- Keep eye and nose highlights controlled under the authored studio key light.
- Do not derive strong geometric normals directly from albedo details.
- Retain AgX and the authored studio lighting as the production reference.

## A-roll Action Library

Create or refine the following reusable Actions:

- `Aroll_Idle_Listening`
- `Aroll_Greeting_Wave`
- `Aroll_OpenPalm_Explain`
- `Aroll_Explain_Left`
- `Aroll_Explain_Right`
- `Aroll_Count_One`
- `Aroll_Count_Two`
- `Aroll_Count_Three`
- `Aroll_Point_Left`
- `Aroll_Point_Right`
- `Aroll_Pinch_Detail`
- `Aroll_Emphasis_SoftFist`
- `Aroll_Think`
- `Aroll_Agree_Nod`
- `Aroll_Disagree_Shake`
- `Aroll_Transition_Reset`

Actions use a shoulder-to-elbow-to-wrist-to-digit motion chain. Hands must remain outside the face-safe region unless the action intentionally touches the chin. Legs and hips receive restrained weight shifts so full-body framing does not look frozen.

The MCP maps script beats to these named Actions and blends back through `Aroll_Transition_Reset`. It may add low-amplitude breathing, gaze, blink, and antenna-like hair motion, but it must not stack incompatible hand gestures on the same arm.

## Pre-Shoot Reel

Before approving the master, render a 2K, 30fps action reel in the authored editorial studio:

1. Neutral idle and listening.
2. Camera-facing greeting wave.
3. Open-palm explanation.
4. Count one, two, and three.
5. Left and right pointing.
6. Pinch/detail gesture.
7. Soft-fist emphasis.
8. Thinking pose.
9. Agreement and disagreement.
10. Full-body weight shift and reset.

The reel includes medium-close and full-body cuts. A separate hand QA reel shows open, closed, and one-digit-at-a-time motion from a stable camera.

## Production Voice

### Persona

- Warm Mandarin male.
- Perceived age 28-35.
- Mid-low register with natural breath and a slight smile.
- Conversational knowledge-creator delivery.
- Clear opinions without broadcaster projection, cartoon exaggeration, or excessive enthusiasm.

### Provider Policy

- Production providers: HeyGen Starfish or ElevenLabs multilingual.
- Preview-only providers: Apple and Kokoro.
- The selected provider and `voiceId` are pinned in the character profile.
- Production rendering fails clearly when the pinned provider is unavailable. It must never silently substitute Apple or Kokoro.

### Audition And Mastering

- Generate three candidates using the same 15-20 second Mandarin script.
- Normalize auditions to comparable loudness and hide provider/voice labels during selection.
- Pin the selected provider and voice ID after user approval.
- Apply only light high-pass filtering, de-essing, compression, and loudness normalization.
- Target approximately `-16 LUFS` integrated and at most `-1.5 dBTP` true peak.
- Do not use heavy EQ, pitch shifting, or time stretching to disguise a synthetic source.

The current shared HeyGen credential exists, but provider requests timed out during design preflight. Auditions begin only after provider connectivity succeeds; no low-quality local sample is accepted as a substitute.

## MCP Interfaces And Data Flow

### Character Preparation

Add a one-time character preparation path for the curated main IP:

```text
prepare_character_master(sourceModel, characterProfile, qualityTier="aroll_close")
  -> masterBlendPath
  -> exportGlbPath
  -> rigReportPath
  -> handQaPath
  -> faceQaPath
```

For unknown IP models, the preparation path first detects existing finger, face, and humanoid controls. Conservative augmentation is allowed when the geometry is suitable. Aggressive local retopology requires an explicit character-specific preparation profile.

### Video Rendering

The existing talking-video tool consumes the master:

```text
render_talking_video(
  script,
  characterProfilePath,
  sceneBlendPath,
  outputDir,
  qualityPreset="production_2k"
)
```

Data flow:

1. Validate master collection, Armature, hand semantics, visemes, Actions, materials, and cameras.
2. Resolve the pinned production voice and synthesize narration.
3. Convert script clauses into expression and gesture beats.
4. Build a non-conflicting motion timeline from the A-roll Action library.
5. Apply visemes, jaw, blinks, gaze, and corrective shapes.
6. Append the master collection into the authored scene and bind cameras/lights.
7. Render 2K PNG frames, encode constant-frame-rate video, and mux approved audio.
8. Emit validation, render, motion, and voice provenance reports.

## Failure Handling

- Missing or incompatible master Blend: stop before rendering and report the missing collection or capability.
- Unsupported hand topology on a new IP: return a partial preparation report; do not invent visible replacement hands.
- Unweighted vertices, more than four influences, or missing digit roles: fail master validation.
- Self-intersection or excessive hand deformation in required poses: fail hand QA and retain the previous approved master.
- Missing production voice credential or provider timeout: fail production audio generation; do not silently downgrade.
- Missing authored camera: report the camera name and use no unreviewed automatic framing in production mode.

## Testing And Acceptance

### Automated Rig Tests

- Both hands expose three digits with proximal, middle, and distal roles.
- Every digit can move through its independent channel while non-selected fingertips remain within a small stability tolerance.
- Open-to-fist fingertip displacement is at least 25% of that digit's rest length.
- Open, fist, pinch, count, and point poses keep normalized weights, at most four influences, and zero unweighted vertices.
- Required Action names, face Shape Keys, eye/jaw bones, and production cameras exist.

### Render Tests

- Compare stable hand crops for open, fist, pinch, counts, and independent digits.
- Require a clearly different open/fist silhouette and no visible spikes, palm collapse, wrist seam, or neighboring-digit drag.
- Render face crops for neutral, happy, serious, blink, `A`, `E`, `O`, and `MBP`.
- Render the complete A-roll reel at 2560x1440, 30fps, constant frame rate, with no adjacent exact duplicate frames.
- Review medium-close and full-body contact sheets against the reference image.

### Voice Tests

- Production metadata reports a pinned HeyGen or ElevenLabs provider and voice ID.
- Audio contains no clipping and meets the mastering target.
- Apple/Kokoro output is rejected in production mode.
- Final voice acceptance requires the user's blind audition choice; automated audio metrics alone cannot prove human likeness.

## Deliverables

- `outputs/MainIP_Sloth_Aroll_Master.blend`
- `outputs/MainIP_Sloth_Aroll_Rigged.glb`
- `outputs/MainIP_Sloth_Aroll_Rig_Report.json`
- `outputs/MainIP_Sloth_Aroll_Action_Reel_2K.mp4`
- `outputs/MainIP_Sloth_Aroll_Hand_QA_2K.mp4`
- `outputs/MainIP_Sloth_Aroll_Face_QA.png`
- `outputs/MainIP_Sloth_Voice_Auditions/`
- Updated `ip形象/main_ip/character-profile.json` with the approved master and production voice.

## Non-Goals

- Extreme hand macro-shot quality.
- Full-body production retopology.
- Replacing the sloth's three-digit anatomy with a five-finger human hand.
- Dynamic or AI-generated A-roll backgrounds.
- Silent production fallback to preview voices.
- Re-running character retopology for every video render.
