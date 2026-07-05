import React, { useState, useEffect } from 'react'
import { FiCheck, FiCheckCircle, FiCopy, FiCpu, FiDownload, FiFilm, FiImage, FiKey, FiMessageSquare, FiRefreshCw, FiSave, FiUserCheck, FiX } from 'react-icons/fi'
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
  const [jimengSetupStatus, setJimengSetupStatus] = useState<JiMengSetupStatusResponse | null>(null)
  const [jimengSetupLoading, setJimengSetupLoading] = useState(false)
  const [jimengSetupError, setJimengSetupError] = useState<string | null>(null)
  const api = getElectronAPI()

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

  const loadModelProviderSettings = async () => {
    setProviderLoading(true)
    setProviderMessage(null)
    try {
      const response = await fetchModelProviderSettings()
      const merged = mergeModelProviderSettings(response.providers)
      setProviderSettings(merged)
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
      setProviderMessage({
        type: 'success',
        text: '模型 API 设置已保存到本机，不会上传云端',
      })
    } catch (error) {
      setProviderMessage({ type: 'error', text: error instanceof Error ? error.message : '保存模型设置失败' })
    } finally {
      setProviderSaving(false)
    }
  }

  const loadJiMengSetupStatus = async () => {
    setJimengSetupLoading(true)
    setJimengSetupError(null)
    try {
      const status = await fetchJiMengSetupStatus()
      setJimengSetupStatus(status)
    } catch (error) {
      setJimengSetupStatus(null)
      setJimengSetupError(settingsErrorMessage(error, '读取即梦设置失败'))
    } finally {
      setJimengSetupLoading(false)
    }
  }

  const handleInstallJiMengCLI = async () => {
    setJimengSetupLoading(true)
    setJimengSetupError(null)
    try {
      await installJiMengCLI()
      await loadJiMengSetupStatus()
    } catch (error) {
      setJimengSetupError(settingsErrorMessage(error, '安装即梦 CLI 失败'))
    } finally {
      setJimengSetupLoading(false)
    }
  }

  const handleRegisterJiMengMCP = async () => {
    setJimengSetupLoading(true)
    setJimengSetupError(null)
    try {
      await registerJiMengMCP({ transport: 'stdio' })
      await loadJiMengSetupStatus()
    } catch (error) {
      setJimengSetupError(settingsErrorMessage(error, '注册即梦 MCP 失败'))
    } finally {
      setJimengSetupLoading(false)
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
            <div className="w-10 h-10 bg-primary-soft rounded-xl flex items-center justify-center">
              <svg className="w-5 h-5 text-primary-dark" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </div>
            <div>
              <h2 className="text-2xl font-bold text-ink">桌面工具</h2>
              <p className="text-sm text-ink-soft">系统状态监控与本机模型设置</p>
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
                  <span className="font-mono">0.0.1</span>
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
            </div>
          </div>

          {/* Right: Model Configuration */}
          <div className="lg:col-span-2 space-y-6">
            <div className="bg-background-card rounded-2xl border border-line p-5 shadow-card">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between mb-5">
                <div>
                  <h3 className="text-sm font-semibold text-ink flex items-center gap-2">
                    <FiCpu className="w-4 h-4 text-ink-soft" />
                    基础模型 API
                  </h3>
                  <p className="mt-1 text-xs text-ink-soft">OpenAI-compatible 接口地址 / 模型名 / 密钥，按能力分别配置，仅保存到本机</p>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={loadModelProviderSettings}
                    disabled={providerLoading || providerSaving}
                    className="h-8 w-8 rounded-lg border border-line text-ink-soft hover:bg-background disabled:opacity-50 flex items-center justify-center"
                    title="重新读取"
                  >
                    <FiRefreshCw className={`w-4 h-4 ${providerLoading ? 'animate-spin' : ''}`} />
                  </button>
                  <button
                    type="button"
                    onClick={handleSaveProviders}
                    disabled={providerLoading || providerSaving}
                    className="h-8 px-3 rounded-lg bg-primary-dark text-white text-xs font-medium hover:bg-[#1A0B02] disabled:opacity-50 flex items-center gap-1.5"
                  >
                    {providerSaving ? <FiRefreshCw className="w-3.5 h-3.5 animate-spin" /> : <FiSave className="w-3.5 h-3.5" />}
                    保存
                  </button>
                </div>
              </div>

              {/* Tab bar */}
              <div className="flex rounded-lg border border-line bg-background p-1 mb-4">
                {providerRows.map((row) => (
                  <button
                    key={row.id}
                    type="button"
                    onClick={() => setActiveTab(row.id)}
                    className={`flex-1 h-9 rounded-md text-sm font-medium flex items-center justify-center gap-2 transition-colors ${
                      activeTab === row.id
                        ? 'bg-white text-ink shadow-sm'
                        : 'text-ink-soft hover:text-ink-muted'
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
                  <div className="rounded-lg border border-line bg-background/70 p-5 space-y-4">
                    <div className="flex items-center gap-3 pb-3 border-b border-line">
                      <div className="w-9 h-9 rounded-xl bg-white border border-line flex items-center justify-center text-primary-dark">
                        <row.icon className="w-5 h-5" />
                      </div>
                      <div>
                        <div className="text-sm font-semibold text-ink">{row.label} · API 配置</div>
                        <div className="text-xs text-ink-soft">{row.desc}</div>
                      </div>
                    </div>

                    {/* API URL — full width */}
                    <label className="block">
                      <span className="text-xs font-semibold text-ink-muted">接口地址 (Base URL)</span>
                      <input
                        value={provider.baseUrl}
                        onChange={(event) => updateProvider(activeTab, 'baseUrl', event.target.value)}
                        className="mt-1.5 w-full h-10 rounded-lg border border-line bg-white px-3 text-sm font-mono text-ink-muted outline-none focus:border-primary focus:ring-1 focus:ring-primary/20"
                        placeholder="https://api.openai.com/v1"
                      />
                      <span className="mt-1 text-[11px] text-ink-soft">OpenAI-compatible 端点，例如 https://ark.cn-beijing.volces.com/api/coding/v3</span>
                    </label>

                    {/* Model + Token side by side */}
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <label className="block">
                        <span className="text-xs font-semibold text-ink-muted">模型名称 (Model)</span>
                        <input
                          value={provider.model}
                          onChange={(event) => updateProvider(activeTab, 'model', event.target.value)}
                          className="mt-1.5 w-full h-10 rounded-lg border border-line bg-white px-3 text-sm font-mono text-ink-muted outline-none focus:border-primary focus:ring-1 focus:ring-primary/20"
                          placeholder="model-name"
                        />
                        <span className="mt-1 text-[11px] text-ink-soft">例如 gpt-4.1 / doubao-seed-2.0-pro</span>
                      </label>
                      <label className="block">
                        <span className="text-xs font-semibold text-ink-muted flex items-center gap-1">
                          <FiKey className="w-3 h-3" />
                          API 密钥 (Token)
                        </span>
                        <input
                          value={provider.apiKey || ''}
                          onChange={(event) => updateProvider(activeTab, 'apiKey', event.target.value)}
                          className="mt-1.5 w-full h-10 rounded-lg border border-line bg-white px-3 text-sm font-mono text-ink-muted outline-none focus:border-primary focus:ring-1 focus:ring-primary/20"
                          placeholder={tokenPlaceholder}
                          type="password"
                          autoComplete="off"
                        />
                        <span className="mt-1 text-[11px] text-ink-soft">密钥仅保存在本机，不上传云端</span>
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

            <JiMengSettingsPanel
              status={jimengSetupStatus}
              loading={jimengSetupLoading}
              error={jimengSetupError}
              onRefresh={loadJiMengSetupStatus}
              onInstallCLI={handleInstallJiMengCLI}
              onRegisterMCP={handleRegisterJiMengMCP}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

function JiMengSettingsPanel(props: {
  status: JiMengSetupStatusResponse | null
  loading: boolean
  error: string | null
  onRefresh: () => void
  onInstallCLI: () => void
  onRegisterMCP: () => void
}) {
  const { status, loading, error, onRefresh, onInstallCLI, onRegisterMCP } = props
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
  const [loginError, setLoginError] = useState<string | null>(null)
  const loginData = loginResult?.structuredContent || {}
  const verificationUri = stringRecordValue(loginData, 'verification_uri') || stringRecordValue(loginData, 'verificationUri')
  const userCode = stringRecordValue(loginData, 'user_code') || stringRecordValue(loginData, 'userCode')
  const deviceCode = stringRecordValue(loginData, 'device_code') || stringRecordValue(loginData, 'deviceCode')

  const handleLoginHeadless = async () => {
    setLoginLoading(true)
    setLoginError(null)
    try {
      const result = await loginJiMengHeadless()
      setLoginResult(result)
      if (result.isError) setLoginError(result.content?.[0]?.text || '即梦登录启动失败')
    } catch (error) {
      setLoginError(settingsErrorMessage(error, '即梦登录启动失败'))
    } finally {
      setLoginLoading(false)
    }
  }

  const handleCheckLogin = async () => {
    if (!deviceCode) return
    setLoginLoading(true)
    setLoginError(null)
    try {
      const result = await checkJiMengLogin(deviceCode, 30)
      setLoginResult(result)
      if (result.isError) setLoginError(result.content?.[0]?.text || '即梦登录未完成')
    } catch (error) {
      setLoginError(settingsErrorMessage(error, '即梦登录检查失败'))
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
            ? 'bg-green-50 text-green-700 ring-green-200'
            : mcpRegistered || status?.dreaminaAvailable
              ? 'bg-amber-50 text-primary-dark ring-amber-200'
              : 'bg-stone-50 text-stone-600 ring-stone-200'
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
            <span className="rounded bg-white px-2 py-1 text-[10px] font-bold text-primary-dark ring-1 ring-line">文生图片</span>
            <span className="rounded bg-white px-2 py-1 text-[10px] font-bold text-primary-dark ring-1 ring-line">文生视频</span>
          </div>
        </div>

        {error ? <div className="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs font-semibold text-red-700">{error}</div> : null}

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
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-3 py-2 text-xs font-black text-white shadow-glow disabled:cursor-not-allowed disabled:opacity-50"
          >
            <FiDownload /> 安装/更新 CLI
          </button>
          <button
            type="button"
            onClick={onRegisterMCP}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
          >
            <FiCheck /> 注册 MCP
          </button>
          <button
            type="button"
            onClick={onRefresh}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-xs font-black text-ink-muted ring-1 ring-line hover:bg-background-card disabled:cursor-not-allowed disabled:opacity-50"
          >
            <FiRefreshCw className={loading ? 'animate-spin' : ''} /> 刷新状态
          </button>
        </div>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        <div className="rounded-lg bg-white p-4 ring-1 ring-line">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-black text-primary-dark">MCP 启动命令</span>
            <SettingsCopyButton value={startCommand} label="复制命令" />
          </div>
          <code className="mt-2 block break-all rounded bg-ink px-3 py-2 font-mono text-[11px] leading-5 text-white">{startCommand}</code>
        </div>

        <div className="rounded-lg bg-white p-4 ring-1 ring-line">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs font-black text-primary-dark">首次登录授权</span>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={handleLoginHeadless}
                disabled={!mcpReachable || loginLoading}
                className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft disabled:cursor-not-allowed disabled:opacity-50"
              >
                <FiUserCheck /> 获取登录码
              </button>
              <button
                type="button"
                onClick={handleCheckLogin}
                disabled={!deviceCode || loginLoading}
                className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-ink-muted ring-1 ring-line hover:bg-background-card disabled:cursor-not-allowed disabled:opacity-50"
              >
                <FiRefreshCw className={loginLoading ? 'animate-spin' : ''} /> 检查登录
              </button>
            </div>
          </div>
          {loginError ? <div className="mt-2 text-xs font-semibold text-red-700">{loginError}</div> : null}
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

function JiMengStep({ label, detail, done }: { label: string; detail: string; done: boolean }) {
  return (
    <div className={`rounded-lg border p-3 ${done ? 'border-green-100 bg-green-50/70' : 'border-line bg-white'}`}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-black text-ink">{label}</span>
        <span className={`grid h-5 w-5 place-items-center rounded-full text-[11px] ${done ? 'bg-green-600 text-white' : 'bg-stone-100 text-ink-soft'}`}>
          {done ? <FiCheck /> : <FiX />}
        </span>
      </div>
      <p className="mt-2 line-clamp-2 break-all text-[11px] leading-4 text-ink-muted">{detail}</p>
    </div>
  )
}

function SettingsCopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    await copyToClipboard(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }
  return (
    <button
      type="button"
      onClick={handleCopy}
      className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1.5 text-xs font-black text-primary-dark ring-1 ring-line hover:bg-primary-soft"
    >
      <FiCopy /> {copied ? '已复制' : label}
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

export default DesktopPage
