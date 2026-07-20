import { useRef, useState, type ChangeEvent } from 'react'
import { createVideoProject, startAgentRun } from '../../services/api'
import { uploadLocalArtifactFile, type LocalArtifactUploadResponse } from '../../services/localAgent'
import { registerProjectMaterial } from '../../services/creatorApi'
import type { ProjectMaterial, ProjectMaterialKind } from './types'
import { buildCreationRequest } from './logic'

type MaterialStatus = 'ready' | 'uploading' | 'success' | 'failed'

interface MaterialItem {
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

export default function StartCreationPage({ onOpenProject }: StartCreationPageProps) {
  const [prompt, setPrompt] = useState('')
  const [durationValue, setDurationValue] = useState('')
  const [aspectRatio, setAspectRatio] = useState('9:16')
  const [platform, setPlatform] = useState('')
  const [materials, setMaterials] = useState<MaterialItem[]>([])
  const [projectId, setProjectId] = useState<string>()
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string>()
  const fileInputRef = useRef<HTMLInputElement>(null)

  const updateMaterial = (id: string, patch: Partial<MaterialItem>) => {
    setMaterials(current => current.map(item => item.id === id ? { ...item, ...patch } : item))
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

  const processMaterial = async (item: MaterialItem, nextProjectId: string): Promise<boolean> => {
    updateMaterial(item.id, { status: 'uploading' })
    try {
      const upload = item.upload || await uploadLocalArtifactFile({
        projectId: nextProjectId,
        id: item.id,
        file: item.file,
        mimeType: item.file.type || undefined,
        metadata: {
          artifactType: 'project_source_material',
          source: 'creator_studio',
          localOnly: true,
        },
      })
      updateMaterial(item.id, { upload })
      await registerProjectMaterial(nextProjectId, materialFromUpload(item.file, upload))
      updateMaterial(item.id, { status: 'success', upload })
      return true
    } catch {
      updateMaterial(item.id, { status: 'failed' })
      return false
    }
  }

  const processRemainingMaterials = async (nextProjectId: string): Promise<boolean> => {
    let allSucceeded = true
    for (const item of materials) {
      if (item.status === 'success') continue
      const succeeded = await processMaterial(item, nextProjectId)
      allSucceeded = allSucceeded && succeeded
    }
    return allSucceeded
  }

  const retryMaterial = async (item: MaterialItem) => {
    if (!projectId || starting) return
    setError(undefined)
    await processMaterial(item, projectId)
  }

  const startCreation = async () => {
    if (!prompt.trim() || starting) return
    setStarting(true)
    setError(undefined)
    try {
      const durationSec = durationValue ? Number(durationValue) : undefined
      const request = buildCreationRequest({
        prompt,
        durationSec,
        aspectRatio,
        platform: platform || undefined,
        materialCount: materials.length,
      })
      let nextProjectId = projectId
      if (!nextProjectId) {
        const project = await createVideoProject(request.project)
        nextProjectId = project.id
        setProjectId(project.id)
      }
      const materialsReady = await processRemainingMaterials(nextProjectId)
      if (!materialsReady) {
        setError('部分素材未准备好，请重试失败的文件后再开始创作。')
        return
      }
      await startAgentRun({
        ...request.agentRun,
        context: { ...request.agentRun.context, projectId: nextProjectId },
      })
      onOpenProject(nextProjectId)
    } catch {
      setError(projectId ? '暂时无法开始创作，请重试。' : '项目创建未完成，请重试。')
    } finally {
      setStarting(false)
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
      />

      <div className="creator-materials" aria-label="参考素材">
        <div>
          <h2>让它更像你想要的样子</h2>
          <p>可以添加图片、视频、音频或文档作为参考。</p>
        </div>
        <button type="button" className="creator-secondary-button" onClick={() => fileInputRef.current?.click()}>添加素材</button>
        <input ref={fileInputRef} className="creator-visually-hidden" type="file" multiple onChange={addMaterials} />
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

      <details className="creator-options">
        <summary>调整创作选项</summary>
        <div className="creator-option-grid">
          <label>时长
            <select value={durationValue} onChange={(event) => setDurationValue(event.target.value)}>
              {durationOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label>画面比例
            <select value={aspectRatio} onChange={(event) => setAspectRatio(event.target.value)}>
              {aspectOptions.map(option => <option key={option} value={option}>{option}</option>)}
            </select>
          </label>
          <label>发布平台
            <select value={platform} onChange={(event) => setPlatform(event.target.value)}>
              {platformOptions.map(option => <option key={option} value={option}>{option || '暂不设定'}</option>)}
            </select>
          </label>
        </div>
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
