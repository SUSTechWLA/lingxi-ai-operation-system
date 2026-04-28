import React, { useState, useEffect, useRef } from 'react'
import UploadCard from '../components/UploadCard'
import TitleInput from '../components/TitleInput'
import DescriptionInput from '../components/DescriptionInput'
import KeywordInput from '../components/KeywordInput'
import AIHelperPanel from '../components/AIHelperPanel'
import ContentWorkbench from '../components/ContentWorkbench'
import PlatformSelector from '../components/PlatformSelector'
import PublishButton from '../components/PublishButton'
import MediaLibraryPanel from '../components/MediaLibraryPanel'
import GenerateModal from '../components/GenerateModal'
import AIAssistantTab from '../components/AIAssistantTab'
import { useAppStore } from '../stores/appStore'
import { publishContent, aiPolishText, fetchRecentTrace } from '../services/api'
import type { AIPolishData, TraceData, MediaAsset } from '../utils/types'

const PublishPage: React.FC = () => {
  const {
    title,
    description,
    keywords,
    videos,
    images,
    isPublishing,
    getSelectedPlatforms,
    setTitle,
    setDescription,
    setIsPublishing,
    clearAll,
    addImages,
    addVideos,
  } = useAppStore()

  const [showResult, setShowResult] = useState(false)
  const [resultMessage, setResultMessage] = useState('')
  const [resultType, setResultType] = useState<'success' | 'error'>('success')
  const [isAILoading, setIsAILoading] = useState(false)
  const [aiLoadingMessage, setAILoadingMessage] = useState('')
  const [aiProgressText, setAIProgressText] = useState('')
  const [showDebugTrace, setShowDebugTrace] = useState(false)
  const [debugTraceData, setDebugTraceData] = useState<TraceData | null>(null)
  const [debugLoading, setDebugLoading] = useState(false)
  const abortControllerRef = useRef<AbortController | null>(null)

  const [showMediaLibrary, setShowMediaLibrary] = useState(false)
  const [showChatModal, setShowChatModal] = useState(false)
  const [rightPanelTab, setRightPanelTab] = useState<'platform' | 'ai-assistant'>('platform')

  const handleSelectMedia = (asset: MediaAsset) => {
    if (asset.mimeType?.startsWith('image/')) {
      const fakeFile = new File([], asset.originalName)
      addImages([{ file: fakeFile, name: asset.originalName, size: asset.size }])
    } else if (asset.mimeType?.startsWith('video/')) {
      const fakeFile = new File([], asset.originalName)
      addVideos([{ file: fakeFile, name: asset.originalName, size: asset.size }])
    }
    setShowMediaLibrary(false)
  }

  const handleDebugTrace = async () => {
    setDebugLoading(true)
    try {
      const data = await fetchRecentTrace()
      setDebugTraceData(data)
      setShowDebugTrace(true)
    } catch (e) {
      console.error('Failed to fetch recent trace', e)
    } finally {
      setDebugLoading(false)
    }
  }

  const showResultPopup = (message: string, type: 'success' | 'error' = 'success') => {
    setResultMessage(message)
    setResultType(type)
    setShowResult(true)
    setTimeout(() => setShowResult(false), 2500)
  }

  const startAILoading = (message: string) => {
    setIsAILoading(true)
    setAILoadingMessage(message)
    setAIProgressText('准备中...')
  }

  const stopAILoading = () => {
    setIsAILoading(false)
    setAILoadingMessage('')
    setAIProgressText('')
  }

  // Animated progress dots during AI loading
  const [progressDots, setProgressDots] = useState('')
  useEffect(() => {
    if (!isAILoading) {
      setProgressDots('')
      return
    }
    const interval = setInterval(() => {
      setProgressDots(prev => prev.length >= 3 ? '' : prev + '.')
    }, 500)
    return () => clearInterval(interval)
  }, [isAILoading])

  const cancelAILoading = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
    }
    stopAILoading()
    showResultPopup('已取消 AI 操作', 'error')
  }

  const handleAIPolish = async (type: 'title' | 'description') => {
    const text = type === 'title' ? title : description
    if (!text.trim()) {
      showResultPopup(`请先输入${type === 'title' ? '标题' : '简介'}内容`, 'error')
      return
    }
    // Create new AbortController and store in ref
    const controller = new AbortController()
    abortControllerRef.current = controller

    startAILoading(`AI 正在润色${type === 'title' ? '标题' : '简介'}`)
    setAIProgressText('AI 正在优化文字表达...')
    try {
      const result: AIPolishData = await aiPolishText(text, type, controller.signal)
      if (controller.signal.aborted) return
      if (type === 'title') {
        setTitle(result.content)
      } else {
        setDescription(result.content)
      }
      stopAILoading()
      showResultPopup('AI 润色完成！')
    } catch (error: any) {
      if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
      console.error('AI润色失败:', error)
      stopAILoading()
      showResultPopup('AI 润色失败，请重试', 'error')
    } finally {
      if (abortControllerRef.current === controller) {
        abortControllerRef.current = null
      }
    }
  }

  const handlePublish = async () => {
    const selectedPlatforms = getSelectedPlatforms()

    if (selectedPlatforms.length === 0) {
      showResultPopup('请至少选择一个发布平台', 'error')
      return
    }

    if (!title.trim()) {
      showResultPopup('请输入标题', 'error')
      return
    }

    if (!description.trim()) {
      showResultPopup('请输入简介', 'error')
      return
    }

    setIsPublishing(true)

    try {
      const videoFiles = videos.map((v) => v.file)
      const imageFiles = images.map((i) => i.file)

      const result = await publishContent(
        title,
        description,
        keywords,
        selectedPlatforms,
        videoFiles,
        imageFiles
      )

      showResultPopup(`发布任务已创建！任务ID: ${result.taskId}`)
    } catch (error) {
      console.error('发布失败:', error)
      showResultPopup('发布失败，请重试', 'error')
    } finally {
      setIsPublishing(false)
    }
  }

  const handleClear = () => {
    clearAll()
    showResultPopup('内容已清空')
  }

  const selectedCount = getSelectedPlatforms().length

  return (
    <div className="flex min-h-screen bg-background">
      {/* AI loading overlay */}
      {isAILoading && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="bg-white rounded-3xl shadow-2xl w-96 p-8 flex flex-col items-center gap-6 animate-in">
            {/* Animated spinner */}
            <div className="relative w-16 h-16">
              <div className="absolute inset-0 rounded-full border-4 border-gray-100"></div>
              <div className="absolute inset-0 rounded-full border-4 border-transparent border-t-primary animate-spin"></div>
              <div className="absolute inset-0 flex items-center justify-center">
                <svg className="w-7 h-7 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
            </div>

            {/* Message */}
            <div className="text-center">
              <p className="text-base font-semibold text-gray-800">{aiLoadingMessage}</p>
              <p className="text-sm text-gray-500 mt-1">
                {aiProgressText}{progressDots}
              </p>
            </div>

            {/* Animated progress bar */}
            <div className="w-full h-2 bg-gray-100 rounded-full overflow-hidden">
              <div className="h-full bg-gradient-to-r from-primary via-purple-500 to-primary rounded-full animate-progress"></div>
            </div>

            <button
              onClick={cancelAILoading}
              className="px-6 py-2 bg-gray-100 hover:bg-gray-200 text-gray-600 rounded-xl text-sm transition-colors"
            >
              取消
            </button>
          </div>
        </div>
      )}

      {/* Result popup overlay */}
      {showResult && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={() => setShowResult(false)}>
          <div className="bg-white rounded-2xl shadow-2xl p-8 flex flex-col items-center gap-4 min-w-[300px] animate-in" onClick={(e) => e.stopPropagation()}>
            <div className={`w-16 h-16 rounded-full flex items-center justify-center ${
              resultType === 'error' ? 'bg-red-100' : 'bg-green-100'
            }`}>
              {resultType === 'error' ? (
                <svg className="w-8 h-8 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              ) : (
                <svg className="w-8 h-8 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              )}
            </div>
            <p className={`text-lg font-semibold ${
              resultType === 'error' ? 'text-red-700' : 'text-green-700'
            }`}>{resultMessage}</p>
          </div>
        </div>
      )}

      <div className="flex-1 flex">
        <main className="flex-1 p-8 overflow-y-auto">
          <div className="max-w-3xl mx-auto">
            <div className="mb-8">
              <div className="flex items-center gap-3 mb-2">
                <h2 className="text-2xl font-bold text-gray-800">创作发布</h2>
              </div>
              <p className="text-sm text-gray-500">
                创作优质内容，一键发布到各大自媒体平台
              </p>
            </div>

            <div className="space-y-6">
              <div>
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-sm font-medium text-gray-700">上传素材</h3>
                  <button
                    onClick={() => setShowMediaLibrary(true)}
                    className="text-xs text-primary hover:text-primary-dark flex items-center gap-1 transition-colors"
                  >
                    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 10h16M4 14h16M4 18h16" />
                    </svg>
                    素材库
                  </button>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <UploadCard type="video" />
                  <UploadCard type="image" />
                </div>
              </div>

              <div className="space-y-6">
                <TitleInput onPolish={handleAIPolish} />
                <DescriptionInput onPolish={handleAIPolish} />
                <KeywordInput />
              </div>

              <div className="flex items-center gap-4 pt-4 border-t border-gray-100">
                <button
                  onClick={handleClear}
                  className="px-6 py-2.5 border border-gray-200 rounded-xl text-gray-600 hover:bg-gray-50 transition-colors flex items-center gap-2"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                  清空内容
                </button>

                <div className="flex-1" />

                <button
                  onClick={() => setShowChatModal(true)}
                  className="px-6 py-2.5 bg-primary/10 text-primary rounded-xl hover:bg-primary/20 transition-colors flex items-center gap-2"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
                  </svg>
                  AI 生成全部内容
                </button>

                <button
                  onClick={handlePublish}
                  className="px-8 py-2.5 bg-primary text-white rounded-xl hover:bg-primary-dark transition-colors flex items-center gap-2 shadow-lg shadow-primary/25"
                >
                  下一步：选择发布平台
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </main>

        <aside className="w-96 bg-white border-l border-gray-100 flex flex-col">
          {/* Right panel tab bar */}
          <div className="flex border-b border-gray-100">
            <button
              onClick={() => setRightPanelTab('platform')}
              className={`flex-1 px-5 py-3.5 text-sm font-medium transition-colors relative ${
                rightPanelTab === 'platform'
                  ? 'text-primary'
                  : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              发布平台
              {rightPanelTab === 'platform' && (
                <div className="absolute bottom-0 left-5 right-5 h-0.5 bg-primary rounded-full" />
              )}
            </button>
            <button
              onClick={() => setRightPanelTab('ai-assistant')}
              className={`flex-1 px-5 py-3.5 text-sm font-medium transition-colors relative ${
                rightPanelTab === 'ai-assistant'
                  ? 'text-primary'
                  : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              AI 助手
              {rightPanelTab === 'ai-assistant' && (
                <div className="absolute bottom-0 left-5 right-5 h-0.5 bg-primary rounded-full" />
              )}
            </button>
          </div>

          {/* Tab content */}
          <div className="flex-1 p-6 overflow-y-auto">
            {rightPanelTab === 'platform' ? (
              <div className="space-y-6">
                <AIHelperPanel
                  onGenerate={() => setShowChatModal(true)}
                  onPolish={handleAIPolish}
                />

                <div className="pt-2">
                  <ContentWorkbench />
                </div>

                <div className="pt-6 border-t border-gray-100">
                  <PlatformSelector />
                </div>

                <div className="pt-4">
                  <PublishButton
                    onClick={handlePublish}
                    disabled={selectedCount === 0}
                    loading={isPublishing}
                  />
                  <p className="text-xs text-gray-400 mt-3 text-center">
                    发布前请确保内容遵守各平台规范
                  </p>
                </div>
              </div>
            ) : (
              <AIAssistantTab />
            )}
          </div>
        </aside>
      </div>

      {/* AI Chat Generation Modal */}
      <GenerateModal
        isOpen={showChatModal}
        onClose={() => setShowChatModal(false)}
      />

      {/* Media Library Panel */}
      <MediaLibraryPanel
        isOpen={showMediaLibrary}
        onClose={() => setShowMediaLibrary(false)}
        onSelectMedia={handleSelectMedia}
      />

      {/* Debug: float button for recent trace */}
      <button
        onClick={handleDebugTrace}
        disabled={debugLoading}
        className="fixed bottom-6 right-6 z-40 w-10 h-10 bg-gray-800 text-white rounded-full shadow-lg hover:bg-gray-700 flex items-center justify-center text-xs opacity-50 hover:opacity-100 transition-opacity"
        title="查看最近任务追踪"
      >
        {debugLoading ? '…' : '🔍'}
      </button>

      {/* Debug trace modal */}
      {showDebugTrace && debugTraceData && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setShowDebugTrace(false)}>
          <div className="bg-white rounded-2xl shadow-2xl w-full max-w-4xl max-h-[85vh] overflow-y-auto m-4" onClick={(e) => e.stopPropagation()}>
            <div className="sticky top-0 bg-white border-b border-gray-100 px-6 py-4 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold text-gray-800">任务追踪</span>
                <span className="text-xs text-gray-400 font-mono">{debugTraceData.task.taskId}</span>
                <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${
                  debugTraceData.task.status === 'SUCCESS' ? 'bg-green-100 text-green-700' :
                  debugTraceData.task.status === 'FAILED' ? 'bg-red-100 text-red-700' : 'bg-blue-100 text-blue-700'
                }`}>{debugTraceData.task.status}</span>
              </div>
              <button onClick={() => setShowDebugTrace(false)} className="p-1.5 hover:bg-gray-100 rounded-lg">
                <svg className="w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="p-6">
              <pre className="text-xs text-gray-700 bg-gray-50 rounded-xl p-4 overflow-x-auto max-h-[60vh] font-mono whitespace-pre-wrap break-all">
                {JSON.stringify(debugTraceData, null, 2)}
              </pre>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default PublishPage
