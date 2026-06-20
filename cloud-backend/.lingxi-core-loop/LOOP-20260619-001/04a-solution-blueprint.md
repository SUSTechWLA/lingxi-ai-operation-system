# 04a Solution Blueprint

## Runtime Shape

```text
Codex skill source docs
→ production skill package under skills/<name>/1.0.0
→ skillruntime loads manifest and validates stage files
→ workflow compiler emits DAG
→ workflow template registered with stable ID
→ orchestrator runs DAG
→ external tool bridge calls registered tools
→ CONTROL nodes pause for review
→ artifacts/trace record outputs and evidence
```

## Business Skill Mapping

- `create-opinion-videos`: viewpoint dossier → recording script review → HyperFrames reference review → optional image assets review → build → render → render review.
- `film-shot-reconstruction`: source prepare → motion analysis → reconstruction plan review → prompt review → reference image review → keyframe review → storyboard review → package → asset guard → optional material library import review.
- `video-creator`: IP/story → script/profiles → material match → asset plan review → asset generation review → SHOT design review → prompts/keyframes review → storyboard review → video generation → director cut review → final assembly → final review.
- `voice-post-production`: audio intake → voice processing → quality review → artifact package.

## Stable Runtime Principle

The skill method becomes deterministic stage instructions and workflow gates. Non-deterministic media operations are external tools with schema/timeout/retry/trace contracts.
