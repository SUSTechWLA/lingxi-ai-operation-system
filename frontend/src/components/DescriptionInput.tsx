import React from 'react'
import { useAppStore } from '../stores/appStore'

interface DescriptionInputProps {
  onPolish?: (type: 'title' | 'description') => void
}

const DescriptionInput: React.FC<DescriptionInputProps> = ({ onPolish }) => {
  const description = useAppStore((state) => state.description)
  const setDescription = useAppStore((state) => state.setDescription)
  const maxLength = 1000

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <label className="text-sm font-medium text-gray-700">简介</label>
        <button
          onClick={() => onPolish?.('description')}
          className="flex items-center gap-1 px-3 py-1.5 bg-primary/10 text-primary rounded-lg text-sm hover:bg-primary/20 transition-colors"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" />
          </svg>
          AI润色
        </button>
      </div>
      <div className="relative">
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="请输入内容简介，介绍你的作品亮点、价值或核心内容..."
          maxLength={maxLength}
          rows={6}
          className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-sm resize-none"
        />
        <span className="absolute right-3 bottom-3 text-xs text-gray-400">
          {description.length}/{maxLength}
        </span>
      </div>
    </div>
  )
}

export default DescriptionInput
