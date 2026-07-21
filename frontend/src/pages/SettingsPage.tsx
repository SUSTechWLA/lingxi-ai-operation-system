import React, { useState, useEffect, type KeyboardEvent } from 'react'
import { FiArrowLeft, FiCheck, FiCheckCircle, FiCopy, FiCpu, FiDownload, FiFilm, FiImage, FiKey, FiMessageSquare, FiMonitor, FiMoon, FiRefreshCw, FiSave, FiSun, FiUserCheck, FiX } from 'react-icons/fi'
import {
  checkJiMengLogin,
  fetchLocalAgentHealth,
  fetchJiMengSetupStatus,
  fetchModelProviderSettings,
  getLocalAgentBaseUrl,
  installJiMengCLI,
  loginJiMengHeadless,
  mergeModelProviderSettings,
  registerJiMengMCP,
  saveModelProviderSettings,
  type JiMengSetupStatusResponse,
  type LocalMCPProviderConfig,
  type MCPToolCallResult,
  type ModelCapability,
  type ModelProviderConfig,
} from '../services/localAgent'
import { getElectronAPI } from '../utils/electron'
import { useTheme } from '../theme/ThemeContext'
import type { ThemeMode } from '../theme/theme'

const providerRows: Array<{
  id: ModelCapability
  label: string
  desc: string
  icon: typeof FiMessageSquare
}> = [
  {
    id: 'text_to_text',
    label: '文本生成',
    desc: '脚本、标题、分镜、Prompt 等文本生成',
    icon: FiMessageSquare,
  },
  {
    id: 'text_to_image',
    label: '图片生成',
    desc: '关键帧、封面、视觉参考图生成',
    icon: FiImage,
  },
  {
    id: 'text_to_video',
    label: '视频生成',
    desc: '成片、镜头片段、动态素材生成',
    icon: FiFilm,
  },
]

type SettingsTab = ModelCapability | 'appearance'

const settingsTabs: Array<{
  id: SettingsTab
  label: string
  icon: typeof FiMessageSquare
}> = [
  ...providerRows.map(({ id, label, icon }) => ({ id, label, icon })),
  { id: 'appearance', label: '外观', icon: FiSun },
]

const settingsTabId = (tab: SettingsTab) => `settings-tab-${tab}`
const settingsPanelId = (tab: SettingsTab) => `settings-panel-${tab}`

const appearanceModes: Array<{
  id: ThemeMode
  label: string
  detail: string
  icon: typeof FiSun
}> = [
  { id: 'system', label: '跟随系统', detail: '自动匹配 macOS 外观', icon: FiMonitor },
  { id: 'light', label: '浅色', detail: '始终使用明亮界面', icon: FiSun },
  { id: 'dark', label: '深色', detail: '始终使用夜间界面', icon: FiMoon },
]

type SettingsActionMessage = {
  type: 'info' | 'success' | 'error'
  text: string
}

type JiMengSetupAction = 'refresh' | 'install' | 'register' | null

interface SettingsPageProps {
  variant?: 'creator' | 'developer'
  onBack?: () => void
}

const SettingsPage: React.FC<SettingsPageProps> = ({ variant = 'developer', onBack }) => {
  const [backendStatus, setBackendStatus] = useState<'checking' | 'connected' | 'disconnected'>('checking')
  const [serviceInfo, setServiceInfo] = useState({ host: getLocalAgentBaseUrl(), pid: '' })
  const [localDirectory, setLocalDirectory] = useState('未选择')
  const [directoryMessage, setDirectoryMessage] = useState<SettingsActionMessage | null>(null)
  const [providerSettings, setProviderSettings] = useState(() => mergeModelProviderSettings())
  const [providerLoading, setProviderLoading] = useState(true)
  const [providerSaving, setProviderSaving] = useState(false)
  const [providerMessage, setProviderMessage] = useState<SettingsActionMessage | null>(null)
  const [activeTab, setActiveTab] = useState<SettingsTab>('text_to_text')
  const [jimengSetupStatus, setJimengSetupStatus] = useState<JiMengSetupStatusResponse | null>(null)
  const [jimengSetupLoading, setJimengSetupLoading] = useState(false)
  const [jimengSetupAction, setJimengSetupAction] = useState<JiMengSetupAction>(null)
  const [jimengSetupMessage, setJimengSetupMessage] = useState<SettingsActionMessage | null>(null)
  const api = getElectronAPI()
  const { mode: themeMode, setMode: setThemeMode } = useTheme()

  useEffect(() => {
    const checkHealth = async () => {
      try {
        if (api?.runtimeConfig?.localAgentUrl) {
          setServiceInfo(prev => ({ ...prev, host: api.runtimeConfig.localAgentUrl }))
        }
        const health = await fetchLocalAgentHealth()
        setBackendStatus(health.status === 'ok' ? 'connected' : 'disconnected')
      } catch {
        setBackendStatus('disconnected')
      }
    }

    checkHealth()
    const interval = setInterval(checkHealth, 15000)
    return () => clearInterval(interval)
  }, [api])

  useEffect(() => {
    loadModelProviderSettings()
    loadJiMengSetupStatus()
  }, [])

  const loadModelProviderSettings = async (showFeedback = false) => {
    setProviderLoading(true)
    setProviderMessage(showFeedback ? { type: 'info', text: '正在刷新模型 API 设置...' } : null)
    try {
      const response = await fetchModelProviderSettings()
      const merged = mergeModelProviderSettings(response.providers)
      setProviderSettings(merged)
      if (showFeedback) {
        setProviderMessage({ type: 'success', text: '模型 API 设置已刷新' })
      }
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
    setProviderMessage({ type: 'info', text: '正在保存模型 API 设置...' })
    try {
      const response = await saveModelProviderSettings(providerSettings)
      setProviderSettings(mergeModelProviderSettings(response.providers))
      setProviderMessage({
        type: 'success',
        text: '模型 API 设置已在本机持久化',
      })
    } catch (error) {
      setProviderMessage({ type: 'error', text: error instanceof Error ? error.message : '保存模型设置失败' })
    } finally {
      setProviderSaving(false)
    }
  }

  const handleClearProviderKey = async (capability: ModelCapability) => {
    if (!window.confirm('确定清除该生成能力已保存的 API 密钥？')) return
    setProviderSaving(true)
    setProviderMessage({ type: 'info', text: '正在清除已保存密钥...' })
    try {
      const response = await saveModelProviderSettings({
        [capability]: {
          clearApiKey: true,
        },
      })
      setProviderSettings(mergeModelProviderSettings(response.providers))
      setProviderMessage({ type: 'success', text: '已从本机清除该 API 密钥' })
    } catch (error) {
      setProviderMessage({ type: 'error', text: error instanceof Error ? error.message : '清除 API 密钥失败' })
    } finally {
      setProviderSaving(false)
    }
  }

  const handleSettingsTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let nextIndex: number | undefined
    if (event.key === 'ArrowRight') nextIndex = (index + 1) % settingsTabs.length
    if (event.key === 'ArrowLeft') nextIndex = (index - 1 + settingsTabs.length) % settingsTabs.length
    if (event.key === 'Home') nextIndex = 0
    if (event.key === 'End') nextIndex = settingsTabs.length - 1
    if (nextIndex === undefined) return
    event.preventDefault()
    const nextTab = settingsTabs[nextIndex].id
    setActiveTab(nextTab)
    document.getElementById(settingsTabId(nextTab))?.focus()
  }

  const loadJiMengSetupStatus = async (showFeedback = false) => {
    setJimengSetupLoading(true)
    setJimengSetupAction(showFeedback ? 'refresh' : null)
    setJimengSetupMessage(showFeedback ? { type: 'info', text: '正在刷新即梦配置状态...' } : null)
    try {
      const status = await fetchJiMengSetupStatus()
      setJimengSetupStatus(status)
      if (showFeedback) {
        setJimengSetupMessage({ type: 'success', text: '即梦配置状态已刷新' })
      }
    } catch (error) {
      setJimengSetupStatus(null)
      setJimengSetupMessage({ type: 'error', text: settingsErrorMessage(error, '读取即梦设置失败') })
    } finally {
      setJimengSetupLoading(false)
      setJimengSetupAction(null)
    }
  }

  const handleInstallJiMengCLI = async () => {
    setJimengSetupLoading(true)
    setJimengSetupAction('install')
    setJimengSetupMessage({ type: 'info', text: '正在安装/更新 Dreamina CLI，这可能需要几分钟，请不要关闭本地服务。' })
    try {
      const result = await installJiMengCLI()
      if (result.status === 'failed') {
        throw new Error(result.error || result.stderr || '安装即梦 CLI 失败')
      }
      const status = await fetchJiMengSetupStatus()
      setJimengSetupStatus(status)
      setJimengSetupMessage({ type: 'success', text: 'Dreamina CLI 安装/更新完成，已刷新即梦配置状态。' })
    } catch (error) {
      setJimengSetupMessage({ type: 'error', text: settingsErrorMessage(error, '安装即梦 CLI 失败') })
    } finally {
      setJimengSetupLoading(false)
      setJimengSetupAction(null)
    }
  }

  const handleRegisterJiMengMCP = async () => {
    setJimengSetupLoading(true)
    setJimengSetupAction('register')
    setJimengSetupMessage({ type: 'info', text: '正在注册即梦 MCP provider...' })
    try {
      await registerJiMengMCP({ transport: 'stdio' })
      const status = await fetchJiMengSetupStatus()
      setJimengSetupStatus(status)
      setJimengSetupMessage({ type: 'success', text: '即梦 MCP 已注册，已刷新连接状态。' })
    } catch (error) {
      setJimengSetupMessage({ type: 'error', text: settingsErrorMessage(error, '注册即梦 MCP 失败') })
    } finally {
      setJimengSetupLoading(false)
      setJimengSetupAction(null)
    }
  }

  const handlePickSaveDir = async () => {
    if (!api) {
      setDirectoryMessage({ type: 'error', text: '当前环境不支持选择本地目录。' })
      return
    }
    setDirectoryMessage({ type: 'info', text: '正在打开目录选择窗口...' })
    try {
      const paths = await api.openDirectoryDialog()
      if (paths?.length) {
        setLocalDirectory(paths[0])
        setDirectoryMessage({ type: 'success', text: '已选择本地目录。' })
      } else {
        setDirectoryMessage({ type: 'info', text: '未选择新目录。' })
      }
    } catch (error) {
      setDirectoryMessage({ type: 'error', text: settingsErrorMessage(error, '选择本地目录失败') })
    }
  }

  const activeProviderRow = activeTab === 'appearance'
    ? undefined
    : providerRows.find((row) => row.id === activeTab)
  const activeProvider = activeTab === 'appearance' ? undefined : providerSettings[activeTab]

  return (
    <div className={`settings-page flex-1 overflow-y-auto ${variant === 'creator' ? 'py-2' : 'p-8'}`}>
      <div className="max-w-5xl mx-auto">
        <div className="mb-8">
          {variant === 'creator' && onBack && (
            <button type="button" className="settings-back-button" onClick={onBack}>
              <FiArrowLeft aria-hidden="true" />
              返回创作
            </button>
          )}
          <div className="flex items-center gap-3 mb-2">
            <div className="w-10 h-10 bg-primary-soft rounded-xl flex items-center justify-center">
              <svg className="w-5 h-5 text-primary-dark" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </div>
            <div>
              <h1 className="text-2xl font-bold text-ink">设置</h1>
              <p className="text-sm text-ink-soft">管理生成能力和界面外观，密钥仅在本地 Agent 持久化</p>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Left: Status & Info */}
          <div className="lg:col-span-1 space-y-6">
            {/* Backend Status */}
            <div className="bg-background-card rounded-2xl border border-line p-5 shadow-card">
              <h3 className="text-sm font-semibold text-ink mb-4 flex items-center gap-2">
                <svg className="w-4 h-4 text-ink-soft" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
                本地服务
              </h3>
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-ink-soft">连接状态</span>
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
                  <span className="text-xs text-ink-soft">服务地址</span>
                  <span className="text-xs font-mono text-ink-muted">{serviceInfo.host}</span>
                </div>
                <div className="pt-2">
                  <button
                    onClick={handlePickSaveDir}
                    className="w-full px-3 py-2 bg-background hover:bg-background-mist text-ink-muted rounded-lg text-xs transition-colors flex items-center justify-center gap-1.5"
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
            <div className="bg-background-card rounded-2xl border border-line p-5 shadow-card">
              <h3 className="text-sm font-semibold text-ink mb-4 flex items-center gap-2">
                <svg className="w-4 h-4 text-ink-soft" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                应用信息
              </h3>
              <div className="space-y-2 text-xs text-ink-muted">
                <div className="flex justify-between">
                  <span>版本</span>
                  <span className="font-mono">0.2.1</span>
                </div>
                <div className="flex justify-between">
                  <span>运行环境</span>
                  <span className="font-mono">Electron</span>
                </div>
                <div className="flex justify-between">
                  <span>本地目录</span>
                  <span className="font-mono text-ink-soft truncate w-32 text-right">{localDirectory}</span>
                </div>
              </div>
              <SettingsActionNotice message={directoryMessage} compact className="mt-3" />
            </div>
          </div>

          <div className="lg:col-span-2 space-y-6">
            <div className="bg-background-card rounded-2xl border border-line p-5 shadow-card">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between mb-4">
                <div>
                  <h3 className="text-sm font-semibold text-ink flex items-center gap-2">
                    <FiCpu className="w-4 h-4 text-ink-soft" />
                    生成与外观
                  </h3>
                  <p className="mt-1 text-xs text-ink-soft">三种生成能力分别配置，密钥仅在本机持久化</p>
                </div>
              </div>

              <div className="settings-tab-list" role="tablist" aria-label="设置分类">
                {settingsTabs.map((row, index) => (
                  <button
                    key={row.id}
                    id={settingsTabId(row.id)}
                    type="button"
                    role="tab"
                    aria-selected={activeTab === row.id}
                    aria-controls={activeTab === row.id ? settingsPanelId(row.id) : undefined}
                    tabIndex={activeTab === row.id ? 0 : -1}
                    onClick={() => setActiveTab(row.id)}
                    onKeyDown={(event) => handleSettingsTabKeyDown(event, index)}
                    className={activeTab === row.id ? 'settings-tab is-active' : 'settings-tab'}
                  >
                    <row.icon aria-hidden="true" />
                    {row.label}
                  </button>
                ))}
              </div>

              {activeTab === 'appearance' ? (
                <section
                  id={settingsPanelId(activeTab)}
                  className="settings-appearance"
                  role="tabpanel"
                  aria-labelledby={settingsTabId(activeTab)}
                >
                  <div>
                    <h3>选择界面外观</h3>
                    <p>跟随环境切换，也可以固定使用浅色或夜间皮肤。</p>
                  </div>
                  <div className="settings-theme-options" role="radiogroup" aria-label="界面外观">
                    {appearanceModes.map((appearance) => (
                      <label key={appearance.id} className={themeMode === appearance.id ? 'settings-theme-option is-selected' : 'settings-theme-option'}>
                        <input
                          type="radio"
                          name="theme-mode"
                          value={appearance.id}
                          checked={themeMode === appearance.id}
                          onChange={() => setThemeMode(appearance.id)}
                        />
                        <appearance.icon aria-hidden="true" />
                        <span>
                          <strong>{appearance.label}</strong>
                          <small>{appearance.detail}</small>
                        </span>
                      </label>
                    ))}
                  </div>
                </section>
              ) : activeProviderRow && activeProvider ? (
                <section
                  id={settingsPanelId(activeTab)}
                  className="settings-provider-panel"
                  role="tabpanel"
                  aria-labelledby={settingsTabId(activeTab)}
                >
                  <div className="settings-panel-heading">
                    <div>
                      <h3>{activeProviderRow.label}</h3>
                      <p>{activeProviderRow.desc}</p>
                    </div>
                    <div className="settings-panel-actions">
                      <button type="button" onClick={() => loadModelProviderSettings(true)} disabled={providerLoading || providerSaving} title="重新读取">
                        <FiRefreshCw className={providerLoading ? 'animate-spin' : ''} aria-hidden="true" />
                        <span className="creator-visually-hidden">重新读取</span>
                      </button>
                      <button type="button" className="is-primary" onClick={handleSaveProviders} disabled={providerLoading || providerSaving}>
                        {providerSaving ? <FiRefreshCw className="animate-spin" aria-hidden="true" /> : <FiSave aria-hidden="true" />}
                        {providerSaving ? '保存中' : '保存设置'}
                      </button>
                    </div>
                  </div>

                  <label className="settings-field">
                    <span>接口地址</span>
                    <input value={activeProvider.baseUrl} onChange={(event) => updateProvider(activeProviderRow.id, 'baseUrl', event.target.value)} placeholder="https://api.openai.com/v1" />
                    <small>填写 OpenAI-compatible Base URL</small>
                  </label>
                  <div className="settings-field-grid">
                    <label className="settings-field">
                      <span>模型名称</span>
                      <input value={activeProvider.model} onChange={(event) => updateProvider(activeProviderRow.id, 'model', event.target.value)} placeholder="model-name" />
                    </label>
                    <div className="settings-field">
                      <label htmlFor={`provider-api-key-${activeProviderRow.id}`}><FiKey aria-hidden="true" /> API 密钥</label>
                      <input
                        id={`provider-api-key-${activeProviderRow.id}`}
                        value={activeProvider.apiKey || ''}
                        onChange={(event) => updateProvider(activeProviderRow.id, 'apiKey', event.target.value)}
                        placeholder={activeProvider.hasApiKey ? `已保存 ${activeProvider.apiKeyPreview || 'token'}，留空保留` : '输入 API Key'}
                        type="password"
                        autoComplete="off"
                      />
                      <small>密钥仅在本机持久化；开始生成时会随本次已认证生成请求传输给云端编排，不写入项目配置或数据库。</small>
                      {activeProvider.hasApiKey && (
                        <button
                          type="button"
                          className="settings-clear-key"
                          disabled={providerLoading || providerSaving}
                          onClick={() => handleClearProviderKey(activeProviderRow.id)}
                        >
                          清除已保存密钥
                        </button>
                      )}
                    </div>
                  </div>

                  <SettingsActionNotice message={providerMessage} className="mt-4" />
                </section>
              ) : null}
            </div>

            {activeTab === 'text_to_video' && (
              <JiMengSettingsPanel
                status={jimengSetupStatus}
                loading={jimengSetupLoading}
                action={jimengSetupAction}
                message={jimengSetupMessage}
                onRefresh={() => loadJiMengSetupStatus(true)}
                onInstallCLI={handleInstallJiMengCLI}
                onRegisterMCP={handleRegisterJiMengMCP}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function JiMengSettingsPanel(props: {
  status: JiMengSetupStatusResponse | null
  loading: boolean
  action: JiMengSetupAction
  message: SettingsActionMessage | null
  onRefresh: () => void
  onInstallCLI: () => void
  onRegisterMCP: () => void
}) {
  const { status, loading, action, message, onRefresh, onInstallCLI, onRegisterMCP } = props
  const providerStatus = status?.mcpProviders?.find((item) => item.id === 'jimeng')
  const mcpRegistered = Boolean(status?.mcpProvider)
  const mcpReachable = providerStatus?.reachable === true
  const ready = isJiMengReady(status)
  const startCommand = status?.mcpStartCommand || 'python3 mcp/jimeng/server.py'
  const providerDetail = status?.mcpProvider ? mcpProviderDetail(status.mcpProvider) : 'stdio provider'
  const installCommand = status?.installCommand || 'curl -fsSL https://jimeng.jianying.com/cli | bash'
  const headline = ready
    ? '即梦 CLI 已可用于图片 / 视频生成'
    : status?.dreaminaAvailable
      ? '即梦 CLI 已安装，等待 MCP 连接'
      : '即梦 CLI 未检测到'
  const [loginResult, setLoginResult] = useState<MCPToolCallResult | null>(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginMessage, setLoginMessage] = useState<SettingsActionMessage | null>(null)
  const loginData = loginResult?.structuredContent || {}
  const verificationUri = stringRecordValue(loginData, 'verification_uri') || stringRecordValue(loginData, 'verificationUri')
  const userCode = stringRecordValue(loginData, 'user_code') || stringRecordValue(loginData, 'userCode')
  const deviceCode = stringRecordValue(loginData, 'device_code') || stringRecordValue(loginData, 'deviceCode')

  const handleLoginHeadless = async () => {
    setLoginLoading(true)
    setLoginMessage({ type: 'info', text: '正在获取即梦登录码...' })
    try {
      const result = await loginJiMengHeadless()
      setLoginResult(result)
      if (result.isError) {
        setLoginMessage({ type: 'error', text: result.content?.[0]?.text || '即梦登录启动失败' })
      } else {
        setLoginMessage({ type: 'success', text: '登录码已生成，请在即梦页面完成授权。' })
      }
    } catch (error) {
      setLoginMessage({ type: 'error', text: settingsErrorMessage(error, '即梦登录启动失败') })
    } finally {
      setLoginLoading(false)
    }
  }

  const handleCheckLogin = async () => {
    if (!deviceCode) return
    setLoginLoading(true)
    setLoginMessage({ type: 'info', text: '正在检查即梦登录状态...' })
    try {
      const result = await checkJiMengLogin(deviceCode, 30)
      setLoginResult(result)
      if (result.isError) {
        setLoginMessage({ type: 'error', text: result.content?.[0]?.text || '即梦登录未完成' })
      } else {
        setLoginMessage({ type: 'success', text: '即梦登录已确认。' })
      }
    } catch (error) {
      setLoginMessage({ type: 'error', text: settingsErrorMessage(error, '即梦登录检查失败') })
    } finally {
      setLoginLoading(false)
    }
  }

  return (
    <div className="bg-background-card rounded-2xl border border-line p-5 shadow-card">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-ink flex items-center gap-2">
            <FiFilm className="w-4 h-4 text-ink-soft" />
            即梦 CLI / 图片与视频生成
          </h3>
          <p className="mt-1 text-xs text-ink-soft">
            安装 Dreamina CLI、注册本地 MCP provider，并完成即梦登录。该配置和文生图片、文生视频能力一起用于 AIGC 素材生成。
          </p>
        </div>
        <span className={`inline-flex shrink-0 items-center rounded-full px-2.5 py-1 text-xs font-bold ring-1 ${
          ready
            ? 'bg-success/10 text-success ring-success/25'
            : mcpRegistered || status?.dreaminaAvailable
              ? 'bg-primary-soft text-primary-dark ring-line'
              : 'bg-background text-ink-muted ring-line'
        }`}>
          {ready ? '可用于生成' : '需配置'}
        </span>
      </div>

      <div className="mt-4 rounded-lg border border-line bg-background/70 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="text-sm font-black text-ink">{headline}</div>
            <p className="mt-1 text-xs leading-5 text-ink-muted">
              用户自己的 Dreamina 登录态保留在本机；云端只通过本地 runner 调用已注册的 JiMeng MCP。
            </p>
          </div>
          <div className="flex flex-wrap gap-1.5">
            <span className="rounded bg-background-card px-2 py-1 text-[10px] font-bold text-primary-dark ring-1 ring-line">文生图片</span>
            <span className="rounded bg-background-card px-2 py-1 text-[10px] font-bold text-primary-dark ring-1 ring-line">文生视频</span>
          </div>
        </div>

        <SettingsActionNotice message={message} className="mt-3" />

        <div className="mt-4 grid gap-3 md:grid-cols-3">
          <JiMengStep label="Dreamina CLI" detail={status?.dreaminaVersion || installCommand} done={status?.dreaminaAvailable === true} />
          <JiMengStep label="MCP 注册" detail={providerDetail} done={mcpRegistered} />
          <JiMengStep label="MCP 连接" detail={providerStatus?.error || (mcpReachable ? 'tools/list 正常' : '等待服务启动')} done={mcpReachable} />
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <button
            type="button"
            onClick={onInstallCLI}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-3 py-2 text-xs font-black text-on-primary shadow-glow disabled:cursor-not-allowed disabled:opacity-50"
          >
            {action === 'install' ? <FiRefreshCw className="animate-spin" /> : <FiDownload />} {action === 'install' ? '正在安装/更新' : '安装/更新 CLI'}
          </button>
          <button
            type="button"
            onClick={onRegisterMCP}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-lg bg-background-card px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
          >
            {action === 'register' ? <FiRefreshCw className="animate-spin" /> : <FiCheck />} {action === 'register' ? '正在注册' : '注册 MCP'}
          </button>
          <button
            type="button"
            onClick={onRefresh}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-lg bg-background-card px-3 py-2 text-xs font-black text-ink-muted ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
          >
            <FiRefreshCw className={action === 'refresh' ? 'animate-spin' : ''} /> {action === 'refresh' ? '正在刷新' : '刷新状态'}
          </button>
        </div>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        <div className="rounded-lg bg-background-card p-4 ring-1 ring-line">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-black text-primary-dark">MCP 启动命令</span>
            <SettingsCopyButton value={startCommand} label="复制命令" />
          </div>
          <code className="mt-2 block break-all rounded bg-ink px-3 py-2 font-mono text-[11px] leading-5 text-white">{startCommand}</code>
        </div>

        <div className="rounded-lg bg-background-card p-4 ring-1 ring-line">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs font-black text-primary-dark">首次登录授权</span>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={handleLoginHeadless}
                disabled={!mcpReachable || loginLoading}
                className="inline-flex items-center gap-1.5 rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
              >
                <FiUserCheck /> 获取登录码
              </button>
              <button
                type="button"
                onClick={handleCheckLogin}
                disabled={!deviceCode || loginLoading}
                className="inline-flex items-center gap-1.5 rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black text-ink-muted ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
              >
                <FiRefreshCw className={loginLoading ? 'animate-spin' : ''} /> 检查登录
              </button>
            </div>
          </div>
          <SettingsActionNotice message={loginMessage} compact className="mt-2" />
          {verificationUri || userCode ? (
            <div className="mt-3 space-y-2">
              {verificationUri ? <LoginCopyRow label="授权页面" value={verificationUri} /> : null}
              {userCode ? <LoginCopyRow label="用户码" value={userCode} /> : null}
            </div>
          ) : (
            <p className="mt-2 text-xs leading-5 text-ink-muted">MCP 连接后可生成登录码，按即梦页面提示完成授权。</p>
          )}
        </div>
      </div>
    </div>
  )
}

function LoginCopyRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-background-card px-3 py-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-black text-ink-soft">{label}</span>
        <SettingsCopyButton value={value} label="复制" />
      </div>
      <div className="mt-1 break-all font-mono text-[11px] leading-4 text-ink">{value}</div>
    </div>
  )
}

function SettingsActionNotice({ message, compact = false, className = '' }: { message: SettingsActionMessage | null; compact?: boolean; className?: string }) {
  if (!message) return null
  const toneClass = message.type === 'success'
    ? 'border-success/25 bg-success/10 text-success'
    : message.type === 'info'
      ? 'border-primary/15 bg-primary-soft text-primary-dark'
      : 'border-danger/25 bg-danger/10 text-danger'
  return (
    <div className={`${className} flex items-center gap-2 rounded-lg border px-3 ${compact ? 'py-1.5' : 'py-2'} text-xs font-semibold ${toneClass}`}>
      {message.type === 'success' ? (
        <FiCheckCircle className="h-4 w-4 shrink-0" />
      ) : message.type === 'info' ? (
        <FiRefreshCw className="h-4 w-4 shrink-0 animate-spin" />
      ) : (
        <FiKey className="h-4 w-4 shrink-0" />
      )}
      <span className="min-w-0">{message.text}</span>
    </div>
  )
}

function JiMengStep({ label, detail, done }: { label: string; detail: string; done: boolean }) {
  return (
    <div className={`rounded-lg border p-3 ${done ? 'border-success/25 bg-success/10' : 'border-line bg-background-card'}`}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-black text-ink">{label}</span>
        <span className={`grid h-5 w-5 place-items-center rounded-full text-[11px] ${done ? 'bg-success text-background-card' : 'bg-background-mist text-ink-soft'}`}>
          {done ? <FiCheck /> : <FiX />}
        </span>
      </div>
      <p className="mt-2 line-clamp-2 break-all text-[11px] leading-4 text-ink-muted">{detail}</p>
    </div>
  )
}

function SettingsCopyButton({ value, label }: { value: string; label: string }) {
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const handleCopy = async () => {
    try {
      await copyToClipboard(value)
      setCopyState('copied')
    } catch {
      setCopyState('failed')
    }
    window.setTimeout(() => setCopyState('idle'), 1400)
  }
  return (
    <button
      type="button"
      onClick={handleCopy}
      className={`inline-flex items-center gap-1.5 rounded-lg bg-background-card px-2.5 py-1.5 text-xs font-black ring-1 ring-line hover:bg-primary-soft ${copyState === 'failed' ? 'text-red-700' : 'text-primary-dark'}`}
    >
      <FiCopy /> {copyState === 'copied' ? '已复制' : copyState === 'failed' ? '复制失败' : label}
    </button>
  )
}

async function copyToClipboard(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

function mcpProviderDetail(provider: LocalMCPProviderConfig): string {
  if (provider.transport === 'stdio') {
    return [provider.command, ...(provider.args || [])].filter(Boolean).join(' ') || 'stdio'
  }
  return provider.endpoint || provider.transport || 'mcp provider'
}

function isJiMengReady(status: JiMengSetupStatusResponse | null): boolean {
  if (!status?.dreaminaAvailable) return false
  const provider = status.mcpProviders?.find((item) => item.id === 'jimeng')
  if (!provider?.reachable) return false
  return Boolean(provider.tools?.some((tool) => tool.name === 'jimeng.generate_video'))
}

function stringRecordValue(record: Record<string, unknown>, key: string): string {
  const value = record[key]
  return typeof value === 'string' ? value : ''
}

function settingsErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

export default SettingsPage
