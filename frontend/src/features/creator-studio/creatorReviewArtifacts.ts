import type { CreatorArtifactDescriptor } from './types'
import { classifyCreatorReviewArtifact, type CreatorReviewCategory } from '../../utils/artifactClassification'

export { classifyCreatorReviewArtifact, type CreatorReviewCategory } from '../../utils/artifactClassification'

function normalized(value: string | undefined): string {
  return value?.trim().toLocaleLowerCase() || ''
}

export interface CreatorReviewArtifact extends CreatorArtifactDescriptor {
  reviewCategory: CreatorReviewCategory
  reviewLabel: string
  shotLabel?: string
}

const reviewCategoryLabels: Record<CreatorReviewCategory, string> = {
  text: '文字内容',
  image: '参考图',
  video: '视频片段',
  audio: '语音',
}

const artifactTypeReviewLabels: Readonly<Record<string, string>> = {
  shot_keyframe: '参考图',
  reference_image: '参考图',
  shot_reference: '参考图',
  shot_video_clip: '视频片段',
  composited_shot_video: '合成视频',
  hyperframes_shot: '视频片段',
  shot_audio: '语音',
  narration_master: '旁白',
  recorded_narration: '录制旁白',
  uploaded_narration: '录制旁白',
  voice_reference: '参考语音',
  publish_copy: '发布文案',
  video_script: '视频脚本',
  video_prompts: '视频提示词',
  keyframe_prompts: '关键帧提示词',
}

const generationRequestLabels: Readonly<Record<string, string>> = {
  image: '图片提示词',
  video: '视频提示词',
  audio: '语音提示词',
}

function creatorReviewLabel(
  artifactType: string | undefined,
  generationKind: string | undefined,
  reviewCategory: CreatorReviewCategory,
): string {
  const safeArtifactType = normalized(artifactType)
  const safeGenerationKind = normalized(generationKind)
  if (safeArtifactType === 'external_generation_request') {
    return generationRequestLabels[safeGenerationKind] || reviewCategoryLabels.text
  }
  return artifactTypeReviewLabels[safeArtifactType] || reviewCategoryLabels[reviewCategory]
}

function creatorShotLabel(value: string | undefined): string | undefined {
  const shotId = value?.trim()
  if (!shotId || !/^(?:shot[-_ ]?)?\d{1,4}$/iu.test(shotId)) return undefined
  return shotId
}

export function projectCreatorReviewArtifacts(
  artifacts: readonly CreatorArtifactDescriptor[],
): CreatorReviewArtifact[] {
  return artifacts
    .flatMap((artifact) => {
      const reviewCategory = classifyCreatorReviewArtifact(artifact)
      if (!reviewCategory) return []
      const shotLabel = creatorShotLabel(artifact.relatedShotId)
      return [{
        ...artifact,
        reviewCategory,
        reviewLabel: creatorReviewLabel(artifact.artifactType, artifact.generationKind, reviewCategory),
        ...(shotLabel ? { shotLabel } : {}),
      }]
    })
    .sort((left, right) => {
      if (left.isCurrent !== right.isCurrent) return left.isCurrent ? -1 : 1
      if (left.isStale !== right.isStale) return left.isStale ? 1 : -1
      if (left.version !== right.version) return right.version - left.version
      return left.artifactId.localeCompare(right.artifactId)
    })
}

export function selectCreatorReviewArtifact(
  artifacts: readonly CreatorReviewArtifact[],
  explicitArtifactId?: string,
): CreatorReviewArtifact | undefined {
  if (explicitArtifactId) {
    const explicit = artifacts.find((artifact) => artifact.artifactId === explicitArtifactId)
    if (explicit) return explicit
  }
  return artifacts.find((artifact) => artifact.isCurrent && !artifact.isStale) ||
    artifacts.find((artifact) => artifact.isCurrent) ||
    artifacts[0]
}
