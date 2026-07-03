import { useState } from 'react'
import { FiFileText, FiLogOut, FiServer, FiUser } from 'react-icons/fi'
import BiaoshuWorkbench from '../pages/BiaoshuWorkbench'
import ModelProviderSettingsPage from '../pages/ModelProviderSettingsPage'
import type { AuthUser } from '../services/auth'

type AppTab = 'workbench' | 'model-settings'

interface BiaoshuAppShellProps {
  user: AuthUser
  onLogout: () => void
}

export function BiaoshuAppShell({ user, onLogout }: BiaoshuAppShellProps) {
  const [activeTab, setActiveTab] = useState<AppTab>('workbench')

  return (
    <div className="h-screen flex flex-col bg-[#FBF7ED]">
      {/* Top Navigation */}
      <header className="flex items-center justify-between h-14 px-6 bg-white border-b border-gray-100 flex-shrink-0">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 text-sm font-bold text-gray-800">
            <div className="w-7 h-7 bg-amber-100 rounded-lg flex items-center justify-center">
              <span className="text-amber-600 text-sm font-extrabold">T</span>
            </div>
            躺营 AIOS
          </div>

          <nav className="flex items-center bg-gray-50 rounded-lg p-0.5">
            <button
              onClick={() => setActiveTab('workbench')}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition-colors ${
                activeTab === 'workbench'
                  ? 'bg-white text-amber-700 shadow-sm'
                  : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              <FiFileText className="w-3.5 h-3.5" />
              标书工作台
            </button>
            <button
              onClick={() => setActiveTab('model-settings')}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition-colors ${
                activeTab === 'model-settings'
                  ? 'bg-white text-blue-700 shadow-sm'
                  : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              <FiServer className="w-3.5 h-3.5" />
              模型接口配置
            </button>
          </nav>
        </div>

        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2 text-xs text-gray-500">
            <FiUser className="w-3.5 h-3.5" />
            <span className="font-medium text-gray-600">{user.nickname || user.email}</span>
          </div>
          <button
            onClick={onLogout}
            className="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-gray-500 hover:bg-gray-100 hover:text-red-500 transition-colors"
          >
            <FiLogOut className="w-3.5 h-3.5" />
            退出登录
          </button>
        </div>
      </header>

      {/* Main Content */}
      <main className="flex-1 flex overflow-hidden">
        {activeTab === 'workbench' && <BiaoshuWorkbench />}
        {activeTab === 'model-settings' && <ModelProviderSettingsPage />}
      </main>
    </div>
  )
}
