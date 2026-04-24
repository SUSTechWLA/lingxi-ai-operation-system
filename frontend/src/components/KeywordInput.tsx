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
      <label className="text-sm font-medium text-gray-700 mb-2 block">
        关键词（选填）
      </label>
      <div className="relative">
        <input
          type="text"
          value={keywords}
          onChange={(e) => setKeywords(e.target.value)}
          placeholder="请输入关键词，用于提高内容曝光和精准推荐，多个关键词请用逗号分隔"
          maxLength={maxLength}
          className="w-full px-4 py-3 border border-gray-200 rounded-xl focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-sm"
        />
        <span className="absolute right-3 bottom-3 text-xs text-gray-400">
          {keywords.length}/{maxLength}
        </span>
      </div>
      <p className="mt-2 text-xs text-gray-400">
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
