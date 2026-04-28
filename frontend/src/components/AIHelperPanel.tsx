import React, { useState } from 'react'
import { aiPolishText, aiGenerateFromMedia } from '../services/api'
import { useAppStore } from '../stores/appStore'

const AIHelperPanel: React.FC = () => {
  const { title, description, images, videos, setTitle, setDescription, contentType } = useAppStore()

  const [generating, setGenerating] = useState(false)
  const [polishing, setPolishing] = useState(false)
  const [msg, setMsg] = useState<{ text: string; type: 'success' | 'error' } | null>(null)

  const showMsg = (text: string, type: 'success' | 'error' = 'success') => {
    setMsg({ text, type })
    setTimeout(() => setMsg(null), 2500)
  }

  const handleGenerate = async () => {
    if (images.length === 0 && videos.length === 0) {
      showMsg('请先上传素材', 'error')
      return
    }
    setGenerating(true)
    try {
      const imageFiles = images.map(i => i.file)
      const videoFiles = videos.map(v => v.file)
      const result = await aiGenerateFromMedia('根据素材自动生成标题和简介', imageFiles, videoFiles)
      if (result.title) setTitle(result.title)
      if (result.description) setDescription(result.description)
      showMsg('内容已生成')
    } catch (e) {
      console.error('AI生成失败:', e)
      showMsg('生成失败，请重试', 'error')
    } finally {
      setGenerating(false)
    }
  }

  const handlePolish = async () => {
    const text = title || description
    if (!text.trim()) {
      showMsg('请先输入标题或简介内容', 'error')
      return
    }
    setPolishing(true)
    try {
      const target = title ? 'title' : 'description'
      const result = await aiPolishText(text, target)
      if (target === 'title') {
        setTitle(result.content)
      } else {
        setDescription(result.content)
      }
      showMsg('润色完成')
    } catch (e) {
      console.error('AI润色失败:', e)
      showMsg('润色失败，请重试', 'error')
    } finally {
      setPolishing(false)
    }
  }

  return (
    <div className="grid grid-cols-2 gap-3">
      {/* Card 1: AI生成标题和简介 */}
      <div className="bg-gradient-to-br from-primary/5 to-purple-50 rounded-xl p-4 border border-primary/10">
        <div className="flex items-start gap-2.5">
          <div className="w-8 h-8 bg-primary/20 rounded-lg flex items-center justify-center flex-shrink-0">
            <svg className="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <div className="flex-1 min-w-0">
            <h4 className="text-xs font-medium text-gray-800 mb-0.5">AI生成标题和简介</h4>
            <p className="text-[11px] text-gray-500 mb-2.5 leading-tight">
              根据{contentType === 'video' ? '视频' : '图片'}内容自动生成
            </p>
            <button
              onClick={handleGenerate}
              disabled={generating}
              className="px-3 py-1.5 bg-primary text-white rounded-lg text-xs hover:bg-primary-dark transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
            >
              {generating && <div className="w-3 h-3 border-2 border-white border-t-transparent rounded-full animate-spin" />}
              {contentType === 'image' ? '生成图文内容' : '生成短视频文案'}
            </button>
          </div>
        </div>
      </div>

      {/* Card 2: AI润色优化 */}
      <div className="bg-gradient-to-br from-amber-50 to-orange-50 rounded-xl p-4 border border-amber-100">
        <div className="flex items-start gap-2.5">
          <div className="w-8 h-8 bg-amber-100 rounded-lg flex items-center justify-center flex-shrink-0">
            <svg className="w-4 h-4 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
            </svg>
          </div>
          <div className="flex-1 min-w-0">
            <h4 className="text-xs font-medium text-gray-800 mb-0.5">AI润色优化</h4>
            <p className="text-[11px] text-gray-500 mb-2.5 leading-tight">
              优化标题和简介，提升吸引力
            </p>
            <button
              onClick={handlePolish}
              disabled={polishing}
              className="px-3 py-1.5 bg-amber-500 text-white rounded-lg text-xs hover:bg-amber-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
            >
              {polishing && <div className="w-3 h-3 border-2 border-white border-t-transparent rounded-full animate-spin" />}
              一键优化
            </button>
          </div>
        </div>
      </div>

      {/* Toast message */}
      {msg && (
        <div className={`col-span-2 text-[11px] text-center py-1.5 px-3 rounded-lg ${
          msg.type === 'error' ? 'bg-red-50 text-red-600' : 'bg-green-50 text-green-600'
        }`}>
          {msg.text}
        </div>
      )}
    </div>
  )
}

export default AIHelperPanel
