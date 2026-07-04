import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const tempDir = await mkdtemp(join(tmpdir(), 'biaoshu-artifact-logic-'))
const outfile = join(tempDir, 'biaoshuArtifactLogic.mjs')

try {
  await build({
    entryPoints: [fileURLToPath(new URL('../src/pages/biaoshuArtifactLogic.ts', import.meta.url))],
    outfile,
    bundle: true,
    format: 'esm',
    platform: 'node',
    logLevel: 'silent',
  })

  const {
    buildBiaoshuArtifacts,
    createManualReportArtifact,
    displayNameForBiaoshuArtifact,
    biaoshuArtifactToCopyText,
    mergeManualReportArtifact,
  } = await import(pathToFileURL(outfile))

  const run = {
    id: 'run-bid-1',
    status: 'RUNNING',
    domain: 'bid_writing',
    message: 'generate technical bid',
    createdAt: '2026-06-29T08:00:00Z',
    updatedAt: '2026-06-29T08:01:00Z',
    plan: {
      goal: 'generate technical bid document',
      domain: 'bid_writing',
      mode: 'dynamic_agent',
      steps: [
        { id: 'parse', tool: 'parse_bid_files', arguments: {}, expectedOutput: ['BID_RAW_TEXT', 'BID_ANALYSIS'] },
        { id: 'outline', tool: 'outline_generator', arguments: {}, expectedOutput: ['BID_OUTLINE'] },
        { id: 'word', tool: 'convert_to_word', arguments: {}, expectedOutput: ['TECHNICAL_BID_DOCX'] },
      ],
    },
  }

  const artifacts = buildBiaoshuArtifacts(run, {
    nodes: [
      {
        id: 'parse_exec',
        name: 'parse_bid_files',
        status: 'SUCCESS',
        output: {
          summary: 'parsed tender raw text',
          stdout: JSON.stringify({
            raw_text_path: 'E:/bid/out/00_raw.md',
            report_path: 'E:/bid/out/00_report.md',
          }),
        },
        completedAt: '2026-06-29T08:02:00Z',
      },
      {
        id: 'word_exec',
        name: 'external',
        status: 'SUCCESS',
        input: { capabilityTool: 'convert_to_word' },
        output: {
          output_file: 'E:/bid/out/technical_bid.docx',
          summary: 'Word exported',
        },
        completedAt: '2026-06-29T08:05:00Z',
      },
    ],
  })

  assert.equal(artifacts.length, 4)
  assert.equal(artifacts[0].kind, 'BID_RAW_TEXT')
  assert.equal(artifacts[0].status, 'valid')
  assert.equal(artifacts[0].storageRef, 'E:/bid/out/00_raw.md')
  assert.equal(artifacts[1].kind, 'BID_ANALYSIS')
  assert.equal(artifacts[1].status, 'valid')
  assert.equal(artifacts[1].storageRef, 'E:/bid/out/00_report.md')
  assert.equal(artifacts[2].kind, 'BID_OUTLINE')
  assert.equal(artifacts[2].status, 'pending')
  assert.equal(artifacts[3].kind, 'TECHNICAL_BID_DOCX')
  assert.equal(artifacts[3].storageRef, 'E:/bid/out/technical_bid.docx')
  assert.equal(displayNameForBiaoshuArtifact('MERGED_DRAFT'), '整合成稿')

  const copyText = biaoshuArtifactToCopyText(artifacts[3])
  assert.match(copyText, /TECHNICAL_BID_DOCX/)
  assert.match(copyText, /Word exported/)

  const regeneratedReport = createManualReportArtifact(
    {
      id: 'manual-report-1',
      name: 'Bid analysis report',
      storageRef: 'E:/bid/out/00_regenerated_report.md',
      summary: 'Regenerated report',
      metadata: { previous: 'kept' },
    },
    'E:/bid/out/00_regenerated_report.md',
    'E:/bid/source.pdf',
  )
  const mergedArtifacts = mergeManualReportArtifact(artifacts, regeneratedReport)
  assert.equal(mergedArtifacts.length, artifacts.length)
  assert.equal(mergedArtifacts[1].kind, 'BID_ANALYSIS')
  assert.equal(mergedArtifacts[1].storageRef, 'E:/bid/out/00_regenerated_report.md')
  assert.equal(mergedArtifacts[1].metadata.sourceFile, 'E:/bid/source.pdf')
  assert.equal(mergedArtifacts[1].metadata.manualGenerated, true)

  console.log('biaoshuArtifactLogic tests passed')
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
