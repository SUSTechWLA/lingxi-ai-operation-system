import React, { useState, useRef, useEffect } from 'react'
import { chatGenerate } from '../services/api'
import { useAppStore } from '../stores/appStore'
import type { ChatMessageItem } from '../utils/types'

interface GenerateModalProps {
  isOpen: boolean
  onClose: () => void
}

const GenerateModal: React.FC<GenerateModalProps> = ({ isOpen, onClose }) => {
  const {
    title,
    description,
    body,
    keywords,
    images,
    videos,
    chatSessionId,
    setChatSessionId,
    applyFields,
  } = useAppStore()

  const [messages, setMessages] = useState<ChatMessageItem[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [sessionId, setSessionId] = useState<string | null>(chatSessionId)
  const [error, setError] = useState('')
  const [showSuccess, setShowSuccess] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  // Track whether initial message has been sent
  const [hasInitialized, setHasInitialized] = useState(false)

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // Focus input when modal opens
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 300)
    }
  }, [isOpen])

  // Send initial context message when modal opens
  useEffect(() => {
    if (!isOpen || hasInitialized) return

    setError('')
    setShowSuccess(false)
    setHasInitialized(true)

    // Build initial message based on existing content/media
    if (images.length > 0 || videos.length > 0) {
      const msg = title || description
        ? `我已上传了素材，当前标题是"${title}"，简介是"${description}"。请根据素材优化这些内容。`
        : '我已上传了素材，请根据素材帮我创作内容。'
      // Use setTimeout to break out of render cycle
      setTimeout(() => handleSendMessage(msg, true), 0)
    } else if (title || description) {
      setTimeout(() => handleSendMessage(`当前标题是"${title}"，简介是"${description}"。我想继续完善这些内容。`, true), 0)
    } else {
      setTimeout(() => handleSendMessage('', true), 0)
    }
  }, [isOpen])

  // Reset initialized state when modal closes
  useEffect(() => {
    if (!isOpen) {
      setTimeout(() => setHasInitialized(false), 300)
    }
  }, [isOpen])

  const handleSendMessage = async (msg?: string, isInitial = false) => {
    const text = msg || inputValue.trim()
    if (!text && !isInitial) return
    if (isLoading) return

    setIsLoading(true)
    setError('')

    // Cancel previous request if any
    if (abortRef.current) {
      abortRef.current.abort()
    }
    const controller = new AbortController()
    abortRef.current = controller

    // Add user message (skip for initial auto-message)
    if (!isInitial) {
      setMessages(prev => [...prev, { role: 'user', content: text }])
      setInputValue('')
    }

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
        isInitial ? text : text,
        currentContext,
        sessionId || undefined,
        controller.signal
      )

      if (controller.signal.aborted) return

      // Save session ID
      if (result.session_id) {
        setSessionId(result.session_id)
        setChatSessionId(result.session_id)
      }

      // Check if we got final fields
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
        }

        // Show success
        setShowSuccess(true)
        const successMsg = result.reply || '内容已生成！'
        setMessages(prev => [...prev, { role: 'assistant', content: successMsg, fields: result.fields }])

        // Auto-close after showing success
        setTimeout(() => {
          setShowSuccess(false)
          onClose()
        }, 2000)
      } else {
        // Add AI response with suggestions
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: result.reply,
          suggestions: result.suggestions,
        }])
      }
    } catch (error: any) {
      if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
      console.error('Chat generate failed:', error)
      setError('生成失败，请重试')
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
      handleSendMessage()
    }
  }

  const handleSuggestionClick = (suggestionText: string) => {
    handleSendMessage(suggestionText)
  }

  const handleClose = () => {
    if (abortRef.current) {
      abortRef.current.abort()
      abortRef.current = null
    }
    setMessages([])
    setSessionId(chatSessionId)
    setError('')
    setShowSuccess(false)
    onClose()
  }

  // Auto-resize textarea
  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInputValue(e.target.value)
    e.target.style.height = 'auto'
    e.target.style.height = `${Math.min(e.target.scrollHeight, 120)}px`
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div
        className="bg-white rounded-2xl shadow-2xl w-[560px] max-w-[90vw] max-h-[85vh] flex flex-col overflow-hidden animate-in"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 bg-primary/10 rounded-lg flex items-center justify-center">
              <svg className="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <h3 className="text-base font-semibold text-gray-800">AI 智能创作</h3>
          </div>
          <button
            onClick={handleClose}
            className="p-1.5 hover:bg-gray-100 rounded-lg transition-colors"
          >
            <svg className="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Success overlay */}
        {showSuccess && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-white/80 backdrop-blur-sm">
            <div className="flex flex-col items-center gap-3">
              <div className="w-14 h-14 bg-green-100 rounded-full flex items-center justify-center">
                <svg className="w-7 h-7 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <p className="text-base font-semibold text-green-700">内容已生成!</p>
            </div>
          </div>
        )}

        {/* Messages area */}
        <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4 min-h-[300px]">
          {messages.length === 0 && !isLoading && (
            <div className="flex flex-col items-center justify-center h-48 text-gray-400">
              <svg className="w-12 h-12 mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
              </svg>
              <p className="text-sm">正在启动对话...</p>
            </div>
          )}

          {messages.map((msg, index) => (
            <div key={index} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div className={`max-w-[80%] ${
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
        <div className="border-t border-gray-100 px-6 py-4">
          <div className="flex items-end gap-2">
            <div className="flex-1 relative">
              <textarea
                ref={inputRef}
                value={inputValue}
                onChange={handleInputChange}
                onKeyDown={handleKeyDown}
                placeholder="输入你的想法，按 Enter 发送..."
                rows={1}
                disabled={isLoading}
                className="w-full px-4 py-2.5 text-sm bg-gray-50 border border-gray-200 rounded-xl focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none placeholder:text-gray-400 disabled:opacity-50"
              />
            </div>
            <button
              onClick={() => handleSendMessage()}
              disabled={isLoading || !inputValue.trim()}
              className="px-4 py-2.5 bg-primary text-white rounded-xl hover:bg-primary-dark transition-colors disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-1.5"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
              </svg>
              <span className="text-sm">发送</span>
            </button>
          </div>
          <p className="text-[10px] text-gray-400 mt-1.5">Shift + Enter 换行</p>
        </div>
      </div>
    </div>
  )
}

export default GenerateModal
