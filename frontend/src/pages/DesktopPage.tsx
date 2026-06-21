import React, { useState, useEffect } from 'react'
import { FiCheckCircle, FiCpu, FiFilm, FiImage, FiKey, FiMessageSquare, FiRefreshCw, FiSave } from 'react-icons/fi'
import CommandPanel from '../components/CommandPanel'
import {
  fetchModelProviderSettings,
  getLocalAgentBaseUrl,
  mergeModelProviderSettings,
  saveModelProviderSettings,
  type ModelCapability,
  type ModelProviderConfig,
} from '../services/localAgent'
import { getElectronAPI } from '../utils/electron'

const providerRows: Array<{
  id: ModelCapability
  label: string
  desc: string
  icon: typeof FiMessageSquare
}> = [
  {
    id: 'text_to_text',
    label: '文生文',
    desc: '脚本、标题、分镜、Prompt 等文本生成',
    icon: FiMessageSquare,
  },
  {
    id: 'text_to_image',
    label: '文生图片',
    desc: '关键帧、封面、视觉参考图生成',
    icon: FiImage,
  },
  {
    id: 'text_to_video',
    label: '文生视频',
    desc: '成片、镜头片段、动态素材生成',
    icon: FiFilm,
  },
]

const DesktopPage: React.FC = () => {
  const [backendStatus, setBackendStatus] = useState<'checking' | 'connected' | 'disconnected'>('checking')
  const [serviceInfo, setServiceInfo] = useState({ host: getLocalAgentBaseUrl(), pid: '' })
  const [localDirectory, setLocalDirectory] = useState('未选择')
  const [providerSettings, setProviderSettings] = useState(() => mergeModelProviderSettings())
  const [providerLoading, setProviderLoading] = useState(true)
  const [providerSaving, setProviderSaving] = useState(false)
  const [providerMessage, setProviderMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null)
  const [activeTab, setActiveTab] = useState<ModelCapability>('text_to_text')
  const api = getElectronAPI()

  useEffect(() => {
    const checkHealth = async () => {
      if (!api) {
        setBackendStatus('disconnected')
        return
      }
      try {
        if (api.runtimeConfig?.localAgentUrl) {
          setServiceInfo(prev => ({ ...prev, host: api.runtimeConfig.localAgentUrl }))
        }
        const status = await api.checkServiceHealth()
        setBackendStatus(status === 'ok' ? 'connected' : 'disconnected')
      } catch {
        setBackendStatus('disconnected')
      }
    }

    checkHealth()
    const interval = setInterval(checkHealth, 15000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    loadModelProviderSettings()
  }, [])

  const loadModelProviderSettings = async () => {
    setProviderLoading(true)
    setProviderMessage(null)
    try {
      const response = await fetchModelProviderSettings()
      setProviderSettings(mergeModelProviderSettings(response.providers))
    } catch (error) {
      setProviderMessage({ type: 'error', text: error instanceof Error ? error.message : '读取模型设置失败' })
    } finally {
      setProviderLoading(false)
    }
  }

  const updateProvider = (capability: ModelCapability, field: keyof Pick<ModelProviderConfig, 'baseUrl' | 'model' | 'apiKey'>, value: string) => {
    setProviderSettings(prev => ({
      ...prev,
      [capability]: {
        ...prev[capability],
        [field]: value,
      },
    }))
  }

  const handleSaveProviders = async () => {
    setProviderSaving(true)
    setProviderMessage(null)
    try {
      const response = await saveModelProviderSettings(providerSettings)
      setProviderSettings(mergeModelProviderSettings(response.providers))
      setProviderMessage({ type: 'success', text: '模型 API 设置已保存到本地' })
    } catch (error) {
      setProviderMessage({ type: 'error', text: error instanceof Error ? error.message : '保存模型设置失败' })
    } finally {
      setProviderSaving(false)
    }
  }

  const handlePickSaveDir = async () => {
    if (!api) return
    try {
      const paths = await api.openDirectoryDialog()
      if (paths?.length) setLocalDirectory(paths[0])
    } catch { /* cancelled */ }
  }

  return (
    <div className="flex-1 p-8 overflow-y-auto">
      <div className="max-w-5xl mx-auto">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-10 h-10 bg-purple-100 rounded-xl flex items-center justify-center">
              <svg className="w-5 h-5 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </div>
            <div>
              <h2 className="text-2xl font-bold text-gray-800">桌面工具</h2>
              <p className="text-sm text-gray-500">系统状态监控与命令执行</p>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Left: Status & Info */}
          <div className="lg:col-span-1 space-y-6">
            {/* Backend Status */}
            <div className="bg-white rounded-2xl border border-gray-100 p-5">
              <h3 className="text-sm font-semibold text-gray-800 mb-4 flex items-center gap-2">
                <svg className="w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
                本地服务
              </h3>
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-gray-500">连接状态</span>
                  <span className={`inline-flex items-center gap-1.5 text-xs font-medium ${
                    backendStatus === 'connected' ? 'text-green-600' :
                    backendStatus === 'checking' ? 'text-yellow-600' : 'text-red-600'
                  }`}>
                    <span className={`w-2 h-2 rounded-full ${
                      backendStatus === 'connected' ? 'bg-green-500' :
                      backendStatus === 'checking' ? 'bg-yellow-500 animate-pulse' : 'bg-red-500'
                    }`} />
                    {backendStatus === 'connected' ? '已连接' :
                     backendStatus === 'checking' ? '检查中...' : '未连接'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-xs text-gray-500">服务地址</span>
                  <span className="text-xs font-mono text-gray-700">{serviceInfo.host}</span>
                </div>
                <div className="pt-2">
                  <button
                    onClick={handlePickSaveDir}
                    className="w-full px-3 py-2 bg-gray-50 hover:bg-gray-100 text-gray-600 rounded-lg text-xs transition-colors flex items-center justify-center gap-1.5"
                  >
                    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 12h14M12 5l7 7-7 7" />
                    </svg>
                    选择本地目录
                  </button>
                </div>
              </div>
            </div>

            {/* App Info */}
            <div className="bg-white rounded-2xl border border-gray-100 p-5">
              <h3 className="text-sm font-semibold text-gray-800 mb-4 flex items-center gap-2">
                <svg className="w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                应用信息
              </h3>
              <div className="space-y-2 text-xs text-gray-600">
                <div className="flex justify-between">
                  <span>版本</span>
                  <span className="font-mono">0.0.1</span>
                </div>
                <div className="flex justify-between">
                  <span>运行环境</span>
                  <span className="font-mono">Electron</span>
                </div>
                <div className="flex justify-between">
                  <span>本地目录</span>
                  <span className="font-mono text-gray-400 truncate w-32 text-right">{localDirectory}</span>
                </div>
              </div>
            </div>
          </div>

          {/* Right: Model Configuration & Command Execution */}
          <div className="lg:col-span-2 space-y-6">
            <div className="bg-white rounded-2xl border border-gray-100 p-5">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between mb-5">
                <div>
                  <h3 className="text-sm font-semibold text-gray-800 flex items-center gap-2">
                    <FiCpu className="w-4 h-4 text-gray-500" />
                    基础模型 API
                  </h3>
                  <p className="mt-1 text-xs text-gray-500">OpenAI-compatible 接口地址 / 模型名 / 密钥，按能力分别配置</p>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={loadModelProviderSettings}
                    disabled={providerLoading || providerSaving}
                    className="h-8 w-8 rounded-lg border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-50 flex items-center justify-center"
                    title="重新读取"
                  >
                    <FiRefreshCw className={`w-4 h-4 ${providerLoading ? 'animate-spin' : ''}`} />
                  </button>
                  <button
                    type="button"
                    onClick={handleSaveProviders}
                    disabled={providerLoading || providerSaving}
                    className="h-8 px-3 rounded-lg bg-gray-900 text-white text-xs font-medium hover:bg-gray-800 disabled:opacity-50 flex items-center gap-1.5"
                  >
                    {providerSaving ? <FiRefreshCw className="w-3.5 h-3.5 animate-spin" /> : <FiSave className="w-3.5 h-3.5" />}
                    保存
                  </button>
                </div>
              </div>

              {/* Tab bar */}
              <div className="flex rounded-lg border border-gray-200 bg-gray-100 p-1 mb-4">
                {providerRows.map((row) => (
                  <button
                    key={row.id}
                    type="button"
                    onClick={() => setActiveTab(row.id)}
                    className={`flex-1 h-9 rounded-md text-sm font-medium flex items-center justify-center gap-2 transition-colors ${
                      activeTab === row.id
                        ? 'bg-white text-gray-800 shadow-sm'
                        : 'text-gray-500 hover:text-gray-700'
                    }`}
                  >
                    <row.icon className="w-4 h-4" />
                    {row.label}
                  </button>
                ))}
              </div>

              {/* Active tab content */}
              {(() => {
                const row = providerRows.find((r) => r.id === activeTab)!
                const provider = providerSettings[activeTab]
                const tokenPlaceholder = provider.hasApiKey
                  ? `已保存 ${provider.apiKeyPreview || 'token'}，留空保留原密钥`
                  : 'sk-... 输入 API Key'

                return (
                  <div className="rounded-lg border border-gray-100 bg-gray-50/70 p-5 space-y-4">
                    <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
                      <div className="w-9 h-9 rounded-xl bg-white border border-gray-100 flex items-center justify-center text-gray-600">
                        <row.icon className="w-5 h-5" />
                      </div>
                      <div>
                        <div className="text-sm font-semibold text-gray-800">{row.label} · API 配置</div>
                        <div className="text-xs text-gray-500">{row.desc}</div>
                      </div>
                    </div>

                    {/* API URL — full width */}
                    <label className="block">
                      <span className="text-xs font-semibold text-gray-600">接口地址 (Base URL)</span>
                      <input
                        value={provider.baseUrl}
                        onChange={(event) => updateProvider(activeTab, 'baseUrl', event.target.value)}
                        className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-gray-400 focus:ring-1 focus:ring-gray-200"
                        placeholder="https://api.openai.com/v1"
                      />
                      <span className="mt-1 text-[11px] text-gray-400">OpenAI-compatible 端点，例如 https://ark.cn-beijing.volces.com/api/coding/v3</span>
                    </label>

                    {/* Model + Token side by side */}
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <label className="block">
                        <span className="text-xs font-semibold text-gray-600">模型名称 (Model)</span>
                        <input
                          value={provider.model}
                          onChange={(event) => updateProvider(activeTab, 'model', event.target.value)}
                          className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-gray-400 focus:ring-1 focus:ring-gray-200"
                          placeholder="model-name"
                        />
                        <span className="mt-1 text-[11px] text-gray-400">例如 gpt-4.1 / doubao-seed-2.0-pro</span>
                      </label>
                      <label className="block">
                        <span className="text-xs font-semibold text-gray-600 flex items-center gap-1">
                          <FiKey className="w-3 h-3" />
                          API 密钥 (Token)
                        </span>
                        <input
                          value={provider.apiKey || ''}
                          onChange={(event) => updateProvider(activeTab, 'apiKey', event.target.value)}
                          className="mt-1.5 w-full h-10 rounded-lg border border-gray-200 bg-white px-3 text-sm font-mono text-gray-700 outline-none focus:border-gray-400 focus:ring-1 focus:ring-gray-200"
                          placeholder={tokenPlaceholder}
                          type="password"
                          autoComplete="off"
                        />
                        <span className="mt-1 text-[11px] text-gray-400">密钥仅保存在本机，不上传云端</span>
                      </label>
                    </div>
                  </div>
                )
              })()}

              {providerMessage && (
                <div className={`mt-4 flex items-center gap-2 rounded-lg px-3 py-2 text-xs ${
                  providerMessage.type === 'success'
                    ? 'bg-green-50 text-green-700'
                    : 'bg-red-50 text-red-700'
                }`}>
                  {providerMessage.type === 'success' ? <FiCheckCircle className="w-4 h-4" /> : <FiKey className="w-4 h-4" />}
                  <span>{providerMessage.text}</span>
                </div>
              )}
            </div>
            <CommandPanel />
          </div>
        </div>
      </div>
    </div>
  )
}

export default DesktopPage
