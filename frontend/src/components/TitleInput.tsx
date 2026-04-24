import React from 'react'
import { useAppStore } from '../stores/appStore'

interface TitleInputProps {
  onPolish?: () => void
}

const TitleInput: React.FC<TitleInputProps> = ({ onPolish }) => {
  const title = useAppStore((state) => state.title)
  const setTitle = useAppStore((state) => state.setTitle)
  const maxLength = 100

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <label className="text-sm font-medium text-gray-700">标题</label>
        <button
          onClick={onPolish}
          className="flex items-center gap-1 px-3 py-1.5 bg-primary/10 text-primary rounded-lg text-sm hover:bg-primary/20 transition-colors"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" />
          </svg>
          AI润色
        </button>
      </div>
      <div className="relative">
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="请输入吸引人的标题..."
          maxLength={maxLength}
          className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-sm"
        />
        <span className="absolute right-3 bottom-3 text-xs text-gray-400">
          {title.length}/{maxLength}
        </span>
      </div>
    </div>
  )
}

export default TitleInput
