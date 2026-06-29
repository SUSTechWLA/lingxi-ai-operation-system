import React, { useState, useEffect, useCallback } from 'react'
import { fetchMediaList } from '../services/api'
import { MediaAsset } from '../utils/types'

interface MediaLibraryPanelProps {
  isOpen: boolean
  onClose: () => void
  onSelectMedia: (asset: MediaAsset) => void
}

const MediaLibraryPanel: React.FC<MediaLibraryPanelProps> = ({ isOpen, onClose, onSelectMedia }) => {
  const [assets, setAssets] = useState<MediaAsset[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [tagFilter, setTagFilter] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const limit = 20

  const loadMedia = useCallback(async (loadOffset: number, append: boolean) => {
    setLoading(true)
    setError('')
    try {
      const data = await fetchMediaList(loadOffset, limit, tagFilter || undefined)
      setAssets(prev => append ? [...prev, ...data.items] : data.items)
      setTotal(data.total)
      setOffset(loadOffset + data.items.length)
    } catch {
      setError('加载素材库失败')
    } finally {
      setLoading(false)
    }
  }, [tagFilter, limit])

  useEffect(() => {
    if (isOpen) {
      setAssets([])
      setOffset(0)
      loadMedia(0, false)
    }
  }, [isOpen, tagFilter, loadMedia])

  const handleLoadMore = () => {
    if (!loading && assets.length < total) {
      loadMedia(offset, true)
    }
  }

  const formatFileSize = (bytes: number) => {
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
    return `${(bytes / 1024 / 1024).toFixed(1)}MB`
  }

  const allTags = Array.from(new Set(assets.flatMap(a => a.tags || [])))

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex">
      <div className="absolute inset-0 bg-black/30" onClick={onClose} />
      <div className="relative ml-auto w-[480px] max-w-[90vw] h-full bg-white shadow-2xl flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100">
          <h3 className="text-lg font-semibold text-gray-800">素材库</h3>
          <button onClick={onClose} className="p-1.5 hover:bg-gray-100 rounded-lg">
            <svg className="w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Tag filter */}
        <div className="px-6 py-3 border-b border-gray-50">
          <div className="flex items-center gap-2">
            <input
              type="text"
              placeholder="筛选标签..."
              value={tagFilter}
              onChange={(e) => setTagFilter(e.target.value)}
              className="flex-1 px-3 py-1.5 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-primary"
            />
            {tagFilter && (
              <button
                onClick={() => setTagFilter('')}
                className="text-xs text-gray-400 hover:text-gray-600 px-2"
              >
                清除
              </button>
            )}
          </div>
          {allTags.length > 0 && !tagFilter && (
            <div className="flex flex-wrap gap-1.5 mt-2">
              {allTags.slice(0, 10).map(tag => (
                <button
                  key={tag}
                  onClick={() => setTagFilter(tag)}
                  className="px-2 py-0.5 text-xs bg-gray-100 text-gray-600 rounded-full hover:bg-primary/10 hover:text-primary transition-colors"
                >
                  {tag}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {loading && assets.length === 0 ? (
            <div className="flex items-center justify-center h-40">
              <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
            </div>
          ) : error ? (
            <div className="flex flex-col items-center justify-center h-40 text-gray-400">
              <svg className="w-10 h-10 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <p className="text-sm">{error}</p>
            </div>
          ) : assets.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-40 text-gray-400">
              <svg className="w-12 h-12 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
              <p className="text-sm">暂无素材</p>
              <p className="text-xs mt-1">上传图片或视频后，素材将出现在这里</p>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-3">
              {assets.map(asset => {
                const isImage = asset.mimeType?.startsWith('image/')
                return (
                  <button
                    key={asset.id}
                    onClick={() => onSelectMedia(asset)}
                    className="group relative bg-gray-50 rounded-xl overflow-hidden border border-gray-100 hover:border-primary/40 hover:shadow-md transition-all text-left"
                  >
                    <div className="aspect-video bg-gray-100 flex items-center justify-center overflow-hidden">
                      {isImage && asset.url ? (
                        <img src={asset.url} alt={asset.originalName} className="w-full h-full object-cover" />
                      ) : (
                        <svg className="w-8 h-8 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                      )}
                    </div>
                    <div className="p-2.5">
                      <p className="text-xs font-medium text-gray-700 truncate">{asset.originalName}</p>
                      <div className="flex items-center justify-between mt-1">
                        <span className="text-[10px] text-gray-400">{formatFileSize(asset.size)}</span>
                        <span className="text-[10px] text-gray-400">
                          {new Date(asset.createdAt).toLocaleDateString('zh-CN')}
                        </span>
                      </div>
                      {asset.tags && asset.tags.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-1.5">
                          {asset.tags.slice(0, 3).map(tag => (
                            <span key={tag} className="px-1.5 py-0.5 text-[10px] bg-primary/5 text-primary rounded">
                              {tag}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                    {/* Hover overlay */}
                    <div className="absolute inset-0 bg-primary/0 group-hover:bg-primary/5 transition-colors" />
                  </button>
                )
              })}
            </div>
          )}

          {/* Load more */}
          {assets.length > 0 && assets.length < total && (
            <div className="mt-4 text-center">
              <button
                onClick={handleLoadMore}
                disabled={loading}
                className="px-6 py-2 text-sm text-primary border border-primary/30 rounded-xl hover:bg-primary/5 disabled:opacity-50 transition-colors"
              >
                {loading ? '加载中...' : `加载更多 (${assets.length}/${total})`}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

export default MediaLibraryPanel
