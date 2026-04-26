import React from 'react'
import CommandPanel from './CommandPanel'

const DesktopToolbar: React.FC = () => {
  return (
    <div className="space-y-4">
      <h3 className="text-base font-semibold text-gray-800 flex items-center gap-2">
        <svg className="w-5 h-5 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        桌面工具
      </h3>
      <CommandPanel />
    </div>
  )
}

export default DesktopToolbar
