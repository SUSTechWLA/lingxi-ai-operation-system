import { useEffect, useState } from 'react'
import { fetchVideoProjects } from '../../services/api'
import { getCreationView } from '../../services/creatorApi'
import type { CreationView } from './types'
import type { VideoProject } from '../../utils/types'
import { creatorProjectProgress, mapWithConcurrency, prioritizeCreationViewProjects } from './logic'

interface VideoLibraryPageProps {
  onContinueProject: (projectId: string, stepId: string) => void
}

interface ProjectWithProgress {
  project: VideoProject
  view?: CreationView
}

export default function VideoLibraryPage({ onContinueProject }: VideoLibraryPageProps) {
  const [projects, setProjects] = useState<ProjectWithProgress[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    let active = true
    const controller = new AbortController()
    const load = async () => {
      setLoading(true)
      setError(false)
      try {
        const response = await fetchVideoProjects()
        const sorted = sortProjects(response.projects)
        if (!active) return
        setProjects(sorted.map(project => ({ project })))
        setLoading(false)
        void mapWithConcurrency(prioritizeCreationViewProjects(sorted), 4, async project => {
          try {
            return [project.id, await getCreationView(project.id, controller.signal)] as const
          } catch {
            return [project.id, undefined] as const
          }
        }, ([projectId, view]) => {
          if (!active) return
          setProjects(current => current.map(item => item.project.id === projectId ? { ...item, view } : item))
        })
      } catch {
        if (active) setError(true)
      } finally {
        if (active) setLoading(false)
      }
    }
    void load()
    return () => {
      active = false
      controller.abort()
    }
  }, [])

  if (loading) return <section className="creator-library-state" aria-live="polite">正在整理你的视频…</section>
  if (error) return <section className="creator-library-state" role="alert">暂时无法读取视频，请稍后再试。</section>

  const activeProjects = projects.filter(({ project }) => !isHistoryProject(project))
  const historyProjects = projects.filter(({ project }) => isHistoryProject(project))
  return (
    <section className="creator-library" aria-labelledby="creator-page-title">
      <p className="creator-eyebrow">我的视频</p>
      <h1 id="creator-page-title">把故事继续下去</h1>
      <p className="creator-intro">最近创作的内容都在这里。</p>
      {projects.length === 0 ? (
        <div className="creator-empty-library">
          <h2>还没有视频</h2>
          <p>从一句想法开始，做出你的第一支视频。</p>
        </div>
      ) : (
        <div className="creator-library-groups">
          <ProjectGroup title="正在创作" projects={activeProjects} onContinueProject={onContinueProject} />
          <ProjectGroup title="已完成与归档" projects={historyProjects} onContinueProject={onContinueProject} />
        </div>
      )}
    </section>
  )
}

function ProjectGroup({ title, projects, onContinueProject }: { title: string; projects: ProjectWithProgress[]; onContinueProject: VideoLibraryPageProps['onContinueProject'] }) {
  if (projects.length === 0) return null
  return (
    <section className="creator-library-group" aria-label={title}>
      <h2>{title}</h2>
      <div className="creator-project-grid">
        {projects.map(({ project, view }) => {
          const progress = creatorProjectProgress(project.status, view)
          const stepId = view?.activeStep || 'requirements'
          return (
            <article key={project.id} className="creator-project-card">
              <div className="creator-project-card-top">
                <span className="creator-project-status">{projectStatusLabel(project.status)}</span>
                <time dateTime={project.updatedAt}>更新于 {formatUpdatedAt(project.updatedAt)}</time>
              </div>
              <h3>{project.name}</h3>
              <p>{project.description || '还没有补充说明'}</p>
              <div className="creator-project-progress" aria-label={progress.label}>
                <span>{progress.label}</span>
                <div aria-hidden="true"><i style={{ width: `${progress.percent}%` }} /></div>
              </div>
              <button type="button" className="creator-secondary-button" onClick={() => onContinueProject(project.id, stepId)}>继续创作</button>
            </article>
          )
        })}
      </div>
    </section>
  )
}

function sortProjects(projects: VideoProject[]): VideoProject[] {
  return [...projects].sort((left, right) => Date.parse(right.updatedAt) - Date.parse(left.updatedAt))
}

function isHistoryProject(project: VideoProject): boolean {
  return project.status === 'COMPLETED' || project.status === 'ARCHIVED'
}

function projectStatusLabel(status: VideoProject['status']): string {
  if (status === 'RUNNING') return '创作中'
  if (status === 'PAUSED') return '已暂停'
  if (status === 'COMPLETED') return '已完成'
  if (status === 'ARCHIVED') return '已归档'
  return '待开始'
}

function formatUpdatedAt(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date)
}
