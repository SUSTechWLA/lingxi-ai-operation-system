import { useEffect, useRef, useState } from 'react'
import type { ArtifactContentResponse } from '../../../utils/types'
import { previewStepRegeneration, rebuildFinalAssembly, regenerateStep } from '../../../services/creatorApi'
import { getLocalAgentBaseUrl } from '../../../services/localAgent'
import type { CreatorStep } from '../types'
import {
  canDeliverCreatorFinalVideo,
  completedDeliveryRepairSuccess,
  deliveryArtifactPassesFinalReview,
  completedRepairScope,
  creatorStepRegenerationIdempotencyKey,
  isCreatorConflict,
  resolveCreatorArtifactMediaUrl,
  shouldProbeCreatorDeliveryMedia,
  type CreatorArtifactLoadState,
  type CreatorMediaState,
} from '../logic'
import { classifyArtifactPresentation } from '../artifactPresentation'
import type { CreationView } from '../types'
import ArtifactProofingCanvas from './ArtifactProofingCanvas'
import SimpleVideoPlayer from './SimpleVideoPlayer'

interface PreviewDeliveryPanelProps {
  projectId: string
  step: CreatorStep
  content: ArtifactContentResponse | null
  artifactLoadState: CreatorArtifactLoadState
  assemblyDirty: boolean
  completedProject?: boolean
  viewingHistorical?: boolean
  onViewChanged: (view: CreationView) => void
  onAssemblyUpdated: () => Promise<void>
}

// The workspace owns artifact selection. This panel renders exactly that
// selection and never replaces it with the step's default artifact.
export default function PreviewDeliveryPanel({ projectId, step, content, artifactLoadState, assemblyDirty, completedProject = false, viewingHistorical = false, onViewChanged, onAssemblyUpdated }: PreviewDeliveryPanelProps) {
	const currentContent = content
  const [notice, setNotice] = useState('')
  const [working, setWorking] = useState(false)
  const [mediaStatus, setMediaStatus] = useState<{ url?: string; state: CreatorMediaState }>({
    state: 'loading',
  })
  const controllerRef = useRef<AbortController | null>(null)
  const assemblyKeyRef = useRef<string | null>(null)
  const repairKeyRef = useRef<string | null>(null)
  const isDelivery = step.id === 'delivery'
  const isFinalReviewPassed = isDelivery && deliveryArtifactPassesFinalReview(currentContent)
	const previewReady = step.state === 'confirmed' || step.state === 'needs_review'
	const mediaUrl = resolveCreatorArtifactMediaUrl(projectId, currentContent, getLocalAgentBaseUrl())
	const presentation = classifyArtifactPresentation({
		kind: currentContent?.artifact.kind,
		mimeType: currentContent?.artifact.mimeType,
		name: currentContent?.artifact.name,
	})
  const proofingContent = currentContent && mediaUrl && !currentContent.mediaUrl
		? { ...currentContent, mediaUrl }
		: currentContent
  const mediaState = mediaStatus.url === mediaUrl ? mediaStatus.state : 'loading'
  const deliveryMediaState: CreatorMediaState = isDelivery && artifactLoadState === 'ready' && (!mediaUrl || presentation !== 'video') ? 'missing' : mediaState
  const repairScope = completedRepairScope(
    { project: { status: completedProject ? 'COMPLETED' : 'RUNNING' }, activeStep: step.id },
    { delivery: deliveryMediaState },
    {
      artifactLoadState,
      viewingHistorical,
      currentArtifactId: step.currentArtifactId,
      selectedArtifactId: currentContent?.artifact.id,
      currentArtifactVersion: step.currentVersion,
      selectedArtifactVersion: currentContent?.artifact.version,
    },
  )
  const shouldProbeMedia = shouldProbeCreatorDeliveryMedia({
    artifactLoadState,
    presentation,
    mediaUrl,
    finalReviewPassed: isFinalReviewPassed,
  })
  const finalVideoReady = isDelivery && canDeliverCreatorFinalVideo({
    finalReviewPassed: isFinalReviewPassed,
    mediaUrl,
    mediaState,
  })
  const confirmedFinal = presentation === 'video'
    ? step.state === 'confirmed' && finalVideoReady
    : step.state === 'confirmed'

  useEffect(() => () => controllerRef.current?.abort(), [])

  const rebuild = async () => {
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setWorking(true)
    setNotice('')
    try {
      const idempotencyKey = assemblyKeyRef.current ?? `creator-assembly:${crypto.randomUUID()}`
      assemblyKeyRef.current = idempotencyKey
      const result = await rebuildFinalAssembly(projectId, idempotencyKey, controller.signal)
      if (controller.signal.aborted) return
      if (result.status === 'queued' || result.status === 'dispatching' || result.status === 'validated') {
        setNotice('已开始重新拼接成片。镜头审核可继续进行；新预览完成前不会显示旧成片。')
      } else {
        setNotice('还不能拼接成片：请先完成需要处理的镜头。')
        assemblyKeyRef.current = null
      }
      await onAssemblyUpdated()
    } catch (caught) {
      if (!controller.signal.aborted) {
        if (isCreatorConflict(caught)) {
          assemblyKeyRef.current = null
          setNotice('镜头内容已更新，请重新点击拼接成片。')
        } else if (typeof caught === 'object' && caught !== null && 'response' in caught) {
          // The server returned a definite failure, so the next click is a new
          // attempt. A response-less network failure keeps the UUID because
          // the original request may already have reached the server.
          assemblyKeyRef.current = null
          setNotice('这次拼接未能继续，请重新点击拼接成片。')
        } else {
          setNotice('网络状态不确定，请稍后重试。系统会继续核对同一次拼接，避免重复生成。')
        }
      }
    } finally {
      if (!controller.signal.aborted && controllerRef.current === controller) setWorking(false)
    }
  }

  const repairDelivery = async () => {
    if (!repairScope || working) return
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setWorking(true)
    setNotice('')
    try {
      const impact = await previewStepRegeneration(projectId, repairScope.stepId, controller.signal)
      if (controller.signal.aborted) return
      if (impact.affectedStepIds.length > 0) {
        repairKeyRef.current = null
        setNotice('系统核对发现重新生成会影响前置内容，已停止操作以保留需求、创意、脚本和分镜。')
        return
      }
      const idempotencyKey = repairKeyRef.current ?? creatorStepRegenerationIdempotencyKey(projectId, repairScope.stepId, crypto.randomUUID())
      repairKeyRef.current = idempotencyKey
      const result = await regenerateStep(projectId, repairScope.stepId, {
        ...(currentContent ? { baseArtifactId: currentContent.artifact.id, baseVersion: currentContent.artifact.version } : {}),
        instruction: '仅重新生成当前成片交付文件，保留已确认的需求、创意、脚本和分镜。',
        runId: step.runId,
        reviewId: step.reviewId,
        confirmedAffectedStepIds: impact.affectedStepIds,
      }, idempotencyKey, controller.signal)
      if (controller.signal.aborted) return
      const success = completedDeliveryRepairSuccess(result.view)
      onViewChanged(result.view)
      repairKeyRef.current = null
      setNotice(success.notice)
      if (success.refreshAfterMutation) void onAssemblyUpdated().catch(() => undefined)
    } catch (caught) {
      if (!controller.signal.aborted) {
        if (isCreatorConflict(caught) || (typeof caught === 'object' && caught !== null && 'response' in caught)) {
          repairKeyRef.current = null
          setNotice('成片状态已更新，请重新点击重新生成成片。')
        } else {
          setNotice('网络状态不确定；再次点击会沿用同一次请求，避免重复生成。')
        }
      }
    } finally {
      if (!controller.signal.aborted && controllerRef.current === controller) setWorking(false)
    }
  }

  return (
    <section className="preview-delivery-panel artifact-review-panel" aria-labelledby="preview-delivery-title">
      <header className="artifact-review-heading">
        <div>
          <p className="creator-eyebrow">{isDelivery ? '交付准备' : '成片预览'}</p>
          <h2 id="preview-delivery-title">{isDelivery ? '检查完成后交付' : '观看当前成片'}</h2>
        </div>
        <span className={`artifact-state is-${confirmedFinal ? 'confirmed' : 'needs_review'}`}>{confirmedFinal ? '已确认' : '等待处理'}</span>
      </header>

			{viewingHistorical && <div className="artifact-history-notice" role="status"><strong>正在查看历史产物</strong><span>当前交付版本没有被替换。</span></div>}

      {assemblyDirty && <div className="preview-delivery-warning" role="status">
        <strong>镜头有更新，成片需要重新拼接。</strong>
        <p>你可以继续回到镜头审核；重新拼接不会重新生成任何单个镜头。</p>
        <button className="creator-primary-button" type="button" disabled={working} onClick={() => void rebuild()}>{working ? '正在核对镜头…' : '重新拼接成片'}</button>
      </div>}

			{!assemblyDirty && previewReady && shouldProbeMedia && <SimpleVideoPlayer src={mediaUrl!} title="当前成片" downloadName="当前成片" onMediaStateChange={state => setMediaStatus({ url: mediaUrl!, state })} />}
			{!assemblyDirty && previewReady && currentContent && presentation !== 'video' && <ArtifactProofingCanvas content={proofingContent} reviewLabel="当前成片" />}
			{!assemblyDirty && artifactLoadState === 'error' && <p className="artifact-empty">暂时无法读取当前成片，请稍后重试。</p>}
			{!assemblyDirty && artifactLoadState !== 'error' && (!previewReady || !currentContent || (presentation === 'video' && !mediaUrl)) && <p className="artifact-empty">系统正在准备当前产物；完成后会在这里显示可审阅内容。</p>}
      {repairScope && !assemblyDirty && <section className="preview-delivery-warning" role="status">
        <strong>{deliveryMediaState === 'unsupported' ? '当前成片编码暂不受客户端支持。' : '成片文件缺失。'}</strong>
        <p>重新生成只会更新当前成片，已确认的需求、创意、脚本和分镜会保留。</p>
        <button className="creator-primary-button" type="button" disabled={working} onClick={() => void repairDelivery()}>{working ? '正在核对成片…' : '重新生成成片'}</button>
      </section>}

      <section className="preview-delivery-checklist" aria-label="成片检查">
        <h3>成片检查</h3>
        <p><span aria-hidden="true">{step.state === 'confirmed' ? '✓' : '○'}</span> 字幕是否易读、时间是否准确</p>
        <p><span aria-hidden="true">{step.state === 'confirmed' ? '✓' : '○'}</span> 旁白、音乐和画面衔接是否自然</p>
      </section>

      {isDelivery && (finalVideoReady ? <section className="preview-delivery-package">
        <h3>交付文件</h3>
        <p>成片检查通过后，才会显示最终视频和交付文件。</p>
        <a className="creator-primary-button" href={mediaUrl} download>下载最终视频</a>
      </section> : <section className="preview-delivery-package" aria-live="polite">
        <h3>交付文件</h3>
        <p>成片检查通过后，才会显示最终视频和交付文件。</p>
      </section>)}
      {notice && <p className="creator-form-error" role="status">{notice}</p>}
    </section>
  )
}
