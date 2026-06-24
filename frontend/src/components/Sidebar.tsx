import React from 'react'
import { FiLogOut, FiMonitor, FiVideo, FiZap } from 'react-icons/fi'
import type { AuthUser } from '../services/auth'
import { APP_ICON_PATH } from '../utils/brand'

interface SidebarProps {
  activeNav: string
  onNavChange: (nav: string) => void
  user?: AuthUser
  onLogout?: () => void
}

const Sidebar: React.FC<SidebarProps> = ({ activeNav, onNavChange, user, onLogout }) => {
  const navItems = [
    { id: 'oneclick', label: '视频创作', icon: <FiZap className="h-4 w-4" /> },
    { id: 'creator', label: '创作台', icon: <FiVideo className="h-4 w-4" /> },
    { id: 'system', label: '系统', icon: <FiMonitor className="h-4 w-4" /> },
  ]

  return (
    <aside className="w-64 bg-[#2B1708] text-white border-r border-[#2B1708] flex flex-col h-screen">
      <div className="p-6">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg flex items-center justify-center overflow-hidden">
            <img src={APP_ICON_PATH} alt="躺营 AI" className="w-full h-full object-cover" />
          </div>
          <div>
            <h1 className="text-base font-semibold">躺营 AI</h1>
            <p className="text-xs text-white/55">Guided Video Studio</p>
          </div>
        </div>
      </div>

      <nav className="flex-1 px-4 py-2">
        <div className="space-y-1">
          {navItems.map((item) => (
            <button
              key={item.id}
              onClick={() => onNavChange(item.id)}
              className={`w-full flex items-center gap-3 px-4 py-3 rounded-lg font-medium text-sm transition-colors ${
                activeNav === item.id
                  ? 'bg-[#FFCF4A] text-[#2B1708]'
                  : 'text-white/70 hover:bg-white/10 hover:text-white'
              }`}
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </div>
      </nav>

      <div className="space-y-3 p-4 border-t border-white/10">
        {user && (
          <div className="flex items-center gap-3 rounded-lg border border-white/10 bg-white/[0.04] p-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-[#FFCF4A] text-xs font-semibold text-[#2B1708]">
              {(user.nickname || user.email).slice(0, 1).toUpperCase()}
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-white">{user.nickname || '已登录'}</p>
              <p className="truncate text-xs text-white/45">{user.email}</p>
            </div>
            <button
              type="button"
              onClick={onLogout}
              className="flex h-8 w-8 items-center justify-center rounded-lg text-white/55 hover:bg-white/10 hover:text-white"
              title="退出登录"
            >
              <FiLogOut className="h-4 w-4" />
            </button>
          </div>
        )}
        <div className="rounded-lg border border-white/10 bg-white/5 p-4">
          <p className="text-xs font-semibold uppercase text-[#FFCF4A]">Runtime</p>
          <p className="mt-2 text-sm text-white/80">Skill 驱动</p>
          <p className="mt-1 text-xs leading-5 text-white/45">图片走 Codex imagegen，视频先交付外部生成素材包。</p>
        </div>
      </div>
    </aside>
  )
}

export default Sidebar
