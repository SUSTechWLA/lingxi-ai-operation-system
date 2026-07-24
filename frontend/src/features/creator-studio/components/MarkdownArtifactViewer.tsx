import type { ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import { markdownHeadings, safeCreatorReviewText } from '../artifactPresentation'
import JsonArtifactViewer from './JsonArtifactViewer'

export default function MarkdownArtifactViewer({ content }: { content: unknown }) {
  const markdown = safeCreatorReviewText(content)
  if (markdown === undefined) return <JsonArtifactViewer content={content} />
  const headings = markdownHeadings(markdown)

  return (
    <section className="artifact-markdown-viewer" aria-label="文稿审阅">
      <div className="artifact-markdown-layout">
        {headings.length > 1 && (
          <nav className="artifact-markdown-toc" aria-label="文档目录">
            <strong>目录</strong>
            <ol>{headings.map(heading => <li key={`${heading.id}-${heading.depth}`} className={`depth-${heading.depth}`}><a href={`#${heading.id}`}>{heading.text}</a></li>)}</ol>
          </nav>
        )}
        <article className="artifact-markdown-preview">
          <ReactMarkdown components={{
            h1: ({ children }) => <h1 id={headingId(children, headings)}>{children}</h1>,
            h2: ({ children }) => <h2 id={headingId(children, headings)}>{children}</h2>,
            h3: ({ children }) => <h3 id={headingId(children, headings)}>{children}</h3>,
            a: ({ href, children }) => <a href={safeMarkdownHref(href)} target="_blank" rel="noreferrer">{children}</a>,
          }}>{markdown}</ReactMarkdown>
        </article>
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
