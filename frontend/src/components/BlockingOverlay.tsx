import React from 'react'
import { cancelAI } from '../utils/ai-loading'

interface BlockingOverlayProps {
  message: string
}

const BlockingOverlay: React.FC<BlockingOverlayProps> = ({ message }) => {
  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/30 backdrop-blur-sm">
      <div className="bg-white rounded-2xl shadow-2xl p-8 flex flex-col items-center gap-5 min-w-[320px]">
        {/* Spinner */}
        <div className="relative w-16 h-16">
          <div className="absolute inset-0 rounded-full border-4 border-gray-100" />
          <div className="absolute inset-0 rounded-full border-4 border-primary border-t-transparent animate-spin" />
        </div>

        {/* Message */}
        <p className="text-base font-medium text-gray-700 text-center">{message}</p>

        {/* Cancel button */}
        <button
          onClick={() => cancelAI()}
          className="px-8 py-2.5 text-sm font-medium text-gray-600 border border-gray-200 rounded-xl hover:bg-gray-50 hover:text-gray-800 transition-colors"
        >
          取消
        </button>
      </div>
    </div>
  )
}

export default BlockingOverlay
