import React, { useState, useRef, useEffect } from 'react'
import { createSkillSession, chatSkillSession, getSkillSession, terminateSkillSession, uploadMedia } from '../services/api'
import { useAppStore } from '../stores/appStore'
import { setAIAbort, cancelAI } from '../utils/ai-loading'
import type { ChatMessageItem } from '../utils/types'

const AIAssistantTab: React.FC = () => {
  const {
    title,
    description,
    keywords,
    body,
    images,
    videos,
    cover,
    chatSessionId,
    setChatSessionId,
    applyFields,
    setAILoadingMessage,
  } = useAppStore()

  const [messages, setMessages] = useState<ChatMessageItem[]>([])
  const [inputValue, setInputValue] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [sessionId, setSessionId] = useState<string | null>(chatSessionId)
  const [error, setError] = useState('')
  const [loadingText, setLoadingText] = useState('')
  const [showResult, setShowResult] = useState<{ text: string; type: 'success' | 'error' } | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)
  const prevChatSessionIdRef = useRef<string | null>(chatSessionId)
  const [hasInitialized, setHasInitialized] = useState(false)
  const [messageHistory, setMessageHistory] = useState<string[]>([])
  const [historyIndex, setHistoryIndex] = useState(-1)
  const savedInputRef = useRef('')

  // Auto-resize textarea when inputValue changes
  useEffect(() => {
    if (inputRef.current) {
      inputRef.current.style.height = 'auto'
      inputRef.current.style.height = `${Math.min(inputRef.current.scrollHeight, 80)}px`
    }
  }, [inputValue])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // Focus input on mount
  useEffect(() => {
    setTimeout(() => inputRef.current?.focus(), 200)
  }, [])

  // When chatSessionId is cleared externally (e.g. content type switch),
  // reset the AI assistant conversation state
  useEffect(() => {
    if (prevChatSessionIdRef.current && !chatSessionId) {
      if (sessionId) {
        terminateSkillSession(sessionId).catch(() => {})
      }
      setSessionId(null)
      setMessages([])
      showWelcome()
    }
    prevChatSessionIdRef.current = chatSessionId
  }, [chatSessionId])

  // Progress text that updates while loading — both inline and overlay
  useEffect(() => {
    if (!isLoading) {
      setLoadingText('')
      return
    }
    setLoadingText('正在理解需求...')
    setAILoadingMessage('正在理解你的需求...')
    const t1 = setTimeout(() => {
      setLoadingText('正在分析需求...')
      setAILoadingMessage('正在分析内容方向，马上就好...')
    }, 2000)
    const t2 = setTimeout(() => {
      setLoadingText('正在生成内容...')
      setAILoadingMessage('AI 正在为你创作内容...')
    }, 5000)
    const t3 = setTimeout(() => {
      setLoadingText('内容生成中，请耐心等待...')
      setAILoadingMessage('内容生成中，请耐心等待...')
    }, 15000)
    const t4 = setTimeout(() => {
      setLoadingText('正在优化结果...')
      setAILoadingMessage('正在做最后的润色优化...')
    }, 30000)
    return () => {
      clearTimeout(t1); clearTimeout(t2); clearTimeout(t3); clearTimeout(t4)
    }
  }, [isLoading])

  // On mount: restore existing session history or show welcome message
  useEffect(() => {
    if (hasInitialized) return
    setHasInitialized(true)

    if (chatSessionId) {
      // Resume existing conversation
      getSkillSession(chatSessionId)
        .then((session) => {
          if (session.messages && session.messages.length > 0) {
            setMessages(session.messages)
            setSessionId(session.session_id)
          } else {
            showWelcome()
          }
        })
        .catch((err) => {
          console.warn('Failed to load session history, starting fresh:', err)
          setChatSessionId(null)
          showWelcome()
        })
    } else {
      showWelcome()
    }
  }, [])

  const showWelcome = () => {
    setMessages([{
      role: 'assistant',
      content: '你好！我是 AI 创作助手，可以帮你：\n\n• 根据素材生成标题和简介\n• 优化和完善现有内容\n• 创作灵感和建议\n• 回答自媒体相关问题\n\n有什么我可以帮你的吗？',
    }])
  }

  const showToast = (text: string, type: 'success' | 'error' = 'success') => {
    setShowResult({ text, type })
    setTimeout(() => setShowResult(null), 2000)
  }

  const handleSendMessage = async () => {
    const text = inputValue.trim()
    if (!text || isLoading) return

    setIsLoading(true)
    setLoadingText('正在理解需求...')
    setError('')
    setAILoadingMessage('AI 助手正在处理任务...')

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

    // Add to history for arrow-key navigation
    setMessageHistory(prev => [...prev, text])
    setHistoryIndex(-1)
    savedInputRef.current = ''

    // Add user message to UI
    setMessages(prev => [...prev, { role: 'user', content: text }])
    setInputValue('')

    // Upload media files and return their server-side IDs.
    // Only called on the first attempt (not on retry), since retries re-use
    // media that was already uploaded.
    const uploadMediaFiles = async (): Promise<{ mediaIds: string[]; mediaNames: string[] }> => {
      const mediaIds: string[] = []
      const mediaNames: string[] = []
      const allMedia = [...images, ...videos]
      if (cover) allMedia.push(cover)
      if (allMedia.length === 0) return { mediaIds, mediaNames }

      setLoadingText('正在上传素材...')
      for (const media of allMedia) {
        try {
          const asset = await uploadMedia(media.file, controller.signal)
          if (asset?.id) {
            mediaIds.push(asset.id)
            mediaNames.push(media.name)
          }
        } catch (uploadErr: any) {
          if (uploadErr?.name === 'CanceledError') throw uploadErr
          console.warn('Media upload failed, continuing without ID:', media.name, uploadErr)
        }
      }
      return { mediaIds, mediaNames }
    }

    // Core send logic extracted so we can retry on stale-session errors.
    // - isRetry=false: normal flow — upload media, create session if needed, then chat.
    // - isRetry=true : stale session cleared — always create a fresh session, skip re-upload.
    const sendWithSession = async (isRetry: boolean): Promise<void> => {
      let currentSessionId = sessionId

      if (!currentSessionId || isRetry) {
        const uploadResult = isRetry ? { mediaIds: [] as string[], mediaNames: [] as string[] } : await uploadMediaFiles()
        if (controller.signal.aborted) return

        const allMediaNames = [
          ...images.map(i => i.name),
          ...videos.map(v => v.name),
          ...(cover ? [cover.name] : []),
        ]

        const currentContext = {
          title,
          description,
          body,
          keywords: keywords ? keywords.split(/[,，、\s]+/).filter(Boolean) : [],
          media_count: images.length + videos.length + (cover ? 1 : 0),
          media_names: allMediaNames,
          media_ids: uploadResult.mediaIds,
        }
        const session = await createSkillSession(currentContext)
        currentSessionId = session.session_id
        setSessionId(currentSessionId)
        setChatSessionId(currentSessionId)
      }

      const result = await chatSkillSession(
        currentSessionId,
        text,
        controller.signal
      )

      if (controller.signal.aborted) return

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
        // Conversational response (may include suggestions and/or progress)
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: result.reply,
          suggestions: result.suggestions,
        }])
      }
    }

    try {
      await sendWithSession(false)
    } catch (error: any) {
      if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return

      const backendMsg = error?.response?.data?.message

      // If the session was not found (expired in Redis), clear and retry once
      if (error?.response?.status === 404 && backendMsg === 'session not found') {
        setSessionId(null)
        setChatSessionId(null)
        try {
          await sendWithSession(true)
          return
        } catch (retryErr: any) {
          if (retryErr?.name === 'CanceledError' || retryErr?.code === 'ERR_CANCELED') return
          console.error('AI assistant retry failed:', retryErr)
          const retryMsg = retryErr?.response?.data?.message
          setError(retryMsg || '请求失败，请重试')
          return
        }
      }

      console.error('AI assistant failed:', error)
      if (error?.code === 'ECONNABORTED') {
        setError('请求超时，请重试')
      } else if (backendMsg) {
        setError(backendMsg)
      } else {
        setError('请求失败，请重试')
      }
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
      return
    }

    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (messageHistory.length === 0) return

      if (historyIndex === -1) {
        savedInputRef.current = inputValue
      }

      const newIndex = historyIndex === -1 ? messageHistory.length - 1 : Math.max(0, historyIndex - 1)
      setHistoryIndex(newIndex)
      setInputValue(messageHistory[newIndex])
      return
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (historyIndex === -1) return

      if (historyIndex >= messageHistory.length - 1) {
        setHistoryIndex(-1)
        setInputValue(savedInputRef.current)
      } else {
        const newIndex = historyIndex + 1
        setHistoryIndex(newIndex)
        setInputValue(messageHistory[newIndex])
      }
      return
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
    if (sessionId) {
      terminateSkillSession(sessionId).catch(() => {})
    }
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

        {/* Loading indicator with progress text and stop button */}
        {isLoading && (
          <div className="flex justify-start">
            <div className="bg-gray-50 rounded-2xl rounded-tl-md px-4 py-3">
              <div className="flex items-center gap-3">
                <div className="relative w-5 h-5 flex-shrink-0">
                  <div className="absolute inset-0 rounded-full border-2 border-gray-200" />
                  <div className="absolute inset-0 rounded-full border-2 border-primary border-t-transparent animate-spin" />
                </div>
                <div className="flex-1 min-w-0">
                  {loadingText && (
                    <p className="text-sm text-gray-500">{loadingText}</p>
                  )}
                </div>
                <button
                  onClick={() => cancelAI()}
                  className="flex-shrink-0 px-3 py-1 text-xs font-medium text-red-500 border border-red-200 rounded-lg hover:bg-red-50 hover:border-red-300 transition-colors"
                >
                  停止
                </button>
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
