import React from 'react'
import { useAppStore } from '../stores/appStore'

interface KeywordInputProps {
  examples?: string[]
}

const KeywordInput: React.FC<KeywordInputProps> = ({
  examples = ['旅行', '风景', '治愈', 'vlog'],
}) => {
  const keywords = useAppStore((state) => state.keywords)
  const setKeywords = useAppStore((state) => state.setKeywords)
  const maxLength = 200

  const handleExampleClick = (example: string) => {
    const currentKeywords = keywords
      .split(/[,，]/)
      .map((k) => k.trim())
      .filter((k) => k)
    if (!currentKeywords.includes(example)) {
      setKeywords([...currentKeywords, example].join(', '))
    }
  }

  return (
    <div>
      <label className="text-xs font-medium text-gray-500 mb-1.5 block">
        关键词（选填）
      </label>
      <div className="relative">
        <input
          type="text"
          value={keywords}
          onChange={(e) => setKeywords(e.target.value)}
          placeholder="关键词，用逗号分隔"
          maxLength={maxLength}
          className="w-full px-3.5 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-sm"
        />
        <span className="absolute right-3 bottom-2 text-xs text-gray-400">
          {keywords.length}/{maxLength}
        </span>
      </div>
      <p className="mt-1.5 text-[11px] text-gray-400">
        例如：
        {examples.map((ex, i) => (
          <span key={i}>
            <button
              onClick={() => handleExampleClick(ex)}
              className="text-primary hover:text-primary-dark cursor-pointer"
            >
              {ex}
            </button>
            {i < examples.length - 1 && '、'}
          </span>
        ))}
      </p>
    </div>
  )
}

export default KeywordInput
