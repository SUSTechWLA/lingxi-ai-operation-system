import React, { useState } from 'react'
import UploadCard from '../components/UploadCard'
import TitleInput from '../components/TitleInput'
import DescriptionInput from '../components/DescriptionInput'
import KeywordInput from '../components/KeywordInput'
import AIHelperPanel from '../components/AIHelperPanel'
import PlatformSelector from '../components/PlatformSelector'
import PublishButton from '../components/PublishButton'
import DesktopToolbar from '../components/DesktopToolbar'
import { useAppStore } from '../stores/appStore'
import { publishContent, aiGenerateContent, aiPolishText } from '../services/api'
import { isElectron } from '../utils/electron'

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
  } = useAppStore()

  const [showToast, setShowToast] = useState(false)
  const [toastMessage, setToastMessage] = useState('')
  const [toastType, setToastType] = useState<'success' | 'error'>('success')

  const showNotification = (message: string, type: 'success' | 'error' = 'success') => {
    setToastMessage(message)
    setToastType(type)
    setShowToast(true)
    setTimeout(() => setShowToast(false), 3000)
  }

  const handleAIGenerate = async () => {
    try {
      showNotification('AI正在生成内容，请稍候...')
      const prompt = title || description || '自媒体内容创作'
      const result = await aiGenerateContent(prompt)
      if (result.title) setTitle(result.title)
      if (result.description) setDescription(result.description)
      showNotification('AI内容生成成功！')
    } catch (error) {
      console.error('AI生成失败:', error)
      showNotification('AI生成失败，请重试', 'error')
    }
  }

  const handleAIPolish = async (type: 'title' | 'description') => {
    try {
      const text = type === 'title' ? title : description
      if (!text.trim()) {
        showNotification(`请先输入${type === 'title' ? '标题' : '简介'}内容`, 'error')
        return
      }
      showNotification('AI正在优化内容，请稍候...')
      const polished = await aiPolishText(text, type)
      if (type === 'title') {
        setTitle(polished)
      } else {
        setDescription(polished)
      }
      showNotification('AI润色完成！')
    } catch (error) {
      console.error('AI润色失败:', error)
      showNotification('AI润色失败，请重试', 'error')
    }
  }

  const handlePublish = async () => {
    const selectedPlatforms = getSelectedPlatforms()

    if (selectedPlatforms.length === 0) {
      showNotification('请至少选择一个发布平台', 'error')
      return
    }

    if (!title.trim()) {
      showNotification('请输入标题', 'error')
      return
    }

    if (!description.trim()) {
      showNotification('请输入简介', 'error')
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

      showNotification(`发布任务已创建！任务ID: ${result.taskId}`)
    } catch (error) {
      console.error('发布失败:', error)
      showNotification('发布失败，请重试', 'error')
    } finally {
      setIsPublishing(false)
    }
  }

  const handleClear = () => {
    clearAll()
    showNotification('内容已清空')
  }

  const selectedCount = getSelectedPlatforms().length

  return (
    <div className="flex min-h-screen bg-background">
      {showToast && (
        <div className="fixed top-4 right-4 z-50 animate-slide-in">
          <div className="bg-white rounded-xl shadow-lg border border-gray-100 px-6 py-4 flex items-center gap-3">
            <svg className={`w-5 h-5 ${toastType === 'error' ? 'text-red-500' : 'text-green-500'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              {toastType === 'error' ? (
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              )}
            </svg>
            <span className="text-sm text-gray-700">{toastMessage}</span>
          </div>
        </div>
      )}

      <div className="flex-1 flex">
        <main className="flex-1 p-8 overflow-y-auto">
          <div className="max-w-4xl mx-auto">
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
                <h3 className="text-sm font-medium text-gray-700 mb-4">上传素材</h3>
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
                  onClick={handleAIGenerate}
                  className="px-6 py-2.5 bg-primary/10 text-primary rounded-xl hover:bg-primary/20 transition-colors flex items-center gap-2"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" />
                  </svg>
                  AI 生成标题和简介
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

        <aside className="w-80 bg-white border-l border-gray-100 p-6 overflow-y-auto">
          <div className="space-y-6">
            {isElectron() && (
              <div className="pb-6 border-b border-gray-100">
                <DesktopToolbar />
              </div>
            )}
            <AIHelperPanel
              onGenerate={handleAIGenerate}
              onPolish={handleAIPolish}
            />

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
        </aside>
      </div>
    </div>
  )
}

export default PublishPage
