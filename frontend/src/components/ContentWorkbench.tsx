import React, { useState } from 'react'
import { aiGenerateFromMedia } from '../services/api'
import { useAppStore } from '../stores/appStore'

const platforms = ['抖音', '小红书', '微博', 'B站', '公众号', '快手', '知乎']

const ContentWorkbench: React.FC = () => {
  const [selectedPlatform, setSelectedPlatform] = useState('小红书')
  const [styleKeywords, setStyleKeywords] = useState('')
  const [isGenerating, setIsGenerating] = useState(false)
  const [progressText, setProgressText] = useState('')
  const [progressDots, setProgressDots] = useState('')

  const images = useAppStore((state) => state.images)
  const videos = useAppStore((state) => state.videos)
  const setTitle = useAppStore((state) => state.setTitle)
  const setDescription = useAppStore((state) => state.setDescription)
  const clearAll = useAppStore((state) => state.clearAll)

  // Animated dots
  React.useEffect(() => {
    if (!isGenerating) {
      setProgressDots('')
      return
    }
    const interval = setInterval(() => {
      setProgressDots(prev => prev.length >= 3 ? '' : prev + '.')
    }, 500)
    return () => clearInterval(interval)
  }, [isGenerating])

  const hasMedia = images.length > 0 || videos.length > 0

  const handleGenerate = async () => {
    if (!hasMedia) return

    setIsGenerating(true)
    setProgressText('AI 正在分析素材并生成内容')

    try {
      const prompt = styleKeywords || `为${selectedPlatform}生成标题和简介`
      const imageFiles = images.map(i => i.file)
      const videoFiles = videos.map(v => v.file)
      const result = await aiGenerateFromMedia(prompt, imageFiles, videoFiles)
      if (result.title) setTitle(result.title)
      if (result.description) setDescription(result.description)
    } catch (e) {
      console.error('内容生成失败:', e)
    } finally {
      setIsGenerating(false)
      setProgressText('')
    }
  }

  const handleClearAndGenerate = async () => {
    clearAll()
    setTimeout(() => handleGenerate(), 100)
  }

  return (
    <div className="bg-gradient-to-br from-blue-50 to-indigo-50 rounded-2xl p-5 border border-blue-100">
      <div className="flex items-center gap-2 mb-4">
        <div className="w-9 h-9 bg-blue-100 rounded-xl flex items-center justify-center">
          <svg className="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
        </div>
        <h4 className="text-sm font-semibold text-gray-800">内容生成工作台</h4>
      </div>

      <div className="space-y-3">
        {/* Target platform */}
        <div>
          <label className="text-xs text-gray-500 mb-1.5 block">目标平台</label>
          <select
            value={selectedPlatform}
            onChange={(e) => setSelectedPlatform(e.target.value)}
            className="w-full px-3 py-2 text-sm bg-white border border-gray-200 rounded-xl focus:outline-none focus:ring-1 focus:ring-blue-400 appearance-none"
          >
            {platforms.map(p => (
              <option key={p} value={p}>{p}</option>
            ))}
          </select>
        </div>

        {/* Style keywords */}
        <div>
          <label className="text-xs text-gray-500 mb-1.5 block">风格/关键词（选填）</label>
          <textarea
            value={styleKeywords}
            onChange={(e) => setStyleKeywords(e.target.value)}
            placeholder="例：轻松活泼、专业深度、带emoji..."
            rows={2}
            className="w-full px-3 py-2 text-sm bg-white border border-gray-200 rounded-xl focus:outline-none focus:ring-1 focus:ring-blue-400 resize-none"
          />
        </div>

        {/* Generate button */}
        <button
          onClick={handleGenerate}
          disabled={isGenerating || !hasMedia}
          className="w-full py-2.5 bg-blue-500 text-white text-sm font-medium rounded-xl hover:bg-blue-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors flex items-center justify-center gap-2"
        >
          {isGenerating ? (
            <>
              <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
              生成中{progressDots}
            </>
          ) : (
            <>
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" />
              </svg>
              AI 智能生成
            </>
          )}
        </button>

        {!hasMedia && (
          <p className="text-xs text-gray-400 text-center">请先上传素材后再生成内容</p>
        )}

        {hasMedia && (
          <button
            onClick={handleClearAndGenerate}
            disabled={isGenerating}
            className="w-full py-2 text-xs text-gray-400 hover:text-gray-600 border border-dashed border-gray-200 rounded-xl transition-colors"
          >
            清空并重新生成
          </button>
        )}

        {/* Progress indicator */}
        {isGenerating && (
          <>
            <div className="w-full h-1.5 bg-blue-100 rounded-full overflow-hidden">
              <div className="h-full bg-gradient-to-r from-blue-400 to-indigo-400 rounded-full animate-progress" />
            </div>
            {progressText && (
              <p className="text-xs text-blue-500 text-center">{progressText}{progressDots}</p>
            )}
          </>
        )}

        {/* Media count info */}
        <div className="flex items-center gap-3 text-xs text-gray-400 pt-1">
          <span className="flex items-center gap-1">
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            {images.length} 张图片
          </span>
          <span className="flex items-center gap-1">
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
            {videos.length} 个视频
          </span>
        </div>
      </div>
    </div>
  )
}

export default ContentWorkbench
