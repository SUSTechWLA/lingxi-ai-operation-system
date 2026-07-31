# Sloth Reference-Convergence Design

## Intent

Continue from `ip形象/main_ip/character_sloth_final.blend` and improve the same formal character asset toward the visual language of `target_character.png`: a compact, appealing commercial mascot with a soft facial mask, full muzzle and cheeks, layered expressive eyes, controlled fur, convincing knitwear, and a reference-like frontal portrait.

## Approved direction

The user has already approved continuous autonomous implementation. The pass therefore uses the safest of three possible approaches:

1. **Recommended: conservative convergence pass.** Preserve the production mesh and rig, refine independent formal components, materials, Groom, and review-camera framing, and use only reversible deformation where the existing topology can support it.
2. **Aggressive head rebuild.** Could approach the reference silhouette more closely, but would require topology, UV, weight, and expression migration and is outside this pass because it risks the production asset.
3. **Camera-only likeness.** Fast but insufficient; it would hide rather than solve model-quality gaps and violate the requirement that proportions work from multiple views.

## Visual design

- Make the face read as soft brown fur surrounding an off-white muzzle/mask, not a flat white decal.
- Preserve the corrected recessed globe construction while improving upper-lid elegance, iris hierarchy, catchlights, and gaze.
- Increase perceived cheek and muzzle volume through safe corrective deformation and soft material/fur transitions, not topology changes.
- Keep the recognizable outfit while adding restrained knit, seam, ribbing, button, and fabric-scale response.
- Match the target's frontal product-photography feel with an 80 mm camera, compact headroom, eye-line placement, neutral warm-white background, and large soft lights.
- Validate front, three-quarter, profile, back, blink, smile, wide eyes, mouth open, and representative rig poses.

## Asset protection

- Do not change `GEO_HeadBody` vertex count or vertex order.
- Preserve UV layers, original Shape Key order/deltas, Armature hierarchy, Actions, drivers, vertex groups, and modifier relative order.
- Use one formal `COL_CHR_SLOTH_FINAL`, one `RIG_Sloth`, one material system, one Groom system, and one expression system.
- Save a checkpoint before each coherent mutation and remove temporary or superseded objects before final delivery.
- Every Blender Python script is saved under `ip形象/main_ip/scripts/` before MCP execution and writes a report under `ip形象/main_ip/reports/`.

## Acceptance

- The frontal render is recognizably closer to the reference in head framing, eye appeal, muzzle softness, facial-mask transition, and outfit readability.
- The eye globes remain contained by the sockets in frontal, three-quarter, and strict profile views; Blink closes cleanly with matching skin/fur color.
- No production invariant regresses, and final validation passes with zero temporary objects or duplicate formal systems.

