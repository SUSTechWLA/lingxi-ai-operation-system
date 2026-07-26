import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const tempDir = await mkdtemp(join(tmpdir(), 'director-studio-logic-'))
const outfile = join(tempDir, 'directorStudioLogic.mjs')
const apiResponseOutfile = join(tempDir, 'apiResponse.mjs')
const layerSelectorsOutfile = join(tempDir, 'talkingHeadLayerSelectors.mjs')
const creatorRoutesOutfile = join(tempDir, 'creatorRoutes.mjs')
const focusCycleOutfile = join(tempDir, 'focusCycle.mjs')
const developerDiagnosticsOutfile = join(tempDir, 'developerDiagnostics.mjs')

try {
  await build({
    entryPoints: [new URL('../src/pages/directorStudioLogic.ts', import.meta.url).pathname],
    outfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  await build({
    entryPoints: [new URL('../src/features/director-studio/talking-head/selectors.ts', import.meta.url).pathname],
    outfile: layerSelectorsOutfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  await build({
    entryPoints: [new URL('../src/utils/apiResponse.ts', import.meta.url).pathname],
    outfile: apiResponseOutfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  await build({
    entryPoints: [new URL('../src/creatorRoutes.ts', import.meta.url).pathname],
    outfile: creatorRoutesOutfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  await build({
    entryPoints: [new URL('../src/features/creator-studio/focusCycle.ts', import.meta.url).pathname],
    outfile: focusCycleOutfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })
  await build({
    entryPoints: [new URL('../src/features/developer-console/developerDiagnostics.ts', import.meta.url).pathname],
    outfile: developerDiagnosticsOutfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })

  const {
    applyOptimisticRunningStage,
    buildDirectorArtifacts,
    buildPublishCopies,
    buildShotReviewGroups,
    buildDirectorStages,
    canStartProject,
    formatDirectorErrorMessage,
    findFinalVideoArtifact,
    findPublishCopyArtifact,
    getArtifactViewerSelection,
    isProjectSessionStarted,
    localArtifactIdFromStorageRef,
    localServiceStatusDisplay,
    normalizeDirectorErrorMessage,
    nextStageIdAfterReview,
    nextSelectedReviewId,
    overviewProjectStatus,
    projectPrimaryAction,
    publishCopiesToJSON,
    publishCopiesToMarkdown,
    preferredActiveReview,
    qualityGateTargetLabel,
    qualityGateTargetLines,
    reviewDisplayTitle,
    reviewOutputPanelHint,
    reviewOutputPanelTitle,
    reviewQualityReportLines,
    reviewStatusLabel,
    reviewOutputText,
    buildDirectorTraceNodes,
    buildEnvironmentChecklist,
    buildExportDeliveryItems,
    buildExternalGenerationTaskPackage,
    buildProjectAssemblySummary,
    creationProfileSummary,
    deriveNextAction,
    displayNameForArtifact,
    externalGenerationGuideSteps,
    externalGenerationReferenceCopyText,
    externalGenerationReferencesFromArtifacts,
    buildImageRegenerationInstruction,
    videoCreationProfileForId,
    videoCreationProfiles,
    normalizeVideoCreationProfileId,
    mergeExternalGenerationTaskReferences,
    timeWindowPlanSummary,
    traceNodeHasError,
    unresolvedMaterialDependencyCount,
    visibleReviewHistory,
    isActionablePendingReview,
    visibleEnvironmentIssues,
  } = await import(pathToFileURL(outfile))
  const { unwrapApiData } = await import(pathToFileURL(apiResponseOutfile))
  const { buildTalkingHeadLayerDisplays } = await import(pathToFileURL(layerSelectorsOutfile))
  const { parseAppRoute, replaceHashRoute } = await import(pathToFileURL(creatorRoutesOutfile))
  const { cycleFocusIndex } = await import(pathToFileURL(focusCycleOutfile))
  const {
    buildDiagnosticsNodes,
    classifyDiagnosticTransport,
    diagnosticDurationMs,
    isTerminalAgentRunStatus,
    loadProjectDiagnostics,
    redactDiagnosticValue,
    selectDiagnosticsProject,
    serializeRedactedDiagnosticValue,
  } = await import(pathToFileURL(developerDiagnosticsOutfile))
  const diagnosticsApiSource = await readFile(new URL('../src/services/api.ts', import.meta.url), 'utf8')
  const directorPageSource = await readFile(new URL('../src/pages/DirectorStudioPage.tsx', import.meta.url), 'utf8')
  const appSource = await readFile(new URL('../src/App.tsx', import.meta.url), 'utf8')
  const creatorShellSource = await readFile(new URL('../src/features/creator-studio/CreatorShell.tsx', import.meta.url), 'utf8')
  const developerConsoleSource = await readFile(new URL('../src/features/developer-console/DeveloperConsolePage.tsx', import.meta.url), 'utf8')
  const developerDiagnosticsSource = await readFile(new URL('../src/features/developer-console/developerDiagnostics.ts', import.meta.url), 'utf8')
  const diagnosticsSummarySource = await readFile(new URL('../src/features/developer-console/components/DiagnosticsSummary.tsx', import.meta.url), 'utf8')
  const diagnosticsTimelineSource = await readFile(new URL('../src/features/developer-console/components/DiagnosticsTimeline.tsx', import.meta.url), 'utf8')
  const globalStylesSource = await readFile(new URL('../src/index.css', import.meta.url), 'utf8')
  assert.match(creatorShellSource, /开始创作/)
  assert.match(creatorShellSource, /我的视频/)
  for (const forbiddenCreatorTerm of ['追踪', '角色', '原始产物', 'Provider', 'Run']) {
    assert.doesNotMatch(creatorShellSource, new RegExp(forbiddenCreatorTerm))
  }
  assert.doesNotMatch(creatorShellSource, /developer-console|DeveloperConsolePage|DirectorStudioPage/)
  assert.match(appSource, /hashchange/)
  assert.match(appSource, /removeEventListener\('hashchange', syncRoute\)/)
  assert.match(appSource, /history\.replaceState\(null, '', nextHash\)/)
  assert.match(appSource, /VITE_ENABLE_DEVELOPER_CONSOLE === '1'/)
  assert.match(appSource, /developerConsoleEnabled \? lazy\(\(\) => import\('\.\/features\/developer-console\/DeveloperConsolePage'\)\) : null/)
  assert.match(appSource, /route\.kind === 'developer' && DeveloperConsolePage/)
  assert.match(appSource, /logout\(\)/)
  assert.match(appSource, /setAuthUser\(null\)/)
  assert.match(appSource, /onAuthenticated=\{\(user\) => \{\s+setAuthUser\(user\)\s+syncRoute\(\)/)
  assert.match(creatorShellSource, /aria-expanded=\{profileOpen\}/)
  assert.match(creatorShellSource, /aria-controls="creator-profile-menu"/)
  assert.match(creatorShellSource, /aria-haspopup="menu"/)
  assert.match(creatorShellSource, /role="menu"/)
  assert.match(creatorShellSource, /role="menuitem"/)
  assert.match(creatorShellSource, /event\.key === 'Escape'/)
  assert.match(creatorShellSource, /onPointerDown/)
  assert.match(creatorShellSource, /aria-modal="true"/)
  assert.match(creatorShellSource, /handleDialogKeyDown/)
  assert.match(creatorShellSource, />设置</)
  assert.match(creatorShellSource, /onNavigate\('#\/settings'\)/)
  assert.doesNotMatch(creatorShellSource.match(/<nav[\s\S]*?<\/nav>/)?.[0] || '', />设置</)

  assert.deepEqual(parseAppRoute('', false), { kind: 'creator', page: 'create' })
  assert.deepEqual(parseAppRoute('#/settings', false), { kind: 'creator', page: 'settings' })
  assert.deepEqual(parseAppRoute('#', false), { kind: 'creator', page: 'create', shouldReplace: true })
  assert.deepEqual(parseAppRoute('#/unknown', false), { kind: 'creator', page: 'create', shouldReplace: true })
  assert.deepEqual(parseAppRoute('#/videos/project%20one/steps/script', false), {
    kind: 'creator', page: 'step', projectId: 'project one', stepId: 'script',
  })
  assert.deepEqual(parseAppRoute('#/videos/project/steps/not-a-step', false), { kind: 'creator', page: 'create', shouldReplace: true })
  assert.deepEqual(parseAppRoute('#/videos/project/steps/script/extra', false), { kind: 'creator', page: 'create', shouldReplace: true })
  assert.deepEqual(parseAppRoute('#/videos/%E0%A4%A/steps/script', false), { kind: 'creator', page: 'create', shouldReplace: true })
  const diagnosticsViews = ['summary', 'timeline', 'tools', 'artifacts', 'gates', 'recovery']
  for (const developerRoute of ['#/developer', ...diagnosticsViews.map((view) => `#/developer/${view}`), '#/developer/not-a-view']) {
    assert.deepEqual(parseAppRoute(developerRoute, false), { kind: 'creator', page: 'create', shouldReplace: true })
  }
  assert.deepEqual(parseAppRoute('#/developer', true), { kind: 'developer', view: 'summary' })
  for (const view of diagnosticsViews) {
    assert.deepEqual(parseAppRoute(`#/developer/${view}`, true), { kind: 'developer', view })
  }
  assert.deepEqual(parseAppRoute('#/developer/not-a-view', true), { kind: 'creator', page: 'create', shouldReplace: true })
  assert.doesNotMatch(developerConsoleSource, /DirectorStudioPage/)
  assert.doesNotMatch(developerConsoleSource, /DirectorNavKey/)
  assert.match(developerConsoleSource, /DeveloperDiagnosticsView/)
  for (const apiName of [
    'fetchVideoProjects',
    'getAgentRun',
    'getAgentRunTrace',
    'getAgentRunReviews',
    'fetchProjectArtifacts',
    'getTaskDetails',
    'getTaskContext',
  ]) {
    assert.match(developerConsoleSource, new RegExp(`\\b${apiName}\\b`))
  }
  assert.match(diagnosticsApiSource, /export (?:const|async function) getTaskDetails/)
  assert.match(diagnosticsApiSource, /`\/task\/\$\{encodeURIComponent\(taskId\)\}`/)
  assert.match(diagnosticsApiSource, /export (?:const|async function) getTaskContext/)
  assert.match(diagnosticsApiSource, /`\/task\/\$\{encodeURIComponent\(taskId\)\}\/context`/)
  assert.match(developerConsoleSource, /new AbortController\(\)/)
  assert.match(developerConsoleSource, /controller\.abort\(\)/)
  assert.match(developerConsoleSource, /window\.setInterval/)
  assert.match(developerConsoleSource, /window\.clearInterval/)
  assert.match(developerDiagnosticsSource, /Promise\.allSettled/)
  assert.match(developerConsoleSource, /DiagnosticsSummary/)
  assert.match(developerConsoleSource, /DiagnosticsTimeline/)
  assert.match(developerConsoleSource, /currentView === 'summary'/)
  assert.match(developerConsoleSource, /currentView === 'timeline'/)
  for (const summaryLabel of ['项目', '运行 ID', '任务 ID', '运行状态', '创建时间', '更新时间', '耗时', '节点状态', '待处理门禁', '失败节点', '产物']) {
    assert.match(diagnosticsSummarySource, new RegExp(summaryLabel))
  }
  assert.match(diagnosticsSummarySource, /<select/)
  assert.match(diagnosticsSummarySource, /navigator\.clipboard\.writeText/)
  assert.match(diagnosticsTimelineSource, /<ol/)
  assert.match(diagnosticsTimelineSource, /<details/)
  assert.match(diagnosticsTimelineSource, /type="search"/)
  assert.match(diagnosticsTimelineSource, /serializeRedactedDiagnosticValue/)
  assert.doesNotMatch(diagnosticsTimelineSource, /<details[^>]*\sopen(?:=|\s|>)/)
  assert.match(globalStylesSource, /\.diagnostics-metric-grid/)
  assert.match(globalStylesSource, /\.diagnostics-timeline/)
  assert.match(globalStylesSource, /\.diagnostics-json-panel/)
  assert.match(globalStylesSource, /@media\s*\(max-width:\s*390px\)/)
  assert.equal(replaceHashRoute('#/create'), '#/create')
  assert.equal(cycleFocusIndex(0, 2, false), 1)
  assert.equal(cycleFocusIndex(1, 2, false), 0)
  assert.equal(cycleFocusIndex(0, 2, true), 1)
  assert.equal(cycleFocusIndex(1, 2, true), 0)
  assert.equal(cycleFocusIndex(0, 0, false), -1)

  assert.deepEqual(
    redactDiagnosticValue({
      Authorization: 'Bearer token',
      nested: {
        COOKIE: 'session=abc',
        apiKey: 'key',
        api_key: 'key-2',
        accessToken: 'access',
        refreshToken: 'refresh',
        secret: 'secret',
        password: 'password',
        credential: 'credential',
        signedUrl: 'https://example.test/private',
        signature: 'signature',
        list: [
          { 'Set-Cookie': 'session=def' },
          '/Users/alice/project/file.png',
          String.raw`C:\Users\alice\project\file.png`,
          'local://projects/vp-1/output.mp4',
          'project-17',
          'assets/output.mp4',
        ],
      },
    }),
    {
      Authorization: '[REDACTED]',
      nested: {
        COOKIE: '[REDACTED]',
        apiKey: '[REDACTED]',
        api_key: '[REDACTED]',
        accessToken: '[REDACTED]',
        refreshToken: '[REDACTED]',
        secret: '[REDACTED]',
        password: '[REDACTED]',
        credential: '[REDACTED]',
        signedUrl: '[REDACTED]',
        signature: '[REDACTED]',
        list: [
          { 'Set-Cookie': '[REDACTED]' },
          '<local-path>/file.png',
          String.raw`<local-path>\file.png`,
          'local://projects/vp-1/output.mp4',
          'project-17',
          'assets/output.mp4',
        ],
      },
    },
  )
  const cyclicDiagnostic = { password: 'hidden' }
  cyclicDiagnostic.self = cyclicDiagnostic
  assert.doesNotThrow(() => redactDiagnosticValue(cyclicDiagnostic))
  assert.doesNotThrow(() => redactDiagnosticValue(new Proxy({}, {
    ownKeys() {
      throw new Error('malformed diagnostic value')
    },
  })))
  const revokedDiagnostic = Proxy.revocable({}, {})
  revokedDiagnostic.revoke()
  assert.doesNotThrow(() => redactDiagnosticValue(revokedDiagnostic.proxy))
  assert.doesNotThrow(() => classifyDiagnosticTransport(revokedDiagnostic.proxy))
  assert.deepEqual(buildDiagnosticsNodes(revokedDiagnostic.proxy), [])

  let diagnosticCallbackCalls = 0
  const inheritedToJSON = Object.create({
    toJSON() {
      diagnosticCallbackCalls += 1
      return { leaked: 'inherited-secret' }
    },
  })
  inheritedToJSON.visible = 7n
  const accessorValue = {}
  Object.defineProperty(accessorValue, 'value', {
    enumerable: true,
    get() {
      diagnosticCallbackCalls += 1
      return 'accessor-secret'
    },
  })
  const serializationCycle = {}
  serializationCycle.self = serializationCycle
  const serializationInput = {
    password: 'top-level-secret',
    ownToJSON: {
      visible: 3n,
      toJSON() {
        diagnosticCallbackCalls += 1
        return { leaked: 'own-secret' }
      },
    },
    inheritedToJSON,
    callback() {
      diagnosticCallbackCalls += 1
    },
    values: [2n, undefined, Symbol('private-symbol'), accessorValue, serializationCycle],
  }
  const serializationSafeDiagnostic = redactDiagnosticValue(serializationInput)
  assert.equal(diagnosticCallbackCalls, 0)
  assert.doesNotThrow(() => JSON.stringify(serializationSafeDiagnostic))
  assert.deepEqual(JSON.parse(JSON.stringify(serializationSafeDiagnostic)), {
    password: '[REDACTED]',
    ownToJSON: { visible: '3', toJSON: '[Function]' },
    inheritedToJSON: { visible: '7' },
    callback: '[Function]',
    values: ['2', '[Undefined]', '[Symbol]', { value: '[Accessor]' }, { self: '[Circular]' }],
  })
  assert.equal(
    serializeRedactedDiagnosticValue(serializationInput),
    '{"password":"[REDACTED]","ownToJSON":{"visible":"3","toJSON":"[Function]"},"inheritedToJSON":{"visible":"7"},"callback":"[Function]","values":["2","[Undefined]","[Symbol]",{"value":"[Accessor]"},{"self":"[Circular]"}]}',
  )
  assert.equal(serializeRedactedDiagnosticValue(() => 'callable-secret'), '"[Function]"')
  assert.equal(diagnosticCallbackCalls, 0)

  const embeddedSensitiveNodes = buildDiagnosticsNodes({
    nodes: [{
      id: 'embedded-sensitive',
      status: 'FAILED',
      error: String.raw`Request failed: Authorization: Bearer super-secret at /Users/alice/.config/key.json; UNC \\server\share\secret.txt and shallow /etc; keep https://docs.example.test/help?next=/guides/start#target=/reference/api; retry remains available`,
      request: {
        message: String.raw`Upload failed with Cookie: session=abc123; theme=dark from C:\Users\alice\AppData\Local\agent\session.json; keep this context`,
        context: 'Worker rejected credential=client-credential token: oauth-token secret=hidden-secret; useful tail remains',
        pathBoundaries: String.raw`Inspect \\server\share\secret.txt and /etc, but keep https://docs.example.test/help?next=/guides/start&mode=plain#target=/reference/api`,
        download: 'https://cdn.example.test/render.mp4?X-Amz-Credential=AKIA-EXAMPLE&X-Amz-Signature=deadbeef&variant=preview',
        publicUrl: 'https://cdn.example.test/assets/video.mp4?variant=preview',
      },
    }],
  })
  const embeddedSensitiveError = embeddedSensitiveNodes[0]?.error ?? ''
  assert.match(embeddedSensitiveError, /Request failed:/)
  assert.match(embeddedSensitiveError, /retry remains available/)
  assert.doesNotMatch(embeddedSensitiveError, /super-secret|\/Users\/alice|shallow \/etc/)
  assert.equal(embeddedSensitiveError.includes(String.raw`\\server\share`), false)
  assert.match(embeddedSensitiveError, /UNC <local-path>\\secret\.txt and shallow <local-path>\/etc/)
  assert.match(embeddedSensitiveError, /https:\/\/docs\.example\.test\/help\?next=\/guides\/start#target=\/reference\/api/)
  const embeddedSensitiveJson = serializeRedactedDiagnosticValue(embeddedSensitiveNodes[0]?.request)
  const embeddedSensitivePayload = JSON.parse(embeddedSensitiveJson)
  const embeddedSensitiveText = Object.values(embeddedSensitivePayload).join('\n')
  for (const sensitiveValue of ['session=abc123', 'theme=dark', String.raw`C:\Users\alice`, String.raw`\\server\share`, 'client-credential', 'oauth-token', 'hidden-secret', 'AKIA-EXAMPLE', 'deadbeef']) {
    assert.equal(embeddedSensitiveText.includes(sensitiveValue), false)
  }
  assert.match(embeddedSensitiveJson, /Upload failed with/)
  assert.match(embeddedSensitiveJson, /keep this context/)
  assert.match(embeddedSensitiveJson, /Worker rejected/)
  assert.match(embeddedSensitiveJson, /useful tail remains/)
  assert.match(embeddedSensitiveJson, /https:\/\/cdn\.example\.test\/render\.mp4/)
  assert.match(embeddedSensitiveJson, /variant=preview/)
  assert.match(embeddedSensitiveJson, /https:\/\/cdn\.example\.test\/assets\/video\.mp4\?variant=preview/)
  assert.equal(
    embeddedSensitivePayload.pathBoundaries.includes('https://docs.example.test/help?next=/guides/start&mode=plain#target=/reference/api'),
    true,
  )
  assert.equal(embeddedSensitivePayload.pathBoundaries.includes('and /etc,'), false)

  assert.equal(diagnosticDurationMs('2026-01-01T00:00:00.000Z', '2026-01-01T00:00:01.250Z'), 1250)
  assert.equal(diagnosticDurationMs('invalid', '2026-01-01T00:00:01.250Z'), undefined)
  assert.equal(diagnosticDurationMs('2026-01-01T00:00:02.000Z', '2026-01-01T00:00:01.250Z'), undefined)
  assert.equal(diagnosticDurationMs(Symbol('malformed'), '2026-01-01T00:00:01.250Z'), undefined)
  assert.equal(classifyDiagnosticTransport({ transport: 'control' }), 'control')
  assert.equal(classifyDiagnosticTransport({ transport: 'control:cancel' }), 'control')
  assert.equal(classifyDiagnosticTransport({ transport: 'tool/run' }), 'tool')
  assert.equal(classifyDiagnosticTransport({ server: 'filesystem' }), 'mcp')
  assert.equal(classifyDiagnosticTransport({ provider: 'mcp-provider' }), 'mcp')
  assert.equal(classifyDiagnosticTransport({ provider: 'mcp://video-qa' }), 'mcp')
  assert.equal(classifyDiagnosticTransport({ toolName: 'mcp__video_qa__inspect' }), 'mcp')
  assert.equal(classifyDiagnosticTransport({ tool: 'render_video' }), 'tool')
  assert.equal(classifyDiagnosticTransport({ transport: 'not-mcp' }), 'unknown')
  assert.equal(classifyDiagnosticTransport({ provider: 'not-mcp' }), 'unknown')
  assert.equal(classifyDiagnosticTransport({ transport: 'uncontrolled' }), 'unknown')
  assert.equal(classifyDiagnosticTransport({ type: 'controller' }), 'unknown')
  assert.equal(classifyDiagnosticTransport({ transport: 'toolbox' }), 'unknown')
  assert.equal(classifyDiagnosticTransport(null), 'unknown')

  assert.deepEqual(
    buildDiagnosticsNodes({
      nodes: [
        {
          id: 'node-b',
          name: 'Second',
          type: 'tool',
          status: 'SUCCESS',
          createdAt: '2026-01-01T00:00:02.000Z',
          completedAt: 'invalid',
          retryCount: 2,
          maxRetry: 3,
          tool: 'render_video',
          input: { authorization: 'secret-token', source: '/Users/alice/source.mov' },
          output: { file: String.raw`C:\Users\alice\result.mp4` },
        },
        {
          id: 'node-a',
          name: 'First',
          status: 'RUNNING',
          startedAt: '2026-01-01T00:00:01.000Z',
          completedAt: '2026-01-01T00:00:01.500Z',
          transport: 'mcp',
          request: [{ password: 'hidden' }],
          response: { artifactId: 'local://projects/vp-1/output.mp4' },
        },
        {
          id: 'node-c',
          createdAt: '2026-01-01T00:00:02.000Z',
        },
      ],
    }),
    [
      {
        id: 'node-a',
        name: 'First',
        type: 'unknown',
        status: 'RUNNING',
        startedAt: '2026-01-01T00:00:01.000Z',
        completedAt: '2026-01-01T00:00:01.500Z',
        durationMs: 500,
        retryCount: 0,
        maxRetry: 0,
        transport: 'mcp',
        request: [{ password: '[REDACTED]' }],
        response: { artifactId: 'local://projects/vp-1/output.mp4' },
      },
      {
        id: 'node-b',
        name: 'Second',
        type: 'tool',
        status: 'SUCCESS',
        startedAt: '2026-01-01T00:00:02.000Z',
        completedAt: 'invalid',
        retryCount: 2,
        maxRetry: 3,
        toolName: 'render_video',
        transport: 'tool',
        request: { authorization: '[REDACTED]', source: '<local-path>/source.mov' },
        response: { file: String.raw`<local-path>\result.mp4` },
      },
      {
        id: 'node-c',
        name: 'node-c',
        type: 'unknown',
        status: 'unknown',
        startedAt: '2026-01-01T00:00:02.000Z',
        retryCount: 0,
        maxRetry: 0,
        transport: 'unknown',
      },
    ],
  )
  assert.deepEqual(buildDiagnosticsNodes({ nodes: 'not-an-array' }), [])
  assert.deepEqual(buildDiagnosticsNodes(null), [])

  const projectFixtures = [
    { id: 'newest-without-run', updatedAt: '2026-01-04T00:00:00.000Z' },
    { id: 'older-with-run', currentRunId: 'run-old', updatedAt: '2026-01-02T00:00:00.000Z' },
    { id: 'newest-with-run', currentRunId: 'run-new', updatedAt: '2026-01-03T00:00:00.000Z' },
  ]
  assert.equal(selectDiagnosticsProject(projectFixtures)?.id, 'newest-with-run')
  assert.deepEqual(projectFixtures.map((project) => project.id), [
    'newest-without-run', 'older-with-run', 'newest-with-run',
  ])
  assert.equal(selectDiagnosticsProject([
    { id: 'older', updatedAt: '2026-01-02T00:00:00.000Z' },
    { id: 'newer', updatedAt: '2026-01-03T00:00:00.000Z' },
  ])?.id, 'newer')
  assert.equal(selectDiagnosticsProject([]), undefined)
  assert.equal(isTerminalAgentRunStatus('SUCCESS'), true)
  assert.equal(isTerminalAgentRunStatus('FAILED'), true)
  assert.equal(isTerminalAgentRunStatus('CANCELLED'), true)
  assert.equal(isTerminalAgentRunStatus('RUNNING'), false)
  assert.equal(isTerminalAgentRunStatus('CREATED'), false)

  const scopedCalls = []
  const scopedController = new AbortController()
  const scopedResult = await loadProjectDiagnostics({
    projectId: 'project-selected',
    runId: 'run-selected',
    signal: scopedController.signal,
    api: {
      getRun: async (runId, signal) => {
        scopedCalls.push(['run', runId, signal])
        return { id: runId, taskId: 'task-selected', message: 'diagnose', status: 'RUNNING', createdAt: '2026-01-01T00:00:00.000Z', updatedAt: '2026-01-01T00:00:01.000Z' }
      },
      getTrace: async (runId, signal) => {
        scopedCalls.push(['trace', runId, signal])
        throw new Error('trace unavailable')
      },
      getReviews: async (runId, signal) => {
        scopedCalls.push(['reviews', runId, signal])
        return { runId, reviews: [{ id: 'review-1', nodeId: 'node-1', status: 'PENDING' }] }
      },
      getArtifacts: async (projectId, signal) => {
        scopedCalls.push(['artifacts', projectId, signal])
        return { artifacts: [{ id: 'artifact-1', projectId }] }
      },
      getTask: async (taskId, signal) => {
        scopedCalls.push(['task', taskId, signal])
        return { id: taskId }
      },
      getContext: async (taskId, signal) => {
        scopedCalls.push(['context', taskId, signal])
        return [{ taskId }]
      },
    },
  })
  assert.equal(scopedResult.projectId, 'project-selected')
  assert.equal(scopedResult.runId, 'run-selected')
  assert.equal(scopedResult.run?.id, 'run-selected')
  assert.deepEqual(scopedResult.task, { id: 'task-selected' })
  assert.deepEqual(scopedResult.context, [{ taskId: 'task-selected' }])
  assert.deepEqual(scopedResult.reviews.map((review) => review.id), ['review-1'])
  assert.deepEqual(scopedResult.artifacts.map((artifact) => artifact.id), ['artifact-1'])
  assert.deepEqual(scopedResult.errors, ['trace'])
  assert.equal(scopedCalls.find(([name]) => name === 'artifacts')?.[1], 'project-selected')
  assert.equal(scopedCalls.find(([name]) => name === 'task')?.[1], 'task-selected')
  assert.equal(scopedCalls.find(([name]) => name === 'context')?.[1], 'task-selected')
  assert.equal(scopedCalls.every(([, , signal]) => signal === scopedController.signal), true)

  assert.match(directorPageSource, /放大播放/)
  assert.doesNotMatch(directorPageSource, /图片预览已就绪|照片预览已就绪|视频预览已就绪|参考图已登记|产物已登记/)
  assert.doesNotMatch(directorPageSource, /QA \{openGroup\.production\.qaStatus\}|\{openGroup\.production\.sourceType\}/)
  assert.match(directorPageSource, /artifact\.status === 'valid'\s*\?\s*null\s*:\s*<StatusBadge status=\{artifact\.status\}/)
  assert.match(directorPageSource, /slot\.status === 'valid' && !slotStatusLabel\s*\?\s*null\s*:\s*<StatusBadge status=\{slot\.status\}/)
  const roleAgents = [
    {
      id: 'render_producer',
      name: 'Render Producer',
      displayName: '渲染制片',
      stage: 'render',
      goal: '',
      allowedTools: ['hyperframes_renderer'],
      forbiddenTools: [],
      requiredInputs: [],
      requiredOutputs: ['VIDEO'],
    },
  ]
  const reviews = [
    {
      id: 'review-render',
      nodeId: 'render_review',
      status: 'APPROVED',
      roleAgentId: 'render_producer',
      stage: 'render',
    },
  ]
  const trace = {
    nodes: [
      {
        id: 'render_exec',
        name: 'hyperframes_renderer',
        status: 'SUCCESS',
        input: { stage: 'render', roleAgentId: 'render_producer' },
        output: { success: true, summary: 'render complete but no artifact manifest' },
      },
    ],
  }

  const artifacts = buildDirectorArtifacts(roleAgents, reviews, trace)
  assert.equal(artifacts.length, 1)
  assert.equal(artifacts[0].kind, 'VIDEO')
  assert.equal(artifacts[0].storageRef, '')
  assert.notEqual(artifacts[0].status, 'valid')

  const stagedRoles = [
    { ...roleAgents[0], id: 'script_writer', stage: 'script', displayName: '脚本编剧', allowedTools: ['video_script_generator'], requiredOutputs: ['VIDEO_SCRIPT'] },
    { ...roleAgents[0], id: 'storyboard_artist', stage: 'storyboard', displayName: '卡片设计师', allowedTools: ['card_plan_generator'], requiredOutputs: ['CARD_PLAN'] },
  ]
  const stagedReviews = [{ id: 'review-script', nodeId: 'review-script-node', status: 'PENDING', roleAgentId: 'script_writer', stage: 'script', tool: 'video_script_generator' }]
  const startupRoles = [
    { ...roleAgents[0], id: 'creative_director', stage: 'proposal', displayName: '创意总监', allowedTools: ['proposal_generator'], requiredOutputs: ['VIDEO_PROPOSAL'] },
    stagedRoles[0],
  ]
  const notStartedFlow = buildDirectorStages(startupRoles, [], { nodes: [] })
  assert.deepEqual(notStartedFlow.map((stage) => stage.status), ['pending', 'pending'])

  const failedScriptStage = {
    id: 'script_writer',
    name: 'Script Writer',
    displayName: '脚本编剧',
    stage: 'script',
    goal: '',
    status: 'failed',
    progress: 0,
    allowedTools: ['video_script_generator'],
    forbiddenTools: [],
    requiredInputs: [],
    requiredOutputs: ['VIDEO_SCRIPT'],
    reviewFocus: [],
  }
  const runningStoryboardStage = {
    ...failedScriptStage,
    id: 'storyboard_artist',
    name: 'Storyboard Artist',
    displayName: '分镜设计',
    stage: 'storyboard',
    status: 'running',
    allowedTools: ['shot_generation_planner'],
    requiredOutputs: ['SHOT_LIST'],
  }
  const blockedNextAction = deriveNextAction([failedScriptStage, runningStoryboardStage])
  assert.equal(blockedNextAction.kind, 'blocked')
  assert.equal(blockedNextAction.stageId, 'script_writer')
  assert.equal(overviewProjectStatus([failedScriptStage, runningStoryboardStage], 'RUNNING', 'RUNNING'), 'failed')
  const justStartedFlow = buildDirectorStages(startupRoles, [], { nodes: [] }, true)
  assert.deepEqual(justStartedFlow.map((stage) => stage.status), ['active', 'pending'])
  assert.equal(canStartProject(true, false, false), true)
  assert.equal(canStartProject(true, false, true), false)
  assert.equal(isProjectSessionStarted(false, 'RUNNING', 'FAILED'), false)
  assert.equal(isProjectSessionStarted(false, 'RUNNING', 'SUCCESS'), false)
  assert.equal(isProjectSessionStarted(false, 'RUNNING', 'RUNNING'), true)
  assert.equal(overviewProjectStatus(justStartedFlow, 'RUNNING'), 'active')
  assert.equal(overviewProjectStatus(justStartedFlow, 'RUNNING', 'SUCCESS'), 'done')
  assert.equal(overviewProjectStatus(notStartedFlow), 'pending')
  const rawPreflight = {
    pipeline: 'wf-guided-image-text-video',
    status: 'passed',
    canStart: true,
    capabilityMenu: { localRunner: { available: true } },
  }
  assert.equal(unwrapApiData({ data: rawPreflight }), rawPreflight)
  assert.equal(unwrapApiData(rawPreflight), rawPreflight)
  assert.deepEqual(localServiceStatusDisplay('unknown', true), { label: '本地在线', tone: 'ok' })
  assert.deepEqual(localServiceStatusDisplay('unhealthy', false), { label: '本地离线', tone: 'error' })
  const cinematicImageInstruction = buildImageRegenerationInstruction({
    mode: 'aigc_shot',
    title: '范进参考图',
    sourceLabel: '角色参考图',
    userInstruction: '让人物更瘦弱，眼神更怯懦',
    locks: ['灰色破旧长袍', '瘦弱佝偻体态'],
  })
  assert.match(cinematicImageInstruction, /跨 shot 一致性/)
  assert.match(cinematicImageInstruction, /灰色破旧长袍/)
  assert.match(cinematicImageInstruction, /让人物更瘦弱/)
  assert.doesNotMatch(cinematicImageInstruction, /local:\/\//)
  const voiceImageInstruction = buildImageRegenerationInstruction({
    mode: 'voice_visual',
    title: '流程 b-roll 参考图',
    sourceLabel: 'AIGC 插入素材',
    userInstruction: '右侧留给字幕，画面更轻松',
  })
  assert.match(voiceImageInstruction, /服务口播/)
  assert.match(voiceImageInstruction, /HyperFrames/)
  assert.match(voiceImageInstruction, /右侧留给字幕/)
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'RUNNING',
    runStatus: 'RUNNING',
    stages: justStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'stop', label: '停止项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'RUNNING',
    runStatus: 'FAILED',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'RUNNING',
    runStatus: 'SUCCESS',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'DRAFT',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'DRAFT',
    runStatus: 'CANCELLED',
    stages: notStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })
  assert.deepEqual(projectPrimaryAction({
    preflightCanStart: true,
    loading: false,
    projectStatus: 'PAUSED',
    runStatus: 'CANCELLED',
    stages: justStartedFlow,
    topic: '佛得角世界杯奇迹',
  }), { kind: 'start', label: '开始项目', disabled: false })

  const preRenderReview = {
    id: 'render-review-before',
    nodeId: 'render_review_before',
    status: 'PENDING',
    stage: 'render',
    tool: 'hyperframes_renderer',
    reviewReason: '渲染视频耗时较长且会产生大文件，必须用户确认后执行。',
  }
  const preRenderFlow = buildDirectorStages(roleAgents, [preRenderReview], {
    nodes: [
      {
        id: 'render_review_before',
        name: '审核-render',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { stage: 'render', tool: 'hyperframes_renderer', reviewPhase: 'before_execute' },
        output: {},
      },
      {
        id: 'render_exec',
        name: 'external',
        type: 'TOOL',
        status: 'CREATED',
        input: { tool: 'external', capabilityTool: 'hyperframes_renderer', stage: 'render' },
        output: {},
      },
    ],
  }, true)
  assert.equal(isActionablePendingReview(preRenderReview), true)
  assert.equal(visibleReviewHistory([preRenderReview]).length, 1)
  assert.equal(nextSelectedReviewId('review-proposal', 'review-script', [
    { id: 'review-proposal', status: 'APPROVED' },
    { id: 'review-script', status: 'PENDING', reviewContent: 'script ready' },
  ], 'review-proposal'), 'review-script')
  assert.equal(nextSelectedReviewId('review-proposal', 'review-script', [
    { id: 'review-proposal', status: 'APPROVED' },
    { id: 'review-script', status: 'PENDING', reviewContent: 'script ready' },
  ], 'review-script'), 'review-proposal')
  assert.equal(nextSelectedReviewId('missing-review', undefined, [
    { id: 'review-proposal', status: 'APPROVED' },
  ]), 'review-proposal')
  assert.equal(preRenderFlow[0].status, 'review')
  assert.equal(preRenderFlow[0].reviewId, 'render-review-before')
  assert.equal(reviewDisplayTitle(preRenderReview), '审核最终渲染')

  const failedAfterPreRenderApprovalFlow = buildDirectorStages(roleAgents, [{
    ...preRenderReview,
    status: 'APPROVED',
  }], {
    nodes: [
      {
        id: 'render_review_before',
        name: '审核-render',
        type: 'REVIEW_GATE',
        status: 'SUCCESS',
        input: { stage: 'render', tool: 'hyperframes_renderer', reviewPhase: 'before_execute' },
        output: { approved: true },
      },
      {
        id: 'render_exec',
        name: 'external',
        type: 'TOOL',
        status: 'FAILED',
        input: { tool: 'external', capabilityTool: 'hyperframes_renderer', stage: 'render' },
        output: {},
        error: 'HyperFrames 渲染已禁用',
      },
    ],
  }, true)
  assert.equal(failedAfterPreRenderApprovalFlow[0].status, 'failed')

  const createdDagFlow = buildDirectorStages(startupRoles, [], {
    nodes: [
      {
        id: 'proposal_generator_exec',
        name: 'external',
        type: 'TOOL',
        status: 'CREATED',
        input: { tool: 'external', capabilityTool: 'proposal_generator' },
        output: {},
      },
      {
        id: 'video_script_generator_exec',
        name: 'external',
        type: 'TOOL',
        status: 'CREATED',
        input: { tool: 'external', capabilityTool: 'video_script_generator' },
        output: {},
      },
    ],
  }, true)
  assert.deepEqual(createdDagFlow.map((stage) => stage.status), ['active', 'pending'])
  const bridgedToolFlow = buildDirectorStages(startupRoles, [], {
    nodes: [
      {
        id: 'video_script_generator_exec',
        name: 'external',
        type: 'TOOL',
        status: 'RUNNING',
        input: { tool: 'external', capabilityTool: 'video_script_generator' },
        output: {},
      },
    ],
  }, true)
  assert.deepEqual(bridgedToolFlow.map((stage) => stage.status), ['pending', 'running'])

  const unactionableStartupReviews = startupRoles.map((role) => ({
    id: `${role.stage}_review`,
    nodeId: `${role.stage}_review`,
    status: 'PENDING',
    stage: role.stage,
    tool: role.allowedTools[0],
    reviewPhase: 'after_artifact',
  }))
  assert.equal(isActionablePendingReview(unactionableStartupReviews[0]), false)
  assert.deepEqual(visibleReviewHistory(unactionableStartupReviews), [])
  const generatingFlow = buildDirectorStages(startupRoles, unactionableStartupReviews, { nodes: [] }, true)
  assert.deepEqual(generatingFlow.map((stage) => stage.status), ['active', 'pending'])

  const technicalOutputReviews = startupRoles.map((role) => ({
    id: `${role.stage}_technical_review`,
    nodeId: `${role.stage}_technical_review`,
    status: 'PENDING',
    stage: role.stage,
    tool: role.allowedTools[0],
    reviewPhase: 'after_artifact',
    reviewOutput: {
      stage: role.stage,
      status: 'READY',
      stdout: `${role.stage} gate created`,
    },
  }))
  assert.equal(isActionablePendingReview(technicalOutputReviews[0]), false)
  assert.deepEqual(visibleReviewHistory(technicalOutputReviews), [])
  const technicalOutputFlow = buildDirectorStages(startupRoles, technicalOutputReviews, { nodes: [] }, true)
  assert.deepEqual(technicalOutputFlow.map((stage) => stage.status), ['active', 'pending'])

  const technicalGateTraceFlow = buildDirectorStages(startupRoles, [], {
    nodes: startupRoles.map((role) => ({
      id: `${role.stage}_review`,
      name: `审核-${role.allowedTools[0]}`,
      type: 'REVIEW_GATE',
      status: 'READY',
      input: {
        stage: role.stage,
        tool: role.allowedTools[0],
        reviewPhase: 'after_artifact',
      },
      output: {
        stage: role.stage,
        status: 'READY',
        stdout: `${role.stage} gate created`,
      },
    })),
  }, true)
  assert.deepEqual(technicalGateTraceFlow.map((stage) => stage.status), ['active', 'pending'])

  const actionableGateTraceFlow = buildDirectorStages(startupRoles, [], {
    nodes: [
      {
        id: 'proposal_review',
        name: '审核-proposal_generator',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: {
          stage: 'proposal',
          tool: 'proposal_generator',
          reviewPhase: 'after_artifact',
        },
        output: { content: '# 创作方向\n\n可以审核的方案正文' },
      },
    ],
  }, true)
  assert.deepEqual(actionableGateTraceFlow.map((stage) => stage.status), ['review', 'pending'])

  const stagedFlow = buildDirectorStages(stagedRoles, stagedReviews, { nodes: [] })
  assert.equal(nextStageIdAfterReview(stagedFlow, stagedReviews[0]), 'storyboard_artist')
  const optimisticFlow = applyOptimisticRunningStage(stagedFlow, 'storyboard_artist')
  assert.equal(optimisticFlow[1].status, 'running')
  assert.equal(stagedFlow[1].status, 'pending')

  const duplicateScriptReviewsFlow = buildDirectorStages(
    [stagedRoles[0]],
    [
      {
        id: 'knowledge-approved',
        nodeId: 'knowledge-approved',
        status: 'APPROVED',
        stage: 'script',
        tool: 'knowledge_researcher',
        reviewContent: '已通过的知识调研',
      },
      {
        id: 'script-pending',
        nodeId: 'script-pending',
        status: 'PENDING',
        stage: 'script',
        tool: 'video_script_generator',
        reviewContent: '当前待审核口播脚本',
      },
    ],
    { nodes: [] },
  )
  assert.equal(duplicateScriptReviewsFlow[0].status, 'review')
  assert.equal(duplicateScriptReviewsFlow[0].reviewId, 'script-pending')
  assert.equal(reviewDisplayTitle({
    id: 'knowledge-approved',
    status: 'APPROVED',
    tool: 'knowledge_researcher',
    reviewContent: '已通过的知识调研',
  }), '脚本依据 / 创作依据')
  assert.equal(reviewOutputPanelTitle({ tool: 'knowledge_researcher' }), '脚本依据')
  assert.match(reviewOutputPanelHint({ tool: 'knowledge_researcher' }), /口播脚本/)
  assert.equal(preferredActiveReview([
    {
      id: 'knowledge-pending',
      nodeId: 'knowledge_researcher_review',
      status: 'PENDING',
      tool: 'knowledge_researcher',
      reviewContent: '产品事实材料',
    },
    {
      id: 'script-pending',
      nodeId: 'video_script_generator_review',
      status: 'PENDING',
      tool: 'video_script_generator',
      reviewContent: '当前待审核口播脚本',
    },
  ])?.id, 'script-pending')

  const knowledgeReviewFlow = buildDirectorStages(
    [
      { ...startupRoles[0], allowedTools: ['proposal_generator'] },
      { ...stagedRoles[0], allowedTools: ['video_script_generator', 'script_quality_checker', 'knowledge_researcher'] },
    ],
    [
      {
        id: 'knowledge-pending',
        nodeId: 'knowledge_researcher_review',
        status: 'PENDING',
        tool: 'knowledge_researcher',
        reviewContent: '佛得角是西非岛国。',
      },
    ],
    { nodes: [] },
  )
  assert.equal(knowledgeReviewFlow[0].status, 'review')
  assert.equal(knowledgeReviewFlow[0].reviewId, 'knowledge-pending')
  assert.equal(knowledgeReviewFlow[1].status, 'pending')

  const scriptQualityGateFlow = buildDirectorStages(
    [{
      ...stagedRoles[0],
      allowedTools: ['video_script_generator', 'script_quality_checker'],
    }],
    [
      {
        id: 'script-approved-before-quality',
        nodeId: 'script-approved-before-quality',
        status: 'APPROVED',
        stage: 'script',
        tool: 'video_script_generator',
        reviewContent: '已通过的口播脚本',
      },
      {
        id: 'script-quality-gate-pending',
        nodeId: 'video_script_generator_quality_gate',
        stepId: 'video_script_generator_quality_gate',
        status: 'PENDING',
        reviewPhase: 'quality_gate',
        reviewReason: '质量门禁：script_quality_checker 评分需 >=85',
        reviewOutput: { qualityReport: { score: 82, issues: ['事实年份错误'] } },
      },
    ],
    {
      nodes: [
        {
          id: 'script-exec',
          name: 'external',
          type: 'TOOL',
          status: 'SUCCESS',
          input: { tool: 'external', capabilityTool: 'video_script_generator', stage: 'script', roleAgentId: 'script_writer' },
          output: { artifacts: [{ kind: 'VIDEO_SCRIPT', name: '视频脚本', storageRef: 'local://script.json' }] },
        },
        {
          id: 'script-quality',
          name: 'external',
          type: 'TOOL',
          status: 'SUCCESS',
          input: { tool: 'external', capabilityTool: 'script_quality_checker' },
          output: { artifacts: [{ kind: 'JSON', name: '脚本质检报告' }] },
        },
      ],
    },
  )
  assert.equal(scriptQualityGateFlow[0].status, 'review')
  assert.equal(scriptQualityGateFlow[0].reviewId, 'script-quality-gate-pending')

  const qualityGateScopedFlow = buildDirectorStages(
    [
      {
        ...stagedRoles[0],
        allowedTools: ['video_script_generator', 'script_quality_checker'],
      },
      {
        id: 'quality_reviewer',
        name: 'Quality Reviewer',
        displayName: '质量审核',
        stage: 'quality',
        goal: '',
        allowedTools: ['video_frame_qa', 'ffmpeg_probe', 'final_review_generator'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['VIDEO_VISUAL_QA_REPORT', 'VIDEO_VISUAL_QA_CONTACT_SHEET', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'],
      },
    ],
    [
      {
        id: 'script-quality-gate-pending',
        nodeId: 'video_script_generator_quality_gate',
        stepId: 'video_script_generator_quality_gate',
        status: 'PENDING',
        reviewPhase: 'quality_gate',
        reviewReason: '质量门禁：script_quality_checker 评分需 >=85',
        reviewOutput: { qualityReport: { score: 82, issues: ['事实年份错误'] } },
      },
    ],
    { nodes: [] },
  )
  assert.equal(qualityGateScopedFlow[0].status, 'review')
  assert.equal(qualityGateScopedFlow[1].status, 'pending')

  const storyboardGateAliasFlow = buildDirectorStages(
    [
      {
        ...stagedRoles[1],
        allowedTools: ['card_plan_generator'],
      },
      {
        id: 'quality_reviewer',
        name: 'Quality Reviewer',
        displayName: '质量审核',
        stage: 'quality',
        goal: '',
        allowedTools: ['video_frame_qa', 'ffmpeg_probe', 'final_review_generator'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['VIDEO_VISUAL_QA_REPORT', 'VIDEO_VISUAL_QA_CONTACT_SHEET', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'],
      },
    ],
    [
      {
        id: 'beat-plan-quality-gate-pending',
        nodeId: 'beat_plan_quality_gate',
        stepId: 'beat_plan_quality_gate',
        status: 'PENDING',
        tool: 'shot_splitter',
        reviewPhase: 'quality_gate',
        reviewReason: '质量门禁：shot_quality_checker 评分需 >=85',
        reviewOutput: { qualityReport: { score: 30, issues: ['总时长超出目标'] } },
      },
    ],
    { nodes: [] },
  )
  assert.equal(storyboardGateAliasFlow[0].status, 'review')
  assert.equal(storyboardGateAliasFlow[1].status, 'pending')

  const referenceToolAliasFlow = buildDirectorStages(
    [
      {
        id: 'reference_selector',
        name: 'Reference Selector',
        displayName: '参考资产选择器',
        stage: 'reference',
        goal: '',
        allowedTools: ['style_reference_selector'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['REFERENCE_PACKAGE'],
      },
      {
        id: 'quality_reviewer',
        name: 'Quality Reviewer',
        displayName: '质量审核',
        stage: 'quality',
        goal: '',
        allowedTools: ['video_frame_qa', 'ffmpeg_probe', 'final_review_generator'],
        forbiddenTools: [],
        requiredInputs: [],
        requiredOutputs: ['VIDEO_VISUAL_QA_REPORT', 'VIDEO_VISUAL_QA_CONTACT_SHEET', 'FFMPEG_PROBE_REPORT', 'FINAL_REVIEW'],
      },
    ],
    [
      {
        id: 'video-prompt-review',
        nodeId: 'video_prompt_generator_review',
        stepId: 'video_prompt_generator_review',
        status: 'PENDING',
        tool: 'video_prompt_generator',
        reviewContent: 'SHOT_01 keyframe prompt',
      },
    ],
    { nodes: [] },
  )
  assert.equal(referenceToolAliasFlow[0].status, 'review')
  assert.equal(referenceToolAliasFlow[1].status, 'pending')

  const previewPendingWithUnmaterializedOutputsFlow = buildDirectorStages(
    [{
      id: 'preview_director',
      name: 'Preview Director',
      displayName: '预览导演',
      stage: 'preview',
      goal: '',
      allowedTools: ['hyperframes_project_generator', 'hyperframes_snapshot'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_COMPOSITION_SPEC'],
      requiredOutputs: ['HYPERFRAMES_PROJECT', 'PREVIEW_SNAPSHOTS'],
    }],
    [
      {
        id: 'preview-review',
        nodeId: 'preview_review',
        status: 'PENDING',
        stage: 'preview',
        tool: 'hyperframes_project_generator',
        reviewContent: 'HyperFrames 项目已生成，等待审核。',
      },
    ],
    {
      nodes: [
        {
          id: 'preview_exec',
          name: 'external',
          type: 'TOOL',
          status: 'SUCCESS',
          input: { tool: 'external', capabilityTool: 'hyperframes_project_generator', stage: 'preview' },
          output: { artifacts: [{ kind: 'HTML', name: 'index.html', storageRef: '' }] },
        },
        {
          id: 'preview_review',
          name: '审核-preview',
          type: 'REVIEW_GATE',
          status: 'READY',
          input: { stage: 'preview', tool: 'hyperframes_project_generator', reviewPhase: 'after_artifact' },
          output: {},
        },
      ],
    },
  )
  assert.equal(previewPendingWithUnmaterializedOutputsFlow[0].status, 'review')

  const reviewGateFlow = buildDirectorStages(stagedRoles, [], {
    nodes: [
      {
        id: 'script-review',
        name: '审核-video_script_generator',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { tool: 'video_script_generator', stage: 'script', reviewPhase: 'after_artifact' },
        output: { content: '# 口播脚本' },
      },
    ],
  })
  assert.equal(reviewGateFlow[0].status, 'review')

  const nestedParameterFlow = buildDirectorStages(
    [{
      id: 'storyboard_artist',
      name: 'Storyboard Artist',
      displayName: '卡片设计师',
      stage: 'storyboard',
      goal: '拆画面',
      allowedTools: ['card_plan_generator'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_SCRIPT'],
      requiredOutputs: ['CARD_PLAN'],
    }],
    [{
      id: 'shot_splitter_review',
      nodeId: 'shot_splitter_review',
      status: 'PENDING',
      stage: 'storyboard',
      tool: 'shot_splitter',
      reviewPhase: 'after_artifact',
    }],
    {
      nodes: [
        {
          id: 'shot_splitter_exec',
          name: 'external',
          type: 'TOOL',
          status: 'RUNNING',
          input: {
            tool: 'external',
            capabilityTool: 'shot_splitter',
            parameters: {
              stage: 'storyboard',
              roleAgentId: 'storyboard_artist',
            },
          },
          output: {},
        },
        {
          id: 'shot_splitter_review',
          name: '审核-shot_splitter',
          type: 'REVIEW_GATE',
          status: 'READY',
          input: {
            stage: 'storyboard',
            tool: 'shot_splitter',
            sourceNode: 'shot_splitter_exec',
            reviewPhase: 'after_artifact',
          },
          output: {},
        },
      ],
    },
  )
  assert.equal(nestedParameterFlow[0].status, 'running')

  const nestedParameterTrace = buildDirectorTraceNodes({
    nodes: [
      {
        id: 'shot_splitter_exec',
        name: 'external',
        type: 'TOOL',
        status: 'RUNNING',
        input: {
          tool: 'external',
          capabilityTool: 'shot_splitter',
          parameters: { stage: 'storyboard' },
        },
        output: {},
      },
    ],
  })
  assert.equal(nestedParameterTrace[0].stage, 'storyboard')

  const proposalArtifacts = buildDirectorArtifacts(
    [{
      id: 'creative_director',
      name: 'Creative Director',
      displayName: '创意总监',
      stage: 'proposal',
      goal: '',
      allowedTools: ['proposal_generator'],
      forbiddenTools: [],
      requiredInputs: [],
      requiredOutputs: ['VIDEO_PROPOSAL'],
    }],
    [],
    { nodes: [{ id: 'proposal', name: 'proposal_generator', status: 'SUCCESS', input: { stage: 'proposal' }, output: { artifacts: [{ kind: 'VIDEO_PROPOSAL', name: '创意方案', storageRef: 'cloud://proposal' }] } }] },
  )
  assert.equal(proposalArtifacts[0].storageRef, '本地项目目录（仅同步索引）')

  const compositionRole = {
    id: 'composition_director',
    name: 'Composition Director',
    displayName: '结构导演',
    stage: 'composition',
    goal: '生成时间轴',
    allowedTools: ['video_composition_builder'],
    forbiddenTools: [],
    requiredInputs: ['VIDEO_SCRIPT'],
    requiredOutputs: ['VIDEO_COMPOSITION_SPEC'],
  }
  const missingCompositionTrace = {
    nodes: [
      {
        id: 'composition_exec',
        name: 'external',
        type: 'TOOL',
        status: 'SUCCESS',
        input: { tool: 'video_composition_builder', stage: 'composition', roleAgentId: 'composition_director' },
        output: { content: 'composition finished without artifact manifest' },
      },
      {
        id: 'composition_review_gate',
        name: '审核-视频结构',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { stage: 'composition', requiredOutputs: ['VIDEO_COMPOSITION_SPEC'], reviewPhase: 'after_artifact' },
        output: {},
      },
    ],
  }
  const missingCompositionArtifacts = buildDirectorArtifacts([compositionRole], [], missingCompositionTrace)
  assert.equal(missingCompositionArtifacts[0].status, 'missing')
  const missingCompositionStages = buildDirectorStages([compositionRole], [], missingCompositionTrace, true)
  assert.equal(missingCompositionStages[0].status, 'failed')

  const approvedReviewFallbackRoles = [
    {
      id: 'creative_director',
      name: 'Creative Director',
      displayName: '创意总监',
      stage: 'proposal',
      goal: '定方向',
      allowedTools: ['proposal_generator'],
      forbiddenTools: [],
      requiredInputs: [],
      requiredOutputs: ['VIDEO_PROPOSAL'],
    },
    {
      id: 'script_writer',
      name: 'Script Writer',
      displayName: '脚本编剧',
      stage: 'script',
      goal: '写脚本',
      allowedTools: ['video_script_generator'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_PROPOSAL'],
      requiredOutputs: ['VIDEO_SCRIPT'],
    },
    {
      id: 'storyboard_artist',
      name: 'Storyboard Artist',
      displayName: '分镜导演',
      stage: 'storyboard',
      goal: '拆画面',
      allowedTools: ['shot_splitter'],
      forbiddenTools: [],
      requiredInputs: ['VIDEO_SCRIPT'],
      requiredOutputs: ['CARD_PLAN'],
    },
  ]
  const approvedReviewFallbackReviews = [
    {
      id: 'proposal-approved',
      nodeId: 'proposal-review',
      status: 'APPROVED',
      roleAgentId: 'creative_director',
      stage: 'proposal',
      tool: 'proposal_generator',
      reviewContent: '# 创意方案\n\n佛得角国家介绍与世界杯奇迹。',
    },
    {
      id: 'script-approved',
      nodeId: 'script-review',
      status: 'APPROVED',
      roleAgentId: 'script_writer',
      stage: 'script',
      tool: 'video_script_generator',
      reviewOutput: { script: '佛得角是西非岛国，人口不多，却踢出了世界杯奇迹。' },
    },
    {
      id: 'storyboard-approved',
      nodeId: 'storyboard-review',
      status: 'APPROVED',
      roleAgentId: 'storyboard_artist',
      stage: 'storyboard',
      tool: 'shot_splitter',
      reviewOutput: {
        shotList: [
          {
            shotId: 'SHOT_01',
            durationSec: 7,
            narrationText: '佛得角是西非岛国。',
            visual: '地图上突出佛得角群岛。',
          },
        ],
        totalDurationSec: 7,
      },
    },
  ]
  const approvedReviewFallbackTrace = {
    nodes: approvedReviewFallbackRoles.map((role) => ({
      id: `${role.stage}-exec`,
      name: 'external',
      type: 'TOOL',
      status: 'SUCCESS',
      input: { tool: 'external', capabilityTool: role.allowedTools[0], stage: role.stage, roleAgentId: role.id },
      output: { content: `${role.stage} finished without artifact manifest` },
    })),
  }
  const approvedReviewFallbackStages = buildDirectorStages(
    approvedReviewFallbackRoles,
    approvedReviewFallbackReviews,
    approvedReviewFallbackTrace,
    true,
  )
  assert.deepEqual(approvedReviewFallbackStages.map((stage) => stage.status), ['done', 'done', 'done'])
  const approvedReviewFallbackArtifacts = buildDirectorArtifacts(
    approvedReviewFallbackRoles,
    approvedReviewFallbackReviews,
    approvedReviewFallbackTrace,
  )
  assert.deepEqual(approvedReviewFallbackArtifacts.map((artifact) => artifact.status), ['valid', 'valid', 'valid'])
  assert.deepEqual(approvedReviewFallbackArtifacts.map((artifact) => artifact.humanApproved), [true, true, true])
  const proposalSelection = getArtifactViewerSelection(undefined, approvedReviewFallbackArtifacts[0])
  assert.equal(proposalSelection.shouldLoad, false)
  assert.match(proposalSelection.placeholder || '', /佛得角国家介绍/)
  const storyboardSelection = getArtifactViewerSelection(undefined, approvedReviewFallbackArtifacts[2])
  assert.match(storyboardSelection.placeholder || '', /分镜队列/)
  assert.match(storyboardSelection.placeholder || '', /SHOT_01/)

  const materializedCompositionArtifacts = buildDirectorArtifacts([compositionRole], [], missingCompositionTrace, [
    {
      id: 'art-composition-1',
      projectId: 'vp-1',
      stageName: 'composition',
      unitId: 'composition',
      kind: 'VIDEO_COMPOSITION_SPEC',
      name: '视频结构',
      version: 2,
      status: 'valid',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/composition/composition/hash/composition.json',
      dependsOn: ['VIDEO_SCRIPT'],
      metadata: { producedByRole: '结构导演' },
      updatedAt: '2026-06-29T08:00:00Z',
    },
  ])
  assert.equal(materializedCompositionArtifacts[0].id, 'art-composition-1')
  assert.equal(materializedCompositionArtifacts[0].version, '第2版')
  assert.equal(materializedCompositionArtifacts[0].status, 'valid')
  assert.equal(materializedCompositionArtifacts[0].humanApproved, true)
  assert.equal(materializedCompositionArtifacts[0].storageRef, 'local://projects/vp-1/artifacts/composition/composition/hash/composition.json')

  const artifactsWithPublishCopy = buildDirectorArtifacts([compositionRole], [], missingCompositionTrace, [
    {
      id: 'art-publish-copy-1',
      projectId: 'vp-1',
      stageName: 'publish',
      unitId: 'publish-copy',
      kind: 'PUBLISH_COPY',
      name: '发布文案.json',
      version: 1,
      status: 'valid',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/publish/publish-copy/hash/publish_copy.json',
      metadata: { artifactType: 'publish_copy' },
      updatedAt: '2026-06-29T08:00:00Z',
    },
  ])
  assert.ok(artifactsWithPublishCopy.some((artifact) => artifact.kind === 'PUBLISH_COPY'))
  assert.equal(findPublishCopyArtifact(artifactsWithPublishCopy)?.id, 'art-publish-copy-1')
  assert.equal(
    localArtifactIdFromStorageRef('local://projects/vp-1/artifacts/final-video/final-video/hash/final.mp4'),
    'final-video',
  )
  assert.equal(
    localArtifactIdFromStorageRef('local://projects/vp-1/artifacts/video_prompt/shot_complete_SHOT_01/e837383b0f0802d0d8b4bb67ca8b261c4adacb182da94ec3393980b972563f67/SHOT_01_complete_shot.mp4'),
    'shot_complete_SHOT_01',
  )
  assert.equal(
    localArtifactIdFromStorageRef('local://projects/vp-1/artifacts/shot_complete_SHOT_01/e62dffeea00dbbc1308db2515899e68faecb4195d4305cc8583a6c9b222df936/SHOT_01_complete_shot.mp4'),
    'shot_complete_SHOT_01',
  )
  assert.equal(
    localArtifactIdFromStorageRef('local://projects/20260701090839-40404040/manual-final.mp4'),
    undefined,
  )
  const finalVideoCandidates = [
    {
      id: 'shot-video-result-1',
      kind: 'VIDEO',
      name: 'SHOT_01 视频回填结果',
      stageName: 'external_generation_result',
      unitId: 'extgen_video_SHOT_01',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/shot-video-result-1/shot-video-result-1/hash/result.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_result', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'art-render-placeholder',
      kind: 'VIDEO',
      name: 'final.mp4',
      stageName: 'render',
      unitId: 'final-video',
      status: 'valid',
      owner: '渲染制片',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/20260701090839-40404040/manual-final.mp4',
      metadata: { manualUpload: true, status: 'manual_upload_required' },
    },
    {
      id: 'art-final-upload',
      kind: 'VIDEO',
      name: 'final_video.mp4',
      stageName: 'external_generation_result',
      unitId: 'final-video',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/codex-smoke-final-video/codex-smoke-final-video/hash/final.mp4',
      metadata: { artifactType: 'external_generation_result', generationKind: 'video', generationRequestId: 'final-video', tags: ['final_video'] },
    },
  ]
  assert.equal(findFinalVideoArtifact(finalVideoCandidates)?.id, 'art-final-upload')
  assert.equal(findFinalVideoArtifact(finalVideoCandidates.slice(0, 2))?.id, 'art-render-placeholder')

  const shotReviewGroups = buildShotReviewGroups([
    {
      id: 'shot-packet-1',
      kind: 'SHOT_REVIEW_PACKET',
      name: 'SHOT_01 审核包',
      status: 'review',
      owner: '分镜导演',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://shot-1/review.json',
      metadata: { relatedShotId: 'SHOT_01', narrationText: '佛得角是西非岛国。', artifactType: 'shot_review_packet' },
    },
    {
      id: 'shot-ref-1',
      kind: 'REFERENCE_ASSET_PLAN',
      name: '人物三视角参考图',
      status: 'valid',
      owner: '参考选择',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/character-views.png',
      metadata: { relatedShotId: 'SHOT_01', referenceRole: 'character', viewSet: ['front', 'side', 'back'] },
    },
    {
      id: 'shot-audio-1',
      kind: 'SHOT_AUDIO',
      name: 'SHOT_01 口播音频',
      status: 'valid',
      owner: '音频',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/audio.wav',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'shot_audio' },
    },
    {
      id: 'shot-video-1',
      kind: 'SHOT_VIDEO_CLIP',
      name: 'SHOT_01 视频片段',
      status: 'pending',
      owner: '视频',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://shot-1/clip.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'shot_video_clip' },
    },
    {
      id: 'shot-video-request-1',
      kind: 'EXTERNAL_GENERATION_REQUEST',
      name: 'SHOT_01 视频生成请求',
      status: 'review',
      owner: '素材依赖点',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'inline://extgen-video',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_request', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'shot-video-result-1',
      kind: 'VIDEO',
      name: 'SHOT_01 视频回填结果',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/result.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_result', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'shot-storyboard-result-1',
      kind: 'IMAGE',
      name: 'SHOT_01 故事板上传结果',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/storyboard.png',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_result', assetType: 'image', tags: ['manual_shot_upload', 'shot_storyboard'] },
    },
    {
      id: 'shot-generation-plan',
      kind: 'SHOT_GENERATION_PLAN',
      name: 'Shot generation plan',
      status: 'valid',
      owner: '策略规划',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'inline://shot-generation-plan',
      metadata: {
        artifactType: 'shot_generation_plan',
        inlineContent: {
          shotGenerationPlans: [
            { shotId: 'SHOT_01', mode: 'hybrid_aigc_bg_html_overlay', reason: '需要 AIGC 背景视频配合 HyperFrames 精确文字层。', riskLevel: 'medium' },
            { shotId: 'SHOT_02', mode: 'html_only', reason: '纯文字和图表动画可由 HyperFrames 完成。', riskLevel: 'low' },
          ],
        },
      },
    },
    {
      id: 'shot-packet-2',
      kind: 'SHOT_REVIEW_PACKET',
      name: 'SHOT_02 审核包',
      status: 'valid',
      owner: '分镜导演',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-2/review.json',
      metadata: { shotId: 'SHOT_02', scriptText: '世界杯出线是小国奇迹。', artifactType: 'shot_review_packet' },
    },
  ])
  assert.equal(shotReviewGroups.length, 2)
  assert.equal(shotReviewGroups[0].shotId, 'SHOT_01')
  assert.equal(shotReviewGroups[0].status, 'review')
  assert.equal(shotReviewGroups[0].narrationText, '佛得角是西非岛国。')
  assert.deepEqual(shotReviewGroups[0].referenceRoles, ['character'])
  assert.deepEqual(shotReviewGroups[0].artifactCounts, { total: 7, references: 1, media: 2, reviewPackets: 1 })
  assert.deepEqual(shotReviewGroups[0].slots.map((slot) => slot.kind), ['prompt', 'reference', 'storyboard', 'base-media', 'overlay', 'video'])
  assert.match(shotReviewGroups[0].slots.find((slot) => slot.kind === 'base-media')?.description || '', /AIGC 参考视频、参考图/)
  assert.match(shotReviewGroups[0].slots.find((slot) => slot.kind === 'overlay')?.description || '', /HyperFrames 本地生成/)
  assert.match(shotReviewGroups[0].slots.find((slot) => slot.kind === 'video')?.description || '', /合并后的完整 shot/)
  assert.deepEqual(shotReviewGroups[0].generationStrategy, {
    mode: 'hybrid_aigc_bg_html_overlay',
    label: 'Hybrid',
    reason: '需要 AIGC 背景视频配合 HyperFrames 精确文字层。',
    riskLevel: 'medium',
  })
  assert.equal(shotReviewGroups[0].slots.find((slot) => slot.kind === 'prompt')?.dependencyRequests.some((artifact) => artifact.id === 'shot-video-request-1'), false)
  assert.equal(shotReviewGroups[0].slots.find((slot) => slot.kind === 'base-media')?.dependencyRequests.some((artifact) => artifact.id === 'shot-video-request-1'), false)
  assert.equal(unresolvedMaterialDependencyCount(shotReviewGroups), 0)
  assert.ok(shotReviewGroups[0].slots.find((slot) => slot.kind === 'storyboard')?.artifacts.some((artifact) => artifact.id === 'shot-storyboard-result-1'))
  const resolvedShotReferences = externalGenerationReferencesFromArtifacts(shotReviewGroups[0].slots.flatMap((slot) => slot.artifacts))
  assert.equal(resolvedShotReferences[0]?.label, '人物三视角参考图')
  assert.equal(resolvedShotReferences[0]?.storageRef, 'local://shot-1/character-views.png')
  assert.equal(resolvedShotReferences[0]?.role, 'character')
  const mergedShotTask = mergeExternalGenerationTaskReferences({
    requestId: 'extgen_video_SHOT_01',
    kind: 'video',
    shotId: 'SHOT_01',
    prompt: '生成一段主角走进镜头的 5 秒视频。',
    references: [],
    referenceImageLimit: 6,
  }, resolvedShotReferences)
  assert.equal(mergedShotTask.references.length, 2)
  assert.equal(mergedShotTask.references[0].label, '人物三视角参考图')
  assert.equal(mergedShotTask.references[1].role, 'storyboard')
  assert.equal(shotReviewGroups[1].shotId, 'SHOT_02')
  assert.equal(shotReviewGroups[1].status, 'valid')
  assert.equal(shotReviewGroups[1].generationStrategy?.label, 'HyperFrames')

  const requestOnlyShotGroups = buildShotReviewGroups([
    {
      id: 'external-generation-request-SHOT_01',
      kind: 'EXTERNAL_GENERATION_REQUEST',
      name: 'external_generation_request.json',
      status: 'review',
      owner: '素材依赖点',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://projects/vp-1/artifacts/video_prompt/extgen-video-SHOT_01.json',
      metadata: { artifactType: 'external_generation_request', generationKind: 'video' },
      inlineJson: JSON.stringify({
        requestId: 'extgen_video_SHOT_01',
        kind: 'video',
        shotId: 'SHOT_01',
        narrationText: '本系统已经开源，后续内容都会由它创作。',
        visual: '系统界面打开，开源仓库和视频流水线同时亮起。',
        target: { durationSec: 6, aspectRatio: '16:9', resolution: '1920x1080' },
        prompt: '非真人风格化动画，躺营视频创作助手界面打开，开源仓库 Star 动效弹出。',
      }),
    },
    {
      id: 'generated-video-SHOT_01',
      kind: 'SHOT_VIDEO_CLIP',
      name: 'SHOT_01_video_clip.mp4',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/video_prompt/shot-video.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'shot_video_clip' },
    },
  ])
  assert.equal(requestOnlyShotGroups.length, 1)
  assert.equal(requestOnlyShotGroups[0].title, 'SHOT_01')
  assert.equal(requestOnlyShotGroups[0].narrationText, '本系统已经开源，后续内容都会由它创作。')
  assert.equal(requestOnlyShotGroups[0].visualText, '系统界面打开，开源仓库和视频流水线同时亮起。')
  assert.equal(requestOnlyShotGroups[0].durationSec, 6)

  const autoMcpShotGroups = buildShotReviewGroups([
    {
      id: 'auto-video-request',
      kind: 'EXTERNAL_GENERATION_REQUEST',
      name: 'SHOT_01 视频生成请求',
      status: 'review',
      owner: '素材依赖点',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'inline://extgen-video',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'external_generation_request', generationKind: 'video', externalGenerationRequestId: 'extgen_video_SHOT_01' },
    },
    {
      id: 'auto-video-clip',
      kind: 'SHOT_VIDEO_CLIP',
      name: 'SHOT_01_TW_01_video_clip.mp4',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-1/jimeng-result.mp4',
      metadata: { relatedShotId: 'SHOT_01', artifactType: 'shot_video_clip', contentAvailability: 'local-agent' },
    },
  ])
  assert.equal(unresolvedMaterialDependencyCount(autoMcpShotGroups), 0)
  assert.equal(autoMcpShotGroups[0].slots.find((slot) => slot.kind === 'base-media')?.dependencyRequests.length, 0)

  const betaShotGroups = buildShotReviewGroups([
    {
      id: 'shot-qa-99',
      kind: 'SHOT_QA_REPORT',
      name: 'SHOT_99 QA',
      status: 'valid',
      owner: '视觉质量审核',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://shot-99/qa.json',
      metadata: {
        relatedShotId: 'SHOT_99',
        durationSec: 6,
        qaStatus: 'ACCEPTED_FOR_ASSEMBLY',
        acceptedCandidateId: 'cand-02',
        candidates: [
          {
            candidateId: 'cand-01',
            attemptIndex: 0,
            status: 'SHOT_QA_FAILED',
            sourceType: 'aigc_video',
            isFallback: false,
            qaReport: { passed: false },
            repairPlan: { action: 'RERENDER_HTML', lockedDimensions: ['scene', 'action'] },
          },
          {
            candidateId: 'cand-02',
            attemptIndex: 1,
            status: 'ACCEPTED_FOR_ASSEMBLY',
            sourceType: 'ffmpeg_composite',
            isFallback: false,
            qaReport: { passed: true },
          },
        ],
        repairPlan: { action: 'RERENDER_HTML', lockedDimensions: ['scene', 'action'] },
      },
    },
    {
      id: 'fallback-shot-clip',
      kind: 'SHOT_VIDEO_CLIP',
      name: 'SHOT_99 fallback storyboard',
      status: 'valid',
      owner: '项目产物',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://shot-99/fallback.mp4',
      metadata: { relatedShotId: 'SHOT_99', sourceType: 'fallback_storyboard', isFallback: true, artifactType: 'shot_video_clip' },
    },
  ])
  assert.equal(betaShotGroups[0].production.qaStatus, 'ACCEPTED_FOR_ASSEMBLY')
  assert.equal(betaShotGroups[0].production.attemptCount, 2)
  assert.equal(betaShotGroups[0].production.latestCandidateId, 'cand-02')
  assert.equal(betaShotGroups[0].production.repairPlanAction, 'RERENDER_HTML')
  assert.deepEqual(betaShotGroups[0].production.lockedDimensions, ['scene', 'action'])
  assert.equal(betaShotGroups[0].production.acceptedCandidateId, 'cand-02')
  assert.equal(betaShotGroups[0].production.sourceType, 'fallback_storyboard')
  assert.equal(betaShotGroups[0].production.isFallback, true)
  assert.equal(betaShotGroups[0].production.canEnterAssembly, true)

  const assemblySummary = buildProjectAssemblySummary(betaShotGroups, [
    {
      id: 'final-qa',
      kind: 'FINAL_REVIEW',
      name: 'Final QA',
      status: 'valid',
      owner: '质量审核',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://final-qa.json',
      metadata: { finalQaStatus: 'passed', passed: true },
    },
    {
      id: 'final-video',
      kind: 'VIDEO',
      name: 'final_video.mp4',
      stageName: 'render',
      unitId: 'final-video',
      status: 'valid',
      owner: '渲染制片',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://final.mp4',
      metadata: { sourceType: 'ffmpeg_composite', isFallback: false, tags: ['final_video'] },
    },
  ])
  assert.equal(assemblySummary.allShotsAccepted, true)
  assert.equal(assemblySummary.finalAssemblyStatus, 'ready')
  assert.equal(assemblySummary.finalQAStatus, 'passed')
  assert.equal(assemblySummary.fallbackCount, 1)
  assert.equal(assemblySummary.finalVideoSourceType, 'ffmpeg_composite')

  const profileArtifacts = [
    {
      id: 'video-creation-profile-1',
      kind: 'VIDEO_CREATION_PROFILE',
      name: '创作主线',
      status: 'valid',
      owner: '创意总监',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'inline://video-creation-profile',
      metadata: {
        profileId: 'cinematic_story',
        primaryArtifact: 'CONTINUITY_BIBLE',
        qualityContract: ['aigc_time_windows_3_15s'],
      },
    },
    {
      id: 'time-window-plan-1',
      kind: 'TIME_WINDOW_PLAN',
      name: '时间窗计划',
      status: 'valid',
      owner: '结构导演',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'inline://time-window-plan',
      metadata: {
        windows: [
          {
            id: 'SHOT_01_TW_01',
            shotId: 'SHOT_01_TW_01',
            parentShotId: 'SHOT_01',
            durationSec: 8,
            aigcEligible: true,
          },
        ],
      },
    },
  ]
  const profileSummary = creationProfileSummary(profileArtifacts)
  assert.equal(profileSummary.profileId, 'cinematic_story')
  assert.equal(profileSummary.label, '影视剧情')
  assert.equal(profileSummary.primaryArtifact, 'CONTINUITY_BIBLE')
  assert.ok(profileSummary.qualityContract.includes('aigc_time_windows_3_15s'))
  assert.equal(creationProfileSummary([{
    ...profileArtifacts[0],
    metadata: { profileId: 'talking_head' },
  }]).label, '口播解说')
  assert.equal(creationProfileSummary([]).label, '未选择')
  assert.equal(creationProfileSummary([{
    ...profileArtifacts[0],
    metadata: { cloudPayloadStored: true },
    inlineJson: JSON.stringify({
      profileId: 'cinematic_story',
      primaryArtifact: 'CONTINUITY_BIBLE',
      qualityContract: ['aigc_time_windows_3_15s'],
    }),
  }]).profileId, 'cinematic_story')
  assert.equal(creationProfileSummary([{
    ...profileArtifacts[0],
    metadata: {
      cloudPayloadStored: true,
      inlineContent: {
        creationProfile: {
          profileId: 'talking_head',
          primaryArtifact: 'VIDEO_SCRIPT',
          qualityContract: ['script_timeline_first'],
        },
      },
    },
  }]).primaryArtifact, 'VIDEO_SCRIPT')
  assert.deepEqual(timeWindowPlanSummary(profileArtifacts), {
    totalWindows: 1,
    aigcWindowCount: 1,
    invalidDurationCount: 0,
  })
  assert.deepEqual(timeWindowPlanSummary([{
    ...profileArtifacts[1],
    metadata: {
      timeWindowPlan: {
        windows: [
          { durationSec: '16', aigcEligible: 'true' },
          { durationSec: 2, aigcEligible: false },
          { aigcEligible: true },
        ],
      },
    },
  }]), {
    totalWindows: 3,
    aigcWindowCount: 2,
    invalidDurationCount: 1,
  })
  assert.deepEqual(timeWindowPlanSummary([{
    ...profileArtifacts[1],
    metadata: { cloudPayloadStored: true },
    inlineJson: JSON.stringify({
      timeWindowPlan: {
        windows: [
          { durationSec: 8, aigcEligible: 'yes' },
          { durationSec: 2, aigcEligible: 'off' },
        ],
      },
    }),
  }]), {
    totalWindows: 2,
    aigcWindowCount: 1,
    invalidDurationCount: 0,
  })
  assert.deepEqual(timeWindowPlanSummary([{
    ...profileArtifacts[1],
    metadata: {
      cloudPayloadStored: true,
      inlineContent: {
        timeWindows: [
          { durationSec: 16, aigcEligible: true },
        ],
      },
    },
  }]), {
    totalWindows: 1,
    aigcWindowCount: 1,
    invalidDurationCount: 1,
  })

  assert.deepEqual(
    getArtifactViewerSelection('art-publish-copy-1', artifactsWithPublishCopy.find((artifact) => artifact.id === 'art-publish-copy-1')),
    { selectedId: undefined, shouldLoad: false, placeholder: undefined },
  )

  const readyCompositionTrace = {
    nodes: [
      {
        id: 'composition_exec',
        name: 'external',
        type: 'TOOL',
        status: 'SUCCESS',
        input: { tool: 'video_composition_builder', stage: 'composition', roleAgentId: 'composition_director' },
        output: { artifacts: [{ kind: 'VIDEO_COMPOSITION_SPEC', name: '视频结构', storageRef: 'local://projects/vp-1/artifacts/composition/composition/hash/composition.json' }] },
      },
      {
        id: 'composition_review_gate',
        name: '审核-视频结构',
        type: 'REVIEW_GATE',
        status: 'READY',
        input: { stage: 'composition', requiredOutputs: ['VIDEO_COMPOSITION_SPEC'], reviewPhase: 'after_artifact' },
        output: {},
      },
    ],
  }
  const readyCompositionStages = buildDirectorStages([compositionRole], [], readyCompositionTrace, true)
  assert.equal(readyCompositionStages[0].status, 'review')

  const proposalReview = {
    id: 'td39d3460d8-proposal_generator_review',
    nodeId: 'td39d3460d8-proposal_generator_review',
    status: 'PENDING',
    tool: 'proposal_generator',
    reviewPhase: 'after_artifact',
    humanReview: { title: '审核创作方案' },
    reviewContent: '# Proposal Packet\n\n推荐方案：option_a',
  }
  assert.equal(reviewDisplayTitle(proposalReview), '审核创作方案')
  assert.equal(reviewOutputText(proposalReview), '# Proposal Packet\n\n推荐方案：option_a')
  assert.equal(displayNameForArtifact('COMPOSITED_SHOT_VIDEO'), '完整Shot片段')

  const qualityGateReview = {
    id: 'video_script_generator_quality_gate',
    nodeId: 'video_script_generator_quality_gate',
    status: 'PENDING',
    tool: 'video_script_generator',
    reviewPhase: 'quality_gate',
    reviewReason: '质量门禁：script_quality_checker 评分需 >=85',
    requiredOutputs: ['VIDEO_SCRIPT'],
    reviewContent: '佛得角第一次站上世界杯舞台，这不是冷门，是一代人的坚持。',
    reviewOutput: { qualityReport: { score: 82, issues: ['事实来源需要更明确'] } },
  }
  assert.equal(qualityGateTargetLabel(qualityGateReview), '视频脚本')
  assert.deepEqual(qualityGateTargetLines(qualityGateReview), ['目标产物：视频脚本'])
  assert.equal(reviewDisplayTitle(qualityGateReview), '质量门禁：口播脚本（目标产物：视频脚本）')
  assert.equal(reviewOutputPanelTitle(qualityGateReview), '质量门禁报告：视频脚本')
  assert.match(reviewOutputPanelHint(qualityGateReview), /本门禁只检查目标产物：视频脚本/)
  assert.equal(reviewOutputText(qualityGateReview), '佛得角第一次站上世界杯舞台，这不是冷门，是一代人的坚持。')
  assert.deepEqual(reviewQualityReportLines(qualityGateReview), [
    '目标产物：视频脚本',
    '质量评分 82/100，门禁阈值 85',
    '事实来源需要更明确',
  ])
  const explainableQualityGateReview = {
    ...qualityGateReview,
    reviewOutput: {
      qualityReport: {
        score: 95,
        passed: true,
        analysisSummary: '脚本结构完整，开头钩子明确，时长与事实引用都满足本轮要求。',
        rubricBreakdown: [
          { criterion: '结构完整性', score: 20, maxScore: 20, reason: '包含钩子、主体和总结。' },
          { criterion: '口播自然度', score: 18, maxScore: 20, reason: '表达清楚，个别句子可更短。' },
        ],
        keepDoing: ['保留开头的反差钩子', '继续引用本次知识材料中的事实'],
      },
    },
  }
  assert.deepEqual(reviewQualityReportLines(explainableQualityGateReview), [
    '目标产物：视频脚本',
    '质量评分 95/100，门禁阈值 85',
    '门禁结果：已通过',
    '分析：脚本结构完整，开头钩子明确，时长与事实引用都满足本轮要求。',
    '结构完整性：20/20，包含钩子、主体和总结。',
    '口播自然度：18/20，表达清楚，个别句子可更短。',
    '保持：保留开头的反差钩子',
    '保持：继续引用本次知识材料中的事实',
  ])

  const noisyJsonReview = {
    id: 'knowledge-review',
    nodeId: 'knowledge-review',
    status: 'PENDING',
    tool: 'knowledge_researcher',
    reviewContent: '我们被要求输出一个JSON，先分析。\n{"facts":["佛得角是西非岛国"],"summary":"佛得角首次晋级世界杯。"}\n以上是最终结果。',
  }
  assert.equal(
    reviewOutputText(noisyJsonReview),
    '{\n  "facts": [\n    "佛得角是西非岛国"\n  ],\n  "summary": "佛得角首次晋级世界杯。"\n}',
  )

  const shotJsonReview = {
    id: 'shot-review',
    nodeId: 'shot-review',
    status: 'PENDING',
    tool: 'shot_splitter',
    reviewContent: JSON.stringify({
      shotList: [
        {
          shotId: 'SHOT_01',
          durationSec: 7,
          narrationText: '佛得角是西非岛国。',
          visual: '地图上突出佛得角群岛。',
          materialLibraryHints: ['佛得角群岛地图', '足球场'],
        },
        {
          shotId: 'SHOT_02',
          durationSec: 8,
          narrationText: '世界杯出线是小国奇迹。',
          visual: '球迷庆祝与比分数据可视化。',
        },
      ],
      shotAssetPackages: [{ shotId: 'SHOT_01' }, { shotId: 'SHOT_02' }],
      totalDurationSec: 15,
    }),
  }
  const shotReviewText = reviewOutputText(shotJsonReview)
  assert.match(shotReviewText, /分镜队列/)
  assert.match(shotReviewText, /当前先审核：SHOT_01/)
  assert.match(shotReviewText, /佛得角是西非岛国/)
  assert.ok(!shotReviewText.includes('shotAssetPackages'), shotReviewText)

  const videoPromptQualityGateReview = {
    id: 'video_prompt_generator_quality_gate',
    nodeId: 'video_prompt_generator_quality_gate',
    status: 'PENDING',
    tool: 'video_prompt_generator',
    reviewPhase: 'quality_gate',
    reviewReason: '质量门禁：video_prompt_quality_checker 评分需 >=85',
    requiredOutputs: ['VIDEO_PROMPTS', 'SHOT_ASSET_PACKAGE'],
    reviewContent: JSON.stringify({
      summary: 'Shot 视频生成资料已准备好：每个 shot 都来自口播稿，并明确 HyperFrames / AIGC 分工。',
      videoPrompts: [
        {
          shotId: 'SHOT_01',
          durationSec: 6,
          narrationText: '本系统已经开源。',
          visual: '代码仓库星标弹出，系统界面变成流程图。',
          prompt: '非真人风格化动画，开源项目看板亮起。',
          negativePrompt: '避免水印。',
        },
      ],
      shotAssetPackages: [
        {
          shotId: 'SHOT_01',
          durationSec: 6,
          assetRoute: 'hybrid_aigc_bg_html_overlay',
          whyThisShot: '开头先建立系统身份。',
          actionBeats: ['系统界面亮起', 'Star 贴纸弹出'],
          referenceImages: [{ id: 'ref-ui', label: '系统界面参考图', role: 'reference', storageRef: 'local://projects/p/ref-ui.png' }],
          prompts: { videoPrompt: '非真人风格化动画，开源项目看板亮起。', negativePrompt: '避免水印。' },
          aigcVideo: { requestId: 'extgen_video_SHOT_01' },
        },
      ],
      externalGenerationRequests: [
        {
          requestId: 'extgen_video_SHOT_01',
          kind: 'video',
          shotId: 'SHOT_01',
          prompt: '非真人风格化动画，开源项目看板亮起。',
          references: [{ id: 'ref-ui', label: '系统界面参考图', role: 'reference', storageRef: 'local://projects/p/ref-ui.png' }],
        },
      ],
    }),
  }
  assert.equal(qualityGateTargetLabel(videoPromptQualityGateReview), '视频提示词、Shot独立素材包')
  assert.equal(reviewDisplayTitle(videoPromptQualityGateReview), '质量门禁：Shot 视频生成资料（目标产物：视频提示词、Shot独立素材包）')
  assert.equal(reviewOutputPanelTitle(videoPromptQualityGateReview), 'Shot 制作说明')
  assert.match(reviewOutputPanelHint(videoPromptQualityGateReview), /口播稿拆出的每个 shot/)
  const videoPromptReviewText = reviewOutputText(videoPromptQualityGateReview)
  assert.match(videoPromptReviewText, /Shot 视频生成资料/)
  assert.match(videoPromptReviewText, /质量门禁：Shot 视频生成资料/)
  assert.match(videoPromptReviewText, /来自口播：本系统已经开源/)
  assert.match(videoPromptReviewText, /画面变化：代码仓库星标弹出/)
  assert.match(videoPromptReviewText, /制作方式：AIGC 视频 \+ HyperFrames 合成/)
  assert.match(videoPromptReviewText, /AIGC 参考视频：需要用户在产物页复制提示词生成并上传/)
  assert.match(videoPromptReviewText, /参考图：系统界面参考图/)
  assert.match(videoPromptReviewText, /HyperFrames：本地生成精确文字、UI、字幕或图形包装/)
  assert.match(videoPromptReviewText, /去产物页查看提示词和上传入口/)
  assert.ok(!videoPromptReviewText.includes('shotAssetPackages'), videoPromptReviewText)
  const legacyVideoPromptReviewText = reviewOutputText({
    id: 'video_prompt_generator_quality_gate_legacy',
    nodeId: 'video_prompt_generator_quality_gate_legacy',
    status: 'PENDING',
    tool: 'video_prompt_generator',
    reviewPhase: 'quality_gate',
    reviewContent: '已基于 shotList 本地生成可复制到浏览器外部平台的独立 shot 视频提示词；当前不依赖图片或视频生成 API。',
  })
  assert.match(legacyVideoPromptReviewText, /Shot 视频生成资料/)
  assert.match(legacyVideoPromptReviewText, /质量门禁：Shot 视频生成资料/)
  assert.match(legacyVideoPromptReviewText, /去产物页查看提示词和上传入口/)
  assert.notEqual(legacyVideoPromptReviewText, '已基于 shotList 本地生成可复制到浏览器外部平台的独立 shot 视频提示词；当前不依赖图片或视频生成 API。')

  const legacyQualityGateReview = {
    id: 'legacy_quality_gate',
    nodeId: 'legacy_quality_gate',
    status: 'PENDING',
    tool: '__quality_gate__',
    reviewPhase: 'quality_gate',
  }
  assert.equal(reviewDisplayTitle(legacyQualityGateReview), '质量门禁（目标产物：待识别产物）')
  assert.deepEqual(qualityGateTargetLines(legacyQualityGateReview), ['目标产物：待识别产物'])

  const visibleReviews = visibleReviewHistory([
    { id: 'future-storyboard', nodeId: 'future-storyboard', status: 'CREATED', tool: 'card_plan_generator' },
    { id: 'proposal-review', nodeId: 'proposal-review', status: 'APPROVED', tool: 'proposal_generator' },
    { id: 'script-quality-passed', nodeId: 'script-quality-passed', status: 'APPROVED', tool: 'video_script_generator', reviewPhase: 'quality_gate' },
    { id: 'script-review', nodeId: 'script-review', status: 'PENDING', tool: 'video_script_generator' },
    { id: 'ready-script-review', nodeId: 'ready-script-review', status: 'PENDING', tool: 'video_script_generator', reviewContent: '可审核脚本正文' },
    { id: 'rejected-review', nodeId: 'rejected-review', status: 'REJECTED', tool: 'card_plan_generator' },
  ])
  assert.deepEqual(visibleReviews.map((review) => review.id), ['proposal-review', 'ready-script-review', 'rejected-review'])
  assert.equal(nextSelectedReviewId('script-quality-passed', 'ready-script-review', visibleReviews, 'script-quality-passed'), 'ready-script-review')
  assert.equal(reviewStatusLabel(visibleReviews[0]), '已通过')
  assert.equal(reviewStatusLabel(visibleReviews[1]), '待审核')
  assert.equal(reviewStatusLabel(visibleReviews[2]), '已驳回')

  const nestedTraceNodes = buildDirectorTraceNodes({
    data: {
      task: {
        nodes: [
          {
            id: 'gate-1',
            name: '质量门禁-video_script_generator_quality_gate',
            type: 'REVIEW_GATE',
            status: 'FAILED',
            errorMessage: '评分未达标',
            input: { stage: 'script' },
            output: {},
          },
        ],
      },
    },
  })
  assert.equal(nestedTraceNodes.length, 1)
  assert.equal(nestedTraceNodes[0].error, '评分未达标')
  assert.equal(traceNodeHasError(undefined), false)
  assert.equal(traceNodeHasError(nestedTraceNodes[0]), true)

  assert.equal(
    formatDirectorErrorMessage(
      new Error('CRITICAL_ARTIFACT_SYNC_FAILED: critical artifact sync failed: kind=VIDEO unitID=final-video'),
      'fallback',
    ),
    '关键产物写入失败，最终视频无法进入项目产物库。\n请重新执行当前步骤。',
  )
  assert.equal(
    normalizeDirectorErrorMessage({
      message: 'Request failed with status code 400',
      response: { data: { message: 'guard agent plan: agent plan has no steps' } },
    }),
    'guard agent plan: agent plan has no steps',
  )

  const rawTopic = '帮我介绍一下佛得角国家以及说明佛得角世界杯从小组赛出线是一个奇迹'
  assert.deepEqual(buildPublishCopies(rawTopic), [])

  const copies = buildPublishCopies({
    publishCopies: [
      {
        platform: 'xiaohongshu',
        title: '佛得角出线为什么是奇迹',
        description: '这条口播先介绍佛得角，再解释小国足球如何突破人口、资源与历史成绩限制。',
        keywords: ['佛得角', '世界杯', '小国奇迹'],
        coverText: '小国出线奇迹',
        publishTips: ['前两行直接写结论'],
      },
      {
        platform: 'bilibili',
        title: '佛得角：一个小国的世界杯奇迹',
        description: '从国家背景、足球基础和小组赛突围难度三个层次说明佛得角出线的罕见性。',
        tags: ['佛得角', '世界杯', '足球史'],
        coverText: '佛得角奇迹',
        publishTips: ['分区选择足球或知识'],
      },
    ],
  })
  assert.equal(copies.length, 2)
  assert.equal(copies[0].platform, 'xiaohongshu')
  assert.equal(copies[1].platform, 'bilibili')
  assert.equal(copies[0].tags[0], '佛得角')
  assert.ok(!copies.some((copy) => copy.title.includes('帮我介绍一下')))
  assert.ok(copies.every((copy) => copy.title && copy.description && copy.coverText))
  assert.ok(publishCopiesToMarkdown(copies).includes('## 小红书'))
  assert.ok(publishCopiesToMarkdown(copies).includes('## B站'))
  assert.equal(JSON.parse(publishCopiesToJSON(copies))[0].platform, 'xiaohongshu')

  const videoGuideSteps = externalGenerationGuideSteps({
    kind: 'video',
    references: [{ id: 'keyframe-1' }, { id: 'scene-1' }],
    target: { durationSec: 8, aspectRatio: '16:9', resolution: '1920x1080' },
    promptCharLimit: 2000,
    referenceImageLimit: 6,
  })
  assert.ok(videoGuideSteps[0].includes('没有可用的图片或视频 API 配置'))
  assert.ok(videoGuideSteps.some((step) => step.includes('复制文字提示词') && step.includes('浏览器')))
  assert.ok(videoGuideSteps.some((step) => step.includes('每个 shot 单独生成') && step.includes('转场放在本 shot 结尾')))
  assert.ok(videoGuideSteps.some((step) => step.includes('上传结果') && step.includes('素材库')))

  const profiles = videoCreationProfiles()
  assert.equal(profiles.length, 2, 'director studio should expose two stable video profiles')
  assert.deepEqual(profiles.map((profile) => profile.id), ['talking_head', 'cinematic_story'])
  assert.equal(normalizeVideoCreationProfileId('voice_visual'), 'talking_head')
  assert.equal(normalizeVideoCreationProfileId('wf-guided-image-text-video'), 'talking_head')
  assert.equal(normalizeVideoCreationProfileId('aigc_shot'), 'cinematic_story')
  assert.equal(
    videoCreationProfileForId('cinematic_story').preflightPipeline,
    'cinematic_story',
    'cinematic profile should use the AIGC shot preflight',
  )
  assert.equal(
    videoCreationProfileForId('talking_head').preflightPipeline,
    'talking_head',
    'voice/knowledge profile should use the guided render preflight',
  )
  assert.ok(
    videoCreationProfileForId('voice_visual').requiredLocalCommands.includes('VIDEO_FRAME_QA'),
    'voice/knowledge profile should internally track visual frame QA as a requirement',
  )
  assert.ok(
    videoCreationProfileForId('aigc_shot').requiredLocalCommands.includes('LOCAL_FILE_IMPORT'),
    'cinematic profile should internally track local import as a requirement',
  )
  const layerDisplays = buildTalkingHeadLayerDisplays([
    { id: 'audio-r2', kind: 'AUDIO_MASTER_TIMELINE', status: 'valid', metadata: { revision: 'audio-r2', executionMode: 'real', productionEligible: true } },
    { id: 'text-r3', kind: 'TEXT_LAYER', status: 'stale', metadata: { staleReason: 'subtitle theme changed', revision: 'text-r3' } },
    { id: 'broll-fixture', kind: 'BROLL_MANIFEST', status: 'pending', metadata: { executionMode: 'fixture', productionEligible: false } },
  ])
  assert.equal(layerDisplays.find((layer) => layer.key === 'audio').status, 'current')
  assert.equal(layerDisplays.find((layer) => layer.key === 'text').staleReason, 'subtitle theme changed')
  assert.equal(layerDisplays.find((layer) => layer.key === 'broll').executionMode, 'fixture')
  assert.equal(layerDisplays.find((layer) => layer.key === 'composition').status, 'pending')

  const healthItems = buildEnvironmentChecklist({
    serviceStatus: 'unknown',
    selectedProfile: videoCreationProfileForId('aigc_shot'),
    preflight: {
      pipeline: 'wf-aigc-shot-video',
      status: 'blocked',
      canStart: false,
      capabilityMenu: {
        localRunner: { available: true },
        compositionRuntime: { hyperframes: { available: true } },
        localTools: [
          { command: 'LOCAL_FILE_IMPORT', available: true },
          { command: 'FFMPEG_PROBE', available: false },
          { command: 'ARTIFACT_PACKAGE', available: true },
        ],
        warnings: [],
      },
      blockers: [{ code: 'FFMPEG_PROBE_NOT_AVAILABLE', message: '本地未检测到 FFMPEG_PROBE 执行能力' }],
    },
    modelProviderState: 'missing',
    missingModelCapabilities: ['text_to_video'],
  })
  assert.ok(
    healthItems.some((item) => item.id === 'local-runner' && item.status === 'passed'),
    'environment checklist should trust cloud runner availability when web service status is unknown',
  )
  assert.ok(
    healthItems.some((item) => item.id === 'model-provider' && item.status === 'warning' && item.actionLabel === '打开设置'),
    'manual-import environment checklist should not block when only external image/video providers are missing',
  )
  assert.ok(
    healthItems.some((item) => item.id === 'local-tool-FFMPEG_PROBE' && item.status === 'blocked'),
    'environment checklist should show exact missing local tool blockers',
  )
  const visibleHealthIssues = visibleEnvironmentIssues(healthItems)
  assert.ok(
    visibleHealthIssues.some((item) => item.id === 'local-capabilities' && item.status === 'blocked'),
    'overview should aggregate local tool blockers into a user-facing local capability issue',
  )
  assert.ok(
    visibleHealthIssues.every((item) => !item.id.startsWith('local-tool-') && !item.detail.includes('FFMPEG_PROBE')),
    'overview-visible environment issues should not expose concrete local command names',
  )
  const aigcProviderIssueItems = buildEnvironmentChecklist({
    serviceStatus: 'ok',
    selectedProfile: videoCreationProfileForId('aigc_shot'),
    preflight: {
      pipeline: 'wf-aigc-shot-video',
      status: 'passed',
      canStart: true,
      capabilityMenu: {
        localRunner: { available: true },
        localTools: [
          { command: 'LOCAL_FILE_IMPORT', available: true },
          { command: 'FFMPEG_PROBE', available: true },
          { command: 'ARTIFACT_PACKAGE', available: true },
        ],
      },
      blockers: [],
    },
    modelProviderState: 'missing',
    missingModelCapabilities: ['text_to_image', 'text_to_video'],
  })
  const aigcProviderIssue = aigcProviderIssueItems.find((item) => item.id === 'model-provider')
  assert.equal(aigcProviderIssue?.status, 'warning')
  assert.equal(aigcProviderIssue?.blockerCode, 'MODEL_PROVIDER_MEDIA_OPTIONAL')
  assert.ok(
    aigcProviderIssue?.detail.includes('文生图片、文生视频') &&
      aigcProviderIssue.detail.includes('Dreamina CLI / MCP') &&
      aigcProviderIssue.detail.includes('上传回填'),
    'AIGC entry should treat missing image/video providers as an optional external-generation reminder',
  )
  const visibleIssues = visibleEnvironmentIssues(aigcProviderIssueItems)
  assert.deepEqual(
    visibleIssues.map((item) => item.id),
    [],
    'project overview should hide optional image/video provider reminders from startup health',
  )
  const missingTextProviderItems = buildEnvironmentChecklist({
    serviceStatus: 'ok',
    selectedProfile: videoCreationProfileForId('aigc_shot'),
    preflight: {
      pipeline: 'wf-aigc-shot-video',
      status: 'passed',
      canStart: true,
      capabilityMenu: {
        localRunner: { available: true },
        localTools: [
          { command: 'LOCAL_FILE_IMPORT', available: true },
          { command: 'FFMPEG_PROBE', available: true },
          { command: 'ARTIFACT_PACKAGE', available: true },
        ],
      },
      blockers: [],
    },
    modelProviderState: 'missing',
    missingModelCapabilities: ['text_to_text', 'text_to_image'],
  })
  const missingTextProviderIssue = missingTextProviderItems.find((item) => item.id === 'model-provider')
  assert.equal(missingTextProviderIssue?.status, 'blocked')
  assert.equal(missingTextProviderIssue?.blockerCode, 'MODEL_PROVIDER_TEXT_MISSING')
  assert.deepEqual(
    visibleEnvironmentIssues(missingTextProviderItems).map((item) => item.id),
    ['model-provider'],
    'project overview should still show text model provider issues because script and planning generation need them',
  )

  const externalTaskPackage = buildExternalGenerationTaskPackage({
    requestId: 'extgen_video_SHOT_01',
    kind: 'video',
    shotId: 'SHOT_01',
    prompt: 'A cinematic football underdog scene',
    negativePrompt: 'low quality, blurry',
    references: [
      { id: 'ref-player', label: '主角参考', role: 'character', storageRef: 'local://projects/vp-1/artifacts/ref-player/content.png', locks: ['角色发型', '服装颜色'] },
      { id: 'storyboard-1', label: '故事板 1', role: 'storyboard', storageRef: 'local://projects/vp-1/artifacts/storyboard-1/content.png' },
    ],
    target: { aspectRatio: '16:9', durationSec: 5, resolution: '1080p' },
    promptCharLimit: 2000,
    referenceImageLimit: 6,
  })
  assert.ok(externalTaskPackage.fullText.includes('完整 AI 生成参考资料'), 'external package should be copyable as complete non-technical AI generation material')
  assert.ok(externalTaskPackage.fullText.includes('文字提示词'), 'external package should label the prompt in user-facing Chinese')
  assert.ok(externalTaskPackage.fullText.includes('图片参考资料'), 'external package should label image references as user-facing materials')
  assert.ok(externalTaskPackage.fullText.includes('Positive Prompt'), 'external package should include a positive prompt section')
  assert.ok(externalTaskPackage.fullText.includes('Negative Prompt'), 'external package should include a negative prompt section')
  assert.ok(externalTaskPackage.referenceManifest.includes('参考图 1'), 'external package should include ordered reference image manifest')
  assert.ok(externalTaskPackage.fullText.includes('上传回填'), 'external package should tell users how to upload the result back')
  assert.ok(externalTaskPackage.referenceManifest.includes('锁定：角色发型、服装颜色'), 'external package should preserve reference lock constraints')
  assert.equal(
    externalGenerationReferenceCopyText({
      id: 'ref-player',
      label: '主角参考',
      role: 'character',
      storageRef: 'local://projects/vp-1/artifacts/ref-player/content.png',
      locks: ['角色发型', '服装颜色'],
    }, 1),
    [
      '参考图 1: 主角参考',
      'ID: ref-player',
      '用途: character',
      '锁定: 角色发型、服装颜色',
      '文件: local://projects/vp-1/artifacts/ref-player/content.png',
    ].join('\n'),
    'single reference copy text should include id, role, locks, and storage ref',
  )

  const deliveryItems = buildExportDeliveryItems([
    finalVideoCandidates[2],
    {
      id: 'shot-qa-report',
      name: 'shot_qa_reports.json',
      kind: 'SHOT_QA_REPORT',
      status: 'valid',
      owner: '视觉质量审核',
      version: '第1版',
      updatedAt: '-',
      humanApproved: false,
      storageRef: 'local://projects/vp-1/reports/video_frame_qa/shot_qa_reports.json',
      metadata: { nextAction: 'RERENDER_HTML' },
    },
    {
      id: 'probe-report',
      name: 'ffmpeg_probe.json',
      kind: 'FFMPEG_PROBE_REPORT',
      status: 'valid',
      owner: '质量审核',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/probe-report/report.json',
      metadata: {},
    },
    {
      id: 'publish-copy',
      name: 'publish-copy.md',
      kind: 'PUBLISH_COPY',
      status: 'valid',
      owner: '文案生成',
      version: '第1版',
      updatedAt: '-',
      humanApproved: true,
      storageRef: 'local://projects/vp-1/artifacts/publish-copy/publish.md',
      metadata: { artifactType: 'publish_copy' },
    },
  ])
  assert.deepEqual(
    deliveryItems.map((item) => item.id),
    ['final-video', 'project-package', 'publish-copy', 'quality-report', 'folder-entry'],
    'export delivery checklist should render stable required rows even when optional artifacts are missing',
  )
  assert.equal(
    deliveryItems.find((item) => item.id === 'project-package')?.status,
    'missing',
    'export delivery checklist should turn missing package into a friendly state instead of artifact not found',
  )
  assert.equal(
    deliveryItems.find((item) => item.id === 'quality-report')?.storageRef,
    'local://projects/vp-1/reports/video_frame_qa/shot_qa_reports.json',
    'export delivery checklist should prefer shot-level QA over low-level probe reports',
  )
  assert.equal(
    normalizeDirectorErrorMessage(new Error('artifact not found')),
    '产物文件还没有同步到本机，请重新生成或回到产物页确认该文件是否已上传。',
    'raw artifact not found errors should not be exposed to beta users',
  )

  const pageSource = await readFile(new URL('../src/pages/DirectorStudioPage.tsx', import.meta.url), 'utf8')
  assert.ok(
    pageSource.includes('card col-span-12 overflow-visible p-0'),
    'review page shell must not clip inner review panel borders',
  )
  assert.ok(
    pageSource.includes('mt-4 flex gap-2 overflow-x-auto px-1 py-1'),
    'review history scroller needs padding so item borders are not clipped',
  )
  assert.ok(
    pageSource.includes('reviewOutputPanelTitle(selectedReview)') &&
      pageSource.includes('reviewOutputPanelHint(selectedReview)') &&
      !pageSource.includes('审核阶段产物'),
    'review page should name supporting knowledge as script evidence instead of a generic peer artifact',
  )
  assert.ok(
    pageSource.includes('查看产物页提示词和上传入口') &&
      pageSource.includes('onGoAssets') &&
      pageSource.includes('isShotProductionReview(selectedReview)'),
    'shot production quality gate should provide a visible jump to the assets page for prompts, reference images, and uploads',
  )
  assert.ok(
    pageSource.includes('ShotProductionGuideCard') &&
      pageSource.includes('externalGenerationRequestArtifactsForSlot') &&
      pageSource.includes('shotRequestPreviewEntries(openGroup, promptPreviews)'),
    'shot asset workbench should surface readable shot scripts and generation prompts even after a clip has been generated',
  )
  assert.ok(
    pageSource.includes('shotNarrationForDisplay(group, requestEntries)') &&
      pageSource.includes('shotVisualForDisplay(group, requestEntries, narrationText)') &&
      pageSource.includes('normalizeReadableText') &&
      !pageSource.includes('当前 shot 尚未登记口播脚本。'),
    'shot asset workbench should fall back to request-level narration and avoid repeating narration as visual copy',
  )
  assert.ok(
    pageSource.includes('shotActionSummary(group, requestEntries)') &&
      !pageSource.includes("{group.visualText || group.narrationText || '当前 shot 暂无画面摘要。'}") &&
      !pageSource.includes('ShotTextBlock title="画面说明" value={openGroup.visualText'),
    'shot asset workbench should not repeat the exact visual description as both summary and detail',
  )
  assert.ok(
    pageSource.includes('ShotRequestSummaryCard') &&
      pageSource.includes('素材操作') &&
      pageSource.includes('AIGC 素材提示词') &&
      pageSource.includes('CompactLayerPlanRow') &&
      pageSource.includes('safePromptText(request.prompt)') &&
      pageSource.includes('通过并锁定') &&
      pageSource.includes('已通过，内容已锁定') &&
      pageSource.includes('按要求重新生成提示词') &&
      pageSource.includes('参考图') &&
      pageSource.includes('新增参考图') &&
      !pageSource.includes('浏览器手动生成步骤') &&
      !pageSource.includes('requestDependencyRows') &&
      !pageSource.includes('generationStepLabels') &&
      !pageSource.includes('const guideSteps = externalGenerationGuideSteps(request)'),
    'shot asset workbench should keep material cards focused while still allowing prompt/reference edits and approval locks',
  )
  assert.ok(
    pageSource.includes('Shot 三层画面设计') &&
      pageSource.includes('IP A-roll / 3D 角色拍摄层') &&
      pageSource.includes('HyperFrames / HyperKeyframes 文字特效层') &&
      pageSource.includes('AIGC 丰富层 / 背景与 B-roll') &&
      pageSource.includes('已设计 · 本次不执行') &&
      pageSource.includes('visualLayersFromUnknown'),
    'every Shot must visibly distinguish the IP A-roll, deterministic text/effects, and optional AIGC enrichment layers',
  )
  assert.ok(
    pageSource.includes('disabled={approved}') &&
      pageSource.includes('setApproved(true)') &&
      pageSource.includes('regeneratePromptDraft(') &&
      pageSource.includes('setReferenceDrafts'),
    'shot prompt and reference editing must become read-only after user approval',
  )
  assert.ok(
    pageSource.includes('IP A-roll / 3D 角色拍摄层') &&
      pageSource.includes('AIGC 丰富层 / 背景与 B-roll') &&
      pageSource.includes('HyperFrames / HyperKeyframes 文字特效层') &&
      pageSource.includes('FFmpeg 融合') &&
      pageSource.includes('request.ipArollPlan') &&
      pageSource.includes('request.aigcPlan') &&
      pageSource.includes('request.hyperframesPlan') &&
      pageSource.includes('request.ffmpegFusionPlan') &&
      pageSource.includes('避免乱码') &&
      pageSource.includes('素材操作') &&
      pageSource.includes('AIGC 素材提示词') &&
      pageSource.includes('CompactLayerPlanRow') &&
      !pageSource.includes('给提示词生成要求') &&
      !pageSource.includes('const guideSteps = generationStepLabels(editableRequest)') &&
      !pageSource.includes('const dependencyRows = requestDependencyRows(editableRequest)'),
    'shot material cards should provide a compact, non-repetitive AIGC/HyperFrames/FFmpeg workflow instead of noisy repeated sections',
  )
  assert.ok(
    pageSource.includes('ShotArtifactPreview') &&
      pageSource.includes('<video') &&
      pageSource.includes('controls'),
    'shot asset workbench should support inline image and video previews for uploaded or generated assets',
  )
  assert.ok(
    !pageSource.includes('完整 AI 生成参考资料'),
    'shot generation cards should not expose technical package wording as the primary user-facing title',
  )
  assert.ok(
    !pageSource.includes('复制参数') &&
      !pageSource.includes('下载包') &&
      !pageSource.includes('复制参考图'),
    'shot generation cards should keep visible actions focused on copying/editing prompts and viewing references',
  )
  assert.ok(
    pageSource.includes('门禁目标产物') &&
      pageSource.includes('qualityGateTargetLines(selectedReview)'),
    'review page should show which artifact a quality gate is checking when multiple artifacts are produced in parallel',
  )
  assert.ok(
    pageSource.includes("onAction: (action: 'approve' | 'reject' | 'edit' | 'regenerate', targetReview?: AgentReviewItem) => void") &&
      pageSource.includes("onAction('approve', selectedReview)") &&
      pageSource.includes("onAction('reject', selectedReview)") &&
      pageSource.includes("onAction('edit', selectedReview)") &&
      pageSource.includes("onAction('regenerate', selectedReview)"),
    'review decision buttons must act on the selected review card, not the default active review',
  )
  assert.ok(
    pageSource.includes('createVideoProject'),
    'director studio should create a video project before starting a dynamic agent run',
  )
  assert.ok(
    pageSource.includes('fetchProjectArtifacts'),
    'director studio should refresh project artifacts as part of the shared state source',
  )
  assert.ok(
    pageSource.includes('projectId: nextProject.id'),
    'dynamic agent run context must include the bound project id',
  )
  assert.ok(pageSource.includes('ShotRequestSummaryCard'), 'external generation card should use progressive shot request summaries')
  assert.ok(pageSource.includes('复制提示词'), 'external generation card should keep the primary visible action focused on prompt copying')
  assert.ok(pageSource.includes('可选负面提示词'), 'external generation card should keep negative prompts available without adding another primary button')
  assert.ok(pageSource.includes('复制参考信息'), 'external generation card should expose reference copying')
  assert.ok(
    pageSource.includes('展开处理素材任务') &&
      pageSource.includes('素材操作') &&
      pageSource.includes('AIGC 素材提示词') &&
      pageSource.includes('参考图') &&
      pageSource.includes('通过并锁定') &&
      pageSource.includes('按要求重新生成提示词') &&
      pageSource.includes('新增参考图') &&
      pageSource.includes('正在读取参考图') &&
      pageSource.includes('mergeExternalGenerationTaskReferences') &&
      !pageSource.includes('浏览器手动生成步骤'),
    'assets page should show focused editable prompt and reference materials directly in the client',
  )
  assert.ok(
    !pageSource.includes('installJiMengCLI') &&
      !pageSource.includes('registerJiMengMCP') &&
      !pageSource.includes('loginJiMengHeadless') &&
      !pageSource.includes('checkJiMengLogin'),
    'project page should not own JiMeng CLI setup actions; it should link to settings',
  )
  assert.ok(
    pageSource.includes('配置即梦 CLI') && pageSource.includes('onOpenSettings'),
    'project page should keep only a simple JiMeng settings explainer and settings jump',
  )
  assert.ok(
    !pageSource.includes('profile.requiredLocalCommands.map'),
    'project profile cards should not expose concrete local command names to users',
  )
  assert.ok(
    pageSource.includes('visibleEnvironmentIssues(items)'),
    'project environment panel should filter to visible issues instead of rendering the full technical checklist',
  )
  assert.ok(
    !pageSource.includes('preflight.blockers[0].message'),
    'project overview should not print raw preflight blocker messages with tool names',
  )

  const desktopSource = await readFile(new URL('../src/pages/SettingsPage.tsx', import.meta.url), 'utf8')
  const brandSource = await readFile(new URL('../src/utils/brand.ts', import.meta.url), 'utf8')
  const packageSource = await readFile(new URL('../package.json', import.meta.url), 'utf8')
  const indexSource = await readFile(new URL('../index.html', import.meta.url), 'utf8')
  const electronSource = await readFile(new URL('../electron/main.cjs', import.meta.url), 'utf8')
  assert.ok(
    brandSource.includes("APP_NAME = '躺营AI视频创作助手'") &&
      indexSource.includes('<title>躺营AI视频创作助手</title>') &&
      packageSource.includes('"productName": "Tangying AI Video Creation Assistant"') &&
      electronSource.includes("title: '躺营AI视频创作助手'"),
    'desktop package name should be English while the in-app Chinese brand remains unchanged',
  )
  assert.ok(
    !brandSource.includes('自媒体运营助手') &&
      !packageSource.includes('自媒体内容运营系统') &&
      !indexSource.includes('自媒体运营助手') &&
      !electronSource.includes('自媒体运营助手'),
    'desktop app user-facing branding should not claim social media operation assistant capability',
  )
  assert.ok(
    desktopSource.includes('installJiMengCLI') &&
      desktopSource.includes('registerJiMengMCP') &&
      desktopSource.includes('loginJiMengHeadless') &&
      desktopSource.includes('checkJiMengLogin'),
    'settings page should own JiMeng CLI install, MCP registration, and login actions',
  )
  assert.ok(
    desktopSource.includes('即梦 CLI') && desktopSource.includes('文生图片') && desktopSource.includes('文生视频'),
    'settings page should present JiMeng CLI alongside image and video generation providers',
  )
  assert.ok(
    desktopSource.includes('正在安装/更新 Dreamina CLI') &&
      desktopSource.includes('Dreamina CLI 安装/更新完成') &&
      desktopSource.includes("type: 'error'"),
    'JiMeng CLI install/update must surface running, success, and failure feedback to users',
  )
  assert.ok(
    desktopSource.includes('正在注册即梦 MCP') &&
      desktopSource.includes('即梦 MCP 已注册') &&
      desktopSource.includes('正在刷新即梦配置状态'),
    'JiMeng settings actions must show visible progress and result feedback',
  )
  assert.ok(
    desktopSource.includes('message={jimengSetupMessage}') &&
      desktopSource.includes('message={loginMessage}') &&
      desktopSource.includes('message={providerMessage}') &&
      desktopSource.includes("message.type === 'info'"),
    'settings panels should render action feedback for provider, JiMeng setup, and login interactions',
  )
  assert.ok(
    desktopSource.includes('setDirectoryMessage') &&
      desktopSource.includes('已选择本地目录') &&
      desktopSource.includes('未选择新目录'),
    'local directory selection should give feedback for selected, cancelled, and failed outcomes',
  )
  assert.ok(
    desktopSource.includes('复制失败'),
    'copy actions should expose failure feedback instead of silently swallowing clipboard errors',
  )

  const apiSource = await readFile(new URL('../src/services/api.ts', import.meta.url), 'utf8')
  const agentRunTimeout = Number(apiSource.match(/const AGENT_RUN_REQUEST_TIMEOUT_MS = (\d+)/)?.[1] || 0)
  assert.ok(
    agentRunTimeout >= 300000,
    'agent run start timeout must allow slow provider-backed planning and fallback',
  )
  assert.ok(
    apiSource.includes('AGENT_RUN_REQUEST_TIMEOUT_MS') &&
      /api\.post[\s\S]*?\('\/agent\/runs', payload,\s*\{[\s\S]*timeout:\s*AGENT_RUN_REQUEST_TIMEOUT_MS/.test(apiSource),
    'startAgentRun must override the default 30000ms axios timeout for slower provider-backed planning',
  )

  console.log('director studio logic checks passed')
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
