import { useMemo, useState } from 'react'
import {
  buildArtifactReviewModel,
  buildJsonSummary,
  downloadText,
  filterJsonTree,
  homogeneousJsonColumns,
  jsonScalarText,
  parseArtifactJson,
  type ArtifactReviewModel,
} from '../artifactPresentation'

interface JsonArtifactViewerProps {
  content: unknown
  name: string
}

export default function JsonArtifactViewer({ content, name }: JsonArtifactViewerProps) {
  const parsed = useMemo(() => parseArtifactJson(content), [content])
  const [query, setQuery] = useState('')
  const [mode, setMode] = useState<'structured' | 'raw'>('structured')
  const [copyState, setCopyState] = useState('')
  const reviewModel = parsed.ok ? buildArtifactReviewModel(parsed.value) : null
  const summary = parsed.ok ? buildJsonSummary(parsed.value) : null
  const filtered = parsed.ok ? filterJsonTree(parsed.value, query) : undefined
  const raw = parsed.ok ? JSON.stringify(parsed.value, null, 2) : parsed.raw

  const copyRaw = async () => {
    try {
      await navigator.clipboard.writeText(raw)
      setCopyState('已复制')
    } catch {
      setCopyState('复制失败，请使用下载')
    }
  }

  if (!parsed.ok) {
    return (
      <section className="artifact-json-viewer" aria-label={`${name} JSON 审阅器`}>
        <div className="artifact-parse-warning" role="alert">
          <strong>这份内容暂时无法排版</strong>
          <p>仍可在下方查看原文。{parsed.error}</p>
          <pre>{parsed.raw}</pre>
        </div>
      </section>
    )
  }

  return (
    <section className="artifact-json-viewer" aria-label={`${name} JSON 审阅器`}>
      {reviewModel
        ? <ArtifactReviewDocument model={reviewModel} />
        : (
          <div className="artifact-json-overview">
            <div>
              <p className="creator-eyebrow">内容概览</p>
              <h3>结构化记录</h3>
              <p>复杂字段已收起，点击字段可按需查看。</p>
            </div>
            {filtered === undefined
              ? <p className="artifact-empty">没有找到可展示内容。</p>
              : <JsonTree value={filtered} depth={0} expandAll={false} />}
          </div>
        )}

      <details className="artifact-technical-data">
        <summary>
          <span>查看技术数据</span>
          <small>仅在排查字段或下载原文件时使用</small>
        </summary>
        <div className="artifact-technical-data-body">
          <header className="artifact-viewer-toolbar">
            <div className="artifact-viewer-tabs" role="tablist" aria-label="技术数据查看方式">
              <button type="button" role="tab" aria-selected={mode === 'structured'} onClick={() => setMode('structured')}>字段</button>
              <button type="button" role="tab" aria-selected={mode === 'raw'} onClick={() => setMode('raw')}>原文</button>
            </div>
            <div className="artifact-viewer-tools">
              <button type="button" onClick={() => void copyRaw()}>复制</button>
              <button type="button" onClick={() => downloadText(name || 'artifact.json', raw, 'application/json')}>下载</button>
            </div>
          </header>

          {summary && (
            <div className="artifact-json-summary" aria-label="技术数据摘要">
              <span>{summary.itemCount} 个顶层条目</span>
              <span>{summary.depth} 层结构</span>
            </div>
          )}
          <span className="sr-only" aria-live="polite">{copyState}</span>

          <div className="artifact-technical-scroll">
            {mode === 'raw' ? (
              <pre className="artifact-raw-preview" tabIndex={0}>{raw}</pre>
            ) : (
              <>
                <label className="artifact-json-search">
                  <span>搜索字段或内容</span>
                  <input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="例如：时长、镜头 03" />
                </label>
                {filtered === undefined
                  ? <p className="artifact-empty">没有找到匹配内容。</p>
                  : <JsonTree value={filtered} depth={0} expandAll={Boolean(query.trim())} />}
              </>
            )}
          </div>
        </div>
      </details>
    </section>
  )
}

function ArtifactReviewDocument({ model }: { model: ArtifactReviewModel }) {
  const visibleSections = model.sections.slice(0, 6)
  const remainingSections = model.sections.slice(6)
  return (
    <article className="artifact-review-document">
      <header className="artifact-review-document-heading">
        <div>
          <p className="creator-eyebrow">可审阅内容</p>
          <h3>{model.title}</h3>
          {model.summary && <p>{model.summary}</p>}
        </div>
        {model.metrics.length > 0 && (
          <dl className="artifact-review-metrics">
            {model.metrics.map(metric => <div key={metric.label}><dt>{metric.label}</dt><dd>{metric.value}</dd></div>)}
          </dl>
        )}
      </header>

      {model.script && (
        <section className="artifact-review-script" aria-labelledby="artifact-review-script-title">
          <div className="artifact-review-section-heading">
            <p className="creator-eyebrow">口播正文</p>
            <span>建议逐句朗读检查</span>
          </div>
          <h4 id="artifact-review-script-title" className="sr-only">口播正文</h4>
          <p>{model.script}</p>
        </section>
      )}

      {model.characters.length > 0 && (
        <section className="artifact-review-characters" aria-labelledby="artifact-review-characters-title">
          <h4 id="artifact-review-characters-title">出场角色</h4>
          <div>{model.characters.map(character => (
            <article key={character.name}>
              <strong>{character.name}</strong>
              {character.description && <p>{character.description}</p>}
            </article>
          ))}</div>
        </section>
      )}

      {model.sections.length > 0 && (
        <section className="artifact-review-sections" aria-labelledby="artifact-review-sections-title">
          <h4 id="artifact-review-sections-title">内容节奏</h4>
          <ol>
            {visibleSections.map((section, index) => <ReviewSection key={`${section.name}-${index}`} section={section} index={index} />)}
          </ol>
          {remainingSections.length > 0 && (
            <details>
              <summary>再查看 {remainingSections.length} 个段落</summary>
              <ol start={7}>
                {remainingSections.map((section, index) => <ReviewSection key={`${section.name}-${index + 6}`} section={section} index={index + 6} />)}
              </ol>
            </details>
          )}
        </section>
      )}

      {model.warnings.length > 0 && (
        <section className="artifact-review-warnings" aria-labelledby="artifact-review-warnings-title">
          <h4 id="artifact-review-warnings-title">需要留意</h4>
          <ul>{model.warnings.map(warning => <li key={warning}>{warning}</li>)}</ul>
        </section>
      )}
    </article>
  )
}

function ReviewSection({ section, index }: { section: ArtifactReviewModel['sections'][number]; index: number }) {
  return (
    <li>
      <span>{String(index + 1).padStart(2, '0')}</span>
      <div>
        <strong>{section.name}</strong>
        {section.text && <p>{section.text}</p>}
      </div>
      {section.duration && <em>{section.duration}</em>}
    </li>
  )
}

function JsonTree({ value, depth, expandAll }: { value: unknown; depth: number; expandAll: boolean }) {
  if (value === null || typeof value !== 'object') return <JsonScalar value={value} />
  if (Array.isArray(value)) {
    const columns = homogeneousJsonColumns(value)
    if (columns.length > 0) return <JsonTable rows={value as Record<string, unknown>[]} columns={columns} />
    return (
      <details className="json-branch" open={expandAll}>
        <summary>{value.length} 个数组项</summary>
        <ol>
          {value.map((item, index) => <li key={index}><span className="json-key">{index}</span><JsonTree value={item} depth={depth + 1} expandAll={expandAll} /></li>)}
        </ol>
      </details>
    )
  }
  const entries = Object.entries(value as Record<string, unknown>)
  return (
    <details className="json-branch" open={expandAll || depth === 0}>
      <summary>{entries.length} 个字段</summary>
      <dl>
        {entries.map(([key, child]) => (
          <div className="json-field" key={key}>
            <dt>{key}</dt>
            <dd><JsonTree value={child} depth={depth + 1} expandAll={expandAll} /></dd>
          </div>
        ))}
      </dl>
    </details>
  )
}

function JsonScalar({ value }: { value: unknown }) {
  const text = jsonScalarText(value)
  const kind = value === null ? 'null' : typeof value
  if (typeof value === 'string' && text.length > 220) {
    return (
      <details className={`json-long-scalar json-scalar is-${kind}`}>
        <summary>{text.slice(0, 180)}…</summary>
        <span>{text}</span>
      </details>
    )
  }
  return <span className={`json-scalar is-${kind}`}>{text}</span>
}

function JsonTable({ rows, columns }: { rows: Record<string, unknown>[]; columns: string[] }) {
  return (
    <div className="artifact-json-table-wrap" tabIndex={0}>
      <table className="artifact-json-table">
        <thead><tr>{columns.map(column => <th scope="col" key={column}>{column}</th>)}</tr></thead>
        <tbody>{rows.map((row, index) => <tr key={index}>{columns.map(column => <td key={column}>{jsonScalarText(row[column])}</td>)}</tr>)}</tbody>
      </table>
    </div>
  )
}
