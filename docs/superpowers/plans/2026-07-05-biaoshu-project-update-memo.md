# Biaoshu Project Update Memo

> Date: 2026-07-05  
> Scope: Biaoshu staged artifact generation, scoring breakdown, outline quality, and future agent-based context selection.

## Decision Summary

We will first add `02_评分标准拆解表.md` as a deterministic stage artifact, then make `03_技术标四级大纲.md` reference `00 + 01 + 02`.

We will not immediately introduce a fully autonomous agent for artifact generation. Instead, the next architecture phase will add a constrained context-selection agent that can choose previous artifacts under explicit rules, token budgets, and quality gates.

## Why This Split Is Safer

The current outline problem has two causes:

- The outline stage does not consistently reference a dedicated scoring breakdown artifact.
- The generation pipeline does not verify that the model output is complete before marking it valid.

Adding `02_评分标准拆解表.md` fixes the most direct quality issue with limited blast radius. It gives the model a structured scoring map before generating the outline and aligns the system with the `biaoshu-writer` skill's staged artifact requirements.

Agent-based context selection is valuable, but making it the first fix would combine several risks at once: autonomous artifact choice, prompt growth, path safety, token budgeting, and quality evaluation. Those should be introduced after the deterministic stage chain is stable.

## Current State

Implemented or partially implemented stages:

| Stage | Artifact | Status | Notes |
|---|---|---|---|
| 00 | `00_招标文件解析报告.md` | Exists | Generated from raw tender text. |
| 01 | `01_项目背景信息确认表.md` | Exists | Uses user/project context answers. |
| 02 | `02_评分标准拆解表.md` | Missing | Next implementation target. |
| 03 | `03_技术标四级大纲.md` | Exists, incomplete in sample | References 00 and 01, but not a dedicated 02. |
| 04 | `04_章节写作任务书.md` | Not implemented | Needed before chapter writing. |
| 05 | `05_知识库检索报告.md` | Not implemented | Required when RAG is enabled. |

## Near-Term Plan

### Phase 1: Scoring Breakdown Artifact

Goal: Add `02_评分标准拆解表.md`.

Deliverables:

- Backend endpoint `POST /api/biaoshu/scoring-breakdown/generate`.
- Frontend API function `generateScoringBreakdown`.
- Workbench button or stage action: `生成评分拆解表`.
- Artifact kind `BID_SCORING_BREAKDOWN`.
- Path helper: `deriveBiaoshuScoringBreakdownPath`.
- Outline generation passes `scoringReportPath`.

Quality expectations:

- Scoring breakdown must include scoring item, score, required evidence, response strategy, recommended outline mapping, and risk notes.
- It must not invent score values or required proof materials.
- If scoring information is missing, it must mark the information as `招标文件未明确`.

### Phase 2: Outline Completeness Gate

Goal: Prevent incomplete outlines from being saved as valid artifacts.

Deliverables:

- Backend validator for outline structure.
- Minimum required sections derived from scoring breakdown.
- Detection for malformed heading joins, such as a level-one heading appended to the previous paragraph.
- Clear error when the model output is incomplete.

Quality expectations:

- Output must include all scoring-related chapter groups.
- Output must contain valid Markdown heading line breaks.
- Output must end with a short generation completeness checklist or pass an internal structural check.

### Phase 3: Chapter Task Book

Goal: Add `04_章节写作任务书.md`.

Deliverables:

- Generate chapter writing tasks from `03_技术标四级大纲.md`.
- Each chapter task includes input artifacts, target score item, writing focus, evidence needs, forbidden content, and estimated word count.
- User can review and modify the task book before chapter drafting.

Quality expectations:

- Each chapter has clear dependencies.
- Each chapter maps back to scoring items.
- Chapter tasks avoid price information in the technical-business bid.

## Future Agent-Based Context Selection

### Target Component: ArtifactContextBuilder

Purpose: Build a controlled context package before each artifact generation.

Inputs:

- Target artifact kind, such as `BID_OUTLINE` or `BID_CHAPTERS`.
- Current project artifact list.
- Stage rules from the Biaoshu skill profile.
- Token budget.
- User-selected overrides when available.

Outputs:

- Ordered context sections.
- Source artifact references.
- Included excerpt length.
- Excluded artifacts and reason.

Example for outline generation:

| Priority | Artifact | Include Mode | Reason |
|---:|---|---|---|
| 1 | `00_招标文件解析报告.md` | Structured sections only | Tender facts and requirements. |
| 2 | `01_项目背景信息确认表.md` | Full or compacted | Local project context. |
| 3 | `02_评分标准拆解表.md` | Full | Direct scoring-to-outline mapping. |
| 4 | Previous outline revisions | Latest approved version only | Preserve user edits. |
| 5 | Raw tender text | Excerpt only when needed | Avoid prompt overload. |

### Target Component: StageArtifactAgent

Purpose: Decide what to read for a generation stage, but remain constrained by product rules.

Allowed actions:

- List artifacts in the active Biaoshu project.
- Read local Markdown artifacts inside allowed roots.
- Select relevant artifacts from a whitelist.
- Summarize or excerpt selected artifacts.
- Produce a context manifest.

Disallowed actions:

- Read arbitrary filesystem paths.
- Include price bid content in technical bid prompts.
- Override user-approved artifacts without review.
- Mark generated artifacts valid without quality gates.

### Agent Introduction Sequence

1. Implement deterministic `ArtifactContextBuilder`.
2. Add context manifest output beside each generated artifact.
3. Add quality gate checks for outline and chapter task book.
4. Let `StageArtifactAgent` choose among allowed artifacts.
5. Add user-visible context preview: "本次生成参考了哪些产物".
6. Add manual include/exclude controls for advanced users.

## Product UX Updates

The workbench should make the staged pipeline visible:

1. 解析招标文件
2. 生成项目背景确认表
3. 生成评分标准拆解表
4. 生成四级技术标大纲
5. 生成章节写作任务书
6. 检索知识库
7. 并发生成章节
8. 字数检查
9. 去 AI 痕迹
10. 汇总 Word

Each stage should show:

- Input artifacts used.
- Output artifact path.
- Current status.
- User review state.
- Regenerate action.
- Modify action.

## Engineering Rules Going Forward

- Every generated artifact must record its input artifact paths in metadata.
- Every stage must have deterministic required inputs before agent-selected optional inputs are added.
- Every prompt must include the stage's Biaoshu skill requirement summary.
- Long artifacts should be compacted through section extraction, not naive full-text inclusion.
- Output should not be marked valid until the stage quality gate passes.
- User-modified artifacts take precedence over earlier generated versions.

## Open Follow-Up Plans

Recommended future plan files:

- `biaoshu-outline-completeness-gate.md`
- `biaoshu-chapter-task-book.md`
- `biaoshu-artifact-context-builder.md`
- `biaoshu-stage-artifact-agent.md`
- `biaoshu-context-preview-ui.md`

These should be implemented after `02_评分标准拆解表.md` is working and outline generation reliably covers all scoring requirements.

## Acceptance Notes

This memo is a planning artifact, not an implementation patch. It records the chosen sequence:

1. First: deterministic scoring breakdown stage.
2. Next: outline completeness gate.
3. Then: chapter task book.
4. Later: constrained agent-based context selection.
