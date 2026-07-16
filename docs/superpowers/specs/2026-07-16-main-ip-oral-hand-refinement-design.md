# Main IP Oral And Three-Digit Hand Refinement Design

## Context

The curated sloth A-roll master already has the required body rig, jaw, three
tongue bones, wrist controls, and eighteen finger deform bones. Production QA
shows two remaining close-shot quality problems: the upper and lower teeth read
as separate beads, and the three-digit hands read as thick wedges with abrupt
digit-root transitions. The tongue is a single low-resolution ellipsoid and
does not read as a designed oral surface when open vowels expose the cavity.

This pass improves the visible geometry rather than adding controls or replacing
the source character. It preserves the real source lips, face texture, body,
clothing, proportions, materials, humanoid rig, A-roll actions, and three-digit
sloth anatomy.

## Goals

- Replace the fragmented tooth clusters with restrained continuous upper and
  lower dental arches.
- Add a coherent oral cavity, subtle gum transitions, and a smooth articulated
  tongue with a readable tip and shallow center groove.
- Preserve lip closure at rest and expose teeth and tongue only when the jaw and
  viseme justify it.
- Refine the source three-digit hand silhouette, digit taper, joint transitions,
  and palm-to-digit flow without changing the approved finger rig.
- Retain independent three-segment motion for every digit and natural wrist
  rotation in greeting, open-palm, fist, pinch, count, and point poses.
- Produce render evidence from the production camera before replacing the
  approved master asset.

## Non-Goals

- Do not convert the hands to five human fingers.
- Do not replace the real source mouth with a card, curve, texture patch, or
  detached facial overlay.
- Do not rebuild the full face, head, body, clothing, source UVs, or skeleton.
- Do not add individually separated realistic teeth, gums with anatomical
  detail, saliva simulation, or a dental close-up rig.
- Do not change the permanent voice, studio scene, render resolution, or A-roll
  script planner in this pass.

## Selected Approach

Use targeted local retopology and deterministic Blender Python generation.
Internal oral objects are regenerated from continuous parametric surfaces. The
source hand mesh remains integrated with the body; only vertices already
classified as palm and digits are shaped, smoothed, and reweighted. Every
operation is applied to a copied production asset and must pass automated and
render QA before publication.

This approach provides a visible quality improvement while avoiding the identity
drift and UV transfer risk of a complete face-and-hand rebuild.

## Oral Geometry

### Dental Arches

`IP_UpperTeeth` and `IP_LowerTeeth` become one connected rounded band each:

- a shallow elliptical arc follows the source mouth width;
- the front surface is smooth and continuous with no per-tooth seams;
- the upper band is dominant and the lower band is thinner and mostly hidden;
- rounded end caps prevent the arches from reading as cut strips;
- the upper arch follows `Head`; the lower arch follows `Jaw`;
- both use smooth shading and a restrained subdivision level suitable for a
  presenter close shot;
- the material is warm ivory rather than emissive or pure white.

The arches sit behind the source lip boundary in rest and MBP poses. They may be
visible in A, E, O, U, Smile, and Surprise only to the degree allowed by jaw
opening and lip shape.

### Oral Cavity And Gums

The flat cavity disk becomes a shallow concave oral cup. A restrained gum rim
bridges the dark cavity and each dental arch without becoming a visible pink
outline at normal framing. The cavity remains dark burgundy with high roughness
so it provides depth without reflecting studio lights as a black mirror.

### Tongue

`IP_Tongue` becomes one connected rounded surface with:

- a wider rooted body, controlled midsection, and softly tapered tip;
- a shallow longitudinal groove created by geometry, not a painted line;
- enough longitudinal rings to deform smoothly under `Tongue_01` through
  `Tongue_03`;
- normalized overlapping weights across the three tongue bones;
- soft rose-brown material, moderate roughness, and a small restrained specular
  response.

The tongue rests below the lower dental arch. Its authored motion range may not
cross the teeth, lips, or oral-cavity boundary in the production visemes.

## Mouth Performance

The source lip Shape Keys and jaw remain canonical. Refinement adjusts their
readability rather than replacing them:

- `Mouth_Rest` and `Mouth_MBP` keep complete lip closure and hide oral geometry;
- `Mouth_A` increases vertical opening and exposes the tongue body;
- `Mouth_E` widens the lips while limiting lower-teeth exposure;
- `Mouth_O` and `Mouth_U` keep a rounded opening and show oral depth without a
  bright dental stripe;
- `Mouth_Smile` may show the upper arch but does not show both full arches;
- `Mouth_Surprise` uses the largest cavity exposure but remains collision-free.

The system retains viseme attack and release smoothing and does not alter audio
timing.

## Three-Digit Hand Refinement

The existing three digits and three bone segments per digit remain unchanged.
Local source-surface refinement applies these visual rules:

- preserve the wrist boundary and overall palm volume;
- narrow the palm-to-digit valleys and smooth the digit-root transition;
- taper each digit gradually toward a rounded tip;
- retain two readable bend zones without sharp tubular hinges;
- reduce asymmetric lumps while keeping the IP's soft sloth character;
- add only a subtle integrated nail/claw plane when source UV and topology can
  support it without a seam;
- keep neighboring digit silhouettes separated in open and count poses;
- keep a compact, soft silhouette in fist and pinch poses.

Weight refinement uses narrow proximal blending and centered middle/distal
bands. Each weighted vertex remains normalized, has at most four deform
influences, and does not materially drive an adjacent fingertip.

## Materials And Shading

- Teeth: warm ivory, roughness 0.40-0.52, restrained specular, no emission.
- Gums: muted rose-brown, visually subordinate to lips and tongue.
- Tongue: desaturated rose, roughness 0.44-0.58, restrained wet highlight.
- Hands: preserve the source material and UVs; geometric smoothing must not
  erase the existing fur or color identity.
- All new organic surfaces use smooth shading and production-safe subdivision;
  normal smoothing must not introduce dark seams.

## Non-Destructive Build

The source FBX and current approved master remain untouched. The build writes a
new staged Blend first, validates it, then publishes a new master only after all
gates pass. Existing internal oral objects are replaced only in the staged copy.
The previous master remains recoverable as a timestamped backup or immutable
source artifact.

## Automated Validation

Tests must verify:

- each dental arch is one connected mesh component with no duplicated tooth
  islands and a width-to-height ratio appropriate for a dental band;
- upper teeth are bound only to `Head`, lower teeth only to `Jaw`;
- the tongue is one connected component with sufficient longitudinal rings,
  three normalized tongue-bone influences, and a tapered tip;
- rest and MBP hide the dental arches from the production camera;
- open visemes expose distinct cavity, tooth, and tongue areas;
- both hands retain exactly three independently controlled digit chains with
  proximal, middle, and distal segments;
- open, fist, pinch, count, point, and wave poses preserve volume, avoid severe
  self-intersection, and remain within four influences per vertex;
- no source material, UV layer, required Action, or canonical rig role is lost.

## Render Validation

Render fixed 1080p evidence from the approved warm-studio look:

- neutral, MBP, A, E, O, U, Smile, and Surprise face crops;
- relaxed, open, fist, pinch, count one/two/three, point, and camera-facing wave
  hand crops from both sides where relevant;
- a contact sheet comparing the previous and refined master;
- a 15-30 second front-facing A-roll demo using the existing permanent voice.

Visual review rejects bead-like teeth, a flat or spherical tongue, exposed teeth
at rest, lip/teeth/tongue intersections, finger pinching, wrist seams, collapsed
palms, and hand silhouettes that look like five-finger human hands.

## Deliverables

- `ip形象/main_ip/models/main-ip-aroll-master-refined.blend`
- `outputs/MainIP_Sloth_Oral_Hand_QA.png`
- `outputs/MainIP_Sloth_Oral_Hand_Comparison.png`
- `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`
- machine-readable oral, hand, render, and media QA reports.

## Acceptance

The pass is accepted when the teeth read as restrained continuous dental arches,
the tongue reads as one intentional articulated surface, all required visemes
remain distinct and collision-free, and the three-digit hands look smoother and
more graceful while retaining their existing independent motion. The refined
master must pass the complete Blender regression suite and production-camera
visual review before it replaces any approved asset path.
