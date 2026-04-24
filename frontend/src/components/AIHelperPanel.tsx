import React from 'react'

interface AIHelperPanelProps {
  onGenerate?: () => void
  onPolish?: () => void
}

const AIHelperPanel: React.FC<AIHelperPanelProps> = ({ onGenerate, onPolish }) => {
  return (
    <div className="space-y-4">
      <h3 className="text-base font-semibold text-gray-800">AI创作助手</h3>

      <div className="bg-gradient-to-br from-primary/5 to-purple-50 rounded-2xl p-5 border border-primary/10">
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 bg-primary/20 rounded-xl flex items-center justify-center flex-shrink-0">
            <svg className="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <div className="flex-1">
            <h4 className="text-sm font-medium text-gray-800 mb-1">AI生成标题和简介</h4>
            <p className="text-xs text-gray-500 mb-3">
              根据视频或图片内容，AI自动生成吸引人的标题和简介
            </p>
            <button
              onClick={onGenerate}
              className="px-4 py-2 bg-primary text-white rounded-lg text-sm hover:bg-primary-dark transition-colors"
            >
              一键生成
            </button>
          </div>
        </div>
      </div>

      <div className="bg-gradient-to-br from-amber-50 to-orange-50 rounded-2xl p-5 border border-amber-100">
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 bg-amber-100 rounded-xl flex items-center justify-center flex-shrink-0">
            <svg className="w-5 h-5 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
            </svg>
          </div>
          <div className="flex-1">
            <h4 className="text-sm font-medium text-gray-800 mb-1">AI润色优化</h4>
            <p className="text-xs text-gray-500 mb-3">
              优化现有标题和简介，提升内容质量和吸引力
            </p>
            <div className="flex justify-end">
              <button
                onClick={onPolish}
                className="px-4 py-2 bg-amber-500 text-white rounded-lg text-sm hover:bg-amber-600 transition-colors"
              >
                一键优化
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

export default AIHelperPanel
