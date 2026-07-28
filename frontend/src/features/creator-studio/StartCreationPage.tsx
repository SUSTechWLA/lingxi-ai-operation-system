import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { createVideoProject, startAgentRun } from '../../services/api'
import { buildClientModelProvidersForRun, uploadLocalArtifactFile, type LocalArtifactUploadResponse } from '../../services/localAgent'
import { registerProjectMaterial } from '../../services/creatorApi'
import type { ProjectMaterial, ProjectMaterialKind } from './types'
import type { CreatorVoiceMode, CreatorVoiceSelection } from './types'
import { buildCreationRequest, buildProjectMaterialStorageRef, creatorStartIdempotencyKey, validateCreatorVoiceSelection } from './logic'
import type { CreatorAIGCPolicy, CreatorProductionRoute } from './logic'

type MaterialStatus = 'ready' | 'uploading' | 'success' | 'failed'

interface MaterialItem {
  id: string
  file: File
  status: MaterialStatus
  upload?: LocalArtifactUploadResponse
}

interface VoiceFileItem {
  id: string
  file: File
  status: MaterialStatus
  upload?: LocalArtifactUploadResponse
}

interface StartCreationPageProps {
  onOpenProject: (projectId: string) => void
}

const durationOptions = [
  { value: '', label: '暂不设定' },
  { value: '15', label: '15 秒' },
  { value: '30', label: '30 秒' },
  { value: '60', label: '60 秒' },
]

const aspectOptions = ['9:16', '16:9', '1:1']
const platformOptions = ['', '抖音', '小红书', '视频号', 'B站']
const productionRouteOptions: { value: CreatorProductionRoute; label: string }[] = [
  { value: 'talking_head', label: 'IP 口播视频' },
  { value: 'cinematic_story', label: '影视短片' },
]
const aigcPolicyOptions: { value: CreatorAIGCPolicy; label: string }[] = [
  { value: 'auto', label: '智能补充素材' },
  { value: 'disabled', label: '仅使用本地素材' },
]

export default function StartCreationPage({ onOpenProject }: StartCreationPageProps) {
  const [prompt, setPrompt] = useState('')
  const [durationValue, setDurationValue] = useState('')
  const [aspectRatio, setAspectRatio] = useState('9:16')
  const [platform, setPlatform] = useState('')
  const [productionRoute, setProductionRoute] = useState<CreatorProductionRoute>('talking_head')
  const [aigcPolicy, setAigcPolicy] = useState<CreatorAIGCPolicy>('auto')
  const [voiceMode, setVoiceMode] = useState<CreatorVoiceMode>('default_ip')
  const [referenceText, setReferenceText] = useState('')
  const [referenceTextVerified, setReferenceTextVerified] = useState(false)
  const [usageRightsConfirmed, setUsageRightsConfirmed] = useState(false)
  const [voiceFile, setVoiceFile] = useState<VoiceFileItem>()
  const [materials, setMaterials] = useState<MaterialItem[]>([])
  const [projectId, setProjectId] = useState<string>()
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string>()
  const [optionsOpen, setOptionsOpen] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const voiceFileInputRef = useRef<HTMLInputElement>(null)
  const activeRef = useRef(true)
  const projectIdRef = useRef<string>()
  const operationControllerRef = useRef<AbortController>()

  useEffect(() => {
    activeRef.current = true
    return () => {
      activeRef.current = false
      operationControllerRef.current?.abort()
    }
  }, [])

  const isCurrentOperation = (controller: AbortController) => activeRef.current &&
    operationControllerRef.current === controller && !controller.signal.aborted

  const beginOperation = () => {
    operationControllerRef.current?.abort()
    const controller = new AbortController()
    operationControllerRef.current = controller
    return controller
  }

  const setProjectIdSafely = (nextProjectId: string) => {
    projectIdRef.current = nextProjectId
    if (activeRef.current) setProjectId(nextProjectId)
  }

  const updateMaterial = (id: string, patch: Partial<MaterialItem>) => {
    if (!activeRef.current) return
    setMaterials(current => current.map(item => item.id === id ? { ...item, ...patch } : item))
  }

  const updateVoiceFile = (id: string, patch: Partial<VoiceFileItem>) => {
    if (!activeRef.current) return
    setVoiceFile(current => current?.id === id ? { ...current, ...patch } : current)
  }

  const addMaterials = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files || [])
    event.target.value = ''
    if (files.length === 0) return
    setMaterials(current => [
      ...current,
      ...files.map((file, index) => ({
        id: `material-${Date.now()}-${index}`,
        file,
        status: 'ready' as const,
      })),
    ])
  }

  const addVoiceFile = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    if (!isSupportedVoiceFile(file)) {
      setError('录音仅支持 WAV、MP3、M4A 或 FLAC。')
      return
    }
    setError(undefined)
    setVoiceFile({
      id: `voice-${Date.now()}`,
      file,
      status: 'ready',
    })
  }

  const processMaterial = async (item: MaterialItem, nextProjectId: string, signal: AbortSignal): Promise<boolean> => {
    updateMaterial(item.id, { status: 'uploading' })
    try {
      const upload = item.upload || await uploadLocalArtifactFile({
        projectId: nextProjectId,
        id: item.id,
        storageRef: buildProjectMaterialStorageRef(nextProjectId, item.id),
        file: item.file,
        mimeType: item.file.type || undefined,
        signal,
        metadata: {
          artifactType: 'project_source_material',
          source: 'creator_studio',
          localOnly: true,
        },
      })
      if (signal.aborted || !activeRef.current) return false
      updateMaterial(item.id, { upload })
      await registerProjectMaterial(nextProjectId, materialFromUpload(item.file, upload), signal)
      if (signal.aborted || !activeRef.current) return false
      updateMaterial(item.id, { status: 'success', upload })
      return true
    } catch {
      if (!signal.aborted) updateMaterial(item.id, { status: 'failed' })
      return false
    }
  }

  const processRemainingMaterials = async (nextProjectId: string, signal: AbortSignal): Promise<boolean> => {
    let allSucceeded = true
    for (const item of materials) {
      if (signal.aborted || !activeRef.current) return false
      if (item.status === 'success') continue
      const succeeded = await processMaterial(item, nextProjectId, signal)
      allSucceeded = allSucceeded && succeeded
    }
    return allSucceeded
  }

  const processVoiceFile = async (
    item: VoiceFileItem,
    nextProjectId: string,
    signal: AbortSignal,
  ): Promise<LocalArtifactUploadResponse | undefined> => {
    updateVoiceFile(item.id, { status: 'uploading' })
    try {
      const upload = item.upload || await uploadLocalArtifactFile({
        projectId: nextProjectId,
        id: item.id,
        storageRef: buildProjectMaterialStorageRef(nextProjectId, item.id),
        file: item.file,
        mimeType: voiceMimeType(item.file),
        signal,
        metadata: {
          artifactType: voiceMode === 'recorded_narration' ? 'recorded_narration' : 'voice_reference',
          source: 'creator_studio',
          localOnly: true,
          usageRightsConfirmed,
        },
      })
      if (signal.aborted || !activeRef.current || !upload.storageRef || !upload.contentHash) return undefined
      updateVoiceFile(item.id, { status: 'success', upload })
      return upload
    } catch {
      if (!signal.aborted) updateVoiceFile(item.id, { status: 'failed' })
      return undefined
    }
  }

  const voiceSelectionDraft = (): CreatorVoiceSelection => {
    if (voiceMode === 'default_ip') {
      return {
        mode: 'default_ip',
        provider: 'gpt_sovits_local',
        voiceId: 'main_ip_warm_knowledge_host_v1',
      }
    }
    if (voiceMode === 'reference_clone') {
      return {
        mode: 'reference_clone',
        provider: 'gpt_sovits_local',
        voiceId: `project_reference_voice_${projectIdRef.current || 'pending'}`,
        referenceText: referenceText.trim(),
        referenceTextVerified,
        usageRightsConfirmed,
      }
    }
    return {
      mode: 'recorded_narration',
      usageRightsConfirmed,
    }
  }

  const retryMaterial = async (item: MaterialItem) => {
    if (!projectId || starting) return
    const controller = beginOperation()
    if (isCurrentOperation(controller)) {
      setStarting(true)
      setError(undefined)
    }
    try {
      await processMaterial(item, projectId, controller.signal)
    } finally {
      if (isCurrentOperation(controller)) setStarting(false)
    }
  }

  const startCreation = async () => {
    if (!prompt.trim() || starting) return
    const selectedVoice = voiceSelectionDraft()
    if (productionRoute === 'talking_head') {
      const validationError = validateCreatorVoiceSelection(selectedVoice)
      if (validationError) {
        setError(validationError)
        setOptionsOpen(true)
        return
      }
      if (selectedVoice.mode !== 'default_ip' && !voiceFile) {
        setError('请先选择一段本地录音。')
        setOptionsOpen(true)
        return
      }
    }
    const controller = beginOperation()
    if (!isCurrentOperation(controller)) return
    setStarting(true)
    setError(undefined)
    let nextProjectId = projectIdRef.current
    try {
      const durationSec = durationValue ? Number(durationValue) : undefined
      const modelProviders = await buildClientModelProvidersForRun()
      if (!isCurrentOperation(controller)) return
      const request = buildCreationRequest({
        prompt,
        durationSec,
        aspectRatio,
        platform: platform || undefined,
        materialCount: materials.length,
        productionRoute,
        aigcPolicy,
        modelProviders,
        voiceSelection: productionRoute === 'talking_head' ? selectedVoice : undefined,
      })
      if (!nextProjectId) {
        const project = await createVideoProject(request.project, controller.signal)
        if (!isCurrentOperation(controller)) return
        nextProjectId = project.id
        setProjectIdSafely(project.id)
      }
      const materialsReady = await processRemainingMaterials(nextProjectId, controller.signal)
      if (!materialsReady) {
        if (isCurrentOperation(controller)) setError('部分素材未准备好，请重试失败的文件后再开始创作。')
        return
      }
      let runtimeVoiceSelection = request.agentRun.context?.voiceSelection as CreatorVoiceSelection | undefined
      if (productionRoute === 'talking_head' && selectedVoice.mode !== 'default_ip') {
        const currentVoiceFile = voiceFile
        if (!currentVoiceFile) {
          if (isCurrentOperation(controller)) setError('请先选择一段本地录音。')
          return
        }
        const upload = await processVoiceFile(currentVoiceFile, nextProjectId, controller.signal)
        if (!upload?.storageRef || !upload.contentHash) {
          if (isCurrentOperation(controller)) setError('录音未准备好，请重试。')
          return
        }
        runtimeVoiceSelection = selectedVoice.mode === 'reference_clone'
          ? {
              ...selectedVoice,
              voiceId: `project_reference_voice_${nextProjectId}`,
              referenceArtifactId: upload.id,
              referenceStorageRef: upload.storageRef,
              referenceContentHash: upload.contentHash,
              referenceMimeType: upload.mimeType || voiceMimeType(currentVoiceFile.file),
            }
          : {
              ...selectedVoice,
              recordedNarrationArtifactId: upload.id,
              recordedNarrationStorageRef: upload.storageRef,
              recordedNarrationContentHash: upload.contentHash,
              recordedNarrationMimeType: upload.mimeType || voiceMimeType(currentVoiceFile.file),
            }
      }
      await startAgentRun({
        ...request.agentRun,
        context: {
          ...request.agentRun.context,
          projectId: nextProjectId,
          ...(runtimeVoiceSelection ? { voiceSelection: runtimeVoiceSelection } : {}),
        },
      }, {
        idempotencyKey: creatorStartIdempotencyKey(nextProjectId),
        signal: controller.signal,
      })
      if (isCurrentOperation(controller)) onOpenProject(nextProjectId)
    } catch {
      if (isCurrentOperation(controller)) {
        setError(nextProjectId ? '暂时无法开始创作，请重试。' : '项目创建未完成，请重试。')
      }
    } finally {
      if (isCurrentOperation(controller)) setStarting(false)
    }
  }

  return (
    <section className="creator-start-card" aria-labelledby="creator-page-title">
      <p className="creator-eyebrow">开始创作</p>
      <h1 id="creator-page-title">先说一句，你想拍什么？</h1>
      <p className="creator-intro">写下主题、人物或画面感，接下来的创作会从这里开始。</p>
      <label className="creator-prompt-label" htmlFor="creator-prompt">创作想法</label>
      <textarea
        id="creator-prompt"
        className="creator-prompt"
        value={prompt}
        onChange={(event) => setPrompt(event.target.value)}
        placeholder="例如：为夏日咖啡新品拍一支轻快的竖版短片"
        rows={5}
        disabled={starting}
      />

      <div className="creator-materials" aria-label="参考素材">
        <div>
          <h2>让它更像你想要的样子</h2>
          <p>可以添加图片、视频、音频或文档作为参考。</p>
        </div>
        <button type="button" className="creator-secondary-button" onClick={() => fileInputRef.current?.click()} disabled={starting}>添加素材</button>
        <input ref={fileInputRef} className="creator-visually-hidden" type="file" multiple onChange={addMaterials} disabled={starting} />
      </div>
      {materials.length > 0 && (
        <ul className="creator-material-list" aria-live="polite">
          {materials.map(item => (
            <li key={item.id}>
              <span>{item.file.name}</span>
              <span className={`creator-material-status is-${item.status}`}>{materialStatusLabel(item.status)}</span>
              {item.status === 'failed' && projectId && (
                <button type="button" className="creator-text-button" onClick={() => void retryMaterial(item)} disabled={starting}>重试素材</button>
              )}
            </li>
          ))}
        </ul>
      )}

      <details className="creator-options" open={optionsOpen} aria-disabled={starting} onToggle={(event) => {
        if (starting) {
          event.currentTarget.open = optionsOpen
          return
        }
        setOptionsOpen(event.currentTarget.open)
      }}>
        <summary onClick={(event) => {
          if (starting) event.preventDefault()
        }} onKeyDown={(event) => {
          if (starting && (event.key === 'Enter' || event.key === ' ')) event.preventDefault()
        }}>调整创作选项</summary>
        <div className="creator-option-grid">
          <label>时长
            <select value={durationValue} onChange={(event) => setDurationValue(event.target.value)} disabled={starting}>
              {durationOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label>画面比例
            <select value={aspectRatio} onChange={(event) => setAspectRatio(event.target.value)} disabled={starting}>
              {aspectOptions.map(option => <option key={option} value={option}>{option}</option>)}
            </select>
          </label>
          <label>发布平台
            <select value={platform} onChange={(event) => setPlatform(event.target.value)} disabled={starting}>
              {platformOptions.map(option => <option key={option} value={option}>{option || '暂不设定'}</option>)}
            </select>
          </label>
          <label>制作方式
            <select value={productionRoute} onChange={(event) => setProductionRoute(event.target.value as CreatorProductionRoute)} disabled={starting}>
              {productionRouteOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label>补充素材
            <select value={aigcPolicy} onChange={(event) => setAigcPolicy(event.target.value as CreatorAIGCPolicy)} disabled={starting}>
              {aigcPolicyOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
        </div>
        {productionRoute === 'talking_head' && (
          <fieldset className="creator-voice-options">
            <legend>配音</legend>
            <p>默认使用经过确认的树懒 IP 音色；自定义录音只保存在本机。</p>
            <div className="creator-voice-mode-list">
              <label className={voiceMode === 'default_ip' ? 'is-selected' : ''}>
                <input
                  type="radio"
                  name="creator-voice-mode"
                  value="default_ip"
                  checked={voiceMode === 'default_ip'}
                  onChange={() => setVoiceMode('default_ip')}
                  disabled={starting}
                />
                <span><strong>默认 IP 音色</strong><small>使用已确认的树懒声音生成口播</small></span>
              </label>
              <label className={voiceMode === 'reference_clone' ? 'is-selected' : ''}>
                <input
                  type="radio"
                  name="creator-voice-mode"
                  value="reference_clone"
                  checked={voiceMode === 'reference_clone'}
                  onChange={() => setVoiceMode('reference_clone')}
                  disabled={starting}
                />
                <span><strong>上传录音复刻音色</strong><small>用录音音色朗读审核通过的口播稿</small></span>
              </label>
              <label className={voiceMode === 'recorded_narration' ? 'is-selected' : ''}>
                <input
                  type="radio"
                  name="creator-voice-mode"
                  value="recorded_narration"
                  checked={voiceMode === 'recorded_narration'}
                  onChange={() => setVoiceMode('recorded_narration')}
                  disabled={starting}
                />
                <span><strong>直接使用已录口播</strong><small>不复刻声音，直接使用完整成品录音</small></span>
              </label>
            </div>
            {voiceMode !== 'default_ip' && (
              <div className="creator-voice-custom">
                {voiceMode === 'reference_clone' && (
                  <>
                    <label>录音中实际说出的完整文字
                      <textarea
                        value={referenceText}
                        onChange={(event) => setReferenceText(event.target.value)}
                        rows={3}
                        placeholder="请逐字填写，标点也尽量与录音停顿一致"
                        disabled={starting}
                      />
                    </label>
                    <label className="creator-voice-check">
                      <input type="checkbox" checked={referenceTextVerified} onChange={(event) => setReferenceTextVerified(event.target.checked)} disabled={starting} />
                      我已核对，录音原文与音频内容完全一致
                    </label>
                  </>
                )}
                <div className="creator-voice-file-row">
                  <button type="button" className="creator-secondary-button" onClick={() => voiceFileInputRef.current?.click()} disabled={starting}>
                    {voiceFile ? '更换录音' : '选择录音'}
                  </button>
                  <input
                    ref={voiceFileInputRef}
                    className="creator-visually-hidden"
                    type="file"
                    accept="audio/wav,audio/mpeg,audio/mp4,audio/x-m4a,audio/flac,.wav,.mp3,.m4a,.flac"
                    onChange={addVoiceFile}
                    disabled={starting}
                  />
                  <span>{voiceFile ? `${voiceFile.file.name} · ${materialStatusLabel(voiceFile.status)}` : 'WAV、MP3、M4A 或 FLAC'}</span>
                </div>
                <label className="creator-voice-check">
                  <input type="checkbox" checked={usageRightsConfirmed} onChange={(event) => setUsageRightsConfirmed(event.target.checked)} disabled={starting} />
                  我确认拥有该录音和声音的使用授权
                </label>
              </div>
            )}
          </fieldset>
        )}
      </details>

      {error && <p className="creator-form-error" role="alert">{error}</p>}
      <button type="button" className="creator-primary-button" onClick={() => void startCreation()} disabled={!prompt.trim() || starting}>
        {starting ? '正在准备…' : projectId ? '继续开始创作' : '开始创作'}
      </button>
    </section>
  )
}

function materialFromUpload(file: File, upload: LocalArtifactUploadResponse): ProjectMaterial {
  if (!upload.storageRef || !upload.contentHash) throw new Error('素材上传信息不完整')
  return {
    name: file.name,
    kind: materialKind(file),
    storageRef: upload.storageRef,
    mimeType: upload.mimeType || file.type || 'application/octet-stream',
    sizeBytes: upload.sizeBytes ?? file.size,
    contentHash: upload.contentHash,
  }
}

function materialKind(file: File): ProjectMaterialKind {
  if (file.type.startsWith('image/')) return 'image'
  if (file.type.startsWith('video/')) return 'video'
  if (file.type.startsWith('audio/')) return 'audio'
  return 'document'
}

function materialStatusLabel(status: MaterialStatus): string {
  if (status === 'uploading') return '正在准备'
  if (status === 'success') return '已准备好'
  if (status === 'failed') return '未成功'
  return '等待开始'
}

function isSupportedVoiceFile(file: File): boolean {
  if (file.type.startsWith('audio/')) return true
  return /\.(wav|mp3|m4a|flac)$/i.test(file.name)
}

function voiceMimeType(file: File): string {
  if (file.type.startsWith('audio/')) return file.type
  const extension = file.name.split('.').pop()?.toLowerCase()
  if (extension === 'wav') return 'audio/wav'
  if (extension === 'mp3') return 'audio/mpeg'
  if (extension === 'm4a') return 'audio/mp4'
  if (extension === 'flac') return 'audio/flac'
  return 'application/octet-stream'
}
