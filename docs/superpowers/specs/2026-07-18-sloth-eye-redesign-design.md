# Stylized Sloth Eye Redesign

## Objective

Replace the toy-like appearance of the current independent `CXR_` eye layers with the softer, deeper construction seen in `target_character.png`, without changing the production character mesh, vertex order, UVs, original Shape Keys, Armature, vertex groups, modifiers, actions, frame timing, or production cameras.

## Approved Direction

Use a targeted rebuild of the independent `CXR_` eye system. The original fused eye region remains untouched and continues to be masked by the reversible `CXR_EyeSocket` material assignment. No source `.blend` is overwritten.

## Visual Design

### Proportions and Placement

- Reduce visible sclera width by approximately 10–12 percent and height by approximately 16–20 percent.
- Move both eyes slightly inward to reduce the overly wide gaze.
- Recess the sclera several millimeters into the socket while keeping the iris layers readable at the front surface.
- Preserve bilateral symmetry and the existing `Eye.L` / `Eye.R` bone parenting.
- Keep the iris large relative to the reduced sclera so the remaining white area reads as a controlled almond rather than a circular disc.

### Eyelids and Socket

- Replace the angular upper-lid curve with a smooth, thick warm-brown arc.
- Use a stronger upper lid and a much subtler lower transition, matching the target character's dark upper lash line.
- Change the socket response from near-black to a deep warm brown with higher roughness, eliminating the hard black sticker-like outline.
- Keep lower-lid, tearline, and construction helpers hidden unless they improve the final silhouette without forming a second ring.

### Iris, Pupil, Corneal Read and Highlights

- Retain separate limbal ring, amber iris, inner iris, pupil, and catchlight objects.
- Darken and slightly enlarge the limbal ring; add controlled amber tonal separation between the outer and inner iris.
- Enlarge the pupil modestly and keep it dark and glossy.
- Do not re-enable the current transmission-based cornea if it renders black in EEVEE. Obtain corneal read through material coat, layered highlights, and shallow depth separation.
- Add one larger soft-white upper-left catchlight and one smaller secondary catchlight per eye.
- Avoid metallic, neon-orange, or glass-marble appearance.

## Expression and Driver Compatibility

- Preserve the existing `Eye_Squint.L/R` and `Eye_Wide.L/R` driver inputs.
- Rebuild driver expressions only where the revised base scale requires it.
- Squint must reduce the visible vertical eye opening without inversion or intersection.
- Wide-eye must expand the opening modestly without recreating the current circular stare.
- Original gaze, blink, smile, mouth, and action data remain unchanged.

## Blender Objects in Scope

- `CXR_Eye.L/R`
- `CXR_IrisRing.L/R`
- `CXR_Iris.L/R`
- `CXR_IrisInner.L/R`
- `CXR_Pupil.L/R`
- `CXR_Catchlight.L/R`
- `CXR_Cornea.L/R`
- `CXR_LidUpper.L/R`
- `CXR_LidLower.L/R`
- `CXR_Tearline.L/R`
- `CXR_EyeSocket`, `CXR_Sclera`, `CXR_IrisRing`, `CXR_IrisAmber`, `CXR_IrisGold`, `CXR_Pupil`, `CXR_Catchlight`, `CXR_Eyelid`, and `CXR_Cornea` materials
- New objects, if required: `CXR_CatchlightSecondary.L/R`

The main mesh `part_00000001.001`, its materials' original source copy, and all non-eye refinement objects are out of scope.

## Execution and File Safety

1. Open the current canonical refined checkpoint.
2. Save a timestamped `pre_gate6_eye_redesign` checkpoint with `copy=True`.
3. Execute one idempotent Blender script from `blender_scripts/generated/60_gate6_eye_redesign.py`.
4. Render neutral, smile, blink, wide, gaze-left, gaze-right, front three-quarter, and production-studio eye reviews.
5. Iterate only on `CXR_` eye transforms, curves, drivers, and materials.
6. Save a new final character checkpoint and a new versioned production-studio copy. Never overwrite either original source file.

## Validation and Acceptance

- No full black outline around the lower eye.
- No large white crescent or circular startled stare at neutral.
- Iris and pupil remain centered and symmetrical in neutral view.
- Dark upper lid, warm amber iris, dark pupil, and two controlled highlights are visually distinct.
- Blink, wide, left gaze, right gaze, smile, wave, fist, wrist twist, and both-arms-forward tests render without detached layers or intersections.
- Main mesh counts stay at 5478 vertices, 11336 edges, and 5846 faces.
- Basis and UV hashes match Gate 0.
- All 26 original Shape Key hashes and order remain unchanged.
- All 43 vertex groups, 52-bone hierarchy, 60 actions, and the original modifier stack remain unchanged.
- The original character and studio `.blend` metadata remain unchanged.

## Known Risk Controls

- Transform drivers can retain old embedded base-scale constants; the script will recreate only the `CXR_` eye-layer scale drivers after setting the new rest proportions.
- Excessive recess can cause socket occlusion; close-up front and three-quarter renders will be reviewed before saving the approved checkpoint.
- EEVEE transmission can produce black corneas; the cornea remains hidden unless a neutral test proves it clean.
- The fused source eye texture may show through if the reversible socket mask is removed; the mask assignment will be retained and only its material response adjusted.
