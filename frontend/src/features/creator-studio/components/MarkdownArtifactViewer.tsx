import { useEffect, useState, type ReactNode, type RefObject } from 'react'
import ReactMarkdown from 'react-markdown'
import { markdownHeadings, safeCreatorReviewText } from '../artifactPresentation'
import type { TextSelectionDraft } from '../textSelection'
import JsonArtifactViewer from './JsonArtifactViewer'
import { CanonicalTextSelectionSurface } from './TextSelectionAssistant'

interface MarkdownArtifactViewerProps {
  content: unknown
  selectionSource?: string
  selectionEnabled?: boolean
  textSurfaceRef?: RefObject<HTMLPreElement>
  onTextSelectionChange?: (draft: TextSelectionDraft | null) => void
}

export default function MarkdownArtifactViewer({
  content,
  selectionSource,
  selectionEnabled = false,
  textSurfaceRef,
  onTextSelectionChange,
}: MarkdownArtifactViewerProps) {
  const markdown = safeCreatorReviewText(content)
  const [selecting, setSelecting] = useState(false)
  useEffect(() => setSelecting(false), [selectionSource])
  if (markdown === undefined) {
    return <JsonArtifactViewer content={content}
      selectionSource={selectionSource}
      selectionEnabled={selectionEnabled}
      textSurfaceRef={textSurfaceRef}
      onTextSelectionChange={onTextSelectionChange}
    />
  }
  const headings = markdownHeadings(markdown)
  const canSelect = selectionEnabled && selectionSource !== undefined && selectionSource.length > 0 && textSurfaceRef && onTextSelectionChange

  return (
    <section className="artifact-markdown-viewer" aria-label="文稿审阅">
      {canSelect && (
        <div className="artifact-selection-mode">
          <button type="button" className="creator-text-button" aria-pressed={selecting} onClick={() => {
            setSelecting(current => !current)
            onTextSelectionChange(null)
          }}>{selecting ? '返回排版阅读' : '划选文字优化'}</button>
          <span>{selecting ? '当前显示与接口完全一致的原稿，可精确划选。' : '局部优化会先切换到精确原稿。'}</span>
        </div>
      )}
      <div className="artifact-markdown-layout">
        {!selecting && headings.length > 1 && (
          <nav className="artifact-markdown-toc" aria-label="文档目录">
            <strong>目录</strong>
            <ol>{headings.map(heading => <li key={`${heading.id}-${heading.depth}`} className={`depth-${heading.depth}`}><a href={`#${heading.id}`}>{heading.text}</a></li>)}</ol>
          </nav>
        )}
        {selecting && canSelect
          ? <CanonicalTextSelectionSurface source={selectionSource} surfaceRef={textSurfaceRef} onSelectionChange={onTextSelectionChange} />
          : <article className="artifact-markdown-preview">
          <ReactMarkdown components={{
            h1: ({ children }) => <h1 id={headingId(children, headings)}>{children}</h1>,
            h2: ({ children }) => <h2 id={headingId(children, headings)}>{children}</h2>,
            h3: ({ children }) => <h3 id={headingId(children, headings)}>{children}</h3>,
            a: ({ href, children }) => <a href={safeMarkdownHref(href)} target="_blank" rel="noreferrer">{children}</a>,
          }}>{markdown}</ReactMarkdown>
          </article>}
      </div>
    </section>
  )
}

function headingId(children: ReactNode, headings: ReturnType<typeof markdownHeadings>): string | undefined {
  const text = flattenText(children)
  return headings.find(heading => heading.text === text)?.id
}

function flattenText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(flattenText).join('')
  return ''
}

function safeMarkdownHref(href?: string): string | undefined {
  if (!href) return undefined
  if (href.startsWith('#') || href.startsWith('/') || /^https?:\/\//i.test(href) || /^mailto:/i.test(href)) return href
  return undefined
}
