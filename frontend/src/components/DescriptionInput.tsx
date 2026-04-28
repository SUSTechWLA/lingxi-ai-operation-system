import React from 'react'
import { useAppStore } from '../stores/appStore'

const DescriptionInput: React.FC = () => {
  const description = useAppStore((state) => state.description)
  const setDescription = useAppStore((state) => state.setDescription)
  const maxLength = 1000

  return (
    <div>
      <label className="text-xs font-medium text-gray-500 mb-1.5 block">简介</label>
      <div className="relative">
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="请输入内容简介，介绍你的作品亮点、价值或核心内容..."
          maxLength={maxLength}
          rows={4}
          className="w-full px-3.5 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-sm resize-none"
        />
        <span className="absolute right-3 bottom-2 text-xs text-gray-400">
          {description.length}/{maxLength}
        </span>
      </div>
    </div>
  )
}

export default DescriptionInput
