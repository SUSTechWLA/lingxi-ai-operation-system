export type CreatorReviewCategory = 'text' | 'image' | 'video' | 'audio'

export interface CreatorArtifactClassificationInput {
  kind: string
  mimeType?: string
  name?: string
  artifactType?: string
  generationKind?: string
}

const hiddenArtifactTypes = new Set([
  'asset_manifest', 'project_source_material', 'project_source_material_manifest', 'shot_asset_package',
  'video_visual_qa_report', 'video_visual_qa_contact_sheet', 'shot_qa_report', 'shot_repair_plan',
  'final_qa_report', 'final_review', 'project_package', 'stale_artifact_report', 'continuity_report',
])

const hiddenKinds = new Set([
  'BUNDLE', 'LOG', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW', 'PROJECT_PACKAGE', 'SHOT_ASSET_PACKAGE',
  'ASSET_MANIFEST', 'PROJECT_SOURCE_MATERIAL_MANIFEST', 'CONTINUITY_BIBLE', 'CONTINUITY_REPORT',
  'STALE_ARTIFACT_REPORT', 'PREVIEW_REPORT', 'VIDEO_VISUAL_QA_REPORT', 'VIDEO_VISUAL_QA_CONTACT_SHEET',
  'SHOT_QA_REPORT', 'SHOT_REPAIR_PLAN',
])

const textKinds = new Set([
  'VIDEO_SCRIPT', 'VIDEO_PROMPTS', 'KEYFRAME_PROMPTS', 'PUBLISH_COPY', 'VIDEO_PROPOSAL', 'CARD_PLAN',
  'CAPTION_PLAN', 'VIDEO_COMPOSITION_SPEC', 'REFERENCE_ASSET_PLAN', 'MARKDOWN',
])

const imageKinds = new Set(['SHOT_KEYFRAME', 'REFERENCE_IMAGE', 'SHOT_REFERENCE', 'IMAGE'])
const videoKinds = new Set(['SHOT_VIDEO_CLIP', 'COMPOSITED_SHOT_VIDEO', 'HYPERFRAMES_SHOT', 'VIDEO'])
const audioKinds = new Set(['SHOT_AUDIO', 'AUDIO', 'NARRATION_MASTER', 'UPLOADED_NARRATION'])
const hiddenNamePattern = /(?:^|[._\-\s])(qa|manifest|continuity|package|log)(?:[._\-\s]|$)/iu

function normalized(value: string | undefined): string {
  return value?.trim().toLocaleLowerCase() || ''
}

function categoryForCreatorType(artifactType: string, generationKind: string): CreatorReviewCategory | undefined {
  if (artifactType === 'external_generation_request') return 'text'
  if (['shot_keyframe', 'reference_image', 'shot_reference'].includes(artifactType)) return 'image'
  if (['shot_video_clip', 'composited_shot_video', 'hyperframes_shot'].includes(artifactType)) return 'video'
  if (['shot_audio', 'narration_master', 'recorded_narration', 'uploaded_narration', 'voice_reference'].includes(artifactType)) return 'audio'
  if (['publish_copy', 'video_script', 'video_prompts', 'keyframe_prompts'].includes(artifactType)) return 'text'
  if (artifactType === 'external_generation_result') {
    if (generationKind === 'image') return 'image'
    if (generationKind === 'video') return 'video'
    if (generationKind === 'audio') return 'audio'
  }
  return undefined
}

export function classifyCreatorReviewArtifact(
  artifact: CreatorArtifactClassificationInput,
): CreatorReviewCategory | undefined {
  const artifactType = normalized(artifact.artifactType)
  const generationKind = normalized(artifact.generationKind)
  const kind = artifact.kind.trim().toLocaleUpperCase()
  const mimeType = normalized(artifact.mimeType)
  const name = normalized(artifact.name)

  if (hiddenArtifactTypes.has(artifactType) || hiddenKinds.has(kind) || hiddenNamePattern.test(name)) return undefined

  const explicitCategory = categoryForCreatorType(artifactType, generationKind)
  if (explicitCategory) return explicitCategory

  if (mimeType.startsWith('text/')) return 'text'
  if (mimeType.startsWith('image/')) return 'image'
  if (mimeType.startsWith('video/')) return 'video'
  if (mimeType.startsWith('audio/')) return 'audio'

  if (textKinds.has(kind)) return 'text'
  if (imageKinds.has(kind)) return 'image'
  if (videoKinds.has(kind)) return 'video'
  if (audioKinds.has(kind)) return 'audio'

  if (/\.(?:md|txt|markdown)$/iu.test(name)) return 'text'
  if (/\.(?:png|jpe?g|webp|gif)$/iu.test(name)) return 'image'
  if (/\.(?:mp4|mov|webm|m4v)$/iu.test(name)) return 'video'
  if (/\.(?:mp3|wav|m4a|aac|ogg|flac)$/iu.test(name)) return 'audio'
  return undefined
}
