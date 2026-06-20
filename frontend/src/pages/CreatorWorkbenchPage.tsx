import React, { useEffect, useMemo, useState } from 'react'
import {
  FiAlertCircle,
  FiArrowRight,
  FiBookOpen,
  FiCheckCircle,
  FiClock,
  FiCpu,
  FiDownload,
  FiEye,
  FiFileText,
  FiFilm,
  FiImage,
  FiList,
  FiMusic,
  FiPlayCircle,
  FiMessageSquare,
  FiRefreshCw,
  FiTarget,
  FiX,
  FiZap,
} from 'react-icons/fi'
import {
  approveVideoStage,
  createVideoProject,
  createWorkflowRun,
  fetchTrace,
  fetchSkillCatalog,
  fetchSkillDetail,
  fetchArtifactContent,
  fetchArtifactHistory,
  fetchProjectArtifacts,
  fetchVideoProjects,
  fetchWorkflows,
  reviseArtifact,
  routeSkill,
} from '../services/api'
import type {
  Artifact,
  ArtifactContentResponse,
  TraceData,
  SkillCatalogItem,
  SkillStage,
  SkillRouteResponse,
  SkillRuntimeItem,
  VideoGenerationMode,
  VideoProject,
  WorkflowRun,
  WorkflowTemplate,
} from '../utils/types'

type DeliverableKey = 'publish_pack' | 'script_only' | 'keyframes' | 'video_prompt' | 'shot_learning'

interface CreatorRoute {
  id: string
  title: string
  subtitle: string
  reason: string
  tone: 'blue' | 'green' | 'rose' | 'amber'
}

const generationMode: VideoGenerationMode = 'manual_import'

const routeDisplays: Record<string, CreatorRoute> = {
  talking_head: {
    id: 'talking_head',
    title: '口播 / 知识视频',
    subtitle: '观点、知识、课程、解释型内容统一进入口播创作线',
    reason: '系统会重点处理观点结构、口播稿、画面包装、标题简介和关键词。',
    tone: 'blue',
  },
  cinematic_short: {
    id: 'cinematic_short',
    title: '镜头式 AIGC 短片',
    subtitle: '广告短片、剧情短片、产品故事，输出关键帧与视频提示词',
    reason: '系统会优先生成分镜、关键帧请求、视频提示词和可导入网页工具的素材包。',
    tone: 'rose',
  },
  director_pipeline: {
    id: 'director_pipeline',
    title: '导演级视频流水线',
    subtitle: '适合 IP、角色、剧本、素材库、导演剪辑和成片装配',
    reason: '系统会走更长的创作链路，保留更多可审核、可返修的中间资产。',
    tone: 'amber',
  },
  shot_learning: {
    id: 'shot_learning',
    title: '经典镜头学习',
    subtitle: '拆参考片，抽象成可复用的拍摄语法和提示词',
    reason: '系统会把重点放在镜头语言复盘，而不是直接生成新视频。',
    tone: 'green',
  },
}

const examplePrompts = [
  '把我这段观点做成 60 秒口播视频，要有标题、简介和关键词',
  '我只需要 6 张关键帧，给网页视频生成工具做参考',
  '做一条产品故事短片，偏电影感，输出视频提示词和参考帧',
  '拆一个经典镜头，提炼可复用的运镜和构图方式',
]

const abilityTests = [
  {
    route: 'talking_head',
    skillName: 'create-opinion-videos',
    prompt: '把我这段观点做成 60 秒口播知识视频，输出可发布视频素材包、标题、简介和关键词',
  },
  {
    route: 'cinematic_short',
    skillName: 'aigc-shot-video',
    prompt: '做一条产品故事短片，偏电影感，输出视频提示词、关键帧参考和可导入网页视频工具的素材包',
  },
  {
    route: 'director_pipeline',
    skillName: 'video-creator',
    prompt: '做一个系列 IP 的导演级短视频，从角色、剧本、分镜、素材库到导演剪辑都要有中间态',
  },
  {
    route: 'shot_learning',
    skillName: 'film-shot-reconstruction',
    prompt: '拆一个经典镜头，学习构图、运镜和节奏，并沉淀成可复用的视频提示词和参考帧方案',
  },
]

const deliverableLabels: Record<string, string> = {
  publish_pack: '完整发布包',
  script_only: '脚本草稿',
  keyframes: '关键帧素材',
  video_prompt: '视频提示词包',
  shot_learning: '拉片学习包',
}

const stageNameMap: Record<string, string> = {
  viewpoint_dossier: '观点档案',
  recording_script: '口播稿',
  hyperframes_reference: '画面参考',
  image_assets: '参考图请求',
  hyperframes_build: '工程构建',
  hyperframes_render: '视频渲染',
  render_review: '成片审核',
  intent_analysis: '意图分析',
  narration: '讲稿整理',
  visual_design: '视觉方向',
  beat_planning: '节奏规划',
  component_dsl: '画面组件',
  bundle_assemble: '素材包',
  brief: '创作简报',
  script: '剧本',
  shot_plan: '镜头规划',
  storyboard: '故事板',
  keyframe: '关键帧',
  video_prompt: '视频提示词',
  generate: '外部生成包',
  review: '结果审核',
  ip_and_story_seed: 'IP 与故事',
  script_and_profiles: '剧本与角色',
  material_library_match: '素材库匹配',
  reference_asset_plan: '参考资产计划',
  reference_asset_generation: '参考资产',
  shot_design: '导演镜头',
  keyframes_and_prompts: '关键帧与提示词',
  video_generation: '视频生成包',
  director_cut: '导演剪辑',
  final_assembly: '成片装配',
  final_review: '终审',
  input_prepare: '素材准备',
  motion_analysis: '运动分析',
  reconstruction_plan: '重构计划',
  reference_prompts: '参考提示词',
  reference_images: '参考图',
  synthetic_keyframes: '合成关键帧',
  reconstruction_package: '重构包',
  asset_guard: '资产检查',
  material_library_import: '素材库入库',
}

const visibleStageNamesByDeliverable: Record<string, string[]> = {
  script_only: ['viewpoint_dossier', 'recording_script', 'intent_analysis', 'narration', 'brief', 'script', 'script_and_profiles'],
  keyframes: ['brief', 'script', 'visual_design', 'shot_plan', 'storyboard', 'keyframe', 'reference_asset_plan', 'reference_asset_generation', 'shot_design', 'keyframes_and_prompts', 'reference_prompts', 'reference_images', 'synthetic_keyframes'],
  video_prompt: ['brief', 'script', 'shot_plan', 'keyframe', 'video_prompt', 'keyframes_and_prompts', 'video_generation'],
}

const templateIdForSkill = (name: string, version: string) =>
  `wf-${name}-${version.replace(/\./g, '-')}`

const projectModeForSkill = (skillName: string) =>
  skillName === 'aigc-shot-video' || skillName === 'video-creator' ? 'aigc_shot' : 'voice_visual'

const skillKey = (skill: Pick<SkillCatalogItem, 'name' | 'version'>) => `${skill.name}@${skill.version}`

const shortId = (value?: string) => {
  if (!value) return '-'
  if (value.length <= 12) return value
  return `${value.slice(0, 6)}...${value.slice(-4)}`
}

const routeDisplayFor = (route?: string) => routeDisplays[route || 'talking_head'] || routeDisplays.talking_head

const CreatorWorkbenchPage: React.FC = () => {
  const [catalog, setCatalog] = useState<SkillCatalogItem[]>([])
  const [skillDetails, setSkillDetails] = useState<Record<string, SkillRuntimeItem>>({})
  const [workflows, setWorkflows] = useState<WorkflowTemplate[]>([])
  const [projects, setProjects] = useState<VideoProject[]>([])
  const [brief, setBrief] = useState(examplePrompts[0])
  const [routeResult, setRouteResult] = useState<SkillRouteResponse | null>(null)
  const [routing, setRouting] = useState(false)
  const [routeError, setRouteError] = useState('')
  const [loading, setLoading] = useState(true)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')
  const [currentRun, setCurrentRun] = useState<WorkflowRun | null>(null)
  const [currentProject, setCurrentProject] = useState<VideoProject | null>(null)
  const [approvingStage, setApprovingStage] = useState('')
  const [artifacts, setArtifacts] = useState<Artifact[]>([])
  const [selectedArtifactId, setSelectedArtifactId] = useState('')
  const [artifactContent, setArtifactContent] = useState<ArtifactContentResponse | null>(null)
  const [artifactHistory, setArtifactHistory] = useState<Artifact[]>([])
  const [artifactsLoading, setArtifactsLoading] = useState(false)
  const [revisionMessage, setRevisionMessage] = useState('')
  const [revisingArtifact, setRevisingArtifact] = useState(false)
  const [artifactModalOpen, setArtifactModalOpen] = useState(false)
  const [traceOpen, setTraceOpen] = useState(false)
  const [traceLoading, setTraceLoading] = useState(false)
  const [traceData, setTraceData] = useState<TraceData | null>(null)
  const [traceError, setTraceError] = useState('')

  const videoCatalog = useMemo(() => catalog.filter((skill) => skill.category === 'video'), [catalog])
  const visibleVideoWorkflowCount = useMemo(() => {
    const visibleWorkflowIds = new Set(videoCatalog.map((skill) => templateIdForSkill(skill.name, skill.version)))
    return workflows.filter((workflow) => visibleWorkflowIds.has(workflow.id)).length
  }, [videoCatalog, workflows])
  const abilityCards = useMemo(() => abilityTests.map((ability) => ({
    ...ability,
    route: routeDisplayFor(ability.route),
    skill: videoCatalog.find((skill) => skill.name === ability.skillName),
  })), [videoCatalog])

  const selectedSummary = routeResult?.skill
  const selectedSkill = selectedSummary ? skillDetails[skillKey(selectedSummary)] : undefined
  const selectedTemplate = useMemo(() => {
    if (!selectedSummary) return undefined
    return workflows.find((workflow) => workflow.id === templateIdForSkill(selectedSummary.name, selectedSummary.version))
  }, [selectedSummary, workflows])

  const currentDeliverable = (routeResult?.deliverable || 'publish_pack') as DeliverableKey
  const routeDisplay = routeDisplayFor(routeResult?.route)
  const progressStages = selectedSkill?.stages || []
  const visibleStages = useMemo(() => {
    const names = visibleStageNamesByDeliverable[currentDeliverable]
    if (!names) return progressStages
    return progressStages.filter((stage) => names.includes(stage.name))
  }, [currentDeliverable, progressStages])
  const reviewStages = useMemo(() => {
    const stages = visibleStages.length > 0 ? visibleStages : progressStages
    return stages.filter((stage) => stage.approvalRequired)
  }, [progressStages, visibleStages])

  useEffect(() => {
    let mounted = true

    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const [catalogData, workflowsData, projectsData] = await Promise.all([
          fetchSkillCatalog(),
          fetchWorkflows(),
          fetchVideoProjects(),
        ])
        if (!mounted) return
        setCatalog(catalogData.skills)
        setWorkflows(workflowsData.templates)
        setProjects(projectsData.projects)
      } catch (err: any) {
        if (!mounted) return
        setError(err?.response?.data?.message || err?.message || '无法连接视频创作后端')
      } finally {
        if (mounted) setLoading(false)
      }
    }

    load()
    return () => {
      mounted = false
    }
  }, [])

  useEffect(() => {
    const text = brief.trim()
    if (!text) {
      setRouteResult(null)
      setRouteError('')
      setRouting(false)
      return
    }

    let mounted = true
    const timer = window.setTimeout(async () => {
      setRouting(true)
      setRouteError('')
      try {
        const data = await routeSkill(text)
        if (!mounted) return
        setRouteResult(data)
      } catch (err: any) {
        if (!mounted) return
        setRouteResult(null)
        setRouteError(err?.response?.data?.message || err?.message || '自然语言路由失败')
      } finally {
        if (mounted) setRouting(false)
      }
    }, 500)

    return () => {
      mounted = false
      window.clearTimeout(timer)
    }
  }, [brief])

  useEffect(() => {
    if (!selectedSummary) return
    const key = skillKey(selectedSummary)
    if (skillDetails[key]) return

    let mounted = true
    fetchSkillDetail(selectedSummary.name, selectedSummary.version)
      .then((data) => {
        if (!mounted) return
        setSkillDetails((prev) => ({ ...prev, [key]: data.skill }))
      })
      .catch((err: any) => {
        if (!mounted) return
        setError(err?.response?.data?.message || err?.message || '无法加载命中的 Skill 详情')
      })

    return () => {
      mounted = false
    }
  }, [selectedSummary, skillDetails])

  const loadArtifacts = async (projectId: string, keepSelection = true) => {
    setArtifactsLoading(true)
    try {
      const data = await fetchProjectArtifacts(projectId)
      setArtifacts(data.artifacts)
      setSelectedArtifactId((current) => {
        if (keepSelection && current && data.artifacts.some((artifact) => artifact.id === current)) return current
        return data.artifacts[0]?.id || ''
      })
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.message || '无法加载产物')
    } finally {
      setArtifactsLoading(false)
    }
  }

  useEffect(() => {
    if (!currentProject?.id) {
      setArtifacts([])
      setSelectedArtifactId('')
      setArtifactContent(null)
      setArtifactHistory([])
      return
    }

    let mounted = true
    const load = async () => {
      if (!mounted) return
      await loadArtifacts(currentProject.id)
    }
    load()
    const timer = window.setInterval(load, 3000)
    return () => {
      mounted = false
      window.clearInterval(timer)
    }
  }, [currentProject?.id])

  useEffect(() => {
    if (!selectedArtifactId) {
      setArtifactContent(null)
      setArtifactHistory([])
      return
    }
    let mounted = true
    Promise.all([
      fetchArtifactContent(selectedArtifactId),
      fetchArtifactHistory(selectedArtifactId),
    ])
      .then(([contentData, historyData]) => {
        if (!mounted) return
        setArtifactContent(contentData)
        setArtifactHistory(historyData.history)
      })
      .catch((err: any) => {
        if (!mounted) return
        setError(err?.response?.data?.message || err?.message || '无法加载产物内容')
      })
    return () => {
      mounted = false
    }
  }, [selectedArtifactId])

  const handleStart = async () => {
    const text = brief.trim()
    if (!text) return
    setStarting(true)
    setError('')
    setRouteError('')
    setCurrentRun(null)
    setCurrentProject(null)
    setArtifactModalOpen(false)
    setTraceOpen(false)

    try {
      const route = await routeSkill(text)
      setRouteResult(route)

      const summary = route.skill
      const template = workflows.find((workflow) => workflow.id === templateIdForSkill(summary.name, summary.version))
      if (!template) {
        throw new Error(`命中的 Skill 缺少 Workflow：${summary.name}@${summary.version}`)
      }

      const deliverable = route.deliverable || 'publish_pack'
      const aspectRatio = route.aspectRatio || '9:16'
      const targetDurationSec = route.targetDurationSec || 60
      const display = routeDisplayFor(route.route)

      const project = await createVideoProject({
        name: text.slice(0, 32),
        description: text,
        mode: projectModeForSkill(summary.name),
        skillName: summary.name,
        skillVersion: summary.version,
        workflowName: template.name,
        workflowVersion: template.version,
        generationMode,
        aspectRatio,
        targetDurationSec,
        language: 'zh-CN',
        config: {
          brief: text,
          deliverable,
          routeId: route.route,
          routeTitle: display.title,
          routeSource: route.source,
          routeReasoning: route.reasoning,
          routeConfidence: route.confidence,
          llmInferredFields: ['audience', 'style', 'tone', 'platform_copy'],
          imageGeneration: 'codex_imagegen_only',
          videoGeneration: 'external_web_import',
          productSurface: 'natural-language-creator-workbench',
        },
      })

      const run = await createWorkflowRun(project.id, {
        templateId: template.id,
        templateVersion: template.version,
        input: {
          brief: text,
          deliverable,
          route_id: route.route,
          route_source: route.source,
          route_reasoning: route.reasoning,
          route_confidence: route.confidence,
          llm_inferred_fields: ['audience', 'style', 'tone', 'platform_copy'],
          aspect_ratio: aspectRatio,
          target_duration_sec: targetDurationSec,
          generation_mode: generationMode,
          image_generation: 'codex_imagegen_only',
          video_generation: 'external_web_import',
          expected_output: deliverable,
        },
      })

      setCurrentProject(project)
      setCurrentRun(run)
      setProjects((prev) => [project, ...prev])
      setArtifacts([])
      setSelectedArtifactId('')
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.message || '启动创作线失败')
    } finally {
      setStarting(false)
    }
  }

  const handleApproveStage = async (stageName: string) => {
    if (!currentRun || !currentProject) return
    setApprovingStage(stageName)
    setError('')
    try {
      await approveVideoStage(currentProject.id, stageName, {
        runId: currentRun.id,
        output: {
          approvedAt: new Date().toISOString(),
          source: 'creator-workbench',
        },
      })
      window.setTimeout(() => loadArtifacts(currentProject.id), 800)
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.response?.data?.error || err?.message || '确认中间态失败')
    } finally {
      setApprovingStage('')
    }
  }

  const handleAbilitySelect = (prompt: string) => {
    setBrief(prompt)
    setRouteError('')
    setCurrentRun(null)
    setCurrentProject(null)
  }

  const handleReviseArtifact = async () => {
    if (!selectedArtifactId || !revisionMessage.trim()) return
    setRevisingArtifact(true)
    setError('')
    try {
      const revised = await reviseArtifact(selectedArtifactId, revisionMessage.trim())
      setRevisionMessage('')
      setArtifactContent(revised)
      setSelectedArtifactId(revised.artifact.id)
      if (currentProject?.id) {
        await loadArtifacts(currentProject.id, false)
      }
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.message || '返工失败')
    } finally {
      setRevisingArtifact(false)
    }
  }

  const handleOpenArtifact = (artifactId: string) => {
    setSelectedArtifactId(artifactId)
    setArtifactModalOpen(true)
  }

  const handleOpenTrace = async () => {
    if (!currentRun?.taskId) return
    setTraceOpen(true)
    setTraceLoading(true)
    setTraceError('')
    try {
      const data = await fetchTrace(currentRun.taskId)
      setTraceData(data)
    } catch (err: any) {
      setTraceError(err?.response?.data?.message || err?.message || '无法读取制作记录')
      setTraceData(null)
    } finally {
      setTraceLoading(false)
    }
  }

  const canStart = Boolean(brief.trim() && !starting && !loading && !routing)

  return (
    <div className="min-h-screen overflow-y-auto bg-[#F4F6F8] text-[#16181D]">
      <header className="border-b border-[#D9DEE7] bg-white px-7 py-5">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase text-[#245BFF]">Creator Workbench</p>
            <h1 className="mt-1 text-2xl font-semibold">自然语言自媒体创作台</h1>
            <p className="mt-1 text-sm text-[#667085]">输入成片目标，系统自动理解、路由 Skill，并保留可追踪的中间态。</p>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <StatusPill label={`${abilityCards.length} 条基础路径`} tone="dark" />
            <StatusPill label={`${visibleVideoWorkflowCount} 个视频流程`} tone="blue" />
            <StatusPill label="外部生成后导入" tone="amber" />
          </div>
        </div>
      </header>

      <main className="grid gap-5 p-6 xl:grid-cols-[minmax(0,1fr)_320px] 2xl:grid-cols-[minmax(0,1fr)_360px]">
        <section className="space-y-5">
          <div className="rounded-lg border border-[#D9DEE7] bg-white">
            <PanelHeader icon={<FiMessageSquare />} title="一句话创作入口" desc="用户只需要描述想要的成片" />
            <div className="space-y-4 p-5">
              {error && (
                <div className="flex items-start gap-2 rounded-lg border border-[#E85D75]/30 bg-[#E85D75]/10 p-3 text-sm text-[#9D2540]">
                  <FiAlertCircle className="mt-0.5 flex-shrink-0" />
                  <span>{error}</span>
                </div>
              )}
              <textarea
                value={brief}
                onChange={(e) => setBrief(e.target.value)}
                className="min-h-36 w-full resize-y rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] px-4 py-3 text-base leading-7 outline-none transition focus:border-[#245BFF] focus:ring-2 focus:ring-[#245BFF]/15"
              />
              <div className="flex flex-wrap gap-2">
                {examplePrompts.map((prompt) => (
                  <button
                    key={prompt}
                    onClick={() => setBrief(prompt)}
                    className="rounded-full border border-[#D9DEE7] bg-[#F7F9FC] px-3 py-1.5 text-xs text-[#475467] transition hover:border-[#245BFF] hover:text-[#245BFF]"
                  >
                    {prompt}
                  </button>
                ))}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[#D9DEE7] pt-4">
                <div className="grid min-w-0 flex-1 gap-2 sm:grid-cols-4">
                  <Metric label="路由方式" value={routeResult ? (routeResult.source === 'llm' ? 'LLM' : '规则兜底') : routing ? '理解中' : '-'} />
                  <Metric label="交付目标" value={deliverableLabels[currentDeliverable] || currentDeliverable} />
                  <Metric label="画幅" value={routeResult?.aspectRatio || '-'} />
                  <Metric label="时长" value={routeResult?.targetDurationSec ? `${routeResult.targetDurationSec}秒` : '-'} />
                </div>
                <button
                  onClick={handleStart}
                  disabled={!canStart}
                  className="inline-flex items-center gap-2 rounded-lg bg-[#16181D] px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-[#2B2F36] disabled:cursor-not-allowed disabled:bg-[#AAB2C0]"
                >
                  {starting || routing ? <FiRefreshCw className="animate-spin" /> : <FiArrowRight />}
                  {routing ? '理解中' : '启动创作线'}
                </button>
              </div>
            </div>
          </div>

          <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(360px,0.6fr)]">
            <div className="rounded-lg border border-[#D9DEE7] bg-white">
              <PanelHeader icon={<FiTarget />} title="系统理解" desc="由入口 LLM 根据能力目录自动路由" />
              <div className="space-y-4 p-5">
                {routeError && (
                  <div className="flex items-start gap-2 rounded-lg border border-[#F5A524]/35 bg-[#FFF7E6] p-3 text-sm text-[#9A6700]">
                    <FiAlertCircle className="mt-0.5 flex-shrink-0" />
                    <span>{routeError}</span>
                  </div>
                )}
                <RouteCard route={routeDisplay} routing={routing} />
                <div className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-xs font-semibold text-[#667085]">命中能力</p>
                      <h2 className="mt-1 truncate text-lg font-semibold">{selectedSummary ? formatSkillName(selectedSummary) : '等待系统理解'}</h2>
                      <p className="mt-1 text-xs text-[#667085]">
                        {selectedSummary ? `${selectedSummary.name}@${selectedSummary.version}` : '输入变化后自动重新判断'}
                      </p>
                    </div>
                    <StatusPill label={selectedTemplate ? 'Workflow ready' : selectedSummary ? '缺 Workflow' : '待路由'} tone={selectedTemplate ? 'green' : 'amber'} />
                  </div>
                  <p className="mt-3 text-sm leading-6 text-[#475467]">{routeResult?.reasoning || selectedSummary?.description || routeDisplay.reason}</p>
                  <div className="mt-4 grid gap-2 sm:grid-cols-4">
                    <Metric label="阶段" value={String(selectedSummary?.stageCount || 0)} />
                    <Metric label="审核门" value={selectedSummary?.requiresApproval ? '有' : '无'} />
                    <Metric label="长任务" value={selectedSummary?.hasLongRunningStages ? '有' : '少'} />
                    <Metric label="置信度" value={routeResult?.confidence ? `${Math.round(routeResult.confidence * 100)}%` : '-'} />
                  </div>
                </div>
              </div>
            </div>

            <div className="rounded-lg border border-[#D9DEE7] bg-white">
              <PanelHeader icon={<FiCpu />} title="能力管理" desc="点击路径后仍由自然语言路由确认" />
              <div className="space-y-3 p-4">
                {loading && <Skeleton label="正在读取能力目录" />}
                {!loading && videoCatalog.length === 0 && (
                  <EmptyState title="没有视频能力" desc="确认 VIDEO_CREATION_ENABLED=true，且 SKILL_ROOT 指向 skills 目录。" />
                )}
                {abilityCards.map(({ route, skill, prompt, skillName }) => {
                  const selected = selectedSummary?.name === skillName
                  return (
                    <button
                      key={skillName}
                      type="button"
                      onClick={() => handleAbilitySelect(prompt)}
                      disabled={!skill}
                      className={`w-full rounded-lg border p-3 text-left transition ${
                        selected
                        ? 'border-[#245BFF] bg-[#EEF4FF]'
                        : 'border-[#D9DEE7] bg-[#FBFCFE] hover:border-[#245BFF]'
                      } disabled:cursor-not-allowed disabled:opacity-60`}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="truncate text-sm font-semibold">{route.title}</p>
                          <p className="mt-1 line-clamp-2 text-xs leading-5 text-[#667085]">{route.subtitle}</p>
                        </div>
                        <span className="shrink-0 rounded-full bg-white px-2 py-0.5 text-[10px] text-[#667085]">
                          {skill ? skill.stageCount : '缺失'}
                        </span>
                      </div>
                      <div className="mt-3 flex items-center justify-between gap-3">
                        <span className="min-w-0 truncate text-[11px] text-[#667085]">{skill ? `${skill.name}@${skill.version}` : skillName}</span>
                        <span className={`shrink-0 text-[11px] font-semibold ${selected ? 'text-[#245BFF]' : 'text-[#667085]'}`}>
                          {selected ? '当前命中' : '测试'}
                        </span>
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-[#D9DEE7] bg-white">
            <PanelHeader icon={<FiClock />} title="制作进度" desc="按系统判断的交付目标展示中间态" />
            <div className="grid gap-3 p-4 md:grid-cols-2 xl:grid-cols-4 2xl:grid-cols-5">
              {(visibleStages.length > 0 ? visibleStages : progressStages).map((stage, index) => (
                <div key={`${stage.name}-${index}`} className="min-h-[132px] rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-mono text-xs text-[#667085]">{String(index + 1).padStart(2, '0')}</span>
                    <div className="flex shrink-0 gap-1">
                      {stage.approvalRequired && <MiniBadge label="审核" />}
                      {stage.longRunning && <MiniBadge label="长任务" />}
                      {stage.optional && <MiniBadge label="可选" />}
                    </div>
                  </div>
                  <h3 className="mt-3 min-h-[40px] break-words text-sm font-semibold leading-5 text-[#16181D]">{stageNameMap[stage.name] || stage.name}</h3>
                  <p className="mt-2 line-clamp-2 break-words text-xs leading-5 text-[#667085]">{stage.tool ? `工具：${stage.tool}` : 'Markdown 中间态 / 可返修产物'}</p>
                </div>
              ))}
              {!loading && progressStages.length === 0 && <Skeleton label={routeResult ? '命中能力后加载阶段详情' : '等待系统理解后加载阶段详情'} />}
            </div>
          </div>

          <div className="rounded-lg border border-[#D9DEE7] bg-white">
            <PanelHeader icon={<FiPlayCircle />} title="产物审片台" desc="点击产物进入大窗口查看、播放和返工" />
            <div className="p-4">
              <ArtifactWorkbench
                artifacts={artifacts}
                selectedId={selectedArtifactId}
                loading={artifactsLoading}
                currentRun={currentRun}
                onOpen={handleOpenArtifact}
              />
            </div>
          </div>
        </section>

        <aside className="space-y-5">
          <div className="rounded-lg border border-[#D9DEE7] bg-white">
            <PanelHeader icon={<FiZap />} title="最终交付" desc="围绕发布素材，而不是多平台同步" />
            <div className="space-y-3 p-5">
              <MaterialRow icon={<FiBookOpen />} title="脚本 / 结构" desc="观点、剧本、口播稿或导演设计。" />
              <MaterialRow icon={<FiImage />} title="参考帧 / 关键帧" desc="Codex imagegen 请求与导入槽位。" />
              <MaterialRow icon={<FiFilm />} title="视频提示词包" desc="prompt、负面提示、参考帧说明。" />
              <MaterialRow icon={<FiMessageSquare />} title="标题 / 简介 / 关键词" desc="面向发布页的最终文案。" />

              {currentRun ? (
                <div className="rounded-lg border border-[#00A86B]/30 bg-[#ECFDF3] p-4 text-sm">
                  <div className="flex items-center gap-2 font-semibold text-[#027A48]">
                    <FiCheckCircle />
                    创作线已启动
                  </div>
                  <dl className="mt-3 space-y-2 text-xs text-[#24564A]">
                    <KeyValue label="Project" value={shortId(currentProject?.id)} title={currentProject?.id} />
                    <KeyValue label="Run" value={shortId(currentRun.id)} title={currentRun.id} />
                    <KeyValue label="Task" value={shortId(currentRun.taskId)} title={currentRun.taskId} />
                  </dl>
                  <button
                    type="button"
                    onClick={handleOpenTrace}
                    className="mt-4 inline-flex items-center gap-2 rounded-lg bg-[#00A86B] px-3 py-2 text-xs font-semibold text-white"
                  >
                    查看制作记录
                    <FiArrowRight />
                  </button>
                </div>
              ) : (
                <EmptyState title="等待启动" desc="启动后出现 run、task、trace 和中间态入口。" />
              )}
            </div>
          </div>

          <div className="rounded-lg border border-[#D9DEE7] bg-white">
            <PanelHeader icon={<FiDownload />} title="最近作品" desc={`${projects.length} 个项目`} />
            <div className="space-y-2 p-4">
              {projects.length === 0 && <p className="text-sm text-[#667085]">还没有项目。</p>}
              {projects.slice(0, 6).map((project) => (
                <div key={project.id} className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-3">
                  <div className="flex items-center justify-between gap-3">
                    <p className="min-w-0 truncate text-sm font-semibold">{project.name}</p>
                    <span className="shrink-0 rounded-full bg-[#EEF2F6] px-2 py-0.5 text-[10px] text-[#667085]">{project.status}</span>
                  </div>
                  <p className="mt-1 truncate text-xs text-[#667085]">{formatSkillName(project.skillName)} · {project.generationMode}</p>
                </div>
              ))}
            </div>
          </div>
        </aside>
      </main>

      {artifactModalOpen && (
        <ArtifactReviewModal
          content={artifactContent}
          loading={artifactsLoading}
          history={artifactHistory}
          reviewStages={reviewStages}
          revisionMessage={revisionMessage}
          revising={revisingArtifact}
          approvingStage={approvingStage}
          currentRun={currentRun}
          onRevisionChange={setRevisionMessage}
          onRevise={handleReviseArtifact}
          onApprove={handleApproveStage}
          onClose={() => setArtifactModalOpen(false)}
        />
      )}

      {traceOpen && (
        <TraceModal
          trace={traceData}
          loading={traceLoading}
          error={traceError}
          onClose={() => setTraceOpen(false)}
        />
      )}
    </div>
  )
}

const formatSkillName = (skill: string | Pick<SkillCatalogItem, 'name' | 'displayName'>) => {
  const name = typeof skill === 'string' ? skill : skill.name
  if (typeof skill !== 'string' && skill.displayName) return skill.displayName
  const map: Record<string, string> = {
    'create-opinion-videos': '口播 / 知识视频',
    'voice-visual-video': '口播可视化视频',
    'aigc-shot-video': '镜头式 AIGC 短片',
    'video-creator': '导演级视频流水线',
    'film-shot-reconstruction': '经典镜头学习',
  }
  return map[name] || name
}

interface PanelHeaderProps {
  icon: React.ReactNode
  title: string
  desc: string
}

const PanelHeader: React.FC<PanelHeaderProps> = ({ icon, title, desc }) => (
  <div className="flex items-center gap-3 border-b border-[#D9DEE7] px-5 py-4">
    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#16181D] text-white">{icon}</div>
    <div className="min-w-0">
      <h2 className="truncate text-sm font-semibold">{title}</h2>
      <p className="truncate text-xs text-[#667085]">{desc}</p>
    </div>
  </div>
)

const RouteCard: React.FC<{ route: CreatorRoute; routing: boolean }> = ({ route, routing }) => {
  const toneClasses = {
    blue: 'border-[#245BFF]/30 bg-[#EEF4FF] text-[#1A47C9]',
    green: 'border-[#00A86B]/30 bg-[#ECFDF3] text-[#027A48]',
    rose: 'border-[#E85D75]/30 bg-[#FFF1F3] text-[#B8324F]',
    amber: 'border-[#F5A524]/35 bg-[#FFF7E6] text-[#9A6700]',
  }
  return (
    <div className={`rounded-lg border p-4 ${toneClasses[route.tone]}`}>
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-semibold">系统判断路径</p>
        {routing && <FiRefreshCw className="shrink-0 animate-spin" />}
      </div>
      <h2 className="mt-1 text-lg font-semibold">{route.title}</h2>
      <p className="mt-1 text-sm leading-6 opacity-80">{route.subtitle}</p>
    </div>
  )
}

const StatusPill: React.FC<{ label: string; tone: 'dark' | 'blue' | 'green' | 'amber' }> = ({ label, tone }) => {
  const classes = {
    dark: 'bg-[#16181D] text-white',
    blue: 'bg-[#245BFF] text-white',
    green: 'bg-[#00A86B] text-white',
    amber: 'bg-[#FFF7E6] text-[#9A6700]',
  }
  return <span className={`rounded-full px-3 py-1 font-semibold ${classes[tone]}`}>{label}</span>
}

const MiniBadge: React.FC<{ label: string }> = ({ label }) => (
  <span className="rounded-full bg-[#EEF2F6] px-2 py-0.5 text-[10px] text-[#667085]">{label}</span>
)

const Metric: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div className="rounded-lg border border-[#D9DEE7] bg-white px-3 py-2">
    <p className="text-[10px] text-[#667085]">{label}</p>
    <p className="mt-0.5 break-words text-sm font-semibold">{value}</p>
  </div>
)

const Skeleton: React.FC<{ label: string }> = ({ label }) => (
  <div className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4 text-sm text-[#667085]">{label}</div>
)

const EmptyState: React.FC<{ title: string; desc: string }> = ({ title, desc }) => (
  <div className="rounded-lg border border-dashed border-[#C8D0DC] bg-[#FBFCFE] p-4">
    <p className="text-sm font-semibold">{title}</p>
    <p className="mt-1 text-xs text-[#667085]">{desc}</p>
  </div>
)

const ArtifactWorkbench: React.FC<{
  artifacts: Artifact[]
  selectedId: string
  loading: boolean
  currentRun: WorkflowRun | null
  onOpen: (id: string) => void
}> = ({ artifacts, selectedId, loading, currentRun, onOpen }) => (
  <div className="min-h-[220px] rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <p className="text-sm font-semibold">本次创作产物</p>
        <p className="mt-1 text-xs text-[#667085]">只展示可读、可审、可返工的内容；技术细节已收起。</p>
      </div>
      <MiniBadge label={`${artifacts.length} 个`} />
    </div>
    {loading && artifacts.length === 0 && <Skeleton label="正在读取产物" />}
    {!loading && artifacts.length === 0 && (
      <EmptyState
        title={currentRun ? '正在等待第一批产物' : '等待创作线'}
        desc={currentRun ? '流水线执行或审核通过后，这里会出现口播稿、发布文案和视频素材包。' : '启动后这里会出现可查看、可返工的作品卡片。'}
      />
    )}
    <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
      {artifacts.map((artifact) => {
        const selected = artifact.id === selectedId
        return (
          <button
            key={artifact.id}
            type="button"
            onClick={() => onOpen(artifact.id)}
            className={`group min-h-[142px] rounded-lg border p-4 text-left transition ${
              selected ? 'border-[#245BFF] bg-white shadow-sm' : 'border-[#D9DEE7] bg-white hover:border-[#245BFF]'
            }`}
          >
            <div className="flex h-full flex-col justify-between gap-4">
              <div>
                <div className="flex items-start justify-between gap-3">
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#EEF4FF] text-[#245BFF]">{artifactKindIcon(artifact.kind)}</span>
                  <MiniBadge label={`v${artifact.version}`} />
                </div>
                <p className="mt-3 text-base font-semibold">{artifactDisplayName(artifact)}</p>
                <p className="mt-1 line-clamp-2 text-xs leading-5 text-[#667085]">{artifactDisplayDescription(artifact)}</p>
              </div>
              <div className="flex items-center justify-between gap-3 text-xs font-semibold text-[#245BFF]">
                <span>{artifactKindLabel(artifact)}</span>
                <span className="inline-flex items-center gap-1 opacity-80 group-hover:opacity-100">
                  打开查看
                  <FiEye />
                </span>
              </div>
            </div>
          </button>
        )
      })}
    </div>
  </div>
)

const ArtifactReviewModal: React.FC<{
  content: ArtifactContentResponse | null
  loading: boolean
  history: Artifact[]
  reviewStages: SkillStage[]
  revisionMessage: string
  revising: boolean
  approvingStage: string
  currentRun: WorkflowRun | null
  onRevisionChange: (value: string) => void
  onRevise: () => void
  onApprove: (stageName: string) => void
  onClose: () => void
}> = ({ content, loading, history, reviewStages, revisionMessage, revising, approvingStage, currentRun, onRevisionChange, onRevise, onApprove, onClose }) => {
  const artifact = content?.artifact
  return (
    <div className="fixed inset-0 z-50 bg-[#101318]/60 p-3 backdrop-blur-sm sm:p-5" role="dialog" aria-modal="true">
      <div className="mx-auto flex h-full max-w-[1440px] flex-col overflow-hidden rounded-lg bg-white shadow-2xl">
        <div className="flex items-center justify-between gap-3 border-b border-[#D9DEE7] px-5 py-4">
          <div className="min-w-0">
            <p className="truncate text-base font-semibold">{artifact ? artifactDisplayName(artifact) : '产物查看'}</p>
            <p className="mt-1 text-xs text-[#667085]">{artifact ? `${artifactKindLabel(artifact)} · ${stageNameMap[artifact.stageName] || artifact.stageName}` : '正在读取内容'}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-[#D9DEE7] bg-white text-[#475467] hover:bg-[#F7F9FC]"
            aria-label="关闭"
          >
            <FiX />
          </button>
        </div>
        <div className="grid min-h-0 flex-1 gap-0 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="min-h-0 overflow-y-auto bg-[#F7F9FC] p-4 sm:p-6">
            {loading && !content ? (
              <Skeleton label="正在装载产物内容" />
            ) : content ? (
              <ArtifactRenderer content={content} />
            ) : (
              <EmptyState title="暂时没有内容" desc="稍后刷新产物列表，或等待当前阶段执行完成。" />
            )}
          </div>
          <ArtifactRevisionPanel
            content={content}
            history={history}
            reviewStages={reviewStages}
            revisionMessage={revisionMessage}
            revising={revising}
            approvingStage={approvingStage}
            currentRun={currentRun}
            onRevisionChange={onRevisionChange}
            onRevise={onRevise}
            onApprove={onApprove}
          />
        </div>
      </div>
    </div>
  )
}

const ArtifactRenderer: React.FC<{ content: ArtifactContentResponse }> = ({ content }) => {
  const artifact = content.artifact
  const mediaUrls = uniqueMediaUrls([
    ...(content.mediaUrls || []),
    content.mediaUrl || '',
    ...extractMediaUrls(content.content),
  ])
  if (artifact.kind === 'MARKDOWN') {
    return <MarkdownDocument text={String(content.content || '')} />
  }
  if (artifact.kind === 'IMAGE') {
    return <ImageArtifact content={content.content} mediaUrls={mediaUrls} />
  }
  if (artifact.kind === 'VIDEO') {
    return <VideoArtifact content={content.content} mediaUrls={mediaUrls} />
  }
  if (artifact.kind === 'AUDIO') {
    return <AudioArtifact content={content.content} mediaUrls={mediaUrls} />
  }
  return <ReadableArtifact artifact={artifact} content={content.content} />
}

const MarkdownDocument: React.FC<{ text: string }> = ({ text }) => {
  const lines = text.split('\n')
  return (
    <article className="min-h-[520px] rounded-lg bg-white p-6 text-base leading-8 text-[#1D2939] shadow-sm">
      {lines.map((line, index) => {
        if (line.startsWith('## ')) return <h2 key={index} className="mt-4 text-lg font-semibold first:mt-0">{line.slice(3)}</h2>
        if (line.startsWith('### ')) return <h3 key={index} className="mt-3 text-base font-semibold">{line.slice(4)}</h3>
        if (line.startsWith('- ')) return <p key={index} className="pl-3">· {line.slice(2)}</p>
        if (line.trim() === '') return <div key={index} className="h-3" />
        return <p key={index}>{line}</p>
      })}
    </article>
  )
}

const ImageArtifact: React.FC<{ content: unknown; mediaUrls: string[] }> = ({ content, mediaUrls }) => {
  const items = Array.isArray(content) ? content : [content]
  const galleryUrls = useMemo(() => {
    const extracted = uniqueMediaUrls([...mediaUrls, ...extractMediaUrls(content)])
    const images = extracted.filter((url) => isLikelyImageURL(url))
    return images.length > 0 ? images : extracted.filter((url) => !isLikelyAudioURL(url) && !isLikelyVideoURL(url))
  }, [content, mediaUrls])
  const [selectedUrl, setSelectedUrl] = useState(galleryUrls[0] || '')

  useEffect(() => {
    setSelectedUrl(galleryUrls[0] || '')
  }, [galleryUrls])

  return (
    <div className="space-y-4">
      {selectedUrl && (
        <div className="overflow-hidden rounded-lg border border-[#D9DEE7] bg-[#0E1116]">
          <img src={selectedUrl} className="max-h-[560px] w-full object-contain" />
        </div>
      )}
      {galleryUrls.length > 1 && (
        <div className="grid gap-2 sm:grid-cols-3">
          {galleryUrls.map((url, index) => (
            <button
              key={url}
              onClick={() => setSelectedUrl(url)}
              className={`overflow-hidden rounded-lg border bg-[#0E1116] ${selectedUrl === url ? 'border-[#245BFF]' : 'border-[#D9DEE7]'}`}
            >
              <img src={url} className="h-24 w-full object-cover" />
              <span className="block truncate bg-white px-2 py-1 text-left text-[11px] text-[#667085]">图片 {index + 1}</span>
            </button>
          ))}
        </div>
      )}
      <div className="grid gap-3 md:grid-cols-2">
        {items.map((item, index) => (
          <div key={index} className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
            <p className="text-xs font-semibold text-[#667085]">参考帧 {index + 1}</p>
            <p className="mt-2 text-sm leading-6">{String(readField(item, 'prompt') || readField(item, 'description') || '图片产物')}</p>
          </div>
        ))}
      </div>
    </div>
  )
}

const VideoArtifact: React.FC<{ content: unknown; mediaUrls: string[] }> = ({ content, mediaUrls }) => {
  const videoUrls = useMemo(() => {
    const videos = mediaUrls.filter((url) => isLikelyVideoURL(url))
    return videos.length > 0 ? videos : mediaUrls.filter((url) => !isLikelyImageURL(url) && !isLikelyAudioURL(url))
  }, [mediaUrls])
  const poster = useMemo(() => findPosterURL(content, mediaUrls), [content, mediaUrls])
  const [selectedVideo, setSelectedVideo] = useState(videoUrls[0] || '')

  useEffect(() => {
    setSelectedVideo(videoUrls[0] || '')
  }, [videoUrls])

  return (
    <div className="space-y-4">
      <div className="overflow-hidden rounded-lg border border-[#D9DEE7] bg-[#0E1116]">
        {selectedVideo ? (
          <video key={selectedVideo} controls preload="metadata" poster={poster || undefined} className="max-h-[560px] w-full bg-black" src={selectedVideo} />
        ) : (
          <div className="flex min-h-[320px] items-center justify-center px-6 text-center text-sm text-white/75">
            当前产物是视频生成素材包，导入外部网页视频工具后可在这里播放成片。
          </div>
        )}
      </div>
      {videoUrls.length > 0 && (
        <div className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <p className="text-xs font-semibold text-[#667085]">视频轨道</p>
            {selectedVideo && (
              <a href={selectedVideo} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs font-semibold text-[#245BFF]">
                <FiDownload />
                打开素材
              </a>
            )}
          </div>
          <div className="space-y-2">
            {videoUrls.map((url, index) => (
              <button
                key={url}
                onClick={() => setSelectedVideo(url)}
                className={`flex w-full items-center gap-3 rounded-lg border px-3 py-2 text-left ${selectedVideo === url ? 'border-[#245BFF] bg-white' : 'border-[#D9DEE7] bg-white/70'}`}
              >
                <FiPlayCircle className="shrink-0 text-[#245BFF]" />
                <span className="min-w-0 flex-1">
                  <span className="block text-xs font-semibold">视频 {index + 1}</span>
                  <span className="block truncate text-[11px] text-[#667085]">{url}</span>
                </span>
              </button>
            ))}
          </div>
        </div>
      )}
      <ReadableArtifact artifact={{ kind: 'VIDEO', unitId: 'video-package', stageName: '', name: '', version: 1 } as Artifact} content={content} />
    </div>
  )
}

const AudioArtifact: React.FC<{ content: unknown; mediaUrls: string[] }> = ({ content, mediaUrls }) => {
  const audioUrls = useMemo(() => {
    const audios = mediaUrls.filter((url) => isLikelyAudioURL(url))
    return audios.length > 0 ? audios : mediaUrls.filter((url) => !isLikelyImageURL(url) && !isLikelyVideoURL(url))
  }, [mediaUrls])

  return (
    <div className="space-y-4">
      <div className="space-y-3 rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
        {audioUrls.length > 0 ? (
          audioUrls.map((url, index) => (
            <div key={url} className="rounded-lg border border-[#D9DEE7] bg-white p-3">
              <div className="mb-2 flex items-center justify-between gap-2">
                <p className="text-xs font-semibold text-[#667085]">音频 {index + 1}</p>
                <a href={url} target="_blank" rel="noreferrer" className="truncate text-[11px] font-semibold text-[#245BFF]">打开素材</a>
              </div>
              <audio controls preload="metadata" className="w-full" src={url} />
            </div>
          ))
        ) : (
          <p className="text-sm text-[#667085]">当前音频产物没有可播放 URL。</p>
        )}
      </div>
      <ReadableArtifact artifact={{ kind: 'AUDIO', unitId: 'audio-package', stageName: '', name: '', version: 1 } as Artifact} content={content} />
    </div>
  )
}

const ReadableArtifact: React.FC<{ artifact: Artifact; content: unknown }> = ({ artifact, content }) => {
  const record = asRecord(content)
  if (!record) {
    return <MarkdownDocument text={String(content || '')} />
  }

  if (artifact.unitId === 'publish-copy' || record.title || record.description || record.keywords) {
    return <PublishCopyView record={record} />
  }

  return (
    <div className="space-y-4 rounded-lg bg-white p-5 shadow-sm">
      <div>
        <p className="text-sm font-semibold">{artifact.kind === 'VIDEO' ? '视频素材包' : artifact.kind === 'AUDIO' ? '音频素材包' : '素材说明'}</p>
        <p className="mt-1 text-xs text-[#667085]">这里展示可直接拿去制作或返工的信息，已隐藏技术字段。</p>
      </div>
      <ReadableValue value={record} />
    </div>
  )
}

const PublishCopyView: React.FC<{ record: Record<string, unknown> }> = ({ record }) => {
  const keywords = toStringList(record.keywords)
  return (
    <div className="space-y-4 rounded-lg bg-white p-5 shadow-sm">
      <div className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
        <p className="text-xs font-semibold text-[#667085]">标题</p>
        <p className="mt-2 text-xl font-semibold leading-8">{String(record.title || '待补充标题')}</p>
      </div>
      <div className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
        <p className="text-xs font-semibold text-[#667085]">简介</p>
        <p className="mt-2 whitespace-pre-wrap text-sm leading-7">{String(record.description || '待补充简介')}</p>
      </div>
      <div className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4">
        <p className="text-xs font-semibold text-[#667085]">关键词</p>
        <div className="mt-3 flex flex-wrap gap-2">
          {keywords.length > 0 ? keywords.map((keyword) => (
            <span key={keyword} className="rounded-full bg-[#EEF4FF] px-3 py-1 text-xs font-semibold text-[#245BFF]">{keyword}</span>
          )) : <span className="text-sm text-[#667085]">待补充关键词</span>}
        </div>
      </div>
    </div>
  )
}

const ReadableValue: React.FC<{ value: unknown; level?: number }> = ({ value, level = 0 }) => {
  if (value == null || value === '') return null
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return <p className="whitespace-pre-wrap text-sm leading-7 text-[#1D2939]">{String(value)}</p>
  }
  if (Array.isArray(value)) {
    return (
      <div className="space-y-3">
        {value.map((item, index) => (
          <div key={index} className="rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-3">
            <p className="mb-2 text-xs font-semibold text-[#667085]">素材 {index + 1}</p>
            <ReadableValue value={item} level={level + 1} />
          </div>
        ))}
      </div>
    )
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([key, item]) => shouldShowReadableField(key, item))
    if (entries.length === 0) return <p className="text-sm text-[#667085]">暂无可展示内容。</p>
    return (
      <div className="space-y-3">
        {entries.map(([key, item]) => (
          <div key={key} className={level > 0 ? '' : 'rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-4'}>
            <p className="text-xs font-semibold text-[#667085]">{fieldLabel(key)}</p>
            <div className="mt-2">
              <ReadableValue value={item} level={level + 1} />
            </div>
          </div>
        ))}
      </div>
    )
  }
  return null
}

const ArtifactRevisionPanel: React.FC<{
  content: ArtifactContentResponse | null
  history: Artifact[]
  reviewStages: SkillStage[]
  revisionMessage: string
  revising: boolean
  approvingStage: string
  currentRun: WorkflowRun | null
  onRevisionChange: (value: string) => void
  onRevise: () => void
  onApprove: (stageName: string) => void
}> = ({ content, history, reviewStages, revisionMessage, revising, approvingStage, currentRun, onRevisionChange, onRevise, onApprove }) => {
  const stageName = content?.artifact.stageName
  const reviewStage = stageName ? reviewStages.find((stage) => stage.name === stageName) : undefined
  return (
    <div className="min-h-0 overflow-y-auto border-t border-[#D9DEE7] bg-white p-4 lg:border-l lg:border-t-0">
      <div className="flex items-center gap-2 text-sm font-semibold">
        <FiMessageSquare />
        对这份产物说修改意见
      </div>
      <p className="mt-2 text-xs leading-5 text-[#667085]">像给图片补充描述一样，直接说不满意哪里，系统会生成新版本并保留历史。</p>
      <textarea
        value={revisionMessage}
        onChange={(event) => onRevisionChange(event.target.value)}
        disabled={!content || revising}
        className="mt-3 min-h-32 w-full resize-y rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] px-3 py-2 text-sm leading-6 outline-none focus:border-[#245BFF] focus:ring-2 focus:ring-[#245BFF]/15 disabled:bg-[#EEF2F6]"
        placeholder="例如：开头更有冲突感；把屈原和龙舟关系讲清楚；标题更像小红书知识号。"
      />
      <div className="mt-3 flex flex-wrap gap-2">
        <button
          onClick={onRevise}
          disabled={!content || !revisionMessage.trim() || revising}
          className="inline-flex items-center gap-2 rounded-lg bg-[#16181D] px-3 py-2 text-xs font-semibold text-white disabled:bg-[#AAB2C0]"
        >
          {revising ? <FiRefreshCw className="animate-spin" /> : <FiRefreshCw />}
          生成返工版本
        </button>
        <button
          onClick={() => stageName && onApprove(stageName)}
          disabled={!currentRun || !reviewStage || approvingStage === stageName}
          className="inline-flex items-center gap-2 rounded-lg border border-[#D9DEE7] bg-white px-3 py-2 text-xs font-semibold text-[#16181D] disabled:text-[#98A2B3]"
        >
          <FiCheckCircle />
          {approvingStage === stageName ? '确认中' : '确认通过'}
        </button>
      </div>
      <div className="mt-5">
        <p className="text-xs font-semibold text-[#667085]">版本历史</p>
        <div className="mt-2 space-y-2">
          {history.length === 0 && <p className="text-xs text-[#667085]">暂无历史版本。</p>}
          {history.map((artifact) => (
            <div key={artifact.id} className="rounded-lg border border-[#D9DEE7] bg-white px-3 py-2">
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs font-semibold">v{artifact.version}</span>
                <span className="text-[10px] text-[#667085]">{artifact.isCurrent ? '当前' : '历史'}</span>
              </div>
              <p className="mt-1 truncate text-[11px] text-[#667085]">{artifact.provider || artifact.model || artifact.createdAt}</p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

const TraceModal: React.FC<{
  trace: TraceData | null
  loading: boolean
  error: string
  onClose: () => void
}> = ({ trace, loading, error, onClose }) => {
  const nodes = trace?.task.nodes || []
  const contexts = trace?.contexts || []
  return (
    <div className="fixed inset-0 z-50 bg-[#101318]/60 p-3 backdrop-blur-sm sm:p-5" role="dialog" aria-modal="true">
      <div className="mx-auto flex h-full max-w-5xl flex-col overflow-hidden rounded-lg bg-white shadow-2xl">
        <div className="flex items-center justify-between gap-3 border-b border-[#D9DEE7] px-5 py-4">
          <div>
            <p className="text-base font-semibold">制作记录</p>
            <p className="mt-1 text-xs text-[#667085]">{trace?.task.taskId ? `任务 ${shortId(trace.task.taskId)}` : '读取这条创作线的执行过程'}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-[#D9DEE7] bg-white text-[#475467] hover:bg-[#F7F9FC]"
            aria-label="关闭"
          >
            <FiX />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto bg-[#F7F9FC] p-5">
          {loading && <Skeleton label="正在读取制作记录" />}
          {error && (
            <div className="rounded-lg border border-[#E85D75]/30 bg-[#FFF1F3] p-4 text-sm text-[#9D2540]">{error}</div>
          )}
          {!loading && !error && trace && (
            <div className="grid gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
              <div className="space-y-3">
                <div className="rounded-lg border border-[#D9DEE7] bg-white p-4">
                  <p className="text-xs font-semibold text-[#667085]">整体状态</p>
                  <p className="mt-2 text-2xl font-semibold">{statusLabel(trace.task.status)}</p>
                  <p className="mt-2 text-xs leading-5 text-[#667085]">{nodes.length} 个步骤，{contexts.length} 条过程记录。</p>
                </div>
                {nodes.map((node) => (
                  <div key={node.id} className="rounded-lg border border-[#D9DEE7] bg-white p-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold">{stageNameMap[node.id] || stageNameMap[node.name] || node.name || node.id}</p>
                        <p className="mt-1 text-[11px] text-[#667085]">{node.type}</p>
                      </div>
                      <MiniBadge label={statusLabel(node.status)} />
                    </div>
                    {node.errorMessage && <p className="mt-2 text-xs leading-5 text-[#9D2540]">{node.errorMessage}</p>}
                  </div>
                ))}
              </div>
              <div className="rounded-lg border border-[#D9DEE7] bg-white p-4">
                <p className="text-sm font-semibold">过程时间线</p>
                <div className="mt-4 space-y-4">
                  {contexts.map((item) => (
                    <div key={item.id} className="flex gap-3">
                      <div className="mt-1 h-2.5 w-2.5 shrink-0 rounded-full bg-[#245BFF]" />
                      <div className="min-w-0 flex-1 border-b border-[#EEF2F6] pb-4">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-sm font-semibold">{contextLabel(item.contextType)}</p>
                          {item.nodeId && <span className="rounded-full bg-[#EEF2F6] px-2 py-0.5 text-[10px] text-[#667085]">{stageNameMap[item.nodeId] || item.nodeId}</span>}
                        </div>
                        <p className="mt-1 text-sm leading-6 text-[#475467]">{item.message}</p>
                        <p className="mt-1 text-[11px] text-[#98A2B3]">{formatTime(item.createdAt)}</p>
                      </div>
                    </div>
                  ))}
                  {contexts.length === 0 && <p className="text-sm text-[#667085]">暂无过程记录。</p>}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

const artifactDisplayName = (artifact: Artifact) => {
  if (artifact.unitId === 'publish-copy') return '标题、简介和关键词'
  if (artifact.kind === 'VIDEO') return '视频素材包'
  if (artifact.kind === 'AUDIO') return '音频素材'
  if (artifact.kind === 'IMAGE') return '参考帧 / 图片'
  return stageNameMap[artifact.stageName] || artifact.name.replace(/\.(md|json)$/i, '')
}

const artifactKindLabel = (artifact: Artifact) => {
  if (artifact.unitId === 'publish-copy') return '发布文案'
  if (artifact.kind === 'MARKDOWN') return '文稿'
  if (artifact.kind === 'IMAGE') return '图片'
  if (artifact.kind === 'VIDEO') return '视频'
  if (artifact.kind === 'AUDIO') return '音频'
  return '素材'
}

const artifactDisplayDescription = (artifact: Artifact) => {
  if (artifact.unitId === 'publish-copy') return '面向发布页的标题、简介和关键词，不展示原始 JSON。'
  if (artifact.kind === 'MARKDOWN') return '可直接阅读和返工的创作文本。'
  if (artifact.kind === 'VIDEO') return '可播放成片或可导入网页视频工具的素材包。'
  if (artifact.kind === 'AUDIO') return '可播放音频、旁白或音频生成素材。'
  if (artifact.kind === 'IMAGE') return '可查看参考帧、关键帧或图片生成请求。'
  return '可查看、确认和返工的创作素材。'
}

const artifactKindIcon = (kind: Artifact['kind']) => {
  if (kind === 'MARKDOWN') return <FiFileText />
  if (kind === 'IMAGE') return <FiImage />
  if (kind === 'VIDEO') return <FiFilm />
  if (kind === 'AUDIO') return <FiMusic />
  return <FiList />
}

const statusLabel = (status: string) => {
  const map: Record<string, string> = {
    CREATED: '等待中',
    READY: '已就绪',
    RUNNING: '制作中',
    SUCCESS: '已完成',
    FAILED: '失败',
    SKIPPED: '已跳过',
    RETRYING: '重试中',
    PAUSED: '已暂停',
    COMPLETED: '已完成',
  }
  return map[status] || status
}

const contextLabel = (type: string) => {
  const map: Record<string, string> = {
    TASK_CREATED: '创建任务',
    DAG_VALIDATED: '检查流程',
    DAG_SUBMITTED: '提交流程',
    NODE_READY: '步骤就绪',
    NODE_SCHEDULED: '开始制作',
    NODE_SUCCESS: '步骤完成',
    NODE_FAILED: '步骤失败',
    NODE_REVIEW_REQUIRED: '等待确认',
    NODE_SKIPPED: '跳过步骤',
    TASK_SUCCESS: '任务完成',
    TASK_FAILED: '任务失败',
  }
  return map[type] || type
}

const formatTime = (value?: string) => {
  if (!value) return ''
  try {
    return new Date(value).toLocaleString('zh-CN', { hour12: false })
  } catch {
    return value
  }
}

const extractMediaUrls = (value: unknown): string[] => {
  if (!value) return []
  if (typeof value === 'string') {
    const trimmed = value.trim()
    return isMediaURL(trimmed) ? [trimmed] : []
  }
  if (Array.isArray(value)) {
    const urls: string[] = []
    for (const item of value) {
      urls.push(...extractMediaUrls(item))
    }
    return uniqueMediaUrls(urls)
  }
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>
    const urls: string[] = []
    for (const key of ['url', 'mediaUrl', 'dataUrl', 'imageUrl', 'videoUrl', 'audioUrl', 'coverUrl', 'posterUrl', 'thumbnailUrl', 'previewUrl', 'downloadUrl', 'src']) {
      if (typeof record[key] === 'string' && isMediaURL(record[key] as string)) urls.push(record[key] as string)
    }
    Object.entries(record).forEach(([key, item]) => {
      if (!['url', 'mediaUrl', 'dataUrl', 'imageUrl', 'videoUrl', 'audioUrl', 'coverUrl', 'posterUrl', 'thumbnailUrl', 'previewUrl', 'downloadUrl', 'src'].includes(key)) {
        urls.push(...extractMediaUrls(item))
      }
    })
    return uniqueMediaUrls(urls)
  }
  return []
}

const uniqueMediaUrls = (urls: Array<string | undefined>): string[] => {
  const seen = new Set<string>()
  const result: string[] = []
  urls.forEach((url) => {
    const trimmed = (url || '').trim()
    if (!isMediaURL(trimmed) || seen.has(trimmed)) return
    seen.add(trimmed)
    result.push(trimmed)
  })
  return result
}

const isMediaURL = (url: string) => (
  url.startsWith('http://') ||
  url.startsWith('https://') ||
  url.startsWith('data:') ||
  url.startsWith('blob:') ||
  url.startsWith('/')
)

const isLikelyImageURL = (url: string) => /^data:image\//.test(url) || /\.(png|jpe?g|webp|gif|avif|svg)(\?|#|$)/i.test(url)
const isLikelyVideoURL = (url: string) => /^data:video\//.test(url) || /\.(mp4|mov|webm|m4v|avi|mkv)(\?|#|$)/i.test(url)
const isLikelyAudioURL = (url: string) => /^data:audio\//.test(url) || /\.(mp3|wav|m4a|aac|ogg|flac)(\?|#|$)/i.test(url)

const findPosterURL = (content: unknown, mediaUrls: string[]) => {
  if (content && typeof content === 'object' && !Array.isArray(content)) {
    const record = content as Record<string, unknown>
    for (const key of ['posterUrl', 'coverUrl', 'thumbnailUrl', 'imageUrl']) {
      if (typeof record[key] === 'string' && isLikelyImageURL(record[key] as string)) return record[key] as string
    }
  }
  return mediaUrls.find((url) => isLikelyImageURL(url)) || ''
}

const readField = (value: unknown, key: string) => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return (value as Record<string, unknown>)[key]
  }
  return undefined
}

const asRecord = (value: unknown): Record<string, unknown> | null => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return null
}

const toStringList = (value: unknown): string[] => {
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean)
  if (typeof value === 'string') {
    return value.split(/[,，\s]+/).map((item) => item.trim()).filter(Boolean)
  }
  return []
}

const shouldShowReadableField = (key: string, value: unknown) => {
  if (value == null || value === '') return false
  if (['status', 'provider', 'model', 'storageRef', 'contentHash', 'revisionOf'].includes(key)) return false
  if (typeof value === 'string' && isMediaURL(value)) return false
  if (key.toLowerCase().includes('url') && typeof value === 'string') return false
  return true
}

const fieldLabel = (key: string) => {
  const map: Record<string, string> = {
    title: '标题',
    description: '简介',
    keywords: '关键词',
    content: '正文',
    prompt: '提示词',
    videoPrompt: '视频提示词',
    negativePrompt: '负面提示词',
    imagePrompt: '图片提示词',
    transcript: '转写 / 旁白',
    voiceover: '旁白',
    narration: '口播稿',
    script: '脚本',
    scenes: '分镜',
    shots: '镜头',
    clips: '片段',
    referenceFrames: '参考帧',
    keyframes: '关键帧',
    importSteps: '导入步骤',
    revisionInstruction: '返工要求',
    previous: '上一版内容',
    videoImportPackage: '视频生成素材包',
    audioPackage: '音频素材包',
    imageRequests: '图片生成请求',
    publishCopy: '发布文案',
  }
  return map[key] || key.replace(/([A-Z])/g, ' $1').replace(/_/g, ' ').trim()
}

const MaterialRow: React.FC<{ icon: React.ReactNode; title: string; desc: string }> = ({ icon, title, desc }) => (
  <div className="flex gap-3 rounded-lg border border-[#D9DEE7] bg-[#FBFCFE] p-3">
    <div className="mt-0.5 shrink-0 text-[#245BFF]">{icon}</div>
    <div className="min-w-0">
      <p className="text-sm font-semibold">{title}</p>
      <p className="mt-0.5 text-xs leading-5 text-[#667085]">{desc}</p>
    </div>
  </div>
)

const KeyValue: React.FC<{ label: string; value: string; title?: string }> = ({ label, value, title }) => (
  <div className="flex justify-between gap-3">
    <dt>{label}</dt>
    <dd className="font-mono" title={title}>{value}</dd>
  </div>
)

export default CreatorWorkbenchPage
