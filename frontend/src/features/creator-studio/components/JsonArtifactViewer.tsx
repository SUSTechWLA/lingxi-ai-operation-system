import { useMemo, type RefObject } from 'react'
import type { ArtifactReviewModel } from '../artifactPresentation'
import { projectCreatorReviewContent } from '../creatorReviewProjection'
import type { TextSelectionDraft } from '../textSelection'
import ReviewableTextSurface from './ReviewableTextSurface'

interface JsonArtifactViewerProps {
  content: unknown
  selectionSource?: string
  selectionEnabled?: boolean
  textSurfaceRef?: RefObject<HTMLElement>
  onTextSelectionChange?: (draft: TextSelectionDraft | null) => void
}

export default function JsonArtifactViewer({
  content,
  selectionSource,
  selectionEnabled = false,
  textSurfaceRef,
  onTextSelectionChange,
}: JsonArtifactViewerProps) {
  const projection = useMemo(() => projectCreatorReviewContent(content), [content])
  const reviewModel = projection.document
  const reviewSource = projection.canonicalText ?? selectionSource
  const canSelect = selectionEnabled && reviewSource !== undefined && reviewSource.length > 0 && textSurfaceRef && onTextSelectionChange

  if (!reviewModel && !projection.canonicalText && !canSelect) {
    return (
      <section className="artifact-json-viewer" aria-label="内容审阅">
        <div className="artifact-parse-warning" role="alert">
          <strong>关键内容仍在准备中</strong>
          <p>{projection.excerpt}</p>
        </div>
      </section>
    )
  }

  return (
    <section className="artifact-json-viewer" aria-label="内容审阅">
      {canSelect
        ? <ReviewableTextSurface source={reviewSource} surfaceRef={textSurfaceRef} onSelectionChange={onTextSelectionChange} />
        : reviewModel
          ? <ArtifactReviewDocument model={reviewModel} />
          : <PlainReviewDocument text={reviewSource ?? projection.excerpt} />}
      {selectionEnabled && !canSelect && reviewModel && <p className="artifact-selection-unavailable">结构化内容可整体优化，局部划选暂不可用。</p>}
    </section>
  )
}

function PlainReviewDocument({ text }: { text: string }) {
  return (
    <article className="artifact-review-document">
      <section className="artifact-review-script" aria-label="可审阅正文">
        <div className="artifact-review-section-heading">
          <p className="creator-eyebrow">可审阅正文</p>
          <span>建议逐句朗读检查</span>
        </div>
        <p>{text}</p>
      </section>
    </article>
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
