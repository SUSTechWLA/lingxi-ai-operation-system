import React, { useState, useRef } from 'react'
import { chatRevise } from '../services/api'
import { useAppStore } from '../stores/appStore'

const AIAssistantTab: React.FC = () => {
  const { title, description, keywords, setTitle, setDescription, setKeywords } = useAppStore()

  const [inputValue, setInputValue] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [revisionHistory, setRevisionHistory] = useState<string[]>([])
  const abortRef = useRef<AbortController | null>(null)

  const handleRevise = async () => {
    const text = inputValue.trim()
    if (!text || isLoading) return

    setIsLoading(true)

    if (abortRef.current) {
      abortRef.current.abort()
    }
    const controller = new AbortController()
    abortRef.current = controller

    try {
      const result = await chatRevise(
        text,
        { title, description, keywords: keywords ? keywords.split(/[,，、\s]+/).filter(Boolean) : [] },
        controller.signal
      )

      if (controller.signal.aborted) return

      // Update store with returned fields
      let updatedFields: string[] = []
      if (result.fields.title) {
        setTitle(result.fields.title)
        updatedFields.push('标题')
      }
      if (result.fields.description) {
        setDescription(result.fields.description)
        updatedFields.push('简介')
      }
      if (result.fields.keywords && result.fields.keywords.length > 0) {
        setKeywords(result.fields.keywords.join(', '))
        updatedFields.push('关键词')
      }

      const historyEntry = updatedFields.length > 0
        ? `已更新${updatedFields.join('、')}：${result.reply}`
        : result.reply

      setRevisionHistory(prev => [historyEntry, ...prev].slice(0, 10))
      setInputValue('')
    } catch (error: any) {
      if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
      console.error('AI revise failed:', error)
      setRevisionHistory(prev => [`修改失败，请重试`, ...prev].slice(0, 10))
    } finally {
      setIsLoading(false)
      if (abortRef.current === controller) {
        abortRef.current = null
      }
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleRevise()
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Welcome message */}
      <div className="mb-4">
        <h3 className="text-base font-semibold text-gray-800">AI 助手</h3>
        <p className="text-xs text-gray-500 mt-1">输入修改指令，AI 帮你完善内容</p>
      </div>

      {/* Quick actions */}
      <div className="flex flex-wrap gap-2 mb-4">
        {[
          { label: '标题更吸引人', action: '把标题改得更吸引人，加入一些悬念' },
          { label: '简介缩短', action: '把简介缩短到50字以内' },
          { label: '加关键词', action: '添加5个相关关键词' },
          { label: '优化语气', action: '让语气更正式专业一些' },
        ].map((item) => (
          <button
            key={item.label}
            onClick={() => setInputValue(item.action)}
            className="px-2.5 py-1.5 text-xs bg-gray-50 border border-gray-200 rounded-lg text-gray-600 hover:border-primary/30 hover:text-primary hover:bg-primary/5 transition-colors"
          >
            {item.label}
          </button>
        ))}
      </div>

      {/* Input area */}
      <div className="flex items-end gap-2 mb-4">
        <div className="flex-1 relative">
          <textarea
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="例：把标题改得更吸引人..."
            rows={2}
            disabled={isLoading}
            className="w-full px-3 py-2 text-sm bg-gray-50 border border-gray-200 rounded-xl focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none placeholder:text-gray-400 disabled:opacity-50"
          />
        </div>
        <button
          onClick={handleRevise}
          disabled={isLoading || !inputValue.trim()}
          className="px-3 py-2 bg-primary text-white rounded-xl hover:bg-primary-dark transition-colors disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-1 flex-shrink-0"
        >
          {isLoading ? (
            <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
          ) : (
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
            </svg>
          )}
        </button>
      </div>

      {/* Revision history */}
      {revisionHistory.length > 0 && (
        <div className="flex-1 overflow-y-auto">
          <h4 className="text-xs font-medium text-gray-400 mb-2">修改记录</h4>
          <div className="space-y-2">
            {revisionHistory.map((entry, i) => (
              <div
                key={i}
                className={`text-xs px-3 py-2 rounded-xl ${
                  entry.startsWith('已更新')
                    ? 'bg-green-50 text-green-700'
                    : entry.startsWith('修改失败')
                    ? 'bg-red-50 text-red-600'
                    : 'bg-gray-50 text-gray-600'
                }`}
              >
                {entry}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Empty state */}
      {revisionHistory.length === 0 && (
        <div className="flex-1 flex items-center justify-center">
          <div className="text-center">
            <svg className="w-10 h-10 mx-auto mb-2 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
            <p className="text-xs text-gray-400">输入修改指令开始优化内容</p>
          </div>
        </div>
      )}
    </div>
  )
}

export default AIAssistantTab
