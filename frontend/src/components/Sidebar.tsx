import React, { useState } from 'react'
import { isElectron } from '../utils/electron'

const Sidebar: React.FC = () => {
  const [activeNav, setActiveNav] = useState('publish')
  const showDesktopNav = isElectron()
  return (
    <aside className="w-64 bg-white border-r border-gray-100 flex flex-col h-screen">
      <div className="p-6">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 bg-gradient-to-br from-primary to-primary-light rounded-lg flex items-center justify-center">
            <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <div>
            <h1 className="text-base font-semibold text-gray-800">灵犀AI自媒体运营助手</h1>
            <p className="text-xs text-gray-500">让创作更简单，让传播更高效</p>
          </div>
        </div>
      </div>

      <nav className="flex-1 px-4 py-2">
        <div className="space-y-1">
          <button
            onClick={() => setActiveNav('publish')}
            className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl font-medium text-sm transition-colors ${
              activeNav === 'publish'
                ? 'bg-primary/10 text-primary'
                : 'text-gray-600 hover:bg-gray-50'
            }`}
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
            创作发布
          </button>
          {showDesktopNav && (
            <button
              onClick={() => setActiveNav('desktop')}
              className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl font-medium text-sm transition-colors ${
                activeNav === 'desktop'
                  ? 'bg-purple-50 text-purple-600'
                  : 'text-gray-600 hover:bg-gray-50'
              }`}
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
              桌面工具
            </button>
          )}
        </div>
      </nav>

      <div className="p-4 border-t border-gray-100">
        <div className="bg-gradient-to-r from-amber-50 to-orange-50 rounded-xl p-4">
          <div className="flex items-center gap-2 mb-2">
            <svg className="w-4 h-4 text-amber-500" fill="currentColor" viewBox="0 0 20 20">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            <span className="text-sm font-medium text-amber-700">超级会员</span>
            <span className="ml-auto text-xs bg-amber-100 text-amber-600 px-2 py-0.5 rounded-full">尊享权益</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs text-gray-500">有效期至 2025-06-01</span>
            <button className="text-xs text-primary hover:text-primary-dark">去续费</button>
          </div>
        </div>
      </div>
    </aside>
  )
}

export default Sidebar
