import assert from 'node:assert/strict'

import { createExpandedChapterArtifact, createManualChapterArtifact } from '../src/pages/biaoshuArtifactLogic.ts'
import { buildChapterWorkspace } from '../src/pages/biaoshuChapterWorkspaceLogic.ts'

const chapter = createManualChapterArtifact(
  {
    id: 'artifact_bid_chapters',
    chapterNumber: 7,
    chapterTitle: '第七章 人员配置及职责分工',
    metadata: {
      chapterNumber: 7,
      targetWords: 2000,
    },
  },
  'E:/project/章节/第七章 人员配置及职责分工.md',
)

assert.equal(
  chapter.id,
  'chapter-7',
  'a chapter draft must use its chapter number as the persistent identity',
)
assert.equal(chapter.metadata?.chapterNumber, 7)

const expandedChapter = createExpandedChapterArtifact(
  { id: 'artifact_bid_chapters', metadata: { wordCount: 2100 } },
  { chapterNumber: 7, chapterTitle: '第七章 人员配置及职责分工', expandedPath: 'E:/project/章节/第七章_扩写.md', wordCountBefore: 1280, wordCountAfter: 2100, targetWordCount: 2000 },
)
assert.equal(expandedChapter.id, 'chapter-7-expanded')
assert.equal(expandedChapter.metadata?.chapterNumber, 7)

const workspace = buildChapterWorkspace([
  chapter,
  {
    ...expandedChapter,
    updatedAt: '2026-07-10T12:00:00Z',
  },
  {
    id: 'chapter-2',
    name: '第二章 日常种植方案',
    kind: 'BID_CHAPTERS',
    version: '-',
    status: 'valid',
    owner: 'AI写作',
    updatedAt: '2026-07-10T11:00:00Z',
    storageRef: 'E:/project/章节/第二章 日常种植方案.md',
    summary: '初稿',
    sourceTool: 'chapter_generation',
    metadata: { chapterNumber: 2, targetWords: 1800, wordCount: 1200 },
  },
])

assert.deepEqual(workspace.chapters.map((item) => item.chapterNumber), [2, 7])
assert.equal(workspace.chapters[1].versions.length, 2)
assert.equal(workspace.chapters[1].current.id, 'chapter-7-expanded')
assert.deepEqual(workspace.summary, { total: 2, ready: 1, short: 1, unclassified: 0 })

console.log('biaoshu chapter workspace tests passed')
