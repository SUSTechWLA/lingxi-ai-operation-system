import React, { useState, useEffect } from 'react'
import CommandPanel from '../components/CommandPanel'
import { getElectronAPI } from '../utils/electron'

const DesktopPage: React.FC = () => {
  const [backendStatus, setBackendStatus] = useState<'checking' | 'connected' | 'disconnected'>('checking')
  const [serviceInfo, setServiceInfo] = useState({ host: '127.0.0.1:18080', pid: '' })
  const [localDirectory, setLocalDirectory] = useState('未选择')
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

          {/* Right: Command Execution */}
          <div className="lg:col-span-2">
            <CommandPanel />
          </div>
        </div>
      </div>
    </div>
  )
}

export default DesktopPage
