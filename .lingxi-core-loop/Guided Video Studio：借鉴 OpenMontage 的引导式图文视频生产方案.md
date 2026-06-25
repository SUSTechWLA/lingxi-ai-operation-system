# Guided Video Studio：借鉴 OpenMontage 的引导式图文视频生产方案

## 1. 方案定位

本方案用于修正当前“用户一句话后 Agent 直接生成 final.mp4”的错误方向。

正确目标：

```text id="tm3f7y"
一句话启动视频项目
  ≠
一句话无确认直接出片
```

第一版产品应定义为：

```text id="20jvry"
用户一句话
  ↓
系统生成创作方案
  ↓
用户确认
  ↓
系统生成脚本
  ↓
用户确认
  ↓
系统生成视频结构
  ↓
用户确认
  ↓
系统生成预览
  ↓
用户确认
  ↓
系统渲染 final.mp4
```

方案名称：

```text id="9pc2bo"
Guided Video Studio
```

工作流名称：

```text id="mg4okk"
wf-guided-image-text-video
```

不要使用：

```text id="qtx075"
wf-one-click-image-text-video
wf-one-click-video
一键生成视频
一键出片
```

这些名字会诱导 Agent 跳过中间审核。

---

## 2. 从 OpenMontage 学到的核心机制

### 2.1 Pipeline Manifest 驱动

所有视频请求必须进入固定 pipeline，不允许 Agent 自由发挥。

你的系统中应新增：

```text id="sxxzly"
aios-core/internal/core/video_pipeline/
├── manifest.go
├── loader.go
├── validator.go
├── seed_video.go
└── guided_image_text_video.yaml
```

第一版只注册一个 pipeline：

```yaml id="1ermdw"
name: wf-guided-image-text-video
version: "1.0"
description: "引导式一句话图文视频创作流程"
category: video
stability: production
default_checkpoint_policy: guided
requireReviewGates: true
allowFullAuto: false
maxRevisionsPerStage: 3

stages:
  - name: proposal
    skill: video/proposal-director
    produces:
      - video_proposal
    checkpoint_required: true
    human_approval_default: true
    review_focus:
      - "主题是否准确"
      - "目标时长是否合理"
      - "视频结构是否清楚"
      - "是否适合图文视频第一版能力"
    success_criteria:
      - "必须输出 video_proposal"
      - "必须等待用户确认后才能进入 script"

  - name: script
    skill: video/script-director
    required_artifacts_in:
      - video_proposal
    produces:
      - video_script
    checkpoint_required: true
    human_approval_default: true
    review_focus:
      - "开头是否有吸引力"
      - "口播是否自然"
      - "内容是否准确"
      - "时长是否匹配"
    success_criteria:
      - "必须输出 video_script"
      - "必须等待用户确认后才能进入 composition"

  - name: composition
    skill: video/composition-director
    required_artifacts_in:
      - video_script
    produces:
      - video_composition_spec
    checkpoint_required: true
    human_approval_default: true
    review_focus:
      - "卡片顺序是否合理"
      - "每页文字是否过长"
      - "时间轴是否覆盖全片"
      - "是否适合 HyperFrames 图文渲染"
    success_criteria:
      - "必须输出合法 VideoCompositionSpec"
      - "必须等待用户确认后才能生成 HyperFrames 项目"

  - name: preview
    skill: video/preview-director
    required_artifacts_in:
      - video_composition_spec
    produces:
      - hyperframes_project_manifest
      - preview_snapshots
    required_tools:
      - hyperframes_project_generator
      - hyperframes_snapshot
    checkpoint_required: true
    human_approval_default: true
    review_focus:
      - "画面是否可读"
      - "文字是否溢出"
      - "卡片顺序是否正确"
      - "是否允许进入最终渲染"
    success_criteria:
      - "必须生成预览截图"
      - "用户确认前不得调用 hyperframes_renderer"

  - name: render
    skill: video/render-director
    required_artifacts_in:
      - hyperframes_project_manifest
      - preview_approval
    produces:
      - render_report
      - final_review
    required_tools:
      - hyperframes_renderer
      - ffmpeg_probe
    checkpoint_required: true
    human_approval_default: false
    review_focus:
      - "final.mp4 是否存在"
      - "duration 是否合理"
      - "是否通过 ffprobe"
    success_criteria:
      - "必须输出 final.mp4"
      - "必须输出 render_report 和 final_review"

  - name: package
    skill: video/package-director
    required_artifacts_in:
      - render_report
      - final_review
    produces:
      - publish_package
    required_tools:
      - artifact_packager
    checkpoint_required: true
    human_approval_default: true
    review_focus:
      - "最终包是否包含 video、manifest、spec、review"
      - "用户是否确认导出"
    success_criteria:
      - "必须输出 project_package.zip"
```

---

## 3. Pipeline 执行原则

### 3.1 强制规则

```text id="u7sil6"
1. 任何视频请求必须先匹配 pipeline。
2. 任何 pipeline 必须先通过 preflight。
3. 任何 stage 必须读取对应 stage director。
4. 任何 stage 必须产生 canonical artifact。
5. creative stage 必须 checkpoint + human approval。
6. 下游 stage 不得越过未批准 checkpoint。
7. renderer 不得在 preview 未确认前执行。
```

### 3.2 禁止规则

```text id="n556zb"
禁止：用户一句话 → 直接调用 hyperframes_renderer
禁止：script 未确认 → 自动生成 composition
禁止：composition 未确认 → 自动生成项目
禁止：preview 未确认 → 自动 render
禁止：渲染失败后自动换 runtime 或降级输出
禁止：没有 decision_log 的重大执行路径变更
```

---

## 4. Preflight 能力检查

OpenMontage 的关键经验是：生产前先告诉用户当前机器能做什么，不能做什么。

你的系统中需要新增：

```http id="cwby1i"
GET /api/video/preflight
```

返回：

```json id="mf3rbl"
{
  "pipeline": "wf-guided-image-text-video",
  "status": "passed",
  "capabilityMenu": {
    "localRunner": {
      "available": true,
      "runnerId": "runner_mac_001"
    },
    "compositionRuntime": {
      "hyperframes": {
        "available": true,
        "reason": "HyperFrames Render Service health passed"
      }
    },
    "localTools": [
      {
        "command": "HYPERFRAMES_PROJECT_GENERATE",
        "available": true
      },
      {
        "command": "HYPERFRAMES_RENDER",
        "available": true
      },
      {
        "command": "FFMPEG_PROBE",
        "available": true
      },
      {
        "command": "ARTIFACT_PACKAGE",
        "available": true
      }
    ],
    "warnings": []
  },
  "canStart": true
}
```

如果失败：

```json id="wxoams"
{
  "pipeline": "wf-guided-image-text-video",
  "status": "blocked",
  "canStart": false,
  "blockers": [
    {
      "code": "HYPERFRAMES_NOT_AVAILABLE",
      "message": "HyperFrames Render Service 未启动，无法进入视频预览和渲染。"
    }
  ],
  "setupOffers": [
    {
      "title": "启动本地渲染服务",
      "action": "start_hyperframes_service"
    }
  ]
}
```

---

## 5. Stage Director 机制

OpenMontage 不是让 Agent 泛泛执行工具，而是每个 stage 都有 director skill。

你需要新增：

```text id="sjdsq8"
skill-capabilities/video/guided-image-text-video/1.0.0/directors/
├── proposal-director.md
├── script-director.md
├── composition-director.md
├── preview-director.md
├── render-director.md
└── package-director.md
```

### 5.1 proposal-director.md

职责：

```text id="cs9ykb"
只生成创作方案，不生成脚本，不生成视频结构，不调用本地工具。
```

硬规则：

```text id="14v7tw"
1. 输出 video_proposal。
2. requiresApproval 必须为 true。
3. 不允许调用 video_script_generator。
4. 不允许调用 hyperframes_project_generator。
5. 不允许调用 hyperframes_renderer。
```

### 5.2 script-director.md

职责：

```text id="v31hf8"
基于已批准的 proposal 生成脚本。
```

硬规则：

```text id="9y2lel"
1. 必须读取 video_proposal。
2. 只输出 video_script。
3. 不允许输出 compositionSpec。
4. 不允许生成 HyperFrames 项目。
5. requiresApproval 必须为 true。
```

### 5.3 composition-director.md

职责：

```text id="bka9e1"
把已批准的脚本转为 VideoCompositionSpec。
```

硬规则：

```text id="fdbuj2"
1. 必须读取 approved video_script。
2. 输出 video_composition_spec。
3. 第一版只支持 title_card、knowledge_card、summary_card、caption_text。
4. requiresApproval 必须为 true。
```

### 5.4 preview-director.md

职责：

```text id="kctmf6"
生成 HyperFrames 项目和预览截图。
```

硬规则：

```text id="7l70qb"
1. 必须读取 approved video_composition_spec。
2. 可以调用 hyperframes_project_generator。
3. 可以调用 hyperframes_snapshot。
4. 不允许调用 hyperframes_renderer。
5. requiresApproval 必须为 true。
```

### 5.5 render-director.md

职责：

```text id="ij8r1f"
在 preview 已确认后渲染 final.mp4。
```

硬规则：

```text id="4jks8j"
1. 必须读取 preview_approval。
2. preview 未确认时不得执行。
3. 调用 hyperframes_renderer。
4. 渲染后必须调用 ffmpeg_probe 或 final_review。
```

### 5.6 package-director.md

职责：

```text id="w7jqvj"
打包最终项目。
```

硬规则：

```text id="x6x33c"
1. 必须读取 render_report 和 final_review。
2. final_review 未通过不得打包。
3. 输出 publish_package。
```

---

## 6. Canonical Artifact 体系

每个阶段必须输出标准产物，不允许只写自然语言。

```text id="pdkix5"
projects/{projectId}/artifacts/
├── video_proposal.json
├── video_script.json
├── video_composition_spec.json
├── preview_report.json
├── render_report.json
├── final_review.json
├── publish_package.json
└── decision_log.json
```

---

## 7. Checkpoint 设计

每个 stage 结束后必须写 checkpoint。

```text id="g9hfh1"
projects/{projectId}/checkpoints/
├── checkpoint_proposal.json
├── checkpoint_script.json
├── checkpoint_composition.json
├── checkpoint_preview.json
├── checkpoint_render.json
└── checkpoint_package.json
```

### 7.1 checkpoint 结构

```json id="3sa9qc"
{
  "version": "1.0",
  "projectId": "project_001",
  "pipeline": "wf-guided-image-text-video",
  "stage": "script",
  "status": "awaiting_human",
  "humanApprovalRequired": true,
  "humanApproved": false,
  "artifactRefs": [
    {
      "kind": "video_script",
      "path": "local://projects/project_001/artifacts/video_script.json"
    }
  ],
  "review": {
    "decision": "pass",
    "findings": []
  },
  "costSnapshot": {
    "totalCostUsd": 0
  },
  "createdAt": "2026-06-24T00:00:00Z"
}
```

### 7.2 状态

```text id="h3tmbw"
not_started
running
awaiting_human
approved
revision_requested
completed
failed
```

### 7.3 推进规则

```text id="ha4z6d"
awaiting_human 不得自动进入 completed。
只有用户 approve 后，才能推进下一个 stage。
用户 reject 后，当前 stage 进入 revision_requested。
```

---

## 8. Review API 设计

### 8.1 查询待审核

```http id="h4dmo5"
GET /api/video/projects/{projectId}/reviews/pending
```

返回：

```json id="0ey9mi"
{
  "projectId": "project_001",
  "pendingReview": {
    "stage": "script",
    "artifactKind": "video_script",
    "artifact": {},
    "reviewFocus": [
      "开头是否有吸引力",
      "口播是否自然",
      "内容是否准确"
    ],
    "actions": [
      "approve",
      "reject",
      "edit",
      "regenerate"
    ]
  }
}
```

### 8.2 通过

```http id="3a6doj"
POST /api/video/projects/{projectId}/reviews/{stage}/approve
```

### 8.3 驳回

```http id="3edvn0"
POST /api/video/projects/{projectId}/reviews/{stage}/reject
```

请求：

```json id="ar8gpd"
{
  "reason": "脚本开头不够直接",
  "revisionInstruction": "开头更短，第一句话直接提出冲突：很多人误解了 AI Agent。"
}
```

### 8.4 编辑后提交

```http id="bmyef3"
POST /api/video/projects/{projectId}/reviews/{stage}/submit-edited
```

---

## 9. Decision Log 设计

任何重大生产选择都必须记录。

```text id="fv10px"
projects/{projectId}/artifacts/decision_log.json
```

记录内容：

```json id="tb54d6"
{
  "version": "1.0",
  "projectId": "project_001",
  "decisions": [
    {
      "decisionId": "decision_runtime_001",
      "type": "render_runtime_selection",
      "stage": "proposal",
      "optionsConsidered": [
        {
          "runtime": "hyperframes",
          "available": true,
          "reason": "第一版图文卡片视频适合 HTML/CSS/GSAP 合成"
        }
      ],
      "selected": "hyperframes",
      "approvedByUser": true,
      "createdAt": "2026-06-24T00:00:00Z"
    }
  ]
}
```

第一版需要记录：

```text id="e1x2x9"
1. pipeline_selection
2. render_runtime_selection
3. proposal_approval
4. script_approval
5. composition_approval
6. preview_approval
7. final_render_approval
```

---

## 10. Review Gate 不能只靠 Prompt

上一版出问题的根本原因是：只在文档里说“要确认”，Agent 仍然可能跳过。

本次整改必须把 review gate 做成系统约束。

### 10.1 DAG 层约束

```text id="ei31kk"
每个 creative stage 后必须插入 REVIEW_GATE 节点。
```

节点：

```text id="pi24f2"
proposal_review_gate
script_review_gate
composition_review_gate
preview_review_gate
```

### 10.2 状态机约束

```text id="hr9vh6"
REVIEW_GATE_READY
  ↓
WAITING_REVIEW
  ↓
用户 approve
  ↓
REVIEW_APPROVED
  ↓
下游节点 READY
```

### 10.3 PlanGuard 约束

PlanGuard 必须校验：

```text id="3b02jf"
1. workflow.requireReviewGates == true
2. proposal 后必须有 review gate
3. script 后必须有 review gate
4. composition 后必须有 review gate
5. preview 后必须有 review gate
6. hyperframes_renderer 必须依赖 preview_review_gate
7. preview_review_gate 未 approved 时 render 不能 ready
```

如果缺失，拒绝执行：

```json id="rwnfr7"
{
  "code": "REVIEW_GATE_REQUIRED",
  "message": "视频工作流缺少必要审核节点，禁止自动进入最终渲染。"
}
```

---

## 11. UI 设计：Guided Video Studio

页面不要叫“一键生成视频”。

页面名称：

```text id="7893c1"
Guided Video Studio
```

副标题：

```text id="pbbvwp"
一句话启动，分阶段确认，最后生成 MP4
```

### 11.1 页面阶段

```text id="iid6b3"
1. 创作方案
2. 口播脚本
3. 视频结构
4. 画面预览
5. 最终渲染
6. 导出项目
```

### 11.2 每阶段操作

```text id="4if9ws"
确认继续
编辑
重新生成
返回上一步
取消
```

### 11.3 最终渲染按钮

必须单独展示：

```text id="k9xcnh"
开始最终渲染
```

并提示：

```text id="w7qe2i"
最终渲染会调用本地 HyperFrames，可能耗时数分钟。
```

---

## 12. Agent Prompt 修正

所有视频 Agent Prompt 必须加入以下硬约束：

```text id="yik9a6"
你正在执行 Guided Video Studio 工作流。

硬性规则：
1. 用户的一句话只用于启动项目和生成 proposal。
2. 不允许直接生成 final.mp4。
3. 不允许跳过 proposal review。
4. 不允许跳过 script review。
5. 不允许跳过 composition review。
6. 不允许跳过 preview review。
7. preview 未确认前，不允许调用 hyperframes_renderer。
8. renderer 调用前必须存在 preview_approval artifact。
9. 每个 stage 必须输出 canonical artifact。
10. 每个 creative stage 必须进入 awaiting_human checkpoint。
```

---

## 13. 最小第一版流程

### 13.1 用户输入

```text id="ul0ic2"
请帮我做一个 45 秒的视频，讲 AI Agent 为什么改变的是工作流。
```

### 13.2 系统第一步只输出 proposal

不能继续。

```json id="f88u2d"
{
  "artifactKind": "video_proposal",
  "title": "AI Agent 改变的是工作流",
  "targetDurationSec": 45,
  "style": "中文图文口播",
  "structure": [
    "开场钩子",
    "观点一",
    "观点二",
    "观点三",
    "总结"
  ],
  "nextAction": "等待用户确认方案"
}
```

### 13.3 用户确认后才生成 script

```text id="pd5xie"
用户点击：确认方案，生成脚本
```

### 13.4 用户确认 script 后才生成 composition

```text id="pq5ojk"
用户点击：确认脚本，生成视频结构
```

### 13.5 用户确认 composition 后才生成 preview

```text id="gjij7p"
用户点击：确认结构，生成预览
```

### 13.6 用户确认 preview 后才 render

```text id="zqg6m9"
用户点击：开始最终渲染
```

---

## 14. 从 OpenMontage 迁移到你系统的映射

| OpenMontage 机制         | 你的系统落地                                                                |
| ---------------------- | --------------------------------------------------------------------- |
| `pipeline_defs/*.yaml` | `video_pipeline/*.yaml` 或 workflow manifest                           |
| stage director skill   | `directors/*.md` + tool manifest                                      |
| checkpoint_required    | `checkpoint_required` 字段                                              |
| human_approval_default | `REVIEW_GATE` + `WAITING_REVIEW`                                      |
| canonical artifact     | `video_proposal.json` / `video_script.json` / `composition_spec.json` |
| provider_menu_summary  | `/api/video/preflight`                                                |
| decision_log           | `decision_log.json` / `decision_log` 表                                |
| project workspace      | `local://projects/{projectId}/...`                                    |
| final_review           | `final_review_generator`                                              |
| no silent runtime swap | render runtime 变更必须用户确认                                               |

---

## 15. 不直接照搬 OpenMontage 的部分

不要照搬：

```text id="284ptf"
1. Python agent 作为主 orchestrator
2. Remotion-first 结构
3. 全量工具生态
4. 多 pipeline 市场
5. AGPL 代码实现
```

你应该保留：

```text id="gs72qv"
1. Go cloud-backend 作为控制面
2. local-backend 作为本地执行面
3. HyperFrames 作为第一版唯一渲染 runtime
4. VideoCompositionSpec 作为中间协议
5. Review Gate 作为系统状态机，不靠 Prompt
```

---

## 16. 代码整改任务

### P0：Pipeline Manifest

```text id="5dt2zs"
1. 新增 wf-guided-image-text-video.yaml。
2. 删除或禁用 wf-one-click-image-text-video。
3. Manifest 增加：
   - default_checkpoint_policy: guided
   - requireReviewGates: true
   - allowFullAuto: false
4. Manifest 中 proposal/script/composition/preview 全部 human_approval_default: true。
```

### P0：Review Gate 状态机

```text id="t64yah"
1. 新增 REVIEW_GATE node type。
2. 新增 WAITING_REVIEW 状态。
3. 新增 REVIEW_APPROVED、REVIEW_REJECTED。
4. 下游节点必须等待 REVIEW_APPROVED。
5. hyperframes_renderer 必须依赖 preview_review_gate。
```

### P0：Review API

```text id="i0q98m"
1. GET pending review。
2. POST approve。
3. POST reject。
4. POST submit-edited。
5. POST regenerate-stage。
```

### P0：Checkpoint

```text id="1iw5gm"
1. 每个 stage 完成后写 checkpoint。
2. 需要人工确认时状态为 awaiting_human。
3. approve 后改为 completed。
4. reject 后改为 revision_requested。
5. 支持恢复到当前 awaiting_human stage。
```

### P0：Preflight

```text id="5a1toz"
1. 新增 /api/video/preflight。
2. 检查 local runner。
3. 检查 HyperFrames Render Service。
4. 检查 HYPERFRAMES_PROJECT_GENERATE。
5. 检查 HYPERFRAMES_RENDER。
6. 检查 FFMPEG_PROBE。
7. 检查 ARTIFACT_PACKAGE。
8. 不通过则禁止开始 pipeline。
```

### P1：Stage Director

```text id="q1flmj"
1. 新增 proposal-director.md。
2. 新增 script-director.md。
3. 新增 composition-director.md。
4. 新增 preview-director.md。
5. 新增 render-director.md。
6. 每个 director 写明输入、输出、禁止调用的工具、review focus。
```

### P1：Decision Log

```text id="tcpv4q"
1. 新增 decision_log 表或 artifact。
2. 记录 pipeline_selection。
3. 记录 render_runtime_selection。
4. 记录每次 approval。
5. 记录每次 reject/revision。
6. 禁止无 decision_log 的 runtime swap。
```

### P1：Frontend

```text id="a62ndr"
1. 页面改名 Guided Video Studio。
2. 文案改为“一句话启动视频创作”。
3. 每个 stage 展示 artifact。
4. 每个 stage 有确认、编辑、重新生成、返回。
5. 最终渲染按钮独立。
6. 不显示“一键出片”。
```

---

## 17. 验收标准

### 17.1 不能通过的情况

```text id="kbfep7"
输入一句话后直接生成 final.mp4 —— 不通过。
script 未确认就生成 composition —— 不通过。
composition 未确认就生成 project —— 不通过。
preview 未确认就 render —— 不通过。
render runtime 变更未记录 decision_log —— 不通过。
preflight 不通过仍启动 pipeline —— 不通过。
```

### 17.2 必须通过的情况

```text id="28sft5"
输入一句话后，只生成 proposal 并停住。
用户确认 proposal 后，才生成 script。
用户确认 script 后，才生成 compositionSpec。
用户确认 compositionSpec 后，才生成 preview。
用户确认 preview 后，才渲染 final.mp4。
每个阶段都有 checkpoint。
每个阶段都有 canonical artifact。
每个确认都写 decision_log。
```

---

## 18. Coding Agent 修复指令

```text id="6w30ii"
目标：
学习 OpenMontage 的 pipeline manifest、stage director、checkpoint、human approval、preflight、decision_log 机制，修复当前视频 Agent 被误解为“一句话直接生成 final.mp4”的问题。系统必须变为 Guided Video Studio：一句话只启动项目，每个创作阶段必须审核确认。

P0：
1. 禁用 wf-one-click-image-text-video。
2. 新增 wf-guided-image-text-video。
3. 新 workflow 必须包含 proposal、script、composition、preview、render、package 六个 stage。
4. proposal/script/composition/preview 必须 human_approval_default=true。
5. render 依赖 preview approval。
6. workflow 增加 allowFullAuto=false。
7. workflow 增加 requireReviewGates=true。

P1：
1. 新增 REVIEW_GATE node type。
2. 新增 WAITING_REVIEW 状态。
3. TOOL stage 完成后，如果 human_approval_default=true，写 checkpoint 并进入 WAITING_REVIEW。
4. 用户 approve 后，checkpoint 改为 completed，下游节点才 READY。
5. 用户 reject 后，stage 改为 revision_requested，下游不得执行。

P2：
1. 新增 checkpoint 存储。
2. checkpoint 必须包含 stage、status、artifactRefs、review、costSnapshot、humanApprovalRequired、humanApproved。
3. 重启后能恢复到 awaiting_human stage。

P3：
1. 新增 /api/video/preflight。
2. 输出 capability menu。
3. 检查 local runner、HyperFrames、FFmpeg 和四个本地命令。
4. preflight blocked 时禁止创建视频 task。

P4：
1. 新增 directors：
   - proposal-director.md
   - script-director.md
   - composition-director.md
   - preview-director.md
   - render-director.md
   - package-director.md
2. 每个 director 必须声明 allowed_tools 和 forbidden_tools。
3. preview-director forbidden_tools 必须包含 hyperframes_renderer。

P5：
1. 新增 decision_log。
2. 所有 approval/reject/render_runtime_selection 必须写 decision_log。
3. 禁止 silent runtime swap。

P6：
1. 前端页面改名 Guided Video Studio。
2. 文案改为“一句话启动视频创作”。
3. 每个阶段显示 artifact 和审核按钮。
4. 最终渲染必须用户点击“开始最终渲染”。

验收：
1. 输入一句话后只生成 proposal 并等待用户确认。
2. 每个创作阶段都停住等待确认。
3. preview 未确认时，不得出现 HYPERFRAMES_RENDER LocalJob。
4. 每个 stage 都有 checkpoint。
5. 每个 approval 都有 decision_log。
```

---

## 19. 最终结论

OpenMontage 给你的关键启发不是“让 Agent 自动把视频做完”，而是：

```text id="37v3rw"
pipeline 驱动
stage director 约束
canonical artifact 交付
checkpoint 可恢复
human approval 阻断
preflight 能力透明
decision_log 可审计
```

你现在的系统应该从：

```text id="k5i6vn"
OneClick Video
```

改成：

```text id="mzxmjc"
Guided Video Studio
```

最终原则：

```text id="5pi5ag"
一句话启动，不是一句话到底。
自动生成草稿，不是自动跳过审核。
审核节点必须是系统状态机，不是 Prompt 建议。
```
