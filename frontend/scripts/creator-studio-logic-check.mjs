import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import ts from 'typescript'

const creatorFacingAttributes = new Set(['alt', 'aria-label', 'aria-description', 'title', 'placeholder'])

function creatorRenderedSurface(source, fileName) {
  const sourceFile = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  const rendered = []
  const containsJsx = node => {
    let found = false
    const visit = child => {
      if (ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child) || ts.isJsxFragment(child)) {
        found = true
        return
      }
      if (!found) ts.forEachChild(child, visit)
    }
    ts.forEachChild(node, visit)
    return found
  }
  const visit = node => {
    if (ts.isJsxText(node) && node.text.trim()) rendered.push(node.text)
    if (ts.isJsxAttribute(node) && creatorFacingAttributes.has(node.name.getText(sourceFile))) {
      if (node.initializer) rendered.push(node.initializer.getText(sourceFile))
    }
    if (
      ts.isJsxExpression(node) &&
      node.expression &&
      (ts.isJsxElement(node.parent) || ts.isJsxFragment(node.parent)) &&
      !containsJsx(node.expression)
    ) {
      rendered.push(node.expression.getText(sourceFile))
    }
    ts.forEachChild(node, visit)
  }
  visit(sourceFile)
  return rendered.join('\n')
}

function assertCreatorRenderedSurfaceIsSafe(componentSources) {
  const forbidden = [
    ['raw JSON', /\bJSON(?:\.stringify)?\b|parsed\.raw|rawJson/i],
    ['hashes', /\b(?:contentHash|promptHash|sha256)\b|哈希/i],
    ['storage references or local paths', /\b(?:storageRef|storageType)\b|(?:local|file):\/\/|\/(?:Users|home|private|tmp)\//i],
    ['providers or models', /\b(?:modelProviders?|provider)\b|模型供应商/i],
    ['tool or MCP calls', /\b(?:toolCall|toolName|mcpCall|mcpTool|MCP)\b/i],
    ['node, run, or task IDs', /\b(?:nodeId|runId|taskId)\b/i],
    ['retry attempt numbers', /\b(?:attempt|attemptCount|attemptIndex)\b|第\s*\{[^}]+\}\s*轮/i],
    ['QA or continuity report bodies', /\b(?:qaReport|continuityReport)(?:\?\.)?\.(?:summary|body|content|report)\b/i],
    ['technical logs or backend event copy', /\b(?:technicalLog|logBody|logContent)\b|\bevent\.(?:title|summary)\b/i],
    ['raw artifact names', /\bartifact\.name\b/i],
  ]
  for (const [fileName, source] of componentSources) {
    const rendered = creatorRenderedSurface(source, fileName)
    for (const [label, pattern] of forbidden) {
      assert.doesNotMatch(rendered, pattern, `${fileName} must not render creator-facing ${label}`)
    }
  }
}

const temp = await mkdtemp(join(tmpdir(), 'creator-studio-'))
const bundle = join(temp, 'logic.mjs')
const focusBundle = join(temp, 'focus-cycle.mjs')
const presentationBundle = join(temp, 'artifact-presentation.mjs')
const authBundle = join(temp, 'auth.mjs')
const reviewArtifactsBundle = join(temp, 'creator-review-artifacts.mjs')
const completedShotsBundle = join(temp, 'completed-shots.mjs')
const projectionBundle = join(temp, 'creator-review-projection.mjs')
const textSelectionBundle = join(temp, 'text-selection.mjs')
const mediaRangeBundle = join(temp, 'media-range.mjs')

try {
  const textSelectionUrl = new URL('../src/features/creator-studio/textSelection.ts', import.meta.url)
  assert.equal(existsSync(textSelectionUrl), true, 'creator text selection helper must exist')

  execFileSync(process.execPath, [
    new URL('../node_modules/typescript/bin/tsc', import.meta.url).pathname,
    '--noEmit', '--strict', '--skipLibCheck', '--module', 'ESNext', '--moduleResolution', 'bundler',
    '--target', 'ES2020', new URL('./fixtures/creator-contract-types.ts', import.meta.url).pathname,
  ], { stdio: 'inherit' })

  await build({
    entryPoints: [new URL('../src/features/creator-studio/logic.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: bundle,
  })
  const logic = await import(pathToFileURL(bundle))
  await build({
    entryPoints: [new URL('../src/features/creator-studio/focusCycle.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: focusBundle,
  })
  const focus = await import(pathToFileURL(focusBundle))
  await build({
    entryPoints: [new URL('../src/features/creator-studio/artifactPresentation.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: presentationBundle,
  })
  const presentation = await import(pathToFileURL(presentationBundle))
  await build({
    entryPoints: [new URL('../src/services/auth.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: authBundle,
    define: { 'import.meta.env': '{}' },
  })
  const auth = await import(pathToFileURL(authBundle))
  await build({
    entryPoints: [new URL('../src/features/creator-studio/creatorReviewArtifacts.ts', import.meta.url).pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: reviewArtifactsBundle,
  })
  const reviewArtifacts = await import(pathToFileURL(reviewArtifactsBundle))
  const completedShotsUrl = new URL('../src/features/creator-studio/completedShotProjection.ts', import.meta.url)
  assert.equal(completedShotsUrl && existsSync(completedShotsUrl), true, 'completed projects need a historical Shot projection')
  await build({
    entryPoints: [completedShotsUrl.pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: completedShotsBundle,
  })
  const completedShots = await import(pathToFileURL(completedShotsBundle))
  const projectionUrl = new URL('../src/features/creator-studio/creatorReviewProjection.ts', import.meta.url)
  assert.equal(existsSync(projectionUrl), true, 'creator review content needs one safe projection module')
  await build({
    entryPoints: [projectionUrl.pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: projectionBundle,
  })
  const projection = await import(pathToFileURL(projectionBundle))
  await build({
    entryPoints: [textSelectionUrl.pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: textSelectionBundle,
  })
  const textSelection = await import(pathToFileURL(textSelectionBundle))
  const mediaRangeUrl = new URL('../src/features/creator-studio/mediaRange.ts', import.meta.url)
  assert.equal(existsSync(mediaRangeUrl), true, 'video and audio review need shared playhead range state')
  await build({
    entryPoints: [mediaRangeUrl.pathname],
    bundle: true,
    format: 'esm',
    platform: 'node',
    outfile: mediaRangeBundle,
  })
  const mediaRange = await import(pathToFileURL(mediaRangeBundle))

  assert.equal(mediaRange.secondsToIntegerMilliseconds(1.2346), 1235, 'the current playhead is stored as integer milliseconds')
  assert.equal(mediaRange.secondsToIntegerMilliseconds(-1), 0, 'negative media times clamp to zero')
  const pendingEndRange = mediaRange.updateMediaRangeBoundary(null, {}, 'end', 2800.4)
  assert.deepEqual(pendingEndRange, {
    selection: null,
    draft: { endMs: 2800 },
  }, 'one boundary remains a pending range')
  assert.deepEqual(
    mediaRange.updateMediaRangeBoundary(pendingEndRange.selection, pendingEndRange.draft, 'start', 4200),
    {
      selection: { kind: 'time', startMs: 2800, endMs: 4200 },
      draft: {},
    },
    'setting end before start normalizes the completed range through createTimeSelection',
  )
  assert.deepEqual(mediaRange.clearMediaRange(), { selection: null, draft: {} }, 'clearing removes both pending and completed ranges')
  assert.equal(mediaRange.formatMediaTimecode(62_345), '01:02.345', 'range boundaries use readable timecode')
  const completedMediaFailure = mediaRange.mediaReviewAfterPlaybackFailure({
    playbackAvailable: true,
    selection: { kind: 'time', startMs: 1200, endMs: 2800 },
    pending: false,
  })
  assert.deepEqual(completedMediaFailure, {
    playbackAvailable: false,
    selection: null,
    pending: false,
  }, 'playback failure clears a completed media range')
  assert.deepEqual(mediaRange.mediaReviewAfterPlaybackFailure({
    playbackAvailable: true,
    selection: null,
    pending: true,
  }), {
    playbackAvailable: false,
    selection: null,
    pending: false,
  }, 'playback failure clears a pending media boundary')
  assert.deepEqual(mediaRange.audioPlaybackFailure(0), {
    canRetry: true,
    playbackAvailable: false,
  }, 'the first retryable audio failure immediately makes playback unavailable')
  assert.deepEqual(mediaRange.audioPlaybackFailure(1), {
    canRetry: true,
    playbackAvailable: false,
  }, 'the second retryable audio failure remains available')
  assert.deepEqual(mediaRange.audioPlaybackFailure(2), {
    canRetry: false,
    playbackAvailable: false,
  }, 'terminal audio failure also keeps playback unavailable')
  assert.deepEqual(mediaRange.resetMediaTransportState(), {
    currentTime: 0,
    duration: 0,
    volume: 1,
    isMuted: false,
    playbackRate: 1,
    isPlaying: false,
  }, 'retry remounts reset every retained transport value to the media element defaults')
  assert.equal(mediaRange.creatorRevisionInputsLocked(true, false), true, 'request-bearing inputs lock while impact preview is loading')
  assert.equal(mediaRange.creatorRevisionInputsLocked(false, true), true, 'request-bearing inputs stay locked while confirmation owns the snapshot')
  assert.equal(mediaRange.creatorRevisionInputsLocked(false, false), false, 'inputs unlock after impact confirmation closes')

  assert.deepEqual(
    textSelection.buildTextSelection('开头正文结尾', 2, 4),
    { kind: 'text', start: 2, end: 4, text: '正文' },
    'forward selections preserve exact UTF-16 offsets and source text',
  )
  assert.deepEqual(
    textSelection.buildTextSelection('开头正文结尾', 4, 2),
    { kind: 'text', start: 2, end: 4, text: '正文' },
    'backward selections normalize to ascending offsets',
  )
  assert.equal(textSelection.buildTextSelection('正文', 1, 1), null, 'empty selections are ignored')
  assert.deepEqual(
    textSelection.buildTextSelection('前  有空格  后', 1, 8),
    { kind: 'text', start: 1, end: 8, text: '  有空格  ' },
    'whitespace inside a selection is preserved exactly',
  )
  assert.deepEqual(
    textSelection.buildTextSelection('第一句，第二句。', 4, 7),
    { kind: 'text', start: 4, end: 7, text: '第二句' },
    'Chinese text uses JavaScript UTF-16 offsets',
  )
  assert.deepEqual(
    textSelection.buildTextSelection('甲🙂乙', 1, 3),
    { kind: 'text', start: 1, end: 3, text: '🙂' },
    'emoji selection keeps its two UTF-16 code units',
  )
  assert.equal(
    textSelection.buildTextSelection('甲🙂乙', 1, 2),
    null,
    'selection boundaries cannot split an emoji surrogate pair',
  )
  assert.deepEqual(
    textSelection.buildTextSelection('字'.repeat(4_001), 0, 4_000),
    { kind: 'text', start: 0, end: 4_000, text: '字'.repeat(4_000) },
    'a 4,000-code-unit selection is accepted',
  )
  assert.equal(
    textSelection.buildTextSelection('字'.repeat(4_001), 0, 4_001),
    null,
    'a selection over 4,000 UTF-16 code units is rejected',
  )
  assert.deepEqual(
    textSelection.buildTextSelectionFromLengths('开头正文结尾', [2, 2, 2], 1, 0, 1, 2),
    { kind: 'text', start: 2, end: 4, text: '正文' },
    'a selection spanning rendered text nodes resolves against one canonical UTF-16 source',
  )
  assert.equal(
    textSelection.buildTextSelectionFromLengths('开头正文结尾', [2, 2, 2], 0, 1, 3, 0),
    null,
    'selection endpoints outside the rendered text-node list are rejected',
  )
  assert.deepEqual(
    textSelection.rebaseTextSelection(
      '{"detailedScript":"开头\\n正\\"文结尾","script":"开头\\n正\\"文结尾"}',
      '开头\n正"文结尾',
      { kind: 'text', start: 2, end: 6, text: '\n正"文' },
    ),
    { kind: 'text', start: 21, end: 27, text: '\\n正\\"文' },
    'escaped projected text maps to the deterministic detailedScript token in the immutable backend source',
  )
  assert.equal(
    textSelection.rebaseTextSelection('x开头正文结尾|开头正文结尾', '开头正文结尾', { kind: 'text', start: 2, end: 4, text: '正文' }),
    null,
    'ambiguous projections never guess backend offsets',
  )
  assert.equal(
    textSelection.rebaseTextSelection('xx正文', '开头正文', { kind: 'text', start: 2, end: 4, text: '正文' }),
    null,
    'same-offset coincidence never substitutes for canonical source provenance',
  )
  assert.deepEqual(
    textSelection.rebaseTextSelection(
      '{"content":"{\\"script\\":\\"开头\\\\n正文\\"}"}',
      '开头\n正文',
      { kind: 'text', start: 2, end: 5, text: '\n正文' },
    ),
    { kind: 'text', start: 28, end: 33, text: '\\\\n正文' },
    'nested JSON-string envelopes compose decoded offsets back to the immutable outer source',
  )
  assert.equal(
    textSelection.rebaseTextSelection(
      '{"content":{"script":"正文"},"payload":{"script":"正文"}}',
      '正文',
      { kind: 'text', start: 0, end: 2, text: '正文' },
    ),
    null,
    'same-priority JSON candidates fail closed instead of guessing a source token',
  )
  assert.deepEqual(
    textSelection.rebaseTextSelection(
      '{"content":"{\\"script\\":\\"  🙂开头\\\\n正文  \\"}"}',
      '🙂开头\n正文',
      { kind: 'text', start: 0, end: 7, text: '🙂开头\n正文' },
    ),
    { kind: 'text', start: 28, end: 37, text: '🙂开头\\\\n正文' },
    'projector trimming preserves nested escaped JSON and emoji UTF-16 boundaries',
  )

  const reviewClassificationCases = [
    [{ kind: 'JSON', mimeType: 'application/json', name: 'shot-02-image-request.json', artifactType: 'external_generation_request', generationKind: 'image' }, 'text'],
    [{ kind: 'IMAGE', mimeType: 'image/png', name: 'SHOT_02-keyframe.png', artifactType: 'shot_keyframe' }, 'image'],
    [{ kind: 'JSON', name: 'visual-qa-report.json', artifactType: 'video_visual_qa_report' }, undefined],
    [{ kind: 'VIDEO_SCRIPT', name: 'script.json' }, 'text'],
    [{ kind: 'VIDEO_PROMPTS', name: 'video-prompts.json' }, 'text'],
    [{ kind: 'KEYFRAME_PROMPTS', name: 'keyframe-prompts.json' }, 'text'],
    [{ kind: 'PUBLISH_COPY', name: 'publish-copy.json' }, 'text'],
    [{ kind: 'SHOT_VIDEO_CLIP', name: 'shot.mp4' }, 'video'],
    [{ kind: 'COMPOSITED_SHOT_VIDEO', name: 'composited.mp4' }, 'video'],
    [{ kind: 'SHOT_AUDIO', name: 'shot.wav' }, 'audio'],
    [{ kind: 'NARRATION_MASTER', name: 'master.wav', artifactType: 'narration_master' }, 'audio'],
    [{ kind: 'UPLOADED_NARRATION', name: 'uploaded.wav', artifactType: 'recorded_narration' }, 'audio'],
    [{ kind: 'BUNDLE', name: 'delivery.zip' }, undefined],
    [{ kind: 'LOG', name: 'runner.log' }, undefined],
    [{ kind: 'JSON', name: 'asset-manifest.json', artifactType: 'asset_manifest' }, undefined],
    [{ kind: 'CONTINUITY_REPORT', name: 'continuity.json' }, undefined],
    [{ kind: 'PROJECT_PACKAGE', name: 'package.json' }, undefined],
    [{ kind: 'JSON', mimeType: 'application/json', name: 'generic.json' }, undefined],
    [{ kind: 'JSON', mimeType: 'text/plain', name: 'notes.txt' }, 'text'],
    [{ kind: 'UNKNOWN', name: 'fallback.md' }, 'text'],
  ]
  for (const [artifact, expected] of reviewClassificationCases) {
    assert.equal(reviewArtifacts.classifyCreatorReviewArtifact(artifact), expected, `review classification for ${artifact.kind}/${artifact.artifactType || artifact.name}`)
  }
  const projectedReviewArtifacts = reviewArtifacts.projectCreatorReviewArtifacts([
    { artifactId: 'hidden', kind: 'LOG', name: 'worker.log', version: 3, isCurrent: true, isStale: false },
    { artifactId: 'older-script', kind: 'VIDEO_SCRIPT', name: 'script-v1.json', version: 1, isCurrent: false, isStale: true },
    { artifactId: 'current-script', kind: 'VIDEO_SCRIPT', name: 'script-v2.json', version: 2, isCurrent: true, isStale: false, relatedShotId: 'SHOT_02' },
  ])
  assert.deepEqual(projectedReviewArtifacts.map(item => item.artifactId), ['current-script', 'older-script'], 'projection filters hidden artifacts and sorts current versions first')
  assert.equal(projectedReviewArtifacts[0].reviewCategory, 'text')
  assert.equal(projectedReviewArtifacts[0].reviewLabel, '文字内容', 'kind-only text artifacts use the closed generic label')
  assert.equal(projectedReviewArtifacts[0].shotLabel, 'SHOT_02')
  const proposalReviewArtifacts = reviewArtifacts.projectCreatorReviewArtifacts([
    { artifactId: 'proposal', stepId: 'direction', kind: 'JSON', mimeType: 'application/json', name: 'proposal_packet.json', version: 1, isCurrent: true, isStale: false },
    { artifactId: 'strategy', stepId: 'direction', kind: 'JSON', mimeType: 'application/json', name: 'shot_generation_plans.json', version: 1, isCurrent: true, isStale: false },
  ])
  assert.deepEqual(
    proposalReviewArtifacts.map(item => [item.artifactId, item.reviewCategory, item.reviewLabel]),
    [['proposal', 'text', '创意方案']],
    'the named proposal packet is reviewable without exposing unrelated generic planning payloads',
  )
  const currentReviewArtifacts = reviewArtifacts.currentCreatorReviewArtifacts([
    { artifactId: 'script-v1', kind: 'VIDEO_SCRIPT', name: 'script-v1.json', version: 1, isCurrent: false, isStale: true },
    { artifactId: 'script-v2', kind: 'VIDEO_SCRIPT', name: 'script-v2.json', version: 2, isCurrent: false, isStale: true },
    { artifactId: 'script-v3', kind: 'VIDEO_SCRIPT', name: 'script-v3.json', version: 3, isCurrent: true, isStale: false },
    { artifactId: 'shot-01-current', kind: 'SHOT_VIDEO_CLIP', name: 'shot-01.mp4', relatedShotId: 'SHOT_01', version: 4, isCurrent: true, isStale: false },
    { artifactId: 'shot-02-current-stale', kind: 'SHOT_VIDEO_CLIP', name: 'shot-02.mp4', relatedShotId: 'SHOT_02', version: 5, isCurrent: true, isStale: true },
  ])
  assert.deepEqual(
    currentReviewArtifacts.map(item => item.artifactId),
    ['shot-01-current', 'script-v3', 'shot-02-current-stale'],
    'creator-facing content shows only the latest current result for each generated output, including a current result that needs updating',
  )
  assert.equal(
    currentReviewArtifacts.some(item => item.artifactId === 'script-v1' || item.artifactId === 'script-v2'),
    false,
    'previous optimization attempts remain hidden from the primary creator surface',
  )
  assert.equal(
    typeof reviewArtifacts.shouldShowCreatorArtifactSwitcher,
    'function',
    'current artifact navigation exposes one tested visibility rule',
  )
  assert.equal(reviewArtifacts.shouldShowCreatorArtifactSwitcher([]), false)
  assert.equal(reviewArtifacts.shouldShowCreatorArtifactSwitcher(currentReviewArtifacts.slice(0, 1)), false)
  assert.equal(reviewArtifacts.shouldShowCreatorArtifactSwitcher(currentReviewArtifacts.slice(0, 2)), true)
  assert.equal(
    typeof reviewArtifacts.nextCreatorArtifactTabIndex,
    'function',
    'current artifact keyboard navigation is testable without a rendered sidebar',
  )
  assert.equal(reviewArtifacts.nextCreatorArtifactTabIndex(0, 'ArrowRight', 3), 1)
  assert.equal(reviewArtifacts.nextCreatorArtifactTabIndex(2, 'ArrowRight', 3), 0)
  assert.equal(reviewArtifacts.nextCreatorArtifactTabIndex(0, 'ArrowLeft', 3), 2)
  assert.equal(reviewArtifacts.nextCreatorArtifactTabIndex(1, 'Home', 3), 0)
  assert.equal(reviewArtifacts.nextCreatorArtifactTabIndex(1, 'End', 3), 2)
  assert.equal(reviewArtifacts.nextCreatorArtifactTabIndex(0, 'ArrowRight', 0), -1)
  const maliciousNamedArtifacts = reviewArtifacts.projectCreatorReviewArtifacts([
    {
      artifactId: 'path-image', kind: 'IMAGE', mimeType: 'image/png', name: '/private/tmp/foo.png',
      artifactType: 'shot_keyframe', relatedShotId: '/private/tmp/SHOT_02', version: 1, isCurrent: true, isStale: false,
    },
    {
      artifactId: 'hash-video', kind: 'VIDEO', mimeType: 'video/mp4', name: 'sha256:deadbeef.mp4',
      version: 1, isCurrent: true, isStale: false,
    },
    {
      artifactId: 'storage-audio', kind: 'AUDIO', mimeType: 'audio/wav', name: 'local://projects/demo/audio.wav',
      version: 1, isCurrent: true, isStale: false,
    },
    {
      artifactId: 'technical-text', kind: 'VIDEO_SCRIPT', mimeType: 'application/json', name: 'technical-json-name.json',
      version: 1, isCurrent: true, isStale: false,
    },
    {
      artifactId: 'request-text', kind: 'JSON', mimeType: 'application/json', name: '/private/tmp/provider-model-request.json',
      artifactType: 'external_generation_request', generationKind: 'image', version: 1, isCurrent: true, isStale: false,
    },
  ])
  assert.deepEqual(
    Object.fromEntries(maliciousNamedArtifacts.map(item => [item.artifactId, item.reviewLabel])),
    {
      'hash-video': '视频片段',
      'path-image': '参考图',
      'request-text': '图片提示词',
      'storage-audio': '语音',
      'technical-text': '文字内容',
    },
    'creator labels derive only from allow-listed semantics and never from artifact names',
  )
  assert.doesNotMatch(
    maliciousNamedArtifacts.map(item => `${item.reviewLabel} ${item.shotLabel || ''}`).join(' '),
    /private|tmp|foo\.png|sha256|deadbeef|local:\/\/|technical-json|provider|model/i,
    'malicious paths, hashes, storage references, and technical names never reach visible labels',
  )
  assert.equal(maliciousNamedArtifacts.find(item => item.artifactId === 'path-image')?.shotLabel, undefined, 'unsafe shot identifiers are not exposed')
  assert.equal(reviewArtifacts.selectCreatorReviewArtifact(projectedReviewArtifacts, 'older-script').artifactId, 'older-script')
  assert.equal(reviewArtifacts.selectCreatorReviewArtifact(projectedReviewArtifacts).artifactId, 'current-script')

  const authoritativeDelivery = reviewArtifacts.authoritativeDeliveryReviewArtifacts({
    preview: [{ artifactId: 'final-video', stepId: 'preview', kind: 'VIDEO', name: 'final.mp4', version: 2, attempt: 2, isCurrent: true, isStale: false, createdAt: '2026-07-26T00:00:00Z' }],
    delivery: [
      { artifactId: 'publish-copy', stepId: 'delivery', kind: 'MARKDOWN', name: 'publish.md', version: 9, attempt: 9, isCurrent: true, isStale: false, createdAt: '2026-07-26T00:01:00Z' },
      { artifactId: 'export-bundle', stepId: 'delivery', kind: 'BUNDLE', name: 'export.zip', version: 11, attempt: 11, isCurrent: true, isStale: false, createdAt: '2026-07-26T00:02:00Z' },
    ],
  }, 'final-video')
  assert.deepEqual(authoritativeDelivery.map(artifact => artifact.artifactId), ['final-video'], 'delivery review uses only the server-authoritative final video identity')
  assert.deepEqual(
    reviewArtifacts.authoritativeDeliveryReviewArtifacts({ delivery: [{ artifactId: 'publish-copy', stepId: 'delivery', kind: 'MARKDOWN', name: 'publish.md', version: 9, attempt: 9, isCurrent: true, isStale: false, createdAt: '2026-07-26T00:01:00Z' }] }, undefined),
    [],
    'delivery review fails closed instead of selecting a generic artifact when final video identity is absent',
  )
  assert.equal(
    reviewArtifacts.selectCreatorReviewArtifact(
      reviewArtifacts.projectCreatorReviewArtifacts([
        { artifactId: 'hidden-selected', kind: 'LOG', name: 'worker.log', version: 4, isCurrent: true, isStale: false },
        { artifactId: 'visible-current', kind: 'VIDEO_SCRIPT', name: 'script.json', version: 3, isCurrent: true, isStale: false },
      ]),
      'hidden-selected',
    ).artifactId,
    'visible-current',
    'a hidden artifact id can never become the selected creator artifact',
  )

  assert.equal(presentation.classifyArtifactPresentation({ kind: 'JSON', mimeType: 'application/json' }), 'json')
  assert.equal(presentation.classifyArtifactPresentation({ kind: 'MARKDOWN', mimeType: 'text/markdown' }), 'markdown')
  assert.equal(presentation.classifyArtifactPresentation({ kind: 'VIDEO', mimeType: 'video/mp4' }), 'video')
  assert.equal(presentation.classifyArtifactPresentation({ kind: 'BUNDLE', mimeType: 'application/octet-stream' }), 'file')
  assert.equal(presentation.safeCreatorReviewText('普通提示词正文'), '普通提示词正文')
  assert.equal(presentation.safeCreatorReviewText('# 可审阅标题\n\n正常 Markdown 正文'), '# 可审阅标题\n\n正常 Markdown 正文')
  assert.equal(presentation.safeCreatorReviewText('[旁白] 保留这段合法脚本文字'), '[旁白] 保留这段合法脚本文字')
  assert.equal(presentation.safeCreatorReviewText('  {"storageRef":"private"}'), undefined)
  assert.equal(presentation.safeCreatorReviewText('\n[{"kind":"LOG"}]'), undefined)
  assert.equal(presentation.safeCreatorReviewText({ prompt: '不得序列化' }), undefined)
  const exactReviewText = '  原始审阅文本保留前后空白。 \n'
  assert.equal(
    presentation.safeCreatorReviewText(exactReviewText),
    exactReviewText,
    'selection sources preserve exact persisted UTF-16 text including leading and trailing whitespace',
  )
  assert.equal(typeof presentation.creatorDirectEditText, 'function', 'direct editing needs an explicit safe canonical-text policy')
  assert.equal(typeof presentation.reconcileCreatorEditMode, 'function', 'artifact switches need a pure edit-mode reconciler')
  assert.equal(
    presentation.creatorDirectEditText('json', { script: '不得序列化到编辑器' }),
    undefined,
    'object JSON is never direct editable',
  )
  assert.equal(
    presentation.creatorDirectEditText('markdown', '  {"script":"不得放入编辑器"}'),
    undefined,
    'JSON-shaped strings are never direct editable',
  )
  assert.equal(
    presentation.creatorDirectEditText('text', '普通提示词正文'),
    '普通提示词正文',
    'safe canonical plain text is direct editable',
  )
  assert.equal(
    presentation.creatorDirectEditText('markdown', '# 标题\n\n安全 Markdown 正文'),
    '# 标题\n\n安全 Markdown 正文',
    'safe canonical Markdown is direct editable',
  )
  assert.equal(
    presentation.creatorDirectEditText('json', '即使是普通字符串，结构化 JSON presentation 也不能直接编辑'),
    undefined,
    'structured JSON presentation never exposes direct editing',
  )
  assert.equal(
    presentation.reconcileCreatorEditMode('direct', undefined),
    'instruction',
    'switching from a direct-editable artifact to structured content exits direct mode',
  )
  assert.equal(
    presentation.reconcileCreatorEditMode('direct', '安全正文'),
    'direct',
    'direct mode remains available for safe canonical text',
  )
  const parsedJson = presentation.parseArtifactJson('{"shots":[{"id":"s1","duration":3},{"id":"s2","duration":4}],"title":"Demo"}')
  assert.equal(parsedJson.ok, true)
  assert.deepEqual(presentation.buildJsonSummary(parsedJson.value), {
    rootType: 'object', itemCount: 2, depth: 4,
  })
  assert.deepEqual(presentation.homogeneousJsonColumns(parsedJson.value.shots), ['id', 'duration'])
  assert.deepEqual(presentation.filterJsonTree(parsedJson.value, 's2'), { shots: [{ id: 's2', duration: 4 }] })
  assert.equal(presentation.parseArtifactJson('{broken').ok, false)
  assert.equal(presentation.artifactContentNeedsLocalHydration({
    content: { storageRef: 'local://projects/vp-1/script.json', contentAvailability: 'local-agent' },
  }), true)
  assert.equal(presentation.artifactContentNeedsLocalHydration({
    content: { title: '真实脚本', segments: [{ text: '可直接审阅' }] },
  }), false, 'server-hydrated JSON must not be replaced by a failing local-file fetch')
  assert.equal(presentation.artifactContentNeedsLocalHydration({ content: '# 已恢复的正文' }), false)
  assert.equal(auth.isDefinitiveAuthFailure({ status: 401 }), true)
  assert.equal(auth.isDefinitiveAuthFailure({ status: 403 }), true)
  assert.equal(auth.isDefinitiveAuthFailure(new Error('Failed to fetch')), false, 'a transient backend outage must not erase the saved login')
  assert.deepEqual(presentation.markdownHeadings('# 标题\n\n## 详情\n\n正文'), [
    { depth: 1, text: '标题', id: '标题' }, { depth: 2, text: '详情', id: '详情' },
  ])
  assert.equal(
    typeof presentation.buildArtifactReviewModel,
    'function',
    'structured creator output needs a human-readable review model instead of an expanded JSON debugger',
  )
  const mirroredScript = '开场先提出创作者的真实困扰，再用三个步骤说明如何把创作过程变得透明、可审阅、可返修。'
  const readableReview = presentation.buildArtifactReviewModel({
    artifacts: [{ artifactId: 'script-json' }],
    content: JSON.stringify({
      characters: [
        { id: 'host', name: '创作者小唐', description: '负责演示完整工作流' },
        { id: 'task', name: '任务卡片', description: '展示每一步状态' },
      ],
      detailedScript: mirroredScript,
      estimatedDurationSec: 60,
      package: {
        characters: [
          { id: 'host', name: '创作者小唐', description: '负责演示完整工作流' },
          { id: 'task', name: '任务卡片', description: '展示每一步状态' },
        ],
        script: mirroredScript,
        summary: '一条介绍透明创作工作流的 60 秒口播视频。',
        sections: [
          { id: 'opening', name: '开场钩子', durationSec: 15, text: '为什么创作过程总像黑箱？' },
          { id: 'proof', name: '流程演示', durationSec: 45, text: '逐步展示审阅和返修。' },
        ],
      },
    }),
    detailedScript: mirroredScript,
    package: {
      script: mirroredScript,
      summary: '一条介绍透明创作工作流的 60 秒口播视频。',
    },
    script: mirroredScript,
  })
  assert.ok(readableReview)
  assert.equal(readableReview.kind, 'script')
  assert.equal(readableReview.script, mirroredScript, 'mirrored script fields must become one canonical script')
  assert.equal(readableReview.summary, '一条介绍透明创作工作流的 60 秒口播视频。')
  assert.deepEqual(readableReview.metrics, [
    { label: '时长', value: '60 秒' },
    { label: '角色', value: '2' },
    { label: '段落', value: '2' },
  ])
  assert.deepEqual(readableReview.characters.map(character => character.name), ['创作者小唐', '任务卡片'])
  assert.deepEqual(readableReview.sections.map(section => section.name), ['开场钩子', '流程演示'])
  assert.equal(
    [readableReview.script, readableReview.summary, ...readableReview.sections.map(section => section.text)].filter(value => value === mirroredScript).length,
    1,
    'the same long-form content must never be repeated in the human-readable model',
  )
  assert.equal(
    presentation.buildArtifactReviewModel({ knowledgeTrace: { sourceCount: 0 }, artifacts: [] }),
    null,
    'pure technical metadata stays in the collapsed technical drawer',
  )
  const historical = JSON.stringify({ artifacts: [{
    kind: 'VIDEO_SCRIPT',
    content: JSON.stringify({ title: '30秒认识躺营', detailedScript: '真实口播正文。' }),
  }] })
  assert.equal(projection.creatorReviewExcerpt(historical), '真实口播正文。')
  assert.equal(projection.creatorReviewExcerpt(historical).includes('{'), false)
  assert.equal(projection.projectCreatorReviewContent(historical).canonicalText, '真实口播正文。')
  assert.equal(
    presentation.safeCreatorReviewText(historical),
    undefined,
    'historical JSON envelopes remain non-selectable until a direct offset mapping exists',
  )
  assert.equal(projection.creatorReviewExcerpt('{"toolCall":{"name":"debug"}}'), '内容已生成，选择后可查看。')
  assert.equal(projection.creatorReviewExcerpt('[{"toolCall"'), '内容已生成，选择后可查看。')
  const longestBodyProjection = projection.projectCreatorReviewContent({
    title: '短标题',
    summary: '这是给创作者的简短摘要。',
    prompt: '这是一段比标题和摘要更完整的生成提示词正文。',
    script: '这是一段较短的脚本。',
  })
  assert.equal(
    longestBodyProjection.canonicalText,
    '这是一段比标题和摘要更完整的生成提示词正文。',
    'the longest allow-listed creator body wins over titles and summaries',
  )
  assert.equal(
    longestBodyProjection.document?.script,
    longestBodyProjection.canonicalText,
    'a sanitized document must retain the selected canonical creator body',
  )
  assert.equal(
    projection.creatorReviewExcerpt({ data: { diagnostics: { toolCall: { name: 'debug' } } } }),
    '内容已生成，选择后可查看。',
    'unknown nested metadata must not be serialized into creator UI',
  )
  const sanitizedProjection = projection.projectCreatorReviewContent({
    title: '允许的标题',
    summary: '允许的摘要',
    script: '允许的脚本正文。',
    characters: [{ name: '禁止的角色' }],
    cast: [{ name: '禁止的演员' }],
    sections: [{ name: '禁止的段落', text: '禁止的段落正文' }],
    scriptSpans: [{ name: '禁止的脚本片段' }],
    segments: [{ name: '禁止的片段' }],
    logline: '禁止的故事梗概',
    documentTitle: '禁止的文档标题',
    projectName: '禁止的项目名称',
    estimatedDurationSec: 42,
    warnings: ['禁止的警告'],
  })
  assert.equal(sanitizedProjection.document?.script, '允许的脚本正文。')
  assert.doesNotMatch(
    JSON.stringify(sanitizedProjection.document),
    /禁止的角色|禁止的演员|禁止的段落|禁止的脚本片段|禁止的片段|禁止的故事梗概|禁止的文档标题|禁止的项目名称|禁止的警告|42/,
    'creator review documents include only allow-listed creator content',
  )

  const emptyView = { steps: [] }
  assert.deepEqual(logic.nextCreatorAction(emptyView), {
    kind: 'start', stepId: 'requirements', label: '开始创作',
  })
  const steps = [
    { id: 'requirements', label: '需求', state: 'confirmed' },
    { id: 'direction', label: '创作方向', state: 'generating' },
    { id: 'script', label: '口播稿', state: 'needs_review' },
  ]
  assert.deepEqual(logic.nextCreatorAction({ steps }), {
    kind: 'wait', stepId: 'direction', label: '创作方向生成中',
  })
  assert.deepEqual(logic.nextCreatorAction({ steps: [{ id: 'script', label: '口播稿', state: 'needs_review' }] }), {
    kind: 'review', stepId: 'script', label: '审核口播稿',
  })
  assert.deepEqual(logic.nextCreatorAction({ steps: [{ id: 'shots', label: '分镜与素材', state: 'failed' }] }), {
    kind: 'fix', stepId: 'shots', label: '处理分镜与素材',
  })

  assert.deepEqual(logic.CREATOR_WORKSPACE_STEP_IDS, [
    'requirements', 'direction', 'script', 'shots', 'preview', 'delivery',
  ], 'workspace must keep the six creation steps in their real order')
  const canonicalProgressSteps = logic.creatorProgressSteps([
    { id: 'preview', label: '旧成片预览', state: 'confirmed' },
    { id: 'shots', label: '分镜与素材', state: 'confirmed' },
    { id: 'preview', label: '当前成片预览', state: 'needs_review' },
    { id: 'requirements', label: '需求', state: 'confirmed' },
    { id: 'shots', label: '当前分镜与素材', state: 'confirmed' },
  ])
  assert.deepEqual(
    canonicalProgressSteps.map(step => step.id),
    ['requirements', 'shots', 'preview'],
    'creator progress renders each canonical step at most once and in workflow order',
  )
  assert.equal(canonicalProgressSteps[1].label, '当前分镜与素材', 'the latest current-step projection wins over duplicate input')
  assert.equal(canonicalProgressSteps[2].label, '当前成片预览', 'historical preview events cannot create duplicate progress rows')
  assert.deepEqual(
    logic.creatorProjectProgress('COMPLETED', { steps: logic.CREATOR_WORKSPACE_STEP_IDS.map(id => ({ id, state: 'idle' })) }),
    { label: '已完成 6/6 个步骤', percent: 100 },
    'a terminal project must not be shown as 0/6 after the agent run completes',
  )
  assert.deepEqual(
    logic.creatorProjectProgress('RUNNING', { steps: [{ id: 'requirements', state: 'confirmed' }, { id: 'script', state: 'needs_review' }] }),
    { label: '已完成 1/2 个步骤', percent: 50 },
  )
  const localRenderContent = {
    artifact: {
      projectId: 'vp-1',
      metadata: { localOnly: true, localPath: '/tmp/tangying/projects/vp-1/renders/final.mp4' },
    },
  }
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', localRenderContent, 'http://127.0.0.1:18080/'),
    'http://127.0.0.1:18080/api/local/media?projectId=vp-1&path=%2Ftmp%2Ftangying%2Fprojects%2Fvp-1%2Frenders%2Ffinal.mp4',
  )
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: 'https://media.example/final.mp4' }, 'http://127.0.0.1:18080'),
    'https://media.example/final.mp4',
  )
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: 'http://127.0.0.1:18080/api/local/media?projectId=vp-2&storageRef=local%3A%2F%2Fprojects%2Fvp-2%2Fartifacts%2Fvideo-1%2Ffinal.mp4' }, 'http://127.0.0.1:18080'),
    undefined,
    'absolute local media cannot cross the active project boundary',
  )
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: '/api/local/media?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-2%2Fartifacts%2Fvideo-1%2Ffinal.mp4' }, 'http://127.0.0.1:18080'),
    undefined,
    'root-relative local media must carry an active-project storageRef',
  )
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: '/api/local/media?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4' }, ''),
    undefined,
    'root-relative local media is not usable until the owned local-agent origin is known',
  )
  for (const encodedLocalPath of [
    'http://127.0.0.1:18080/api%2Flocal%2Fmedia?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
    'http://127.0.0.1:18080/api%252Flocal%252Fmedia?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
    '/api%2Flocal%2Fmedia?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
    '/api%252Flocal%252Fmedia?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
  ]) {
    assert.equal(
      logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: encodedLocalPath }, 'http://127.0.0.1:18080'),
      undefined,
      'encoded local-agent endpoint paths fail closed instead of becoming external media',
    )
  }
  for (const credentialedLocalPath of [
    'http://user:pass@127.0.0.1:18080/api/local/media?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
    'http://user:pass@127.0.0.1:18080/api%2Flocal%2Fmedia?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
    'http://user:pass@127.0.0.1:18080/api%252Flocal%252Fmedia?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
  ]) {
    assert.equal(
      logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: credentialedLocalPath }, 'http://127.0.0.1:18080'),
      undefined,
      'userinfo cannot disguise a local-agent-origin URL as external media',
    )
  }
  const normalizedRelativeLocalMedia = logic.resolveCreatorArtifactMediaUrl(
    'vp-1',
    { ...localRenderContent, mediaUrl: '/api/local/media?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4' },
    'http://127.0.0.1:18080',
  )
  assert.equal(
    normalizedRelativeLocalMedia,
    'http://127.0.0.1:18080/api/local/media?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Fartifacts%2Fvideo-1%2Ffinal.mp4',
    'a root-relative local media URL is canonicalized once at the resolver boundary',
  )
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', { ...localRenderContent, mediaUrl: '/media/final.mp4' }, 'http://127.0.0.1:18080'),
    '/media/final.mp4',
    'other relative direct media remains document-relative without local-service semantics',
  )
  assert.equal(logic.isCreatorLocalMediaUrl(normalizedRelativeLocalMedia, 'http://127.0.0.1:18080'), true, 'resolver output and local probe classification agree')
  assert.equal(logic.resolveCreatorArtifactMediaUrl('other-project', localRenderContent, 'http://127.0.0.1:18080'), undefined)
  assert.equal(
    logic.resolveCreatorArtifactMediaUrl('vp-1', {
      artifact: {
        projectId: 'vp-1', storageRef: 'local://projects/vp-1/reports/qa.json',
        metadata: { localOnly: true },
      },
    }, 'http://127.0.0.1:18080'),
    'http://127.0.0.1:18080/api/local/media?projectId=vp-1&storageRef=local%3A%2F%2Fprojects%2Fvp-1%2Freports%2Fqa.json',
  )
  assert.equal(logic.creatorMediaStateAfterHttpProbe(200), 'loading', 'an HTTP success still needs browser metadata before playback is confirmed')
  assert.equal(logic.creatorMediaStateAfterHttpProbe(404), 'missing', 'a definite missing media response has its own recovery state')
  assert.equal(logic.creatorMediaStateAfterHttpProbe(503), 'service_unavailable', 'an unhealthy local media response is a service failure')
  assert.equal(logic.creatorMediaStateAfterHttpProbe(undefined), 'service_unavailable', 'a connection failure is a service failure')
  assert.equal(logic.creatorMediaStateAfterLoadedMetadata(), 'playable', 'loaded metadata confirms browser playback')
  assert.equal(logic.creatorMediaStateAfterMediaError(3, 200, true), 'unsupported', 'successful local delivery plus decode failure is unsupported')
  assert.equal(logic.creatorMediaStateAfterMediaError(4, 200, true), 'unsupported', 'successful local delivery plus unsupported source is unsupported')
  assert.equal(logic.creatorMediaStateAfterMediaError(1, 200, true), 'service_unavailable', 'an aborted local load is not mislabeled as a codec failure')
  assert.equal(logic.creatorMediaStateAfterMediaError(2, 200, true), 'service_unavailable', 'a network or range failure is not mislabeled as a codec failure')
  assert.equal(logic.creatorMediaStateAfterMediaError(3, 404, true), 'missing', 'a file that disappears before decode is missing')
  assert.equal(logic.creatorMediaStateAfterMediaError(3, undefined, true), 'service_unavailable', 'a failed local re-probe is a service failure')
  assert.equal(logic.creatorMediaStateAfterMediaError(1, undefined, false), 'unavailable', 'an aborted direct source uses neutral delivery copy')
  assert.equal(logic.creatorMediaStateAfterMediaError(2, undefined, false), 'unavailable', 'a direct network failure uses neutral delivery copy')
  assert.equal(logic.creatorMediaStateAfterMediaError(3, undefined, false), 'unsupported', 'a direct decode failure is unsupported')
  assert.equal(logic.creatorMediaStateAfterMediaError(4, undefined, false), 'unsupported', 'a direct unsupported source is unsupported')
  const localAgentBaseUrl = 'http://127.0.0.1:18080'
  assert.equal(logic.isCreatorLocalMediaUrl('http://127.0.0.1:18080/api/local/media?projectId=vp-1', localAgentBaseUrl), true)
  assert.equal(logic.isCreatorLocalMediaUrl('/api/local/media?projectId=vp-1', localAgentBaseUrl), false, 'relative URLs resolve against the document, so local probing fails closed')
  assert.equal(logic.isCreatorLocalMediaUrl('https://media.example/final.mp4?signature=secret', localAgentBaseUrl), false)
  assert.equal(logic.isCreatorLocalMediaUrl('https://media.example/api/local/media?signature=secret', localAgentBaseUrl), false)
  assert.equal(logic.isCreatorLocalMediaUrl('blob:https://creator.example/id', localAgentBaseUrl), false)
  assert.equal(logic.isCreatorLocalMediaUrl('data:video/mp4;base64,AAAA', localAgentBaseUrl), false)
  assert.equal(logic.canApplyCreatorMediaProbe(4, 4, 'local-a', 'local-a', false), true)
  assert.equal(logic.canApplyCreatorMediaProbe(3, 4, 'local-a', 'local-a', false), false, 'an older probe token is stale')
  assert.equal(logic.canApplyCreatorMediaProbe(4, 4, 'local-a', 'local-b', false), false, 'a probe for an older source is stale')
  assert.equal(logic.canApplyCreatorMediaProbe(4, 4, 'local-a', 'local-a', true), false, 'an aborted probe is stale')
  assert.equal(logic.canonicalCreatorMediaElementSrc('/media/final.mp4', 'https://creator.example/workspace'), 'https://creator.example/media/final.mp4')
  assert.equal(logic.canApplyCreatorMediaElementEvent('https://creator.example/media/a.mp4', 'https://creator.example/media/a.mp4', 7, 7), true)
  assert.equal(logic.canApplyCreatorMediaElementEvent('https://creator.example/media/old.mp4', 'https://creator.example/media/new.mp4', 7, 7), false, 'an event from the prior source is stale')
  assert.equal(logic.canApplyCreatorMediaElementEvent('https://creator.example/media/a.mp4', 'https://creator.example/media/a.mp4', 6, 7), false, 'an event from the prior source token is stale')
  assert.equal(logic.canApplyCreatorMediaElementEvent('', 'https://creator.example/media/a.mp4', 7, 7), true, 'an empty currentSrc still uses the source token for browser load failures')
  assert.equal(logic.isCreatorStepReadable({ state: 'confirmed' }), true)
  assert.equal(logic.isCreatorStepReadable({ state: 'needs_attention' }), true)
  assert.equal(logic.isCreatorStepReadable({ state: 'not_started', hasHistory: true }), true)
  assert.equal(logic.isCreatorStepReadable({ state: 'not_started', hasHistory: false }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'confirmed', allowedActions: ['confirm'] }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'needs_attention', allowedActions: ['confirm'] }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'needs_review', allowedActions: [] }), false)
  assert.equal(logic.canConfirmCreatorStep({ state: 'needs_review', allowedActions: ['confirm'] }), true)
  assert.equal(
    logic.formatStepImpact({ affectedStepIds: ['shots', 'preview', 'delivery'], requiresConfirmation: true }),
    '分镜与素材、成片预览、交付需要更新',
  )
  assert.deepEqual(
    logic.normalizeRectSelection({ x: 80, y: 60, width: -30, height: -20 }, { width: 100, height: 100 }),
    { kind: 'rect', x: 0.5, y: 0.4, width: 0.3, height: 0.2 },
  )
  assert.deepEqual(
    logic.createTimeSelection(2800, -1200),
    { kind: 'time', startMs: 0, endMs: 2800 },
  )
  const instructionMutation = {
    artifactId: 'artifact-1', baseVersion: 2, mode: 'instruction', instruction: '语气更轻快',
    selection: { kind: 'time', startMs: 1200, endMs: 2800 }, confirmedAffectedShotIds: ['shot-02'],
  }
  assert.equal(
    logic.creatorMutationIdempotencyKey('project-1', 'script', instructionMutation),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { ...instructionMutation, confirmedAffectedShotIds: ['shot-02'] }),
    'retries of the same requested revision need a stable idempotency key',
  )
  assert.notEqual(
    logic.creatorMutationIdempotencyKey('project-1', 'script', instructionMutation),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { ...instructionMutation, instruction: '改成沉稳语气' }),
  )
  assert.equal(
    logic.creatorStepRegenerationIdempotencyKey('project-1', 'script', 'nonce-1'),
    'creator-regeneration:project-1:script:nonce-1',
  )
  const artifactDescriptors = [
    { artifactId: 'qa-v2', version: 2, isCurrent: true, isStale: false, kind: 'JSON' },
    { artifactId: 'video-v2', version: 2, isCurrent: true, isStale: false, kind: 'VIDEO' },
    { artifactId: 'video-v1', version: 1, isCurrent: false, isStale: true, kind: 'VIDEO' },
  ]
  assert.equal(logic.selectCreatorStepArtifact(artifactDescriptors, 'video-v1', 'qa-v2', 'VIDEO').artifactId, 'video-v1', 'an explicit artifact choice must always win')
  assert.equal(logic.selectCreatorStepArtifact(artifactDescriptors, 'missing', 'qa-v2', 'VIDEO').artifactId, 'video-v2', 'preview defaults to a current non-stale video before a QA report')
  assert.equal(logic.selectCreatorStepArtifact(artifactDescriptors, 'missing', 'qa-v2').artifactId, 'qa-v2', 'other steps keep their current artifact')
  assert.equal(logic.creatorProjectEntryStep('COMPLETED', 'delivery'), 'preview')
  assert.equal(logic.creatorProjectEntryStep('ARCHIVED', 'requirements'), 'preview')
  assert.equal(logic.creatorProjectEntryStep('RUNNING', 'shots'), 'shots')
  assert.equal(logic.creatorProjectEntryStep('RUNNING'), 'requirements')
  const completedView = { project: { status: 'COMPLETED' }, activeStep: 'preview' }
  assert.equal(logic.completedTaskLandingStep(completedView, { delivery: 'playable' }), 'delivery')
  assert.equal(logic.completedTaskLandingStep(completedView, { delivery: 'missing', preview: 'playable' }), 'delivery')
  assert.deepEqual(
    logic.completedRepairScope(completedView, { delivery: 'missing' }),
    { stepId: 'delivery', preserveUpstream: true },
  )
  assert.deepEqual(
    logic.completedRepairScope(completedView, { delivery: 'unsupported' }),
    { stepId: 'delivery', preserveUpstream: true },
  )
  assert.equal(logic.completedRepairScope(completedView, { delivery: 'playable' }), undefined)
  assert.equal(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'loading',
      viewingHistorical: false,
      currentArtifactId: 'delivery-current',
      selectedArtifactId: 'delivery-current',
    }),
    undefined,
    'a delivery artifact still loading is not conclusive missing media',
  )
  assert.equal(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'error',
      viewingHistorical: false,
      currentArtifactId: 'delivery-current',
      selectedArtifactId: 'delivery-current',
    }),
    undefined,
    'a delivery load failure must not be mistaken for a missing media file',
  )
  assert.equal(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'ready',
      viewingHistorical: true,
      currentArtifactId: 'delivery-current',
      selectedArtifactId: 'delivery-old',
    }),
    undefined,
    'a historical delivery selection never exposes recovery for the current task',
  )
  assert.equal(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'ready',
      viewingHistorical: false,
      currentArtifactId: 'delivery-current',
      selectedArtifactId: 'delivery-other',
    }),
    undefined,
    'recovery remains scoped to the selected current delivery artifact',
  )
  assert.equal(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'ready',
      viewingHistorical: false,
      currentArtifactId: 'delivery-current',
      selectedArtifactId: 'delivery-current',
      currentArtifactVersion: 4,
      selectedArtifactVersion: 3,
    }),
    undefined,
    'recovery never uses an older version that shares the current artifact identity',
  )
  assert.deepEqual(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'ready',
      viewingHistorical: false,
      currentArtifactId: 'delivery-current',
      selectedArtifactId: 'delivery-current',
      currentArtifactVersion: 4,
      selectedArtifactVersion: 4,
    }),
    { stepId: 'delivery', preserveUpstream: true },
    'only a loaded current delivery artifact can be recovered',
  )
  assert.deepEqual(
    logic.completedRepairScope(completedView, { delivery: 'missing' }, {
      artifactLoadState: 'empty',
      viewingHistorical: false,
    }),
    { stepId: 'delivery', preserveUpstream: true },
    'a completed task with no final video can regenerate delivery without a fake base artifact',
  )
  assert.equal(
    logic.shouldProbeCreatorDeliveryMedia({
      artifactLoadState: 'ready',
      presentation: 'video',
      mediaUrl: 'http://127.0.0.1:18080/api/local/media?projectId=p&storageRef=delivery',
      finalReviewPassed: false,
    }),
    true,
    'a delivery video is probed even before optional final QA metadata is present',
  )
  assert.equal(
    logic.shouldProbeCreatorDeliveryMedia({
      artifactLoadState: 'loading',
      presentation: 'video',
      mediaUrl: 'http://127.0.0.1:18080/api/local/media?projectId=p&storageRef=delivery',
      finalReviewPassed: false,
    }),
    false,
    'a delivery media probe waits for the selected artifact content',
  )
  assert.equal(
    logic.canDeliverCreatorFinalVideo({ finalReviewPassed: false, mediaUrl: '/delivery.mp4', mediaState: 'playable' }),
    false,
    'a decodable video without final QA remains diagnostic-only and cannot unlock delivery',
  )
  assert.equal(
    logic.canDeliverCreatorFinalVideo({ finalReviewPassed: true, mediaUrl: '/delivery.mp4', mediaState: 'playable' }),
    true,
    'only a final-reviewed, browser-playable video unlocks delivery',
  )
  assert.deepEqual(
    logic.completedDeliveryRepairSuccess({ activeTasks: [{ id: 'delivery-rebuild' }] }),
    { notice: '已开始重新生成成片，需求、创意、脚本和分镜会保留。', refreshAfterMutation: true },
    'a queued repair immediately adopts its returned view before a best-effort refresh',
  )
  assert.notEqual(
    logic.creatorPollingSignature({
      activeTasks: [{ id: 'delivery-review', scope: 'delivery', status: 'running', label: '正在重新生成交付文件' }],
      steps: [{ id: 'delivery', state: 'generating' }],
    }),
    '',
    'a queued no-final-video recovery keeps frontend polling active',
  )
  assert.equal(
    logic.creatorPollingSignature({ activeTasks: [], steps: [{ id: 'delivery', state: 'confirmed' }] }),
    '',
    'frontend polling stops after durable delivery completion',
  )
  assert.notEqual(
    logic.creatorMutationIdempotencyKey('project-1', 'script', instructionMutation),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { ...instructionMutation, confirmedAffectedShotIds: ['shot-03'] }),
  )
  assert.notEqual(
    logic.creatorMutationIdempotencyKey('project-1', 'script', { artifactId: 'artifact-1', baseVersion: 2, mode: 'restore', version: 1, confirmedAffectedShotIds: [] }),
    logic.creatorMutationIdempotencyKey('project-1', 'script', { artifactId: 'artifact-1', baseVersion: 2, mode: 'restore', version: 2, confirmedAffectedShotIds: [] }),
  )
  assert.equal(logic.creatorPollDelay(0), 500)
  assert.equal(logic.creatorPollDelay(1), 1000)
  assert.equal(logic.creatorPollDelay(20), 5000)
  const scriptKey = logic.workspaceArtifactKey({ stepId: 'script', artifactId: 'script-v1', version: 1 })
  const directionKey = logic.workspaceArtifactKey({ stepId: 'direction', artifactId: 'direction-v1', version: 1 })
  const scriptResult = { key: scriptKey, value: { artifact: { id: 'script-v1' }, versions: ['v1'] } }
  assert.equal(logic.isCurrentWorkspaceArtifact(scriptResult, { stepId: 'direction', artifactId: 'direction-v1', version: 1 }), false, 'step B must not render step A while B loads')
  const directionResult = { key: directionKey, value: { artifact: { id: 'direction-v1' }, versions: ['v1'] } }
  assert.equal(logic.isCurrentWorkspaceArtifact(directionResult, { stepId: 'direction', artifactId: 'direction-v1', version: 1 }), true)
  assert.equal(logic.isCurrentWorkspaceArtifact(scriptResult, { stepId: 'direction', artifactId: 'direction-v1', version: 1 }), false, 'late A response must not overwrite B')
  assert.equal(logic.isLatestWorkspaceRequest(2, 2), true)
  assert.equal(logic.isLatestWorkspaceRequest(1, 2), false)
  const projectRecordSteps = logic.withCreatorProjectRecord([
    { id: 'requirements', label: '需求', state: 'confirmed', hasHistory: false, attemptCount: 0, artifactCount: 0, isStale: false, allowedActions: ['view', 'regenerate'] },
    { id: 'script', label: '脚本', state: 'confirmed', hasHistory: true, attemptCount: 1, artifactCount: 2, isStale: false, allowedActions: ['view', 'regenerate'] },
  ], { name: '30 秒产品介绍', description: '介绍软件的创作流程' })
  assert.deepEqual(
    projectRecordSteps.map(step => [step.id, step.hasHistory, step.attemptCount, step.artifactCount]),
    [['requirements', true, 1, 1], ['script', true, 1, 2]],
    'the persisted project brief is reviewable evidence even when it is not a generated file',
  )
  const loadingSelection = { stepId: 'script', artifactId: 'script-v1', version: 1 }
  assert.equal(logic.creatorArtifactLoadState(loadingSelection, null, ''), 'loading')
  assert.equal(logic.creatorArtifactLoadState(loadingSelection, null, '读取失败'), 'error')
  assert.equal(logic.creatorArtifactLoadState(null, null, ''), 'empty')
  assert.equal(logic.creatorArtifactLoadState(loadingSelection, {
    key: logic.workspaceArtifactKey(loadingSelection), value: {},
  }, ''), 'ready')
  const previousTasks = [{ id: 'task-a', shotId: 'shot-a', status: 'running' }, { id: 'task-b', shotId: 'shot-b', status: 'running' }]
  assert.equal(logic.didSelectedShotTaskChange(previousTasks, [{ id: 'task-b', shotId: 'shot-b', status: 'running' }], 'shot-a'), true, 'selected shot task disappearing must refetch')
  assert.equal(logic.didSelectedShotTaskChange(previousTasks, [{ id: 'task-a', shotId: 'shot-a', status: 'running' }, { id: 'task-b', shotId: 'shot-b', status: 'processing' }], 'shot-a'), false, 'other shot changes must not refetch selected shot')
  assert.equal(logic.didSelectedShotTaskChange(previousTasks, [], undefined), false, 'no selected shot must not fan out callbacks')

  const pendingReview = logic.nextPendingCreatorReview([
    { id: 'approved-script', nodeId: 'script-review', status: 'APPROVED', stage: 'script_generation' },
    { id: 'pending-preview', nodeId: 'preview-review', status: 'PENDING', stage: 'preview', reviewContent: '检查文字层' },
  ])
  assert.equal(pendingReview.id, 'pending-preview', 'creator workspace must surface the next dynamic review gate')
  assert.equal(logic.creatorStepForAgentReview(pendingReview), 'preview')
  assert.equal(logic.creatorStepForAgentReview({ stage: 'script_generation', tool: 'video_script_generator' }), 'script')
  assert.equal(logic.creatorStepForAgentReview({ stage: 'visual_alignment', tool: 'visual_alignment_planner' }), 'shots')
  assert.equal(logic.creatorStepForAgentReview({ stage: 'publish_copy', tool: 'publish_copy_generator' }), 'delivery')

  const failedQualityReview = {
    id: 'script-quality-gate',
    nodeId: 'script-quality-gate',
    status: 'PENDING',
    reviewPhase: 'quality_gate',
    reviewReason: '质量门禁：评分需 >=85',
    reviewContent: '这段是待检查的原脚本，不应覆盖质量报告。',
    reviewOutput: {
      qualityReport: {
        score: 75,
        passed: false,
        analysisSummary: '脚本结构完整，但时长不足。',
        issues: [{ field: '节奏与时长', level: 'warning', message: '实际时长约 30-40 秒。' }],
        repairSuggestions: ['扩充到约 150-180 字。', '增加一个具体应用场景。'],
      },
    },
  }
  assert.match(logic.creatorAgentReviewContent(failedQualityReview), /质量评分：75\/100/)
  assert.match(logic.creatorAgentReviewContent(failedQualityReview), /脚本结构完整，但时长不足/)
  assert.doesNotMatch(logic.creatorAgentReviewContent(failedQualityReview), /待检查的原脚本/)
  assert.equal(logic.creatorAgentReviewCanRegenerate(failedQualityReview), true)
  assert.equal(logic.creatorAgentReviewApprovalBlocked(failedQualityReview), true)
  assert.equal(
    logic.creatorAgentReviewRegenerationHint(failedQualityReview),
    '扩充到约 150-180 字。\n增加一个具体应用场景。',
  )
  assert.equal(logic.creatorAgentReviewCanRegenerate({ ...failedQualityReview, status: 'APPROVED' }), false)
  assert.equal(logic.creatorAgentReviewApprovalBlocked({ ...failedQualityReview, reviewPhase: 'after_artifact' }), false)
  assert.equal(
    logic.creatorAgentReviewRegenerationHint({
      id: 'script-review',
      nodeId: 'script-review',
      status: 'PENDING',
      reviewPhase: 'after_artifact',
      reviewReason: '脚本会影响后续分镜，必须确认后继续。',
    }),
    '',
    'a gate explanation is not a modification request and must not be duplicated into the regeneration textarea',
  )

  const hundredShotWindow = logic.shotQueueWindow({
    total: 100, scrollTop: 0, viewportHeight: 480,
  })
  assert.equal(logic.DEFAULT_SHOT_QUEUE_FILTER.status, 'needs_attention', 'the review queue starts with actionable Shots')
  assert.deepEqual(logic.initialShotFilters('completed'), { status: 'all' }, 'completed projects begin with every durable Shot visible')
  assert.deepEqual(logic.initialShotFilters('active'), { status: 'needs_attention' }, 'active projects begin with actionable Shots')
  assert.equal(logic.isUnfilteredShotPageReset({ status: 'all' }, true), true, 'only a pristine first all-Shots page may establish durable zero state')
  assert.equal(logic.isUnfilteredShotPageReset({ status: 'all', query: 'no match' }, true), false, 'an all-Shots search cannot establish durable zero state')
  assert.equal(logic.isUnfilteredShotPageReset({ status: 'all', chapter: '第一章' }, true), false, 'an all-Shots chapter filter cannot establish durable zero state')
  assert.equal(logic.isUnfilteredShotPageReset({ status: 'all' }, false), false, 'a later durable page cannot establish durable zero state')
  assert.equal(logic.canApplyShotListRequestState(4, 5, false), false, 'a stale Shot-list rejection cannot overwrite a newer request state')
  assert.equal(logic.canApplyShotListRequestState(5, 5, false), true, 'the current Shot-list request may update state')
  assert.equal(logic.canApplyShotListRequestState(5, 5, true), false, 'an aborted Shot-list request cannot update state')
  const historicalShotArtifacts = [
    { artifactId: 'shot-video', relatedShotId: 'shot-01', artifactType: 'composited_shot_video', kind: 'COMPOSITED_SHOT_VIDEO', isCurrent: true, isStale: false },
    { artifactId: 'shot-package', relatedShotId: 'shot-01', artifactType: 'shot_asset_package', kind: 'SHOT_ASSET_PACKAGE', isCurrent: true, isStale: false },
    { artifactId: 'shot-list', kind: 'SHOT_LIST', isCurrent: true, isStale: false },
    { artifactId: 'old-video', relatedShotId: 'shot-01', artifactType: 'composited_shot_video', kind: 'COMPOSITED_SHOT_VIDEO', isCurrent: false, isStale: true },
  ]
  const [historicalShot] = completedShots.projectHistoricalShots(historicalShotArtifacts)
  assert.deepEqual(historicalShot.artifactIds, ['shot-video', 'shot-package', 'shot-list'], 'historical Shot review keeps current direct artifacts and shared structured context without stale duplicates')
  assert.deepEqual(completedShots.projectHistoricalShots([
    { artifactId: 'later', relatedShotId: 'shot_10', artifactType: 'shot_audio', kind: 'SHOT_AUDIO', isCurrent: true, isStale: false },
    { artifactId: 'first', relatedShotId: 'shot 2', artifactType: 'shot_keyframe', kind: 'SHOT_KEYFRAME', isCurrent: true, isStale: false },
    { artifactId: 'ignored', relatedShotId: 'draft-2', kind: 'VIDEO_PROMPTS', isCurrent: true, isStale: false },
  ]).map(shot => shot.sequenceIndex), [2, 10], 'historical recovery sorts numeric Shot groups and ignores unrelated records')

  const review = completedShots.projectHistoricalShotReview(historicalShot, [
    {
      artifactId: 'shot-list',
      content: {
        shotList: [{
          id: 'SHOT_01', title: '开场亮相', durationSec: 6,
          narrationText: '做一条好视频，不该从十几个工具间来回搬运开始。',
          mainAction: '树懒 IP 正对镜头抬手问候', camera: '中景固定机位，轻微推进',
          lighting: '左前方柔和主光，右后方轮廓光', screenText: ['一句创意', '完整成片'],
        }],
      },
    },
    {
      artifactId: 'shot-package',
      content: {
        shotId: 'SHOT_01',
        ipArollPlan: { designSummary: '树懒 IP 全程正对镜头口播并保持眼神交流。' },
        hyperframesPlan: { designSummary: '关键词以两行以内标题卡分段出现。' },
        aigcPlan: { designSummary: '使用本地产品界面截图补充说明，不遮挡角色。' },
        referenceImages: [{ imageUrl: 'https://media.test/reference-01.png', label: '角色构图参考' }],
      },
    },
    { artifactId: 'shot-video', content: {}, mediaUrl: 'https://media.test/shot-01.mp4' },
  ])
  assert.equal(review.title, '开场亮相')
  assert.equal(review.durationSec, 6)
  assert.equal(review.narration, '做一条好视频，不该从十几个工具间来回搬运开始。')
  assert.deepEqual(review.screenText, ['一句创意', '完整成片'])
  assert.match(review.details.map(item => item.value).join('\n'), /正对镜头抬手问候/)
  assert.match(review.layers.find(layer => layer.key === 'ip')?.summary ?? '', /眼神交流/)
  assert.match(review.layers.find(layer => layer.key === 'text')?.summary ?? '', /标题卡/)
  assert.match(review.layers.find(layer => layer.key === 'enrichment')?.summary ?? '', /界面截图/)
  assert.deepEqual(review.media.map(item => [item.kind, item.url]), [
    ['video', 'https://media.test/shot-01.mp4'],
    ['image', 'https://media.test/reference-01.png'],
  ], 'historical Shot review exposes registered video and reference-image previews instead of link-only metadata')

  const canonicalNarrationSegments = [
    '第一镜先提出创作痛点。',
    '第二镜说明一句话就能开始。',
    '第三镜展示脚本与分镜审核。',
    '第四镜说明三层画面协同。',
    '第五镜收束到完整成片交付。',
  ]
  const canonicalNarration = canonicalNarrationSegments.join('')
  const historicalTimelineArtifacts = [
    ...canonicalNarrationSegments.map((_, index) => ({
      artifactId: `derived-package-${index + 1}`,
      relatedShotId: `SHOT_${String(index + 1).padStart(2, '0')}`,
      kind: 'SHOT_ASSET_PACKAGE',
      isCurrent: true,
      isStale: false,
    })),
    { artifactId: 'canonical-time-windows', kind: 'TIME_WINDOW_PLAN', isCurrent: true, isStale: false },
    { artifactId: 'derived-video-prompts', kind: 'VIDEO_PROMPTS', isCurrent: true, isStale: false },
  ]
  const recoveredTimelineShots = completedShots.projectHistoricalShots(historicalTimelineArtifacts)
  assert.equal(recoveredTimelineShots.length, canonicalNarrationSegments.length)
  assert.ok(
    recoveredTimelineShots.every(shot => shot.artifactIds.includes('canonical-time-windows')),
    'every recovered Shot receives the canonical time-window plan as shared context',
  )
  const recoveredNarrations = recoveredTimelineShots.map((shot, index) => {
    const shotId = `SHOT_${String(index + 1).padStart(2, '0')}`
    const loaded = [
      {
        artifactId: `derived-package-${index + 1}`,
        content: { shotId, narrationText: canonicalNarration, sourceScriptSegment: canonicalNarration },
      },
      {
        artifactId: 'derived-video-prompts',
        content: { videoPrompts: canonicalNarrationSegments.map((_, promptIndex) => ({
          shotId: `SHOT_${String(promptIndex + 1).padStart(2, '0')}`,
          narrationText: canonicalNarration,
        })) },
      },
      {
        artifactId: 'canonical-time-windows',
        content: { timeWindows: canonicalNarrationSegments.map((narrationText, windowIndex) => ({
          shotId: `SHOT_${String(windowIndex + 1).padStart(2, '0')}`,
          startMs: windowIndex * 6000,
          endMs: (windowIndex + 1) * 6000,
          durationMs: 6000,
          narrationText,
          timelineRevision: 'timeline-v1',
        })) },
      },
    ]
    return completedShots.projectHistoricalShotReview(shot, loaded).narration
  })
  assert.deepEqual(
    recoveredNarrations,
    canonicalNarrationSegments,
    'historical review prefers canonical per-Shot narration over repeated full-script text in derived artifacts',
  )
  assert.equal(
    recoveredNarrations.join(''),
    canonicalNarration,
    'the recovered Shot narration segments concatenate to the complete narration exactly once',
  )
  assert.ok(hundredShotWindow.items.length <= 12, '480px / 64px rows with overscan must render at most 12 queue rows')
  assert.deepEqual(hundredShotWindow.items, Array.from({ length: hundredShotWindow.items.length }, (_, index) => index))
  assert.equal(logic.selectedShotAfterAppend('shot-024', [{ id: 'shot-001' }], [{ id: 'shot-025' }]), 'shot-024', 'page append never clears the active Shot')
  assert.equal(logic.selectedShotAfterAppend(undefined, [{ id: 'shot-001' }], [{ id: 'shot-025' }]), 'shot-001', 'initial page picks its first Shot')
  assert.equal(logic.selectedShotAfterReplacement('shot-024', [{ id: 'shot-024' }, { id: 'shot-025' }]), 'shot-024', 'a queue refresh preserves the selected Shot when it still matches')
  assert.equal(logic.selectedShotAfterReplacement('shot-024', [{ id: 'shot-025' }]), 'shot-025', 'a queue refresh moves only when the selected Shot no longer matches')
  assert.equal(logic.isShotRetryEligible({ qaStatus: '', candidates: [{ status: 'SHOT_QA_FAILED' }] }), true, 'candidate QA failures offer a scoped retry')
  assert.equal(logic.isShotRetryEligible({ qaStatus: '', candidates: [{ status: 'SHOT_QA_PASSED', qaReport: { status: 'SHOT_QA_FAILED' } }] }), true, 'candidate report failures offer a scoped retry')
  assert.equal(logic.isShotRetryEligible({ qaStatus: 'SHOT_QA_PASSED', candidates: [] }), false)
  const adoptedTasks = logic.adoptCreatorShotTask(
    [{ id: 'task-older', scope: 'shots', shotId: 'shot-001', status: 'running', label: '旧任务' }],
    { id: 'task-new', scope: 'shots', shotId: 'shot-002', status: 'queued', label: '正在重新生成镜头' },
  )
  assert.deepEqual(adoptedTasks.map(task => task.id), ['task-older', 'task-new'], 'adopting a new Shot task keeps other durable Shot work active')
  assert.equal(focus.cycleFocusIndex(1, 3, false), 2)
  assert.equal(focus.cycleFocusIndex(0, 3, true), 2)
  assert.equal(focus.cycleFocusIndex(-1, 3, true), 2)

  const creationRequest = logic.buildCreationRequest({
    prompt: '为夏日咖啡新品拍一支轻快的竖版短片',
    durationSec: 30,
    aspectRatio: '9:16',
    platform: '抖音',
    materialCount: 2,
    productionRoute: 'cinematic_story',
    modelProviders: {
      text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-private' },
    },
  })
  assert.deepEqual(creationRequest.project, {
    name: '为夏日咖啡新品拍一支轻快的竖版短片',
    description: '为夏日咖啡新品拍一支轻快的竖版短片',
    mode: 'aigc_shot',
    skillName: 'video-creator',
    skillVersion: 'v4.0',
    workflowName: 'dynamic-agent-video-creation',
    workflowVersion: 'v4.0',
    generationMode: 'provider_api',
    aspectRatio: '9:16',
    targetDurationSec: 30,
    language: 'zh-CN',
    config: {
      entry: 'creator_studio',
      topic: '为夏日咖啡新品拍一支轻快的竖版短片',
      durationSec: 30,
      targetDurationSec: 30,
      aspectRatio: '9:16',
      platform: '抖音',
      materialCount: 2,
      productionRoute: 'cinematic_story',
      canonicalProfileId: 'cinematic_story',
      aigcEnabled: true,
      aigcProvider: 'auto',
      aigcPolicy: 'auto',
      visualLayerContract: 'shot_visual_layers_v1',
      designedLayers: ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'],
      layerExecutionPolicy: {
        ip_aroll: 'optional', hyperframes_text: 'required', aigc_enrichment: 'auto',
      },
      requiredLayers: ['aigc_main', 'hyperframes_text'],
      modelProviderRefs: {
        text_to_text: { source: 'local_agent', baseUrl: 'https://model.test/v1', model: 'writer' },
      },
    },
  })
  assert.deepEqual(creationRequest.agentRun, {
    message: '创作一支 30 秒、适合抖音发布的视频：为夏日咖啡新品拍一支轻快的竖版短片',
    domain: 'video_creation',
    mode: 'dynamic_agent',
    context: {
      topic: '为夏日咖啡新品拍一支轻快的竖版短片',
      durationSec: 30,
      targetDurationSec: 30,
      aspectRatio: '9:16',
      platform: '抖音',
      materialCount: 2,
      productionRoute: 'cinematic_story',
      canonicalProfileId: 'cinematic_story',
      videoType: 'aigc_shot',
      aigcEnabled: true,
      aigcProvider: 'auto',
      aigcPolicy: 'auto',
      visualLayerContract: 'shot_visual_layers_v1',
      designedLayers: ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'],
      layerExecutionPolicy: {
        ip_aroll: 'optional', hyperframes_text: 'required', aigc_enrichment: 'auto',
      },
      requiredLayers: ['aigc_main', 'hyperframes_text'],
      modelProviders: {
        text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-private' },
      },
    },
  })
  assert.doesNotMatch(JSON.stringify(creationRequest.project), /sk-private/, 'project config must never persist API keys')
  const creationCopy = `${creationRequest.project.description} ${creationRequest.agentRun.message}`
  assert.doesNotMatch(creationCopy, /provider|run|trace|artifact/i, 'creator copy must not expose developer vocabulary')

  const talkingHeadRequest = logic.buildCreationRequest({
    prompt: '树懒阿洛分享三个提高专注力的小技巧，配合简洁标题卡片',
    durationSec: 15,
    aspectRatio: '16:9',
    materialCount: 0,
    productionRoute: 'talking_head',
    modelProviders: {
      text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-text' },
      text_to_image: { baseUrl: 'https://model.test/v1', model: 'image', apiKey: 'sk-image' },
      text_to_video: { baseUrl: 'https://model.test/v1', model: 'video', apiKey: 'sk-video' },
    },
  })
  assert.equal(talkingHeadRequest.project.mode, 'voice_visual')
  assert.equal(talkingHeadRequest.project.generationMode, 'provider_api')
  assert.equal(talkingHeadRequest.project.config.productionRoute, 'talking_head')
  assert.equal(talkingHeadRequest.project.config.canonicalProfileId, 'talking_head')
  assert.equal(talkingHeadRequest.project.config.aigcEnabled, true)
  assert.equal(talkingHeadRequest.project.config.aigcPolicy, 'auto')
  assert.equal(talkingHeadRequest.project.config.visualLayerContract, 'shot_visual_layers_v1')
  assert.deepEqual(talkingHeadRequest.project.config.designedLayers, ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'])
  assert.deepEqual(talkingHeadRequest.project.config.layerExecutionPolicy, {
    ip_aroll: 'required', hyperframes_text: 'required', aigc_enrichment: 'auto',
  })
  assert.deepEqual(talkingHeadRequest.project.config.requiredLayers, ['ip_aroll', 'hyperframes_text'])
  assert.equal(talkingHeadRequest.project.config.ipRenderMode, 'production')
  assert.deepEqual(talkingHeadRequest.project.config.voiceSelection, {
    mode: 'default_ip',
    provider: 'gpt_sovits_local',
    voiceId: 'main_ip_warm_knowledge_host_v1',
  })
  assert.equal(logic.validateCreatorVoiceSelection({ mode: 'default_ip' }), undefined)
  assert.match(
    logic.validateCreatorVoiceSelection({
      mode: 'reference_clone',
      provider: 'gpt_sovits_local',
      referenceText: '',
      usageRightsConfirmed: false,
    }),
    /录音原文/,
  )
  assert.equal(
    logic.validateCreatorVoiceSelection({
      mode: 'reference_clone',
      provider: 'gpt_sovits_local',
      referenceText: '这是一段经过确认的录音原文',
      referenceTextVerified: true,
      usageRightsConfirmed: true,
    }),
    undefined,
  )
  assert.equal(talkingHeadRequest.agentRun.context.videoType, 'voice_visual')
  assert.equal(talkingHeadRequest.agentRun.context.aigcEnabled, true)
  assert.equal(talkingHeadRequest.agentRun.context.aigcProvider, 'auto')
  assert.equal(talkingHeadRequest.agentRun.context.ipRenderMode, 'production')
  assert.deepEqual(talkingHeadRequest.agentRun.context.voiceSelection, {
    mode: 'default_ip',
    provider: 'gpt_sovits_local',
    voiceId: 'main_ip_warm_knowledge_host_v1',
  })
  assert.deepEqual(Object.keys(talkingHeadRequest.agentRun.context.modelProviders), ['text_to_text', 'text_to_image', 'text_to_video'])

  const localTalkingHeadRequest = logic.buildCreationRequest({
    prompt: '纯本地树懒口播，但保留未来 AIGC 丰富层的设计',
    durationSec: 15,
    aspectRatio: '16:9',
    materialCount: 0,
    productionRoute: 'talking_head',
    aigcPolicy: 'disabled',
    modelProviders: {
      text_to_text: { baseUrl: 'https://model.test/v1', model: 'writer', apiKey: 'sk-text' },
      text_to_video: { baseUrl: 'https://model.test/v1', model: 'video', apiKey: 'sk-video' },
    },
  })
  assert.equal(localTalkingHeadRequest.project.config.aigcEnabled, false)
  assert.equal(localTalkingHeadRequest.project.config.aigcPolicy, 'disabled')
  assert.equal(localTalkingHeadRequest.agentRun.context.aigcProvider, 'disabled')
  assert.deepEqual(localTalkingHeadRequest.agentRun.context.designedLayers, ['ip_aroll', 'hyperframes_text', 'aigc_enrichment'])
  assert.deepEqual(localTalkingHeadRequest.agentRun.context.layerExecutionPolicy, {
    ip_aroll: 'required', hyperframes_text: 'required', aigc_enrichment: 'disabled',
  })
  assert.deepEqual(Object.keys(localTalkingHeadRequest.agentRun.context.modelProviders), ['text_to_text'])
  assert.doesNotMatch(JSON.stringify(localTalkingHeadRequest), /sk-video/)

  const defaultCreationRequest = logic.buildCreationRequest({
    prompt: '做一个品牌故事',
    aspectRatio: '16:9',
    materialCount: 0,
  })
  assert.equal(defaultCreationRequest.project.targetDurationSec, undefined)
  assert.equal(defaultCreationRequest.project.mode, 'voice_visual', 'creator defaults to deterministic talking-head production')
  assert.equal(defaultCreationRequest.project.config.aigcEnabled, true)
  assert.equal(defaultCreationRequest.agentRun.context.durationSec, undefined)
  assert.equal(defaultCreationRequest.agentRun.message, '创作视频：做一个品牌故事')

  let activeWorkers = 0
  let maxActiveWorkers = 0
  const concurrentResults = await logic.mapWithConcurrency([1, 2, 3, 4, 5], 2, async value => {
    activeWorkers += 1
    maxActiveWorkers = Math.max(maxActiveWorkers, activeWorkers)
    await new Promise(resolve => setTimeout(resolve, 1))
    activeWorkers -= 1
    return value * 2
  })
  assert.deepEqual(concurrentResults, [2, 4, 6, 8, 10])
  assert.equal(maxActiveWorkers, 2, 'creation view loading must have a bounded concurrency')

  const settledIndexes = []
  await logic.mapWithConcurrency([1, 2], 2, async value => {
    await new Promise(resolve => setTimeout(resolve, value === 1 ? 3 : 1))
    return value
  }, (_value, index) => settledIndexes.push(index))
  assert.deepEqual(settledIndexes, [1, 0], 'each creation view must update as it settles')

  assert.equal(
    logic.buildProjectMaterialStorageRef('project-1', 'material-1'),
    'local://projects/project-1/materials/material-1',
  )
  for (const unsafe of ['material/1', 'material%1', 'material\\1', 'material?1', 'material#1']) {
    assert.throws(() => logic.buildProjectMaterialStorageRef('project-1', unsafe), /safe path segment/)
  }
  assert.equal(logic.creatorStartIdempotencyKey('project-1'), 'creator-start:project-1')

  const viewOrder = logic.prioritizeCreationViewProjects([
    { id: 'completed-new', status: 'COMPLETED', updatedAt: '2026-07-20T12:00:00Z' },
    { id: 'active-old', status: 'RUNNING', updatedAt: '2026-07-20T09:00:00Z' },
    { id: 'active-new', status: 'PAUSED', updatedAt: '2026-07-20T11:00:00Z' },
    { id: 'archived', status: 'ARCHIVED', updatedAt: '2026-07-20T13:00:00Z' },
  ])
  assert.deepEqual(viewOrder.map(project => project.id), ['active-new', 'active-old', 'archived', 'completed-new'])

  for (const accepted of [0.001, 1, 14.999]) assert.equal(logic.canSubmitShotDuration(accepted), true)
  for (const rejected of [-1, 0, 15, 16, Number.NaN, Number.POSITIVE_INFINITY]) {
    assert.equal(logic.canSubmitShotDuration(rejected), false)
  }
  assert.equal(logic.deliveryArtifactPassesFinalReview({ finalQaStatus: 'passed' }), true)
  assert.equal(logic.deliveryArtifactPassesFinalReview({ finalQualityCheck: { passed: true } }), true)
  assert.equal(logic.deliveryArtifactPassesFinalReview({ finalQaStatus: 'failed' }), false)
	assert.equal(logic.deliveryArtifactPassesFinalReview({ status: 'passed' }), false, 'generic artifact status is not final QA')
	assert.equal(logic.deliveryArtifactPassesFinalReview({ qaStatus: 'passed' }), false, 'Shot QA is not final QA')
  assert.equal(logic.deliveryArtifactPassesFinalReview({ artifact: { metadata: { finalQaStatus: 'passed' } } }), true)
	assert.equal(logic.deliveryArtifactPassesFinalReview({ artifact: { metadata: { qaStatus: 'passed' } } }), false)
  assert.equal(logic.deliveryArtifactPassesFinalReview({ mediaUrl: '/video.mp4' }), false, 'a media URL alone must never unlock delivery')

  const shots = Array.from({ length: 100 }, (_, index) => ({
    id: `shot-${String(100 - index).padStart(3, '0')}`,
    sequenceIndex: index % 10,
    title: index % 2 ? `Night ${index}` : `Morning ${index}`,
    chapter: index % 3 ? 'middle' : 'opening',
    reviewStatus: index % 4 ? 'pending' : 'approved',
    generationStatus: index % 5 ? 'idle' : 'generating',
  }))
  const snapshot = JSON.stringify(shots)
  const selected = logic.selectShotListItems(shots, {
    status: 'pending', chapter: 'middle', query: 'night',
  })
  assert.equal(JSON.stringify(shots), snapshot, 'selector must not mutate input')
  assert.ok(selected.length > 0 && selected.length < 100)
  for (const shot of selected) {
    assert.equal(shot.reviewStatus, 'pending')
    assert.equal(shot.chapter, 'middle')
    assert.match(shot.title.toLowerCase(), /night/)
  }
  assert.deepEqual(
    selected.map(({ sequenceIndex, id }) => [sequenceIndex, id]),
    [...selected].map(({ sequenceIndex, id }) => [sequenceIndex, id]).sort(([aSequence, aId], [bSequence, bId]) =>
      aSequence - bSequence || aId.localeCompare(bId)),
    'Shot ordering is deterministic by sequenceIndex then id',
  )
  const ties = [
    { id: 'same', sequenceIndex: 1, title: 'first', reviewStatus: 'pending', generationStatus: 'PLANNED' },
    { id: 'same', sequenceIndex: 1, title: 'second', reviewStatus: 'pending', generationStatus: 'PLANNED' },
  ]
  assert.deepEqual(logic.selectShotListItems(ties, {}).map(item => item.title), ['first', 'second'])

  assert.equal(logic.CREATOR_CONFLICT_COPY, '内容已更新，请刷新后重试')
  assert.equal(logic.isTargetOnlyShotImpact({
    shotId: 'shot-042', affectedShotIds: ['shot-042'], regeneratesOtherShots: false,
  }, 'shot-042'), true)
  assert.equal(logic.isTargetOnlyShotImpact({
    shotId: 'shot-042', affectedShotIds: ['shot-042', 'shot-043'], regeneratesOtherShots: true,
  }, 'shot-042'), false)

  const apiSource = readFileSync(new URL('../src/services/creatorApi.ts', import.meta.url), 'utf8')
  const appSource = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8')
  const runtimeApiSource = readFileSync(new URL('../src/services/api.ts', import.meta.url), 'utf8')
  const typeSource = readFileSync(new URL('../src/features/creator-studio/types.ts', import.meta.url), 'utf8')
  const generatedSource = readFileSync(new URL('../src/utils/api-types.generated.ts', import.meta.url), 'utf8')
  const startPageSource = readFileSync(new URL('../src/features/creator-studio/StartCreationPage.tsx', import.meta.url), 'utf8')
  const librarySource = readFileSync(new URL('../src/features/creator-studio/VideoLibraryPage.tsx', import.meta.url), 'utf8')
  const workspaceSource = readFileSync(new URL('../src/features/creator-studio/ProjectWorkspacePage.tsx', import.meta.url), 'utf8')
  const stripSource = readFileSync(new URL('../src/features/creator-studio/components/CreationStrip.tsx', import.meta.url), 'utf8')
  const reviewSource = readFileSync(new URL('../src/features/creator-studio/components/ArtifactReviewPanel.tsx', import.meta.url), 'utf8')
  const creatorPrerequisiteUrls = [
    '../src/features/creator-studio/artifactPresentation.ts',
    '../src/features/creator-studio/components/ArtifactProofingCanvas.tsx',
    '../src/features/creator-studio/components/CreatorProcessTimeline.tsx',
    '../src/features/creator-studio/components/JsonArtifactViewer.tsx',
    '../src/features/creator-studio/components/MarkdownArtifactViewer.tsx',
    '../src/features/creator-studio/components/ProjectBriefPanel.tsx',
    '../src/features/creator-studio/components/SimpleVideoPlayer.tsx',
    '../src/features/creator-studio/components/StepRegenerationDialog.tsx',
  ].map(path => new URL(path, import.meta.url))
  for (const prerequisiteUrl of creatorPrerequisiteUrls) {
    assert.equal(existsSync(prerequisiteUrl), true, `creator production prerequisite must be committed: ${prerequisiteUrl.pathname}`)
  }
  const proofingSource = readFileSync(new URL('../src/features/creator-studio/components/ArtifactProofingCanvas.tsx', import.meta.url), 'utf8')
  const projectionSource = readFileSync(projectionUrl, 'utf8')
  const timelineSource = readFileSync(new URL('../src/features/creator-studio/components/CreatorProcessTimeline.tsx', import.meta.url), 'utf8')
  const reviewArtifactsSource = readFileSync(new URL('../src/features/creator-studio/creatorReviewArtifacts.ts', import.meta.url), 'utf8')
  const artifactSwitcherUrl = new URL('../src/features/creator-studio/components/CreatorCurrentArtifactSwitcher.tsx', import.meta.url)
  assert.equal(existsSync(artifactSwitcherUrl), true, 'multiple current artifacts need one compact top switcher')
  const artifactSwitcherSource = existsSync(artifactSwitcherUrl) ? readFileSync(artifactSwitcherUrl, 'utf8') : ''
  const contentLibraryUrl = new URL('../src/features/creator-studio/components/CreatorContentLibrary.tsx', import.meta.url)
  const projectBriefUrl = new URL('../src/features/creator-studio/components/ProjectBriefPanel.tsx', import.meta.url)
  assert.equal(existsSync(projectBriefUrl), true, 'requirements need a readable persisted project record')
  const projectBriefSource = readFileSync(projectBriefUrl, 'utf8')
  const regenerationDialogSource = readFileSync(new URL('../src/features/creator-studio/components/StepRegenerationDialog.tsx', import.meta.url), 'utf8')
  const agentReviewSource = readFileSync(new URL('../src/features/creator-studio/components/AgentReviewGatePanel.tsx', import.meta.url), 'utf8')
  const jsonViewerSource = readFileSync(new URL('../src/features/creator-studio/components/JsonArtifactViewer.tsx', import.meta.url), 'utf8')
  const markdownViewerSource = readFileSync(new URL('../src/features/creator-studio/components/MarkdownArtifactViewer.tsx', import.meta.url), 'utf8')
  const textSelectionAssistantUrl = new URL('../src/features/creator-studio/components/TextSelectionAssistant.tsx', import.meta.url)
  assert.equal(existsSync(textSelectionAssistantUrl), true, 'readable creator proofing needs one text selection assistant')
  const textSelectionAssistantSource = readFileSync(textSelectionAssistantUrl, 'utf8')
  const reviewableTextSurfaceUrl = new URL('../src/features/creator-studio/components/ReviewableTextSurface.tsx', import.meta.url)
  assert.equal(existsSync(reviewableTextSurfaceUrl), true, 'readable creator proofing needs one direct selection surface')
  const reviewableTextSurfaceSource = existsSync(reviewableTextSurfaceUrl) ? readFileSync(reviewableTextSurfaceUrl, 'utf8') : ''
  const imageReviewDialogUrl = new URL('../src/features/creator-studio/components/ImageReviewDialog.tsx', import.meta.url)
  assert.equal(existsSync(imageReviewDialogUrl), true, 'image proofing needs a full-screen review dialog')
  const imageReviewDialogSource = existsSync(imageReviewDialogUrl) ? readFileSync(imageReviewDialogUrl, 'utf8') : ''
  const creatorStylesSource = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8')
  const recoverySource = readFileSync(new URL('../src/features/creator-studio/components/TaskRecoveryBanner.tsx', import.meta.url), 'utf8')
  const queueSource = readFileSync(new URL('../src/features/creator-studio/components/ShotReviewQueue.tsx', import.meta.url), 'utf8')
  const inspectorSource = readFileSync(new URL('../src/features/creator-studio/components/ShotInspector.tsx', import.meta.url), 'utf8')
  const improveSource = readFileSync(new URL('../src/features/creator-studio/components/ShotImprovePanel.tsx', import.meta.url), 'utf8')
  const previewSource = readFileSync(new URL('../src/features/creator-studio/components/PreviewDeliveryPanel.tsx', import.meta.url), 'utf8')
  const playerUrl = new URL('../src/features/creator-studio/components/SimpleVideoPlayer.tsx', import.meta.url)
  assert.equal(existsSync(playerUrl), true, 'completed videos need one reusable built-in player')
  const playerSource = readFileSync(playerUrl, 'utf8')
  const audioPlayerUrl = new URL('../src/features/creator-studio/components/SimpleAudioPlayer.tsx', import.meta.url)
  assert.equal(existsSync(audioPlayerUrl), true, 'audio proofing needs the same built-in transport semantics as video')
  const audioPlayerSource = readFileSync(audioPlayerUrl, 'utf8')
  const mediaRangeSource = readFileSync(mediaRangeUrl, 'utf8')
  const mediaRangeControlsUrl = new URL('../src/features/creator-studio/components/MediaRangeControls.tsx', import.meta.url)
  assert.equal(existsSync(mediaRangeControlsUrl), true, 'video and audio review need shared playhead range controls')
  const mediaRangeControlsSource = readFileSync(mediaRangeControlsUrl, 'utf8')
  assertCreatorRenderedSurfaceIsSafe(new Map([
    ['ProjectWorkspacePage.tsx', workspaceSource],
    ['CreatorProcessTimeline.tsx', timelineSource],
    ['ArtifactReviewPanel.tsx', reviewSource],
    ['CreatorCurrentArtifactSwitcher.tsx', artifactSwitcherSource],
    ['CreationStrip.tsx', stripSource],
    ['StepRegenerationDialog.tsx', regenerationDialogSource],
    ['ShotInspector.tsx', inspectorSource],
    ['ArtifactProofingCanvas.tsx', proofingSource],
    ['JsonArtifactViewer.tsx', jsonViewerSource],
    ['PreviewDeliveryPanel.tsx', previewSource],
  ]))
  for (const functionName of [
    'getCreationView', 'getStepVersions', 'previewStepRevision', 'reviseStep', 'confirmStep',
    'restoreStepVersion', 'previewStepRegeneration', 'regenerateStep', 'registerProjectMaterial', 'listShots', 'getShotSummary',
    'getShotWorkspace', 'getShotHistory', 'previewShotRegeneration', 'regenerateShot',
    'acceptShotCandidate', 'restoreShotCandidate', 'rebuildFinalAssembly',
  ]) {
    assert.match(apiSource, new RegExp(`export (?:async )?function ${functionName}\\b`), `${functionName} must be exported`)
  }
  assert.doesNotMatch(apiSource, /\bany\b/, 'creator API boundaries must not use any')
  assert.doesNotMatch(typeSource, /Omit<Generated/, 'honest generated contracts must not be masked by curated Omit types')
  assert.match(apiSource, /'Idempotency-Key': idempotencyKey/g)
  assert.match(appSource, /isDefinitiveAuthFailure/)
  assert.doesNotMatch(appSource, /catch\s*\{\s*logout\(\)/, 'session restore must distinguish expired credentials from a temporary outage')
  assert.match(apiSource, /request: StepRevisionPreviewRequest/)
  assert.match(apiSource, /request: StepRevisionMutationRequest/)
  assert.match(typeSource, /StepRevisionPreviewRequest = GeneratedStepRevisionPreviewRequest/)
  assert.match(typeSource, /StepRevisionMutationRequest = GeneratedStepRevisionMutationRequest/)
  assert.match(typeSource, /StepRegenerationRequest = GeneratedStepRegenerationRequest/)
  assert.match(typeSource, /StepRegenerationResult = StepRegenerationResponse\['data'\]/)
  assert.match(
    generatedSource,
    /export interface CreatorTask \{[\s\S]*status: 'generating' \| 'running' \| 'processing' \| 'queued' \| 'dispatching';[\s\S]*\}/,
    'generated CreatorTask status remains a closed creator-facing union',
  )
  assert.doesNotMatch(
    generatedSource.match(/export interface CreatorTask \{[\s\S]*?\n\}/)?.[0] || '',
    /CREATED|READY|WAITING_LOCAL|LOCAL_CLAIMED|LOCAL_RUNNING|LOCAL_COMPLETED|RETRYING/,
    'generated CreatorTask must not expose durable node lifecycle values',
  )
  assert.match(generatedSource, /scope: 'candidate_accept'/)
  assert.match(generatedSource, /scope: 'candidate_restore'/)
  assert.match(generatedSource, /export interface StepRevisionPreviewRequest \{\s+artifactId: string;\s+baseVersion: number;\s+\}/)
  assert.match(generatedSource, /export type StepRevisionMutationRequest = .*mode: 'direct'.* \| .*mode: 'instruction'.* \| .*mode: 'replace'/)
  assert.match(generatedSource, /mode: 'replace'[\s\S]*replacementMaterial:/)
  assert.doesNotMatch(
    generatedSource.match(/[^;\n]*mode: 'replace'[^;\n]*/)?.[0] || '',
    /instruction|directContent|modelProviders/,
    'typed replacement branch must not expose instruction or provider fields',
  )
  assert.match(generatedSource, /export interface StepRegenerationRequest \{[\s\S]*confirmedAffectedStepIds:/)
  assert.match(apiSource, /assertPositiveVersion\(request\.baseVersion\)/)
  assert.match(apiSource, /signal/g, 'creator requests must support AbortSignal')
  assert.match(startPageSource, /storageRef: buildProjectMaterialStorageRef\(nextProjectId, item\.id\)/)
  assert.match(startPageSource, /creatorStartIdempotencyKey\(nextProjectId\)/)
  assert.match(startPageSource, /productionRoute/)
  assert.match(startPageSource, /IP 口播视频/)
  assert.match(startPageSource, /默认 IP 音色/)
  assert.match(startPageSource, /上传录音复刻音色/)
  assert.match(startPageSource, /直接使用已录口播/)
  assert.match(startPageSource, /录音中实际说出的完整文字/)
  assert.match(startPageSource, /accept="audio\/wav,audio\/mpeg,audio\/mp4,audio\/x-m4a,audio\/flac/)
  assert.match(startPageSource, /usageRightsConfirmed/)
  assert.match(startPageSource, /artifactType: voiceMode === 'recorded_narration' \? 'recorded_narration' : 'voice_reference'/)
  assert.doesNotMatch(typeSource, /chattts_local/i, 'ChatTTS is not a supported creator voice provider')
  assert.doesNotMatch(startPageSource, /ChatTTS|chattts_local/i, 'the creator never offers a non-production ChatTTS route')
  assert.match(startPageSource, /provider: 'gpt_sovits_local'/, 'reference cloning stays pinned to the approved local provider')
  assert.match(startPageSource, /智能补充素材/)
  assert.doesNotMatch(startPageSource, /每个 Shot 都按三层设计|A-roll|HyperFrames|AIGC 丰富层/)
  assert.match(startPageSource, /const modelProviders = await buildClientModelProvidersForRun\(\)/, 'creator start resolves runtime providers')
  assert.match(startPageSource, /buildCreationRequest\(\{[\s\S]*modelProviders,[\s\S]*\}\)/, 'creator start sends provider refs and transient runtime providers through the request builder')
  assert.match(reviewSource, /request\.mode === 'instruction'\s*\? await buildClientModelProvidersForRun\(\)\s*:\s*undefined/, 'instruction revisions resolve runtime providers while direct edits do not')
  assert.ok((startPageSource.match(/disabled=\{starting\}/g) || []).length >= 6, 'creation inputs must lock while starting')
  assert.match(startPageSource, /AbortController/)
  assert.match(startPageSource, /useEffect\(\(\) => \{\s+activeRef\.current = true/)
  assert.match(startPageSource, /signal: controller\.signal/g)
  assert.match(startPageSource, /operationControllerRef\.current\?\.abort\(\)/)
  assert.match(startPageSource, /item\.upload \|\| await uploadLocalArtifactFile/)
  assert.ok(
    startPageSource.indexOf('const upload = item.upload') < startPageSource.indexOf('await registerProjectMaterial') &&
    startPageSource.indexOf('await registerProjectMaterial') < startPageSource.indexOf('await startAgentRun'),
    'materials must upload, register, then start in that order',
  )
  assert.match(runtimeApiSource, /'Idempotency-Key': options\.idempotencyKey/)
  assert.match(runtimeApiSource, /signal: options\.signal/)
  assert.match(runtimeApiSource, /export const retryLatestFailedAgentNode/)
  assert.match(runtimeApiSource, /\/node\/\$\{encodeURIComponent\(failedNode\.id\)\}\/retry/)
  assert.match(librarySource, /prioritizeCreationViewProjects/)
  assert.match(librarySource, /completedTaskLandingStep/)
  assert.match(librarySource, /观看成片/)
  assert.match(workspaceSource, /getCreationView\(projectId/)
  assert.match(workspaceSource, /document\.visibilityState !== 'visible'/)
  assert.match(workspaceSource, /controller\.abort\(\)/)
  assert.match(workspaceSource, /workspaceArtifactKey\(requestSelection\)/)
  assert.match(workspaceSource, /isLatestWorkspaceRequest/)
  assert.match(workspaceSource, /selectedShotId/)
  assert.match(workspaceSource, /retryLatestFailedAgentNode/)
  assert.match(workspaceSource, /CreatorProcessTimeline/)
  assert.match(workspaceSource, /import CreatorCurrentArtifactSwitcher from '.\/components\/CreatorCurrentArtifactSwitcher'/, 'workspace imports the compact current-artifact switcher')
  assert.match(workspaceSource, /<CreatorCurrentArtifactSwitcher\b/, 'workspace renders the compact current-artifact switcher')
  assert.doesNotMatch(workspaceSource, /CreatorContentLibrary/, 'the permanent artifact sidebar is retired')
  assert.doesNotMatch(workspaceSource, /StepArtifactDrawer/, 'the audit drawer is retired from creator workspace usage')
  assert.match(workspaceSource, /const visibleArtifacts = (?:useMemo\([^]*?)?currentCreatorReviewArtifacts\(/, 'creator workspace consumes only the latest current artifacts')
  assert.ok(
    workspaceSource.indexOf('currentCreatorReviewArtifacts(') < workspaceSource.indexOf('selectCreatorReviewArtifact('),
    'current-only projection happens before artifact selection',
  )
  assert.match(workspaceSource, /artifacts=\{visibleArtifacts\}/, 'library counts and cards receive visible artifacts only')
  assert.match(workspaceSource, /artifact=\{selectedArtifact\}/, 'the review panel receives the projected selected artifact')
  assert.match(workspaceSource, /ProjectBriefPanel/)
  assert.match(workspaceSource, /creatorArtifactLoadState/)
  assert.match(workspaceSource, /重新读取当前内容/)
  assert.doesNotMatch(workspaceSource, /view\.processTimeline/, 'creator workspace keeps audit history out of the user-facing progress summary')
  assert.match(workspaceSource, /view\?\.stepArtifacts/)
  assert.match(workspaceSource, /creatorPollingSignature\(view\)/, 'workspace polling uses the tested authoritative view predicate')
  assert.match(workspaceSource, /重试失败步骤/)
  assert.doesNotMatch(workspaceSource, /\[projectId, stepId, view\]/)
  assert.match(recoverySource, /生成仍在后台继续/)
  assert.match(stripSource, /aria-current=\{isCurrent \? 'step' : undefined\}/)
  assert.match(stripSource, /canSelect/)
  assert.match(stripSource, /step\.hasHistory/)
  assert.match(stripSource, /已有内容/)
  assert.match(reviewSource, /确认并继续/)
  assert.match(reviewSource, /创意方案用于在写脚本前确定视频的核心表达、目标受众、叙事节奏和视觉方向/, 'the proposal step explains its creator-facing purpose')
  assert.match(reviewSource, /previewStepRevision/)
  assert.match(reviewSource, /confirmedAffectedShotIds/)
  assert.match(reviewSource, /normalizeRectSelection/)
  assert.match(reviewSource, /CREATOR_CONFLICT_COPY/)
  assert.match(reviewSource, /contentLoadedRef/)
  assert.match(reviewSource, /creatorDirectEditText\(presentation, content\?\.reviewText\)/)
  assert.match(reviewSource, /reconcileCreatorEditMode\(current, directEditText\)/)
  assert.match(reviewSource, /setDirectContent\(directEditText \?\? ''\)/)
  assert.match(reviewSource, /\{canDirectEdit && <button[\s\S]*?>直接编辑<\/button>\}/)
  assert.match(reviewSource, /\{mode === 'direct' && canDirectEdit && \(/)
  assert.doesNotMatch(
    reviewSource,
    /function contentText\b|JSON\.stringify\(content/,
    'direct editing must never serialize arbitrary artifact content',
  )
  assert.doesNotMatch(
    reviewSource,
    /\['json', 'markdown', 'text'\]\.includes\(presentation\)/,
    'structured JSON presentation must not share the direct-edit eligibility flag',
  )
  assert.match(proofingSource, /artifact-selection-overlay/)
  assert.match(proofingSource, /<img\b/, 'creator image proofing uses an actual image preview')
  assert.match(proofingSource, /fetch\(resolvedMediaUrl/)
  assert.match(proofingSource, /AbortController/)
  assert.match(proofingSource, /onPointerCancel/)
  assert.match(proofingSource, /SimpleVideoPlayer/)
  assert.doesNotMatch(proofingSource, /<video\b/, 'artifact proofing delegates video playback to the shared player')
  assert.match(proofingSource, /projectCreatorReviewContent\(displayedContent\)/)
  assert.match(inspectorSource, /projectHistoricalShotReview/, 'completed Shot review projects readable source content')
  assert.match(inspectorSource, /resolveCreatorArtifactMediaUrl\(projectId, response, getLocalAgentBaseUrl\(\)\)/, 'completed Shot review resolves local media through the project-scoped local service')
  assert.match(inspectorSource, /<SimpleVideoPlayer\b/, 'completed Shot review previews its retained video')
  assert.match(inspectorSource, /<SimpleAudioPlayer\b/, 'completed Shot review previews its retained narration')
  assert.match(inspectorSource, /<img\b/, 'completed Shot review renders reference images, rather than links only')
  assert.match(inspectorSource, /画面与动作/)
  assert.match(inspectorSource, /IP A-roll/)
  assert.match(inspectorSource, /文字层/)
  assert.match(inspectorSource, /补充画面/)
  assert.match(workspaceSource, /shot-review-workspace\$\{historicalShotMode \? ' is-historical' : ''\}/, 'historical Shot review uses a dedicated full-width workspace mode')
  assert.match(queueSource, /historical-shot-filmstrip/, 'historical Shot choices render as a horizontal filmstrip')
  assert.match(queueSource, /aria-current=\{selectedHistoricalShotId === shot\.id \? 'true' : undefined\}/, 'the selected historical Shot remains exposed to assistive technology')
  assert.match(inspectorSource, /historical-shot-disclosure/, 'long historical Shot text has an explicit disclosure')
  assert.match(inspectorSource, /查看完整内容/, 'the disclosure explains that it reveals the exact retained content')
  assert.match(creatorStylesSource, /\.shot-review-workspace\.is-historical\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/s, 'historical Shot review removes the sidebar column')
  assert.match(creatorStylesSource, /\.creator-main:has\(\.shot-review-workspace\.is-historical\)/, 'the completed Shot review may use the wider editing canvas without widening unrelated steps')
  assert.match(creatorStylesSource, /\.historical-shot-filmstrip\s*\{[^}]*overflow-x:\s*auto/s, 'the Shot filmstrip stays usable when all choices do not fit')
  assert.match(creatorStylesSource, /\.historical-shot-details\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fit,\s*minmax\(min\(100%,\s*12\.5rem\),\s*1fr\)\)/s, 'historical detail cards fill the available row instead of leaving a one-card orphan row')
  assert.doesNotMatch(inspectorSource, /可回看的创作层|已保留|未保留/, 'completed Shot review never replaces readable content with retention flags')
  assert.doesNotMatch(proofingSource, /artifactContentText\(displayedContent\)/, 'creator plain-text proofing must never serialize non-string content')
  assert.match(proofingSource, /const selectionSource = safeCreatorReviewText\(content\.reviewText\)/, 'scoped selection must use the exact reviewText contract')
  assert.doesNotMatch(
    proofingSource,
    /buildTextSelection\((?:displayedContent|readableText)/,
    'scoped selection must never derive offsets from hydrated, decorated, or projected display content',
  )
  assert.doesNotMatch(reviewSource, /开始时间（秒）|结束时间（秒）|type="number"/, 'creator review no longer exposes numeric seconds inputs')
  assert.doesNotMatch(mediaRangeControlsSource, /type="number"/, 'playhead ranges never expose numeric millisecond inputs')
  assert.match(mediaRangeControlsSource, /从这里开始/)
  assert.match(mediaRangeControlsSource, /到这里结束/)
  assert.match(mediaRangeControlsSource, /清除范围/)
  assert.match(mediaRangeSource, /createTimeSelection/)
  assert.ok((proofingSource.match(/<MediaRangeControls\b/g) || []).length >= 2, 'video and audio use the same MediaRangeControls')
  assert.match(proofingSource, /<SimpleAudioPlayer\b/)
  assert.match(
    proofingSource,
    /<SimpleAudioPlayer[\s\S]*?onError=\{invalidateMediaReview\}[\s\S]*?onReady=\{markPlaybackReady\}[\s\S]*?\{playbackAvailable && onMediaSelectionChange && <MediaRangeControls/,
    'every audio failure clears and hides range controls until the remounted media is ready',
  )
  assert.match(
    proofingSource,
    /state === 'missing' \|\| state === 'unsupported' \|\| state === 'service_unavailable' \|\| state === 'unavailable'[\s\S]*?invalidateMediaReview\(\)/,
    'every explicit video failure clears completed and pending ranges',
  )
  assert.match(proofingSource, /mediaReviewAfterPlaybackFailure\(/, 'video and audio share executable range invalidation behavior')
  assert.ok((proofingSource.match(/disabled=\{mediaRangeDisabled\}/g) || []).length >= 2, 'video and audio range controls freeze while revision impact is loading')
  assert.match(reviewSource, /const revisionInputsLocked = creatorRevisionInputsLocked\(working, pending !== null\)/, 'one lock covers loading and pending confirmation snapshots')
  assert.match(reviewSource, /mediaRangeDisabled=\{!canRevise \|\| revisionInputsLocked\}/, 'historical and locked review surfaces cannot change a media range')
  assert.match(reviewSource, /disabled=\{!canRevise \|\| revisionInputsLocked \|\| mediaRangePending\}/, 'a one-boundary range disables revision submission')
  for (const label of ['调整语气', '优化语速', '调整停顿', '修正发音', '重新生成整段语音']) {
    assert.match(reviewSource, new RegExp(label), `audio review exposes ${label}`)
  }
  assert.ok((reviewSource.match(/<button type="button" disabled=\{!canRevise \|\| revisionInputsLocked\}/g) || []).length >= 5, 'audio quick actions freeze with the revision request snapshot')
  assert.match(reviewSource, /if \(regenerateFullAudio\) \{\s*setSelection\(null\)\s*setMediaRangePending\(false\)/, 'full audio regeneration clears the time selection')
  assert.ok((reviewSource.match(/<textarea[\s\S]*?disabled=\{revisionInputsLocked\}/g) || []).length >= 2, 'instruction and direct-content editors lock to the pending request snapshot')
  assert.match(reviewSource, /onImagePointerDown=\{revisionInputsLocked \? undefined : startRectangle\}/, 'image selection cannot diverge from an in-flight snapshot')
  assert.match(reviewSource, /onTextSelectionChange=\{revisionInputsLocked \? undefined : handleTextSelectionChange\}/, 'text selection cannot diverge from an in-flight snapshot')
  assert.match(reviewSource, /<ImageReviewDialog[\s\S]*?revisionInputsLocked=\{revisionInputsLocked\}/, 'the full-screen image review receives the shared loading and confirmation lock')
  assert.match(reviewSource, /disabled=\{!canRevise \|\| revisionInputsLocked\} onClick=\{\(\) => setMode\('instruction'\)\}/, 'instruction mode cannot replace a pending snapshot')
  assert.match(reviewSource, /disabled=\{!canRevise \|\| revisionInputsLocked\} onClick=\{enterDirectMode\}/, 'direct mode cannot replace a pending snapshot')
  assert.match(reviewSource, /<textarea value=\{directContent\}[\s\S]*?disabled=\{!canRevise \|\| revisionInputsLocked\}[\s\S]*?previewRevision\(\)/, 'direct preview cannot replace a pending snapshot')
  assert.match(reviewSource, /artifact-preview-button" disabled=\{!canRevise \|\| revisionInputsLocked \|\| mediaRangePending\}/, 'instruction preview stays locked while confirmation owns the snapshot')
  assert.match(reviewSource, /disabled=\{!canRevise \|\| revisionInputsLocked\} onClick=\{event => \{ impactTriggerRef\.current = event\.currentTarget; previewRestore\(version\.version\) \}\}>恢复这一版<\/button>/, 'restore cannot open a second snapshot while revision inputs are locked')
  assert.match(reviewSource, /!canRevise \|\| revisionInputsLocked \|\| !file\.type\.startsWith\('image\/'\)/, 'direct replacement calls cannot bypass the shared snapshot lock')
  const replacementSelectionSnapshot = reviewSource.indexOf("const replacementSelection = selection?.kind === 'rect' ? selection : null")
  assert.ok(
    replacementSelectionSnapshot >= 0 &&
    replacementSelectionSnapshot < reviewSource.indexOf('await uploadLocalArtifactFile', replacementSelectionSnapshot) &&
    reviewSource.indexOf('selection: replacementSelection', replacementSelectionSnapshot) > replacementSelectionSnapshot,
    'image replacement captures its immutable rectangle before upload and impact preview begin',
  )
  assert.match(
    reviewSource,
    /const handleMediaSelectionChange = \(nextSelection: TimeSelection \| null\) => \{[\s\S]*?if \(revisionInputsLocked\) \{[\s\S]*?operationControllerRef\.current\?\.abort\(\)[\s\S]*?setWorking\(false\)[\s\S]*?closeImpact\(\)[\s\S]*?onMediaSelectionChange=\{handleMediaSelectionChange\}/,
    'playback invalidation cancels a locked request snapshot before clearing its visible range',
  )
  assert.doesNotMatch(mediaRangeControlsSource, /media-range-summary" aria-live=/, 'the moving playhead is outside every live region')
  assert.match(mediaRangeControlsSource, /creator-visually-hidden" aria-live="polite"/, 'only explicit range actions are announced')
  assert.match(reviewSource, /aria-modal="true"/)
  assert.match(reviewSource, /cycleFocusIndex/)
  assert.match(reviewSource, /operationControllerRef\.current === controller/)
  assert.match(agentReviewSource, /creatorAgentReviewContent/)
  assert.match(agentReviewSource, /regenerateAgentStage/)
  assert.match(agentReviewSource, /按要求重新生成/)
  assert.match(agentReviewSource, /质量未通过时不要直接放行/)
  assert.doesNotMatch(agentReviewSource, /creator-agent-review-content/)
  assert.doesNotMatch(agentReviewSource, /creator-agent-review-details/, 'the gate must not render the same payload a second time below the readable review')
  assert.match(agentReviewSource, /review\.reviewOutput/, 'the readable gate uses the structured source payload when available')
  assert.match(agentReviewSource, /safeCreatorReviewText\(content\)/)
  assert.doesNotMatch(agentReviewSource, /looksLikeStructuredContent|qualityReview && content/, 'quality review content must use the shared safe-text boundary')
  assert.match(jsonViewerSource, /projectCreatorReviewContent/)
  assert.doesNotMatch(projectionSource, /buildArtifactReviewModel/)
  assert.match(jsonViewerSource, /关键内容仍在准备中/)
  assert.match(jsonViewerSource, /artifact-review-script/)
  assert.match(jsonViewerSource, /结构化内容可整体优化，局部划选暂不可用。/)
  assert.doesNotMatch(
    jsonViewerSource,
    /技术数据|查看原文|parsed\.raw|JSON\.stringify|JsonTree|downloadText|navigator\.clipboard|<pre\b/,
    'creator JSON proofing must never expose raw payloads or technical field views',
  )
  assert.match(proofingSource, /projectCreatorReviewContent/)
  assert.match(projectionSource, /creatorPayloadCandidates/)
  assert.match(projectionSource, /MAX_CREATOR_PAYLOAD_DEPTH = 6/)
  assert.match(projectionSource, /MAX_CREATOR_ARRAY_ENTRIES = 64/)
  assert.doesNotMatch(
    proofingSource,
    /<p>[^<]*artifact\.(?:mimeType|kind|sizeBytes)|formatBytes\s*\(|未知文件类型|打开原文件/,
    'creator proofing fallbacks must not render or link to technical artifact descriptors',
  )
  assert.match(markdownViewerSource, /safeCreatorReviewText\(content\)/)
  assert.match(markdownViewerSource, /<JsonArtifactViewer content=\{content\}/)
  assert.match(markdownViewerSource, /<ReviewableTextSurface/, 'Markdown renders its canonical readable text as the direct selection surface')
  assert.match(jsonViewerSource, /<ReviewableTextSurface/, 'JSON renders its projected canonical text as the direct selection surface')
  assert.doesNotMatch(jsonViewerSource, /artifact-selection-mode|aria-pressed=|setSelecting/, 'JSON selection never requires a separate mode button')
  assert.doesNotMatch(markdownViewerSource, /artifact-selection-mode|aria-pressed=|setSelecting/, 'Markdown selection never requires a separate mode button')
  assert.doesNotMatch(
    markdownViewerSource,
    /artifactContentText|源码|navigator\.clipboard|downloadText|artifact-raw-preview|<pre\b/,
    'creator Markdown proofing must preserve readable Markdown without raw source, copy, download, or serialization controls',
  )
  assert.match(creatorStylesSource, /\.artifact-review-document/)
  assert.match(
    creatorStylesSource,
    /\.creator-content-workspace\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/,
    'the completed-project workspace keeps one full-width proofing column',
  )
  assert.doesNotMatch(
    creatorStylesSource,
    /grid-template-columns:\s*minmax\((?:240px|280px),\s*(?:280px|320px)\)\s+minmax\(0,\s*1fr\)/,
    'no creator breakpoint reserves a permanent artifact sidebar',
  )
  assert.match(
    creatorStylesSource,
    /\.creator-current-artifact-switcher[^}]*overflow-x:\s*auto/,
    'multiple current artifacts scroll horizontally without narrowing the proofing canvas',
  )
  assert.match(
    creatorStylesSource,
    /\.artifact-review-document\s*\{[^}]*width:\s*min\(100%,\s*78ch\)[^}]*margin-inline:\s*auto/,
    'long review documents keep a readable line length',
  )
  for (const label of ['更精炼', '增强画面感', '优化节奏', '自定义修改']) {
    assert.match(textSelectionAssistantSource, new RegExp(label), `text selection assistant includes ${label}`)
  }
  assert.match(textSelectionAssistantSource, /只修改所选内容，使表达更精炼；保持上下文含义和未选内容不变。/)
  assert.match(textSelectionAssistantSource, /event\.key === 'Escape'/)
  assert.match(textSelectionAssistantSource, /removeAllRanges\(\)/)
  assert.match(reviewableTextSurfaceSource, /selectionFromDomRange/)
  assert.match(reviewableTextSurfaceSource, /NodeFilter\.SHOW_TEXT/)
  assert.doesNotMatch(reviewableTextSurfaceSource, /textContent[\s\S]*buildTextSelection/, 'selection offsets must never be reconstructed from decorated DOM text')
  assert.match(reviewSource, /内容已更新，请重新选择需要修改的文字。/)
  assert.match(reviewSource, /reviewTextSourceHash/)
  assert.match(reviewSource, /\^sha256:\[0-9a-f\]\{64\}\$/)
  assert.match(reviewSource, /setSelection\(selection\)/)
  assert.match(reviewSource, /rebaseTextSelection/, 'projected readable selections map back to the exact backend review source')
  assert.match(reviewSource, /setInstruction\(nextInstruction\)/)
  assert.match(reviewSource, /setMode\('instruction'\)/)
  assert.match(reviewSource, /const enterDirectMode = \(\) => \{[\s\S]*?setSelection\(current => current\?\.kind === 'text' \? null : current\)[\s\S]*?setTextSelectionDraft\(null\)[\s\S]*?setMode\('direct'\)/, 'entering direct mode clears any text selection before changing mode')
  assert.match(reviewSource, /onClick=\{enterDirectMode\}>\u76f4\u63a5\u7f16\u8f91<\/button>/, 'the direct-edit action uses the selection-clearing transition')
  assert.match(reviewSource, /mode: 'direct', directContent: trimmedContent,[\s\S]*?selection: undefined/, 'direct requests cannot retain text-selection provenance')
  assert.match(reviewSource, /instructionRef\.current\?\.focus\(\)/)
  assert.match(timelineSource, /创作进度/)
  assert.match(timelineSource, /onSelect: \(stepId: CreatorStepId\) => void/)
  assert.match(timelineSource, /steps: readonly CreatorStep\[\]/, 'creator progress consumes the canonical current steps, not the audit-event history')
  assert.match(timelineSource, /creatorProgressSteps\(steps\)/, 'creator progress uses the tested canonical step projection')
  assert.match(timelineSource, /key=\{step\.id\}/, 'each canonical creator step renders at most once')
  assert.match(timelineSource, /onClick=\{\(\) => onSelect\(step\.id\)\}/)
  assert.match(timelineSource, /aria-current=\{step\.id === selectedStepId \? 'step' : undefined\}/)
  assert.doesNotMatch(timelineSource, /events\.map|CreatorProcessEvent|event\.(?:title|summary|attempt)|formatEventTime|可审计记录/, 'creator progress never expands the complete audit history into duplicate rows')
  for (const stateLabel of ['未开始', '生成中', '等待审阅', '已完成', '需要处理']) {
    assert.match(timelineSource, new RegExp(stateLabel), `timeline includes the friendly ${stateLabel} state`)
  }
  assert.match(workspaceSource, /<CreatorProcessTimeline\s+steps=\{displaySteps\}[\s\S]*onSelect=\{navigateToStep\}/, 'timeline uses the deduplicated current-step projection and reuses creator step navigation')
  assert.doesNotMatch(workspaceSource, /<CreatorProcessTimeline\s+events=\{view\.processTimeline/, 'the creator surface must not render raw audit history as progress')
  assert.equal(existsSync(contentLibraryUrl), false, 'the completed-project workspace no longer ships a permanent artifact sidebar')
  assert.match(artifactSwitcherSource, /role="tablist"/)
  assert.match(artifactSwitcherSource, /role="tab"[\s\S]*aria-selected=\{selected\}/, 'current artifacts expose tab semantics and selected state')
  assert.match(artifactSwitcherSource, /tabIndex=\{selected \? 0 : -1\}/, 'the switcher uses roving keyboard focus')
  assert.match(artifactSwitcherSource, /nextCreatorArtifactTabIndex/)
  assert.match(artifactSwitcherSource, /getCreatorArtifactContent/)
  assert.match(artifactSwitcherSource, /resolveCreatorArtifactMediaUrl/)
  assert.match(artifactSwitcherSource, /AbortController/)
  assert.match(artifactSwitcherSource, /Math\.min\(3, mediaArtifacts\.length\)/, 'compact media metadata loads with bounded concurrency')
  assert.doesNotMatch(artifactSwitcherSource, /<(?:video|audio)\b/, 'the compact switcher never mounts a competing media player')
  assert.doesNotMatch(reviewArtifactsSource, /reviewLabel:\s*artifact\.name/, 'review labels never alias raw artifact names')
  assert.match(reviewArtifactsSource, /text:\s*'文字内容'/, 'unknown text artifacts have a semantic generic label')
  for (const [surfaceName, surfaceSource] of [
    ['ArtifactReviewPanel', reviewSource],
    ['ArtifactProofingCanvas', proofingSource],
    ['CreatorCurrentArtifactSwitcher', artifactSwitcherSource],
    ['ImageReviewDialog', imageReviewDialogSource],
    ['SimpleVideoPlayer', playerSource],
    ['SimpleAudioPlayer', audioPlayerSource],
  ]) {
    assert.doesNotMatch(
      surfaceSource,
      /(?:reviewLabel|title|alt|downloadName)=\{[^}]*content\??\.artifact\.name|(?:title|alt|downloadName)=\{[^}]*artifact\.name/,
      `${surfaceName} never falls back from semantic labels to raw artifact names`,
    )
  }
  assert.match(artifactSwitcherSource, /<img[\s\S]*loading="lazy"/, 'image tabs use a lazy real thumbnail')
  assert.doesNotMatch(
    creatorRenderedSurface(artifactSwitcherSource, 'CreatorCurrentArtifactSwitcher.tsx'),
    /\b(?:attempt|sizeBytes|storageType|storageRef|contentHash|promptHash|artifactType|kind)\b/,
    'current artifact tabs never render technical artifact descriptors',
  )
  for (const sourcePattern of [
    /role="dialog"/,
    /aria-modal="true"/,
    /event\.key === 'Escape'/,
    /\.focus\(\)/,
    /zoom/,
    /normalizeRectSelection/,
    /重新加载预览/,
    /局部重绘/,
    /调整构图/,
    /风格与光影/,
    /整张重生成/,
    /替换图片/,
    /保留这张/,
  ]) {
    assert.match(imageReviewDialogSource, sourcePattern, `image review dialog contract requires ${sourcePattern}`)
  }
  assert.match(imageReviewDialogSource, /accept="image\/\*"/, 'replacement picker accepts image files only')
  assert.match(imageReviewDialogSource, /revisionInputsLocked: boolean/, 'image review accepts the shared request snapshot lock')
  assert.match(imageReviewDialogSource, /const busy = revisionInputsLocked \|\| replacementWorking/, 'image review combines parent snapshot and local replacement locks')
  assert.match(imageReviewDialogSource, /const startPointer = [\s\S]*?\{\s*if \(busy\) return/, 'locked image review ignores new pointer selections')
  assert.match(imageReviewDialogSource, /const movePointer = [\s\S]*?\{\s*if \(busy\) return/, 'locked image review cannot move an existing rectangle')
  assert.match(imageReviewDialogSource, /const chooseQuickAction = [\s\S]*?\{\s*if \(busy\) return/, 'locked image review ignores quick-action requests')
  assert.match(imageReviewDialogSource, /const chooseReplacement = [\s\S]*?if \(busy \|\| !file\) return/, 'locked image review ignores replacement picker completion')
  assert.match(imageReviewDialogSource, /aria-pressed=\{selectionMode\}[\s\S]*?disabled=\{busy\}/, 'locked image review disables rectangle selection mode')
  assert.match(imageReviewDialogSource, /selection && <button type="button" disabled=\{busy\}[\s\S]*?>清除选区<\/button>/, 'locked image review cannot clear the captured rectangle')
  assert.match(imageReviewDialogSource, /<textarea\s+value=\{instruction\}\s+disabled=\{busy\}/, 'locked image review disables its request instruction')
  assert.match(imageReviewDialogSource, /type="file" accept="image\/\*" tabIndex=\{-1\} disabled=\{busy\}/, 'locked image review disables the replacement picker itself')
  assert.ok((imageReviewDialogSource.match(/disabled=\{busy\}/g) || []).length >= 6, 'image quick actions, selection controls, replacement, and keep all share the busy lock')
  assert.match(imageReviewDialogSource, /disabled=\{busy \|\| !instruction\.trim\(\)\}/, 'image revision preview shares the busy lock')
  assert.doesNotMatch(
    imageReviewDialogSource,
    /if\s*\(!busy\)\s*onClose\(\)/,
    'Escape must close and cancel image replacement even while work is in flight',
  )
  assert.doesNotMatch(
    imageReviewDialogSource,
    /disabled=\{busy\}\s+onClick=\{onClose\}/,
    'the visible image-review close control must remain available while work is in flight',
  )
  assert.match(
    reviewSource,
    /const cancelImageReview = \(\) => \{[\s\S]*operationControllerRef\.current\?\.abort\(\)[\s\S]*operationControllerRef\.current = null[\s\S]*setWorking\(false\)[\s\S]*setImageDialogOpen\(false\)/,
    'closing image review aborts and detaches the active replacement operation before hiding the dialog',
  )
  assert.match(reviewSource, /onClose=\{cancelImageReview\}/, 'Escape and the close control use the aborting image-review close path')
  assert.match(
    reviewSource,
    /previewStepRevision[\s\S]*if \(!isCurrent\(\)\) throw new Error\('replacement preview was cancelled'\)[\s\S]*setPending/,
    'an aborted or superseded replacement preview cannot open impact confirmation with a late response',
  )
  assert.match(
    imageReviewDialogSource,
    /triggerRef\.current = document\.activeElement[\s\S]*return \(\) => \{[\s\S]*trigger\?\.focus\(\)/,
    'closing the dialog restores focus to its launch control',
  )
  assert.match(imageReviewDialogSource, /getBoundingClientRect\(\)/, 'rectangle selection uses rendered image bounds at every zoom level')
  assert.match(imageReviewDialogSource, /setReloadKey\(value => value \+ 1\)/, 'failed image preview retry remounts the image element')
  assert.match(reviewSource, /uploadLocalArtifactFile[\s\S]*registerProjectMaterial[\s\S]*previewStepRevision/, 'replacement uploads, registers, then previews impact')
  assert.match(reviewSource, /mode: 'replace'/, 'replacement submission uses the typed revision branch')
  assert.match(reviewSource, /确认修改/, 'typed replacement remains behind the existing explicit impact confirmation')
  assert.match(reviewSource, /alt=\{`\$\{artifact\?\.reviewLabel \|\| creatorStepLabel\(step\.id\)\}预览`\}/, 'full-screen image review uses a semantic creator-facing alt')
  assert.match(projectBriefSource, /创作目标/)
  assert.match(projectBriefSource, /目标时长/)
  assert.match(regenerationDialogSource, /previewStepRegeneration/)
  assert.match(regenerationDialogSource, /confirmedAffectedStepIds/)
  assert.match(regenerationDialogSource, /旧版本仍然保留/)
  assert.match(regenerationDialogSource, /triggerRef[\s\S]*trigger\?\.focus\(\)/, 'step regeneration dialog restores focus to its launch control')
  assert.match(agentReviewSource, /approvalBlocked && <p className="creator-form-error" role="alert">/, 'blocking creator review errors use an alert live region')
  assert.match(creatorStylesSource, /\.creator-root :focus-visible\s*\{[^}]*outline:/, 'creator keyboard focus is visibly distinct')
  assert.match(creatorStylesSource, /@media \(max-width: 390px\)[\s\S]*\.creator-workspace[^}]*min-width:\s*0/, '390 px creator workspace constrains intrinsic widths')
  assert.match(creatorStylesSource, /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.creator-root \*[\s\S]*animation-duration:/, 'creator surfaces respect reduced-motion preferences')
  assert.match(queueSource, /height: 480/)
  assert.match(queueSource, /SHOT_QUEUE_ROW_HEIGHT/)
  assert.match(queueSource, /Math\.min\(3, unresolved\.length\)/, 'visible thumbnails must use bounded concurrency')
  assert.match(queueSource, /getCreatorArtifactContent/, 'queue thumbnails use real artifact metadata')
  assert.match(queueSource, /scrollRef\.current\.scrollTop = 0/, 'filter changes reset the virtual scroll origin')
  assert.doesNotMatch(queueSource, /<video\b/, 'the queue never mounts a video player')
  assert.equal((inspectorSource.match(/<video\b/g) || []).length, 1, 'only the selected candidate mounts one video player')
  assert.match(inspectorSource, /getCreatorArtifactContent/)
  assert.match(inspectorSource, /candidateId/)
  assert.match(inspectorSource, /baseVersion: shot\.version/)
  assert.match(inspectorSource, /SHOT_QUEUE_CONFLICT_COPY/)
  assert.match(inspectorSource, /canSubmitShotDuration/)
  assert.match(inspectorSource, /artifactId of candidateRefs/, 'media source falls back through candidate artifacts')
  assert.match(improveSource, /isTargetOnlyShotImpact/)
  assert.match(improveSource, /只会新增 Shot \{sequence\} 的候选，不影响其他 \{Math\.max\(0, totalShots - 1\)\} 个 Shot/)
  assert.match(improveSource, /crypto\.randomUUID/)
  assert.match(improveSource, /aria-modal="true"/)
  assert.match(improveSource, /重试这个 Shot/)
  assert.match(improveSource, /onRegenerationStarted\(result\)/, 'a queued task is handed to the workspace immediately')
  assert.match(workspaceSource, /ShotReviewQueue/)
  assert.match(workspaceSource, /shotWorkspace\?\.shot\.id !== activeShotId/)
  assert.match(workspaceSource, /selectedShotAfterReplacement/)
  assert.match(workspaceSource, /adoptCreatorShotTask/)
  assert.match(workspaceSource, /activeTaskSignature/)
  assert.match(workspaceSource, /viewRequestTokenRef\.current \+= 1[\s\S]*adoptCreatorShotTask/, 'adopting a newer Shot task invalidates an older in-flight creation view before it can overwrite the task set')
  assert.match(workspaceSource, /SHOT_QUEUE_CONFLICT_COPY/)
  assert.match(workspaceSource, /PreviewDeliveryPanel/)
  assert.doesNotMatch(previewSource, /getCreatorArtifactContent/, 'preview must not overwrite the artifact selected by the workspace')
  assert.match(previewSource, /ArtifactProofingCanvas/, 'preview must render JSON, Markdown, image, audio, and other selected evidence')
  assert.match(previewSource, /rebuildFinalAssembly/, 'assembly retry is a dedicated action')
  assert.doesNotMatch(previewSource, /regenerateShot\(/, 'assembly retry must never regenerate a Shot')
  assert.match(previewSource, /assemblyDirty/)
  assert.match(previewSource, /completedRepairScope/, 'delivery recovery is limited to completed delivery output')
  assert.match(previewSource, /previewStepRegeneration/, 'delivery recovery previews its exact impact before queueing')
  assert.match(previewSource, /regenerateStep/, 'delivery recovery uses the existing step regeneration API')
  assert.equal(
    (previewSource.match(/>\{working \? '正在核对成片…' : '重新生成成片'\}<\/button>/g) || []).length,
    1,
    'missing or unsupported delivery exposes one recovery action',
  )
  assert.match(previewSource, /baseArtifactId: currentContent\.artifact\.id/, 'delivery recovery uses the selected current delivery artifact as its base')
  assert.match(previewSource, /baseVersion: currentContent\.artifact\.version/, 'delivery recovery uses the selected current delivery artifact version as its base')
  assert.match(previewSource, /confirmedAffectedStepIds: impact\.affectedStepIds/, 'delivery recovery confirms the server-projected impact')

  assert.match(previewSource, /deliveryArtifactPassesFinalReview/)
  assert.match(previewSource, /result\.status === 'queued'/)
  assert.match(previewSource, /result\.status === 'dispatching'/)
  assert.match(previewSource, /crypto\.randomUUID/)
  assert.match(previewSource, /assemblyKeyRef\.current = null/)
  assert.match(previewSource, /'response' in caught[\s\S]*assemblyKeyRef\.current = null/, 'a definite server failure starts a new assembly attempt')
  assert.match(previewSource, /network failure keeps the UUID|response-less network failure keeps the UUID/, 'an ambiguous network failure must retain the assembly attempt key')
  assert.match(previewSource, /成片检查通过后，才会显示最终视频和交付文件/)
  assert.match(previewSource, /SimpleVideoPlayer/)
  assert.doesNotMatch(previewSource, /<video\b/, 'preview delegates video playback to the shared player')
  assert.match(
    previewSource,
    /step\.state === 'confirmed' && finalVideoReady/,
    'a confirmed final badge waits for final review and loaded browser metadata',
  )
  assert.match(previewSource, /canDeliverCreatorFinalVideo/, 'diagnostic probing never bypasses final delivery gating')
  assert.match(
    previewSource,
    /<SimpleVideoPlayer[\s\S]*allowDownload=\{finalVideoReady\}/,
    'a diagnostic delivery video keeps playback but cannot expose a download before final QA passes',
  )
  assert.match(
    previewSource,
    /const checklistConfirmed = isDelivery \? finalVideoReady : step\.state === 'confirmed'/,
    'preview steps keep their confirmed checklist state while delivery remains final-QA and playable-media gated',
  )
  assert.equal(
    (previewSource.match(/\{checklistConfirmed \? '✓' : '○'\}/g) || []).length,
    2,
    'both checklist rows use the preview-or-delivery-specific confirmation state',
  )
  assert.match(
    previewSource,
    /const success = completedDeliveryRepairSuccess\(result\.view\)\s*onViewChanged\(result\.view\)[\s\S]*if \(success\.refreshAfterMutation\) void onAssemblyUpdated\(\)\.catch\(\(\) => undefined\)/,
    'a successful regeneration adopts its returned view before launching the best-effort refresh',
  )
  assert.equal((playerSource.match(/<video\b/g) || []).length, 1, 'the shared player mounts exactly one media element')
  assert.match(
    playerSource,
    /allowDownload && mediaState === 'unsupported'[\s\S]*download=\{downloadName \|\| true\}/,
    'unsupported diagnostic video cannot expose a download action unless final delivery explicitly enables it',
  )
  assert.match(
    playerSource,
    /\{allowDownload && <a className="simple-video-download"/,
    'normal video controls cannot expose a download action unless final delivery explicitly enables it',
  )
  assert.match(playerSource, /interface MediaPlaybackState/)
  assert.match(playerSource, /onPlaybackStateChange\?:/)
  assert.match(playerSource, /fetch\(src,\s*\{\s*method:\s*'HEAD'/, 'local video delivery is probed before browser decoding')
  assert.match(playerSource, /creatorMediaStateAfterHttpProbe/, 'HTTP failures map independently from browser codec failures')
  assert.match(playerSource, /creatorMediaStateAfterLoadedMetadata\(\)/, 'loaded metadata confirms playable media')
  assert.match(playerSource, /event\.currentTarget\.error\?\.code/, 'browser media failures use the MediaError code')
  assert.match(playerSource, /creatorMediaStateAfterMediaError\(mediaErrorCode, response\.status, true\)/, 'local media errors are reclassified after a fresh delivery probe')
  assert.match(playerSource, /creatorMediaStateAfterMediaError\(mediaErrorCode, undefined, false\)/, 'direct sources never enter the local service failure path')
  assert.match(playerSource, /if \(!localMediaSource\) return/, 'remote, signed, blob, and data sources skip the local HEAD probe')
  assert.match(playerSource, /canApplyCreatorMediaProbe/, 'stale async probes cannot update a newer source')
  assert.match(playerSource, /key=\{src\}/, 'each source gets a distinct media element')
  assert.match(playerSource, /event\.currentTarget\.currentSrc/, 'media events verify the browser-resolved source')
  assert.ok((playerSource.match(/canApplyCreatorMediaElementEvent\(/g) || []).length >= 2, 'loadedmetadata and error events both reject stale source identities')
  assert.match(
    playerSource,
    /mediaState === 'unavailable'\s*\? '视频暂时无法读取，请稍后重试'/,
    'the neutral direct-delivery state renders neutral copy rather than local-service or codec copy',
  )
  for (const copy of [
    '成片文件缺失，可从成片步骤重新生成',
    '本地媒体服务未启动',
    '当前编码不受客户端支持，需要转为 H.264/AAC MP4',
    '视频暂时无法读取，请稍后重试',
  ]) {
    assert.match(playerSource, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')), `video player exposes recovery copy: ${copy}`)
  }
  assert.match(
    proofingSource,
    /onMediaStateChange=\{handleVideoMediaStateChange\}/,
    'proofing invalidates time selections while preserving the explicit video delivery failure',
  )
  assert.match(playerSource, /secondsToIntegerMilliseconds\(currentTime\)/)
  assert.match(playerSource, /\.play\(\)/)
  assert.match(playerSource, /\.pause\(\)/)
  assert.match(playerSource, /currentTime/)
  assert.match(playerSource, /\.volume/)
  assert.match(playerSource, /playbackRate/)
  assert.match(playerSource, /requestFullscreen/)
  assert.match(playerSource, /\bdownload\b/)
  assert.match(playerSource, /event\.code === 'Space'/)
  assert.match(playerSource, /event\.key === 'ArrowLeft'/)
  assert.match(playerSource, /event\.key === 'ArrowRight'/)
  assert.match(playerSource, /event\.key\.toLowerCase\(\) === 'm'/)
  assert.match(playerSource, /event\.key\.toLowerCase\(\) === 'f'/)
  assert.match(playerSource, /aria-label=\{isPlaying \? '暂停' : '播放'\}/, 'the main control announces its next action')
  assert.match(playerSource, /aria-label=\{isMuted \? '取消静音' : '静音'\}/, 'the audio control announces its next action')
  for (const label of ['进度', '音量', '播放速度', '全屏', '下载视频']) {
    assert.match(playerSource, new RegExp(`aria-label="${label}"`), `player exposes the ${label} control`)
  }
  assert.equal((audioPlayerSource.match(/<audio\b/g) || []).length, 1, 'the audio player mounts exactly one media element')
  assert.match(audioPlayerSource, /\.play\(\)/)
  assert.match(audioPlayerSource, /\.pause\(\)/)
  assert.match(audioPlayerSource, /currentTime/)
  assert.match(audioPlayerSource, /\.volume/)
  assert.match(audioPlayerSource, /playbackRate/)
  assert.match(audioPlayerSource, /\bdownload\b/)
  assert.match(audioPlayerSource, /key=\{`\$\{src\}:\$\{retries\}`\}/, 'audio recovery remounts the failed media element')
  assert.match(audioPlayerSource, /const failure = audioPlaybackFailure\(retries\)[\s\S]*onError\?\.\(\)/, 'every retryable and terminal audio error notifies proofing immediately')
  assert.match(audioPlayerSource, /resetMediaTransportState\(\)/, 'retry resets every retained transport field')
  assert.match(audioPlayerSource, /onReady\?\.\(\)/, 'range controls return only after loaded media reports ready')
  assert.match(audioPlayerSource, /simple-audio-time/, 'audio time has a narrow-layout-specific class')
  assert.match(audioPlayerSource, /simple-audio-volume/, 'audio volume has a narrow-layout-specific class')
  for (const rate of ['0.75×', '1×', '1.25×', '1.5×', '2×']) {
    assert.match(audioPlayerSource, new RegExp(rate.replace('×', '\\×')), `audio exposes the ${rate} rate`)
  }
  for (const label of ['进度', '音量', '播放速度', '下载音频']) {
    assert.match(audioPlayerSource, new RegExp(`aria-label="${label}"`), `audio exposes the ${label} control`)
  }
  const compactMediaStyles = creatorStylesSource.match(/@media \(max-width: 700px\) \{([\s\S]*?)\n\}/)?.[1] ?? ''
  assert.match(compactMediaStyles, /\.simple-audio-time\s*\{[\s\S]*?display:\s*inline/, '390px audio keeps elapsed and total time visible')
  assert.match(compactMediaStyles, /\.simple-audio-volume\s*\{[\s\S]*?display:\s*block/, '390px audio keeps the labelled volume slider visible')
  assert.doesNotMatch(compactMediaStyles, /\.simple-video-time,\s*\.simple-video-volume/, 'responsive video hiding no longer removes required audio controls')

  console.log('creator studio logic and client contract checks passed')
} finally {
  await rm(temp, { recursive: true, force: true })
}
