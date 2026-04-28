import React, { useState, useRef } from 'react'
import { aiPolishSubmit, queryPolishResult, aiGenerateFromMedia, failNode, failTask, recordContextEvent } from '../services/api'
import { useAppStore } from '../stores/appStore'
import { setAIAbort } from '../utils/ai-loading'

const MAX_POLL_ATTEMPTS = 75
const POLL_INTERVAL_MS = 800

const AIHelperPanel: React.FC = () => {
  const { title, description, images, videos, setTitle, setDescription, contentType, setAILoadingMessage } = useAppStore()

  const generateAbortRef = useRef<AbortController | null>(null)
  const polishAbortRef = useRef<AbortController | null>(null)
  const polishTaskIdRef = useRef<string | null>(null)
  const polishNodeIdRef = useRef<string | null>(null)
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
    setAILoadingMessage('AI正在生成标题和简介...')
    const controller = new AbortController()
    generateAbortRef.current = controller
    setAIAbort(() => {
      controller.abort()
      setAILoadingMessage(null)
      generateAbortRef.current = null
      recordContextEvent('', 'AI_CANCELLED', '用户取消了AI生成操作')
    })
    try {
      const imageFiles = images.map(i => i.file)
      const videoFiles = videos.map(v => v.file)
      const result = await aiGenerateFromMedia('根据素材自动生成标题和简介', imageFiles, videoFiles, controller.signal)
      if (controller.signal.aborted) return
      if (result.title) setTitle(result.title)
      if (result.description) setDescription(result.description)
      showMsg('内容已生成')
    } catch (e: any) {
      if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
      console.error('AI生成失败:', e)
      showMsg('生成失败，请重试', 'error')
    } finally {
      setAILoadingMessage(null)
      setAIAbort(null)
      if (generateAbortRef.current === controller) {
        generateAbortRef.current = null
      }
    }
  }

  const handlePolish = async () => {
    const hasTitle = title.trim().length > 0
    const hasDesc = description.trim().length > 0

    if (!hasTitle && !hasDesc) {
      showMsg('请先输入标题或简介内容', 'error')
      return
    }

    const tasks: { text: string; type: 'title' | 'description' }[] = []
    if (hasTitle) tasks.push({ text: title, type: 'title' })
    if (hasDesc) tasks.push({ text: description, type: 'description' })

    const controller = new AbortController()
    polishAbortRef.current = controller

    let cancelled = false
    let successCount = 0
    polishTaskIdRef.current = null
    polishNodeIdRef.current = null

    setAIAbort(() => {
      cancelled = true
      controller.abort()

      const nodeId = polishNodeIdRef.current
      const taskId = polishTaskIdRef.current
      if (nodeId) {
        // Mark the node as FAILED + task as FAILED so the backend stops processing
        failNode(nodeId, '用户主动取消').catch(() => {})
        if (taskId) {
          failTask(taskId).catch(() => {})
        }
        // Record cancellation in context log
        recordContextEvent(taskId || '', 'AI_CANCELLED', '用户主动取消了AI润色操作', nodeId)
      } else {
        recordContextEvent('', 'AI_CANCELLED', '用户主动取消了AI润色操作')
      }

      setAILoadingMessage(null)
      polishAbortRef.current = null
    })

    try {
      for (const task of tasks) {
        if (cancelled) return
        setAILoadingMessage(`AI正在润色${task.type === 'title' ? '标题' : '简介'}...`)

        // Step 1: submit the polish DAG
        const submitResp = await aiPolishSubmit(task.text, task.type, controller.signal)
        if (cancelled) return

        polishTaskIdRef.current = submitResp.taskId
        polishNodeIdRef.current = submitResp.nodeId

        // Step 2: poll for result
        let polled = false
        for (let i = 0; i < MAX_POLL_ATTEMPTS; i++) {
          if (cancelled) return
          await new Promise(r => setTimeout(r, POLL_INTERVAL_MS))
          if (cancelled) return

          const queryResp = await queryPolishResult(submitResp.taskId, submitResp.nodeId, controller.signal)
          if (cancelled) return

          if (queryResp.status === 'SUCCESS') {
            if (task.type === 'title') {
              setTitle(queryResp.content || '')
            } else {
              setDescription(queryResp.content || '')
            }
            successCount++
            polled = true
            break
          }

          if (queryResp.status === 'FAILED' || queryResp.status === 'ERROR') {
            console.error('AI润色节点失败:', queryResp.error)
            break
          }

          // RUNNING / READY / CREATED — keep polling
        }

        if (!polled) {
          console.error('AI润色超时或失败:', task.type)
        }
      }

      if (successCount > 0) {
        const label = tasks.length > 1 ? '标题和简介' : (tasks[0].type === 'title' ? '标题' : '简介')
        showMsg(`已润色${label}`)
      } else {
        showMsg('润色失败，请重试', 'error')
      }
    } catch (e: any) {
      if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
      console.error('AI润色失败:', e)
      if (successCount > 0) {
        showMsg('部分润色完成，请检查结果', 'error')
      } else {
        showMsg('润色失败，请重试', 'error')
      }
    } finally {
      setAILoadingMessage(null)
      setAIAbort(null)
      if (polishAbortRef.current === controller) {
        polishAbortRef.current = null
      }
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
              className="px-3 py-1.5 bg-primary text-white rounded-lg text-xs hover:bg-primary-dark transition-colors flex items-center gap-1.5"
            >
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
              className="px-3 py-1.5 bg-amber-500 text-white rounded-lg text-xs hover:bg-amber-600 transition-colors flex items-center gap-1.5"
            >
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
