import { useMemo, useState, type ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import { artifactContentText, downloadText, markdownHeadings } from '../artifactPresentation'

export default function MarkdownArtifactViewer({ content, name }: { content: unknown; name: string }) {
  const markdown = artifactContentText(content)
  const headings = useMemo(() => markdownHeadings(markdown), [markdown])
  const [mode, setMode] = useState<'preview' | 'source'>('preview')

  return (
    <section className="artifact-markdown-viewer" aria-label={`${name} Markdown 审阅器`}>
      <header className="artifact-viewer-toolbar">
        <div className="artifact-viewer-tabs" role="tablist" aria-label="Markdown 查看方式">
          <button type="button" role="tab" aria-selected={mode === 'preview'} onClick={() => setMode('preview')}>排版预览</button>
          <button type="button" role="tab" aria-selected={mode === 'source'} onClick={() => setMode('source')}>源码</button>
        </div>
        <div className="artifact-viewer-tools">
          <button type="button" onClick={() => void navigator.clipboard.writeText(markdown)}>复制</button>
          <button type="button" onClick={() => downloadText(name || 'artifact.md', markdown, 'text/markdown')}>下载</button>
        </div>
      </header>
      {mode === 'source' ? <pre className="artifact-raw-preview" tabIndex={0}>{markdown}</pre> : (
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
      )}
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
