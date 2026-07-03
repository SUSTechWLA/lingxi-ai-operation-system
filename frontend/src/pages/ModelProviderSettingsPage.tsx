import { useCallback, useEffect, useState } from 'react'
import { FiCheck, FiKey, FiSave, FiServer } from 'react-icons/fi'
import {
  fetchLocalAgentHealth,
  fetchModelProviderSettings,
  mergeModelProviderSettings,
  saveModelProviderSettings,
  type LocalAgentHealth,
  type ModelCapability,
  type ModelProviderConfig,
} from '../services/localAgent'

interface EditingProvider {
  baseUrl: string
  model: string
  apiKey: string
}

const CAPABILITY_KEYS: ModelCapability[] = ['text_to_text', 'text_to_image', 'text_to_video']

export default function ModelProviderSettingsPage() {
  const [agentHealth, setAgentHealth] = useState<LocalAgentHealth | null>(null)
  const [agentError, setAgentError] = useState<string | null>(null)
  const [savedProviders, setSavedProviders] = useState<Record<ModelCapability, ModelProviderConfig>>(
    () => mergeModelProviderSettings()
  )
  const [editing, setEditing] = useState<Record<ModelCapability, EditingProvider>>(() => {
    const defaults = mergeModelProviderSettings()
    return {
      text_to_text: { baseUrl: defaults.text_to_text.baseUrl, model: defaults.text_to_text.model, apiKey: '' },
      text_to_image: { baseUrl: defaults.text_to_image.baseUrl, model: defaults.text_to_image.model, apiKey: '' },
      text_to_video: { baseUrl: defaults.text_to_video.baseUrl, model: defaults.text_to_video.model, apiKey: '' },
    }
  })
  const [saving, setSaving] = useState<Partial<Record<ModelCapability, boolean>>>({})
  const [messages, setMessages] = useState<Partial<Record<ModelCapability, { type: 'success' | 'error'; text: string }>>>({})
  const [loading, setLoading] = useState(true)

  const loadSettings = useCallback(async () => {
    setLoading(true)
    setAgentError(null)
    try {
      const health = await fetchLocalAgentHealth()
      setAgentHealth(health)
      const resp = await fetchModelProviderSettings()
      const merged = mergeModelProviderSettings(resp.providers)
      setSavedProviders(merged)
      setEditing(prev => ({
        text_to_text: { ...prev.text_to_text, ...resp.providers.text_to_text, apiKey: '' },
        text_to_image: { ...prev.text_to_image, ...resp.providers.text_to_image, apiKey: '' },
        text_to_video: { ...prev.text_to_video, ...resp.providers.text_to_video, apiKey: '' },
      }))
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      setAgentError(msg)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadSettings()
  }, [loadSettings])

  const handleChange = (capability: ModelCapability, field: keyof EditingProvider, value: string) => {
    setEditing(prev => ({
      ...prev,
      [capability]: { ...prev[capability], [field]: value },
    }))
    setMessages(prev => ({ ...prev, [capability]: undefined }))
  }

  const handleSave = async (capability: ModelCapability) => {
    const cfg = editing[capability]
    if (!cfg.baseUrl.trim()) {
      setMessages(prev => ({ ...prev, [capability]: { type: 'error', text: 'Base URL 不能为空' } }))
      return
    }
    if (!/^https?:\/\//.test(cfg.baseUrl.trim())) {
      setMessages(prev => ({ ...prev, [capability]: { type: 'error', text: 'Base URL 必须以 http:// 或 https:// 开头' } }))
      return
    }
    if (!cfg.model.trim()) {
      setMessages(prev => ({ ...prev, [capability]: { type: 'error', text: '模型名称不能为空' } }))
      return
    }

    setSaving(prev => ({ ...prev, [capability]: true }))
    setMessages(prev => ({ ...prev, [capability]: undefined }))

    try {
      const resp = await saveModelProviderSettings({
        ...savedProviders,
        [capability]: {
          baseUrl: cfg.baseUrl.trim(),
          model: cfg.model.trim(),
          apiKey: cfg.apiKey || undefined,
        },
      })
      const merged = mergeModelProviderSettings(resp.providers)
      setSavedProviders(merged)
      setEditing(prev => ({
        ...prev,
        [capability]: { ...prev[capability], apiKey: '' },
      }))
      setMessages(prev => ({
        ...prev,
        [capability]: { type: 'success', text: '已保存到本机，不上传云端' },
      }))
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      setMessages(prev => ({ ...prev, [capability]: { type: 'error', text: `保存失败: ${msg}` } }))
    } finally {
      setSaving(prev => ({ ...prev, [capability]: false }))
    }
  }

  if (loading) {
    return (
      <div className="flex-1 p-8 overflow-y-auto">
        <div className="max-w-3xl mx-auto text-center text-sm text-gray-500 py-20">正在加载本地配置...</div>
      </div>
    )
  }

  return (
    <div className="flex-1 p-8 overflow-y-auto">
      <div className="max-w-3xl mx-auto">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-10 h-10 bg-blue-100 rounded-xl flex items-center justify-center text-lg">
              <FiServer className="w-5 h-5 text-blue-600" />
            </div>
            <div>
              <h2 className="text-2xl font-bold text-gray-800">LLM 模型接口配置</h2>
              <p className="text-sm text-gray-500">
                配置用于本机 AI 能力调用的 OpenAI-compatible 接口信息，配置仅保存到本机
              </p>
            </div>
          </div>

          {/* Agent Health */}
          <div className={`mt-4 inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium ${
            agentHealth ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-700'
          }`}>
            <span className={`w-2 h-2 rounded-full ${agentHealth ? 'bg-green-500' : 'bg-red-500'}`} />
            {agentHealth ? `Local Agent 已连接 (${agentHealth.os ?? '-'} / ${agentHealth.arch ?? '-'})` : `Local Agent 未连接${agentError ? `: ${agentError}` : ''}`}
          </div>
        </div>

        {/* Provider Cards */}
        <div className="space-y-6">
          {CAPABILITY_KEYS.map((capability) => {
            const saved = savedProviders[capability]
            const edit = editing[capability]
            const msg = messages[capability]
            const isSaving = saving[capability] ?? false
            const labels: Record<ModelCapability, { label: string; icon: string; description: string }> = {
              text_to_text: { label: '文本模型', icon: 'T', description: '用于标书生成、内容改写、审查和问答' },
              text_to_image: { label: '图像模型', icon: 'I', description: '用于后续图文生成、配图生成等能力' },
              text_to_video: { label: '视频模型', icon: 'V', description: '保留 AIOS 视频生成能力兼容，当前标书场景可暂不使用' },
            }
            const meta = labels[capability]

            return (
              <div key={capability} className="bg-white rounded-2xl border border-gray-100 p-5">
                <h3 className="text-sm font-semibold text-gray-800 mb-4 flex items-center gap-2">
                  <span className="w-6 h-6 rounded-md bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-bold">
                    {meta.icon}
                  </span>
                  {meta.label}
                </h3>
                <p className="text-xs text-gray-500 mb-4">{meta.description}</p>

                <div className="space-y-3">
                  {/* Base URL */}
                  <label className="block">
                    <span className="text-xs font-semibold text-gray-600">Base URL</span>
                    <input
                      value={edit.baseUrl}
                      onChange={e => handleChange(capability, 'baseUrl', e.target.value)}
                      className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-blue-400 focus:ring-1 focus:ring-blue-200"
                      placeholder="https://api.openai.com/v1"
                    />
                  </label>

                  {/* Model */}
                  <label className="block">
                    <span className="text-xs font-semibold text-gray-600">Model</span>
                    <input
                      value={edit.model}
                      onChange={e => handleChange(capability, 'model', e.target.value)}
                      className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-blue-400 focus:ring-1 focus:ring-blue-200"
                      placeholder="gpt-4.1 / deepseek-chat / qwen-plus"
                    />
                  </label>

                  {/* API Key */}
                  <label className="block">
                    <span className="text-xs font-semibold text-gray-600 flex items-center gap-2">
                      <FiKey className="w-3 h-3" />
                      API Key / Token
                      {saved.hasApiKey && (
                        <span className="text-[10px] font-normal text-green-600 bg-green-50 px-2 py-0.5 rounded-full inline-flex items-center gap-1">
                          <FiCheck className="w-3 h-3" />
                          已保存: {saved.apiKeyPreview || '••••'}
                        </span>
                      )}
                    </span>
                    <input
                      type="password"
                      value={edit.apiKey}
                      onChange={e => handleChange(capability, 'apiKey', e.target.value)}
                      className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-blue-400 focus:ring-1 focus:ring-blue-200"
                      placeholder="留空表示保留当前已保存的密钥"
                    />
                  </label>

                  {/* Save Button + Message */}
                  <div className="flex items-center gap-3 pt-1">
                    <button
                      onClick={() => handleSave(capability)}
                      disabled={isSaving}
                      className="h-9 px-5 rounded-lg bg-blue-500 text-white text-sm font-semibold hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 transition-colors"
                    >
                      {isSaving ? (
                        <>保存中...</>
                      ) : (
                        <><FiSave className="w-3.5 h-3.5" /> 保存到本机</>
                      )}
                    </button>

                    {msg && (
                      <span className={`text-xs ${msg.type === 'success' ? 'text-green-600' : 'text-red-500'}`}>
                        {msg.text}
                      </span>
                    )}
                  </div>
                </div>
              </div>
            )
          })}
        </div>

        {/* Footer note */}
        <p className="mt-6 text-xs text-gray-400 text-center">
          API Key 仅保存到本机 Local Agent 配置目录的 model-providers.json，不会上传云端
        </p>
      </div>
    </div>
  )
}
