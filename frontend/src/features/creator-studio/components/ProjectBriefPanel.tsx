import type { CreatorProject } from '../types'

export default function ProjectBriefPanel({ project }: { project: CreatorProject }) {
  const duration = project.targetDurationSec ? `${project.targetDurationSec} 秒` : '未限定'
  const route = project.mode === 'voice_visual'
    ? 'IP 口播'
    : project.mode === 'cinematic_story'
      ? '叙事短片'
      : 'AIGC 分镜'

  return (
    <section className="artifact-review-panel project-brief-panel" aria-labelledby="project-brief-title">
      <div className="artifact-review-heading">
        <div>
          <p className="creator-eyebrow">当前内容</p>
          <h2 id="project-brief-title">创作需求</h2>
        </div>
        <span className="artifact-state is-confirmed">已保存</span>
      </div>
      <div className="project-brief-copy">
        <p className="creator-eyebrow">创作目标</p>
        <h3>{project.name || '未命名视频项目'}</h3>
        <p>{project.description || '这个项目尚未填写补充说明。'}</p>
      </div>
      <dl className="project-brief-facts">
        <div><dt>目标时长</dt><dd>{duration}</dd></div>
        <div><dt>画面比例</dt><dd>{project.aspectRatio || '未限定'}</dd></div>
        <div><dt>创作类型</dt><dd>{route}</dd></div>
        <div><dt>语言</dt><dd>{project.language || 'zh-CN'}</dd></div>
      </dl>
      <p className="project-brief-note">这是创建项目时保存的正式需求记录。需要调整方向时，可使用上方“从此步骤重新生成”。</p>
    </section>
  )
}
