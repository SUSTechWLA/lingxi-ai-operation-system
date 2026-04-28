import React, { useState } from 'react'
import UploadCard from '../components/UploadCard'
import TitleInput from '../components/TitleInput'
import DescriptionInput from '../components/DescriptionInput'
import KeywordInput from '../components/KeywordInput'
import AIHelperPanel from '../components/AIHelperPanel'

import ContentTypeSelector from '../components/ContentTypeSelector'
import PlatformSelector from '../components/PlatformSelector'
import PublishButton from '../components/PublishButton'
import MediaLibraryPanel from '../components/MediaLibraryPanel'
import AIAssistantTab from '../components/AIAssistantTab'
import { useAppStore } from '../stores/appStore'
import { publishContent, fetchRecentTrace } from '../services/api'
import type { TraceData, ContentType } from '../utils/types'

const PublishPage: React.FC = () => {
  const {
    title,
    description,
    keywords,
    videos,
    images,
    cover,
    isPublishing,
    contentType,
    getSelectedPlatforms,
    setIsPublishing,
    clearAll,
    setContentType,
    clearMedia,
    setCover,
  } = useAppStore()

  const [showResult, setShowResult] = useState(false)
  const [resultMessage, setResultMessage] = useState('')
  const [resultType, setResultType] = useState<'success' | 'error'>('success')
  const [showDebugTrace, setShowDebugTrace] = useState(false)
  const [debugTraceData, setDebugTraceData] = useState<TraceData | null>(null)
  const [debugLoading, setDebugLoading] = useState(false)

  const [showMediaLibrary, setShowMediaLibrary] = useState(false)
  const [rightPanelTab, setRightPanelTab] = useState<'platform' | 'ai-assistant'>('platform')

  // Type switch confirmation
  const [pendingTypeChange, setPendingTypeChange] = useState<ContentType>(null)
  const [showConfirmSwitch, setShowConfirmSwitch] = useState(false)

  const handleContentTypeChange = (newType: ContentType) => {
    if (newType === contentType) return
    if ((videos.length > 0 || images.length > 0) && newType !== null) {
      setPendingTypeChange(newType)
      setShowConfirmSwitch(true)
    } else {
      setContentType(newType)
    }
  }

  const confirmTypeSwitch = () => {
    clearMedia()
    setContentType(pendingTypeChange)
    setShowConfirmSwitch(false)
    setPendingTypeChange(null)
  }

  const handleSelectMedia = (asset: any) => {
    if (contentType === 'image' && asset.mimeType?.startsWith('image/')) {
      useAppStore.getState().addImages([{ file: new File([], asset.originalName), name: asset.originalName, size: asset.size }])
      setShowMediaLibrary(false)
    } else if (contentType === 'video' && asset.mimeType?.startsWith('video/')) {
      useAppStore.getState().addVideos([{ file: new File([], asset.originalName), name: asset.originalName, size: asset.size }])
      setShowMediaLibrary(false)
    } else if (contentType === null) {
      useAppStore.getState().addImages([{ file: new File([], asset.originalName), name: asset.originalName, size: asset.size }])
      setShowMediaLibrary(false)
    }
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
      // Only send files matching the selected content type
      const videoFiles = contentType === 'video' ? videos.map((v) => v.file) : []
      const imageFiles = contentType === 'image' ? images.map((i) => i.file) : []

      const result = await publishContent(
        title,
        description,
        keywords,
        selectedPlatforms,
        videoFiles,
        imageFiles,
        contentType || undefined,
        cover?.file
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

      {/* Type switch confirmation dialog */}
      {showConfirmSwitch && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={() => setShowConfirmSwitch(false)}>
          <div className="bg-white rounded-2xl shadow-2xl p-6 max-w-sm w-full mx-4" onClick={(e) => e.stopPropagation()}>
            <div className="w-12 h-12 bg-amber-100 rounded-full flex items-center justify-center mb-4 mx-auto">
              <svg className="w-6 h-6 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
              </svg>
            </div>
            <h3 className="text-lg font-semibold text-gray-800 text-center mb-2">切换发布类型</h3>
            <p className="text-sm text-gray-600 text-center mb-6">
              切换类型将清空已上传的素材，但已填写的标题、简介等文字内容会保留。是否继续？
            </p>
            <div className="flex gap-3">
              <button
                onClick={() => setShowConfirmSwitch(false)}
                className="flex-1 px-4 py-2.5 text-sm font-medium text-gray-600 border border-gray-200 rounded-xl hover:bg-gray-50 transition-colors"
              >
                取消
              </button>
              <button
                onClick={confirmTypeSwitch}
                className="flex-1 px-4 py-2.5 text-sm font-medium text-white bg-primary rounded-xl hover:bg-primary-dark transition-colors"
              >
                确认切换
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="flex-1 flex">
        <main className="flex-1 p-6 overflow-y-auto">
          <div className="max-w-2xl mx-auto">
            {/* Header */}
            <div className="mb-4">
              <div className="flex items-center gap-3 mb-1">
                <h2 className="text-xl font-bold text-gray-800">创作发布</h2>
              </div>
              <p className="text-xs text-gray-500">
                创作优质内容，一键发布到各大自媒体平台
              </p>
            </div>

            <div className="space-y-4">
              {/* Content Type Selector */}
              <ContentTypeSelector value={contentType} onChange={handleContentTypeChange} />

              {/* Content area - shown only when a type is selected */}
              {!contentType ? (
                <div className="text-center py-10 bg-gray-50 rounded-xl border-2 border-dashed border-gray-200">
                  <svg className="w-12 h-12 mx-auto mb-3 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9.53 16.122a3 3 0 00-5.78 1.128 2.25 2.25 0 01-2.4 2.245 4.5 4.5 0 008.4-2.245c0-.399-.078-.78-.22-1.128zm0 0a15.998 15.998 0 003.388-1.62m-5.043-.025a15.994 15.994 0 011.622-3.395m3.42 3.42a15.995 15.995 0 004.764-4.648l3.876-5.814a1.151 1.151 0 00-1.597-1.597L14.146 6.32a15.996 15.996 0 00-4.649 4.763m3.42 3.42a6.776 6.776 0 00-3.42-3.42" />
                  </svg>
                  <p className="text-gray-500 text-sm">请先选择发布类型，开始创作</p>
                  <p className="text-gray-400 text-xs mt-0.5">选择图文笔记或短视频，进入专属创作流程</p>
                </div>
              ) : (
                <>
                  {/* 素材区 */}
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <div className="flex items-center gap-2">
                        <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wider">素材区</h3>
                        <span className="text-xs text-gray-400">
                          {contentType === 'image' ? `${images.length}张图片` : `${videos.length}个视频`}
                        </span>
                      </div>
                      {showMediaLibrary === false && (
                        <button
                          onClick={() => setShowMediaLibrary(true)}
                          className="text-xs text-primary hover:text-primary-dark flex items-center gap-1 transition-colors"
                        >
                          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 10h16M4 14h16M4 18h16" />
                          </svg>
                          素材库
                        </button>
                      )}
                    </div>
                    {contentType === 'image' ? (
                      <UploadCard type="image" />
                    ) : (
                      <>
                        <UploadCard type="video" />

                        {/* Cover image upload */}
                        <div className="mt-3">
                          <div className="flex items-center gap-2 mb-2">
                            <h4 className="text-xs font-medium text-gray-500">视频封面</h4>
                            <span className="text-[11px] text-gray-400">选填，建议 16:9 比例</span>
                          </div>
                          {cover ? (
                            <div className="relative rounded-lg overflow-hidden border border-gray-200 bg-gray-50">
                              <img
                                src={cover.preview}
                                alt="封面"
                                className="w-full h-28 object-cover"
                              />
                              <button
                                onClick={() => setCover(null)}
                                className="absolute top-2 right-2 w-6 h-6 bg-black/50 rounded-full flex items-center justify-center hover:bg-black/70 transition-colors"
                              >
                                <svg className="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                </svg>
                              </button>
                              <div className="px-3 py-1.5 text-xs text-gray-500 truncate">{cover.name}</div>
                            </div>
                          ) : (
                            <label className="flex items-center gap-3 px-4 py-3 rounded-lg border-2 border-dashed border-gray-200 bg-gray-50/50 cursor-pointer hover:border-primary/30 hover:bg-primary/5 transition-colors">
                              <div className="w-8 h-8 rounded-lg bg-gray-100 flex items-center justify-center flex-shrink-0">
                                <svg className="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                                </svg>
                              </div>
                              <div>
                                <p className="text-xs text-gray-600">点击上传封面图片</p>
                                <p className="text-[11px] text-gray-400">支持 JPG、PNG</p>
                              </div>
                              <input
                                type="file"
                                accept="image/jpeg,image/png,image/webp"
                                className="hidden"
                                onChange={(e) => {
                                  const file = e.target.files?.[0]
                                  if (file) {
                                    const preview = URL.createObjectURL(file)
                                    setCover({ file, preview, name: file.name, size: file.size })
                                  }
                                  e.target.value = ''
                                }}
                              />
                            </label>
                          )}
                        </div>
                      </>
                    )}
                  </div>

                  {/* AI创作助手卡片 */}
                  <div>
                    <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-3">AI创作助手</h3>
                    <AIHelperPanel />
                  </div>

                  {/* 表单区域 */}
                  <div className="space-y-4">
                    <TitleInput />
                    <DescriptionInput />
                    <KeywordInput />
                  </div>

                  {/* 底部操作按钮 */}
                  <div className="flex items-center gap-3 pt-3 border-t border-gray-100">
                    <button
                      onClick={handleClear}
                      className="px-5 py-2 border border-gray-200 rounded-lg text-gray-600 hover:bg-gray-50 transition-colors flex items-center gap-1.5 text-xs"
                    >
                      <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                      清空内容
                    </button>

                    <div className="flex-1" />

                    <button
                      onClick={handlePublish}
                      className="px-6 py-2 bg-primary text-white rounded-lg hover:bg-primary-dark transition-colors flex items-center gap-1.5 text-xs shadow-lg shadow-primary/25"
                    >
                      下一步：选择发布平台
                      <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
                      </svg>
                    </button>
                  </div>
                </>
              )}
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
                <PlatformSelector />

                <div className="pt-4 border-t border-gray-100">
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
