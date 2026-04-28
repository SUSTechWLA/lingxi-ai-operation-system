import React from 'react'
import { ContentType } from '../utils/types'

interface ContentTypeSelectorProps {
  value: ContentType
  onChange: (type: ContentType) => void
}

const ContentTypeSelector: React.FC<ContentTypeSelectorProps> = ({ value, onChange }) => {
  return (
    <div>
      <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-3">选择发布类型</h3>
      <div className="grid grid-cols-2 gap-3">
        {/* 图文笔记 */}
        <div
          role="button"
          tabIndex={0}
          onClick={() => onChange(value === 'image' ? null : 'image')}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onChange(value === 'image' ? null : 'image') }}
          className={`relative p-4 rounded-xl border-2 cursor-pointer transition-all ${
            value === 'image'
              ? 'border-primary bg-primary/5 shadow-md shadow-primary/10'
              : 'border-gray-200 bg-white hover:border-primary/30 hover:bg-gray-50/50'
          }`}
        >
          <div className="flex flex-col items-center text-center gap-2">
            <div className={`w-10 h-10 rounded-xl flex items-center justify-center transition-colors ${
              value === 'image' ? 'bg-primary/20' : 'bg-gray-100'
            }`}>
              <svg className={`w-5 h-5 transition-colors ${value === 'image' ? 'text-primary' : 'text-gray-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
            </div>
            <div>
              <p className={`text-sm font-semibold transition-colors ${value === 'image' ? 'text-primary' : 'text-gray-700'}`}>
                图文笔记
              </p>
              <p className="text-xs text-gray-500 mt-0.5">发布多张图片 + 文字描述</p>
            </div>
          </div>
          {value === 'image' && (
            <div className="absolute top-2 right-2 w-5 h-5 bg-primary rounded-full flex items-center justify-center">
              <svg className="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
              </svg>
            </div>
          )}
        </div>

        {/* 短视频 */}
        <div
          role="button"
          tabIndex={0}
          onClick={() => onChange(value === 'video' ? null : 'video')}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onChange(value === 'video' ? null : 'video') }}
          className={`relative p-4 rounded-xl border-2 cursor-pointer transition-all ${
            value === 'video'
              ? 'border-primary bg-primary/5 shadow-md shadow-primary/10'
              : 'border-gray-200 bg-white hover:border-primary/30 hover:bg-gray-50/50'
          }`}
        >
          <div className="flex flex-col items-center text-center gap-2">
            <div className={`w-10 h-10 rounded-xl flex items-center justify-center transition-colors ${
              value === 'video' ? 'bg-primary/20' : 'bg-gray-100'
            }`}>
              <svg className={`w-5 h-5 transition-colors ${value === 'video' ? 'text-primary' : 'text-gray-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
            </div>
            <div>
              <p className={`text-sm font-semibold transition-colors ${value === 'video' ? 'text-primary' : 'text-gray-700'}`}>
                短视频
              </p>
              <p className="text-xs text-gray-500 mt-0.5">发布一段视频 + 文字描述</p>
            </div>
          </div>
          {value === 'video' && (
            <div className="absolute top-2 right-2 w-5 h-5 bg-primary rounded-full flex items-center justify-center">
              <svg className="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
              </svg>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default ContentTypeSelector
