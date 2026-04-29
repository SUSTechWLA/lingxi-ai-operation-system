import React, { useState, useRef, useEffect } from 'react'
import { chatGenerate, recordContextEvent } from '../services/api'
import { useAppStore } from '../stores/appStore'
import { setAIAbort } from '../utils/ai-loading'
import type { ChatMessageItem } from '../utils/types'

const AIAssistantTab: React.FC = () => {
  const {
    title,
    description,
    keywords,
    body,
    images,
    videos,
    chatSessionId,
    setChatSessionId,
    setTitle,
    setDescription,
    setKeywords,
    applyFields,
    setAILoadingMessage,
  } = useAppStore()

  const [messages, setMessages] = useState<ChatMessageItem[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [sessionId, setSessionId] = useState<string | null>(chatSessionId)
  const [error, setError] = useState('')
  const [showResult, setShowResult] = useState<{ text: string; type: 'success' | 'error' } | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)
  const [hasInitialized, setHasInitialized] = useState(false)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // Focus input on mount
  useEffect(() => {
    setTimeout(() => inputRef.current?.focus(), 200)
  }, [])

  // Show welcome message on mount
  useEffect(() => {
    if (!hasInitialized) {
      setHasInitialized(true)
      setMessages([{
        role: 'assistant',
        content: '你好！我是 AI 创作助手，可以帮你：\n\n• 根据素材生成标题和简介\n• 优化和完善现有内容\n• 创作灵感和建议\n• 回答自媒体相关问题\n\n有什么我可以帮你的吗？',
      }])
    }
  }, [])

  const showToast = (text: string, type: 'success' | 'error' = 'success') => {
    setShowResult({ text, type })
    setTimeout(() => setShowResult(null), 2000)
  }

  const handleSendMessage = async () => {
    const text = inputValue.trim()
    if (!text || isLoading) return

    setIsLoading(true)
    setError('')

    if (abortRef.current) {
      abortRef.current.abort()
    }
    const controller = new AbortController()
    abortRef.current = controller

    setAIAbort(() => {
      controller.abort()
      setAILoadingMessage(null)
      abortRef.current = null
    })

    // Add user message to UI
    setMessages(prev => [...prev, { role: 'user', content: text }])
    setInputValue('')

    try {
      const currentContext = {
        title,
        description,
        body,
        keywords: keywords ? keywords.split(/[,，、\s]+/).filter(Boolean) : [],
        media_count: images.length + videos.length,
        media_names: [...images.map(i => i.name), ...videos.map(v => v.name)],
      }

      const result = await chatGenerate(
        text,
        currentContext,
        sessionId || undefined,
        controller.signal
      )

      if (controller.signal.aborted) return

      // Save session ID for conversation continuity
      if (result.session_id) {
        setSessionId(result.session_id)
        setChatSessionId(result.session_id)
      }

      // Check if we got complete fields (generation result)
      if (result.fields) {
        const fieldsToApply: Record<string, string> = {}
        if (result.fields.title) fieldsToApply.title = result.fields.title
        if (result.fields.description) fieldsToApply.description = result.fields.description
        if (result.fields.body) fieldsToApply.body = result.fields.body
        if (result.fields.keywords && result.fields.keywords.length > 0) {
          fieldsToApply.keywords = result.fields.keywords.join(', ')
        }

        if (Object.keys(fieldsToApply).length > 0) {
          applyFields(fieldsToApply as any)
          setMessages(prev => [...prev, {
            role: 'assistant',
            content: result.reply || '内容已生成！',
            fields: result.fields,
          }])
          showToast('内容已更新')
        } else {
          setMessages(prev => [...prev, { role: 'assistant', content: result.reply }])
        }
      } else {
        // Conversational response
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: result.reply,
          suggestions: result.suggestions,
        }])
      }
    } catch (error: any) {
      if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
      console.error('AI assistant failed:', error)
      setError('请求失败，请重试')
    } finally {
      setIsLoading(false)
      setAILoadingMessage(null)
      if (abortRef.current === controller) {
        abortRef.current = null
      }
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSendMessage()
    }
  }

  const handleSuggestionClick = (suggestionText: string) => {
    setInputValue(suggestionText)
    // Auto-send after a short delay to let input update
    setTimeout(() => {
      handleSendMessage()
    }, 50)
  }

  const handleQuickAction = (action: string) => {
    setInputValue(action)
    setTimeout(() => handleSendMessage(), 50)
  }

  const clearChat = () => {
    setMessages([{
      role: 'assistant',
      content: '你好！我是 AI 创作助手，有什么我可以帮你的吗？',
    }])
    setSessionId(null)
    setChatSessionId(null)
    setError('')
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="mb-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-base font-semibold text-gray-800">AI 助手</h3>
            <p className="text-xs text-gray-500 mt-1">智能对话，帮你完成内容创作</p>
          </div>
          {messages.length > 1 && (
            <button
              onClick={clearChat}
              className="text-xs text-gray-400 hover:text-gray-600 transition-colors px-2 py-1"
            >
              清空对话
            </button>
          )}
        </div>
      </div>

      {/* Quick actions */}
      <div className="flex flex-wrap gap-2 mb-4">
        {[
          { label: '生成标题和简介', action: images.length > 0 ? '请根据我上传的图片生成标题和简介' : '帮我生成一个吸引人的标题和简介' },
          { label: '标题更吸引人', action: title ? `把标题"${title}"改得更吸引人` : '帮我想一个吸引人的标题' },
          { label: '优化语气', action: description ? `优化这段简介的语气：${description}` : '让语气更正式专业一些' },
          { label: '创作建议', action: '给我一些自媒体内容创作的技巧和建议' },
        ].map((item) => (
          <button
            key={item.label}
            onClick={() => handleQuickAction(item.action)}
            className="px-2.5 py-1.5 text-xs bg-gray-50 border border-gray-200 rounded-lg text-gray-600 hover:border-primary/30 hover:text-primary hover:bg-primary/5 transition-colors"
          >
            {item.label}
          </button>
        ))}
      </div>

      {/* Messages area */}
      <div className="flex-1 overflow-y-auto space-y-4 mb-4 min-h-[200px]">
        {messages.map((msg, index) => (
          <div key={index} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            <div className={`max-w-[85%] ${
              msg.role === 'user'
                ? 'bg-primary text-white rounded-2xl rounded-tr-md px-4 py-2.5'
                : 'bg-gray-50 text-gray-700 rounded-2xl rounded-tl-md px-4 py-2.5'
            }`}>
              <p className="text-sm whitespace-pre-wrap leading-relaxed">{msg.content}</p>

              {/* Suggestions rendered as clickable buttons */}
              {msg.suggestions && msg.suggestions.length > 0 && (
                <div className="flex flex-wrap gap-2 mt-3">
                  {msg.suggestions.map((s, si) => (
                    <button
                      key={si}
                      onClick={() => handleSuggestionClick(s.text)}
                      disabled={isLoading}
                      className="px-3 py-1.5 text-xs bg-white border border-gray-200 rounded-full text-gray-600 hover:border-primary/40 hover:text-primary hover:bg-primary/5 transition-colors disabled:opacity-50"
                    >
                      {s.text}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        ))}

        {/* Loading indicator */}
        {isLoading && (
          <div className="flex justify-start">
            <div className="bg-gray-50 text-gray-500 rounded-2xl rounded-tl-md px-4 py-3">
              <div className="flex items-center gap-1.5">
                <div className="w-2 h-2 bg-gray-300 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                <div className="w-2 h-2 bg-gray-300 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                <div className="w-2 h-2 bg-gray-300 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}

        {/* Error message */}
        {error && (
          <div className="flex justify-center">
            <div className="bg-red-50 text-red-600 rounded-xl px-4 py-2 text-sm">{error}</div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input area */}
      <div>
        <div className="flex items-end gap-2">
          <div className="flex-1 relative">
            <textarea
              ref={inputRef}
              value={inputValue}
              onChange={(e) => {
                setInputValue(e.target.value)
                e.target.style.height = 'auto'
                e.target.style.height = `${Math.min(e.target.scrollHeight, 80)}px`
              }}
              onKeyDown={handleKeyDown}
              placeholder="输入你的想法，按 Enter 发送..."
              rows={1}
              disabled={isLoading}
              className="w-full px-3 py-2 text-sm bg-gray-50 border border-gray-200 rounded-xl focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none placeholder:text-gray-400 disabled:opacity-50"
            />
          </div>
          <button
            onClick={handleSendMessage}
            disabled={isLoading || !inputValue.trim()}
            className="px-3 py-2 bg-primary text-white rounded-xl hover:bg-primary-dark transition-colors disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-1 flex-shrink-0"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
            </svg>
          </button>
        </div>
        <p className="text-[10px] text-gray-400 mt-1.5">Shift + Enter 换行 | 支持自然语言对话</p>
      </div>

      {/* Toast notification */}
      {showResult && (
        <div className={`fixed bottom-6 left-1/2 -translate-x-1/2 z-50 text-sm px-4 py-2 rounded-xl shadow-lg ${
          showResult.type === 'error' ? 'bg-red-500 text-white' : 'bg-green-500 text-white'
        }`}>
          {showResult.text}
        </div>
      )}
    </div>
  )
}

export default AIAssistantTab
