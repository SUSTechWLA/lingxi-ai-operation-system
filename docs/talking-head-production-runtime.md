# Talking-head production runtime

This document records the production execution path audited on 2026-07-10. It is intentionally about runtime reachability, not similarly named files.

## Canonical identity

- Canonical profile: `talking_head`.
- Persisted legacy project mode: `voice_visual`.
- Runtime pipeline identity: `dynamic-agent-video-creation@2.0`.
- Runtime DAG source of truth: `internal/core/agentruntime.PlanCompiler` (`PreparePlan`, profile completion, then `Compile`).
- Deterministic text renderer name: `HyperFrames`. `HyperGen` is not a separate artifact kind or renderer.

`model.NormalizeVideoProfileID` accepts historical values (`voice_visual`, `knowledge-video`, `guided-image-text-video`, and `wf-guided-image-text-video`) and maps them to `talking_head`. Project rows keep their old `mode`; reads and config metadata expose the canonical profile so old data is not rewritten destructively.

## Real execution path

1. `DirectorStudioPage.handleStart` creates a video project and starts `/api/agent/runs` in `dynamic_agent` mode.
2. The agent runtime planner produces an initial plan.
3. `PlanCompiler.PreparePlan` normalizes the profile and completes the talking-head plan.
4. The compiler inserts `profile_selection -> script_generation -> audio_master -> time_window -> visual_alignment -> shot_generation -> ... -> render`.
5. The orchestrator runs the compiled DAG and dispatches registered Native/MCP/local commands.
6. `audio_master_planner` emits `AUDIO_MASTER_TIMELINE`; `time_window_planner` binds every window to that revision.
7. `visual_alignment_planner` emits the existing Shot payload with Audio/IP/Text/B-roll/Composition layer plans plus a separate `BROLL_MANIFEST`.
8. Artifact materialization stores the profile, audio master, time-window plan, and B-roll manifest as reviewable structured artifacts.
9. Review gates approve the structured plans; the current dynamic path then builds/renders the HyperFrames composition and performs whole-video frame QA.
10. Director Studio renders the runtime trace/artifact state and does not construct a separate execution DAG.

`RecordShotCandidateQA`, scoped repair, accepted-shot invalidation, and `BuildFinalAssemblyPlanWithPolicy` now enforce the production contract when called, but the current dynamic worker path does not yet materialize one `ShotCandidate` per rendered layer set or call that Final Assembly service. They are therefore not described as part of the reached runtime chain.

## Similar pipeline definitions

| Definition | Runtime role | Status |
|---|---|---|
| `internal/core/agentruntime.PlanCompiler` | Director Studio dynamic-agent execution DAG | canonical |
| `video-pipelines/knowledge-video.yaml` | loaded only by `pipeline_selector` as proposal/capability metadata | adapter/selection metadata |
| `internal/core/workflow/seed_video.go` | seeds the separately invokable static workflow template `wf-guided-image-text-video` | legacy workflow product, not Director Studio runtime |
| `configs/pipelines/guided-image-text-video.yaml` | no loader was found on the Director Studio call path | dead configuration path |
| frontend `preflightPipeline` | capability-query alias; never selects the execution DAG | preflight adapter |

The static workflow remains readable for existing callers. New talking-head stages must be added to `PlanCompiler` and its tool manifests; do not copy the DAG into another large YAML or seed definition.

## Audit matrix

| Capability | Status | Actual entry | Runtime reached | Problem / boundary |
|---|---|---|---|---|
| profile routing | DONE | `NormalizeVideoProfileID`, `PlanCompiler.PreparePlan` | yes | legacy aliases remain adapters |
| audio master timeline | DONE | `audio_master_planner`, `BuildAudioMasterTimeline` | yes | real speech alignment still provider-dependent |
| IP layer | PARTIAL | `TalkingHeadShotLayers.IP`, visual alignment artifact | yes, planning | no production IP provider E2E or asset-pack registry UI yet |
| deterministic text layer | PARTIAL | HyperFrames tools and `TalkingHeadShotLayers.Text` | yes | layout/source-manifest QA is not complete |
| B-roll layer | PARTIAL | visual alignment + `BROLL_MANIFEST` | yes | replacement endpoint and license UI remain incomplete |
| Shot QA | PARTIAL | `RecordShotCandidateQA` | model/service path | provider metrics and visual-model checks are incomplete |
| scoped repair | DEAD_PATH | `BuildRepairPlanForQA`, layer invalidation graph | no | the dynamic worker does not yet dispatch these repair actions |
| accepted gate | DEAD_PATH | `RecordShotCandidateQA` | no | strict contract is implemented/tested but not called by the dynamic worker |
| final assembly | DUPLICATED | `BuildFinalAssemblyPlanWithPolicy` and whole-video render path | partial | candidate assembly service is not the current renderer path |
| stale propagation | DEAD_PATH | `InvalidateTalkingHeadShot` | no | mutation endpoints do not yet invoke the dependency graph |
| provenance | PARTIAL | layer state, candidate execution mode, artifact materializer | yes | cost/provider-job data varies by provider |
| fallback gate | PARTIAL | local render provenance and artifact materializer; candidate/final gates | partial | reached artifact gate works; candidate gates are not worker-wired |
| Director Studio | PARTIAL | page + talking-head layer feature | yes | audio waveform, animatic, and revision comparison remain incomplete |

## Time and dependency contract

The canonical internal unit is `int64` milliseconds. Legacy `startSec`, `endSec`, and `durationSec` fields remain serialization adapters. `NormalizeShotTiming` migrates old JSON without deleting the legacy fields.

The dependency direction is:

`script revision -> voice revision -> audio master revision -> sentence/word alignment -> shot windows -> text/IP/B-roll timing -> composition -> candidate -> QA -> accepted state -> final assembly`.

A script, audio master, text style, B-roll replacement, or IP motion edit is routed through the explicit invalidation table in `service/talking_head_layers.go`. Accepted candidates are marked stale and unaccepted; old artifacts remain available for comparison.

## Strict production boundary

`executionMode` is one of `real`, `fixture`, `fallback`, or `placeholder`. Non-real modes are never production eligible. The strict accepted-shot and Final Assembly gates reject them even when the media file is playable or a QA fixture says it passed. Draft mode is limited to inspection and diagnostics.

No real TTS/alignment/IP generation provider was exercised in this audit. Estimated timelines and fixtures are labelled as such; missing provider capability must be satisfied by a configured tool or an explicit manual import.
