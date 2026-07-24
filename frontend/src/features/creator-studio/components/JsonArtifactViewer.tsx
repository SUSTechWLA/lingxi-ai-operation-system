import { useMemo } from 'react'
import {
  buildArtifactReviewModel,
  parseArtifactJson,
  type ArtifactReviewModel,
} from '../artifactPresentation'

interface JsonArtifactViewerProps {
  content: unknown
}

export default function JsonArtifactViewer({ content }: JsonArtifactViewerProps) {
  const parsed = useMemo(() => parseArtifactJson(content), [content])
  const reviewModel = parsed.ok ? buildArtifactReviewModel(parsed.value) : null

  if (!reviewModel) {
    return (
      <section className="artifact-json-viewer" aria-label="内容审阅">
        <div className="artifact-parse-warning" role="alert">
          <strong>关键内容仍在准备中</strong>
          <p>{parsed.ok ? '生成完成后会显示在这里。' : '暂时无法整理这份内容，请重新读取当前内容后再试。'}</p>
        </div>
      </section>
    )
  }

  return (
    <section className="artifact-json-viewer" aria-label="内容审阅">
      <ArtifactReviewDocument model={reviewModel} />
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
