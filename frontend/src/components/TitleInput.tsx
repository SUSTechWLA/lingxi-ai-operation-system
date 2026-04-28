import React from 'react'
import { useAppStore } from '../stores/appStore'

const TitleInput: React.FC = () => {
  const title = useAppStore((state) => state.title)
  const setTitle = useAppStore((state) => state.setTitle)
  const maxLength = 100

  return (
    <div>
      <label className="text-xs font-medium text-gray-500 mb-1.5 block">标题</label>
      <div className="relative">
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="请输入吸引人的标题..."
          maxLength={maxLength}
          className="w-full px-3.5 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-sm"
        />
        <span className="absolute right-3 bottom-2 text-xs text-gray-400">
          {title.length}/{maxLength}
        </span>
      </div>
    </div>
  )
}

export default TitleInput
