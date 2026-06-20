import React from 'react'
import { FiArchive, FiCpu, FiMonitor, FiVideo } from 'react-icons/fi'
import appIcon from '../assets/aios-icon.png'

interface SidebarProps {
  activeNav: string
  onNavChange: (nav: string) => void
}

const Sidebar: React.FC<SidebarProps> = ({ activeNav, onNavChange }) => {
  const navItems = [
    { id: 'creator', label: '创作台', icon: <FiVideo className="h-4 w-4" /> },
    { id: 'projects', label: '作品', icon: <FiArchive className="h-4 w-4" /> },
    { id: 'skills', label: '技能', icon: <FiCpu className="h-4 w-4" /> },
    { id: 'system', label: '系统', icon: <FiMonitor className="h-4 w-4" /> },
  ]

  return (
    <aside className="w-64 bg-[#17181A] text-white border-r border-[#17181A] flex flex-col h-screen">
      <div className="p-6">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg flex items-center justify-center overflow-hidden">
            <img src={appIcon} alt="AIOS" className="w-full h-full object-cover" />
          </div>
          <div>
            <h1 className="text-base font-semibold">躺营 AI</h1>
            <p className="text-xs text-white/55">自媒体视频创作台</p>
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
                  ? 'bg-[#D6FF4D] text-[#17181A]'
                  : 'text-white/70 hover:bg-white/10 hover:text-white'
              }`}
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </div>
      </nav>

      <div className="p-4 border-t border-white/10">
        <div className="rounded-lg border border-white/10 bg-white/5 p-4">
          <p className="text-xs font-semibold uppercase text-[#D6FF4D]">Runtime</p>
          <p className="mt-2 text-sm text-white/80">Skill 驱动</p>
          <p className="mt-1 text-xs leading-5 text-white/45">图片走 Codex imagegen，视频先交付外部生成素材包。</p>
        </div>
      </div>
    </aside>
  )
}

export default Sidebar
