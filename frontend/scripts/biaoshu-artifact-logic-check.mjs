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
    displayNameForBiaoshuArtifact,
    biaoshuArtifactToCopyText,
  } = await import(pathToFileURL(outfile))

  const run = {
    id: 'run-bid-1',
    status: 'RUNNING',
    domain: 'bid_writing',
    message: '请生成技术标',
    createdAt: '2026-06-29T08:00:00Z',
    updatedAt: '2026-06-29T08:01:00Z',
    plan: {
      goal: '生成技术标文档',
      domain: 'bid_writing',
      mode: 'dynamic_agent',
      steps: [
        { id: 'parse', tool: 'parse_bid_files', arguments: {}, expectedOutput: ['BID_ANALYSIS'] },
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
          summary: '已解析评分办法和废标条款',
          artifacts: [
            {
              kind: 'BID_ANALYSIS',
              name: '招标文件解析结果',
              storageRef: 'local://artifacts/bid/analysis.json',
            },
          ],
        },
        completedAt: '2026-06-29T08:02:00Z',
      },
      {
        id: 'word_exec',
        name: 'external',
        status: 'SUCCESS',
        input: { capabilityTool: 'convert_to_word' },
        output: {
          output_file: 'E:/bid/out/广惠高速改扩建_技术标.docx',
          summary: 'Word 文档已导出',
        },
        completedAt: '2026-06-29T08:05:00Z',
      },
    ],
  })

  assert.equal(artifacts.length, 3)
  assert.equal(artifacts[0].kind, 'BID_ANALYSIS')
  assert.equal(artifacts[0].status, 'valid')
  assert.equal(artifacts[0].storageRef, 'local://artifacts/bid/analysis.json')
  assert.equal(artifacts[1].kind, 'BID_OUTLINE')
  assert.equal(artifacts[1].status, 'pending')
  assert.equal(artifacts[2].kind, 'TECHNICAL_BID_DOCX')
  assert.equal(artifacts[2].storageRef, 'E:/bid/out/广惠高速改扩建_技术标.docx')
  assert.equal(displayNameForBiaoshuArtifact('MERGED_DRAFT'), '整合成稿')

  const copyText = biaoshuArtifactToCopyText(artifacts[2])
  assert.match(copyText, /TECHNICAL_BID_DOCX/)
  assert.match(copyText, /Word 文档已导出/)

  console.log('biaoshuArtifactLogic tests passed')
} finally {
  await rm(tempDir, { recursive: true, force: true })
}
