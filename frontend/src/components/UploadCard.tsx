import React, { useCallback, useState } from 'react'
import { useDropzone } from 'react-dropzone'
import { useAppStore } from '../stores/appStore'
import { MediaFile } from '../utils/types'
import { isElectron, getElectronAPI } from '../utils/electron'

interface UploadCardProps {
  type: 'video' | 'image'
}

const UploadCard: React.FC<UploadCardProps> = ({ type }) => {
  const [isDragOver, setIsDragOver] = useState(false)
  const addVideos = useAppStore((state) => state.addVideos)
  const addImages = useAppStore((state) => state.addImages)
  const removeVideo = useAppStore((state) => state.removeVideo)
  const removeImage = useAppStore((state) => state.removeImage)
  const videos = useAppStore((state) => state.videos)
  const images = useAppStore((state) => state.images)

  const isVideo = type === 'video'
  const files = isVideo ? videos : images
  const maxFiles = isVideo ? 5 : 9
  const accept: Record<string, readonly string[]> = isVideo
    ? { 'video/*': ['.mp4', '.mov', '.avi'] }
    : { 'image/*': ['.jpg', '.jpeg', '.png', '.webp'] }

  const createMediaFile = useCallback((file: File): MediaFile => ({
    file,
    preview: isVideo ? undefined : URL.createObjectURL(file),
    name: file.name,
    size: file.size,
  }), [isVideo])

  const onDrop = useCallback(
    (acceptedFiles: File[]) => {
      const mediaFiles = acceptedFiles.map(createMediaFile)
      if (isVideo) {
        addVideos(mediaFiles)
      } else {
        addImages(mediaFiles)
      }
    },
    [isVideo, addVideos, addImages, createMediaFile]
  )

  const { getRootProps, getInputProps } = useDropzone({
    onDrop,
    accept,
    maxFiles: maxFiles - files.length,
    onDragEnter: () => setIsDragOver(true),
    onDragLeave: () => setIsDragOver(false),
    onDropAccepted: () => setIsDragOver(false),
    disabled: files.length >= maxFiles,
  })

  const handleElectronFilePick = async () => {
    const api = getElectronAPI()
    if (!api) return
    try {
      const extensions = isVideo ? ['mp4', 'mov', 'avi'] : ['jpg', 'jpeg', 'png', 'webp']
      const paths = await api.openFileDialog({
        properties: ['openFile', 'multiSelections'],
        filters: [{ name: isVideo ? '视频文件' : '图片文件', extensions }],
      })
      if (!paths || paths.length === 0) return
      const mediaFiles: MediaFile[] = await Promise.all(paths.map(async (p) => {
        try {
          const info = await api.readFile(p)
          // Convert base64 to a Blob, then to a File with proper MIME type
          const byteChars = atob(info.data)
          const byteNums = new Uint8Array(byteChars.length)
          for (let i = 0; i < byteChars.length; i++) {
            byteNums[i] = byteChars.charCodeAt(i)
          }
          const blob = new Blob([byteNums], { type: info.mimeType })
          const file = new File([blob], info.name, { type: info.mimeType })
          return {
            file,
            preview: isVideo ? undefined : URL.createObjectURL(blob),
            name: info.name,
            size: info.size,
          }
        } catch {
          // Fallback: use empty file if read fails (unlikely)
          const name = p.split('/').pop() || p
          return { file: new File([], name), preview: undefined, name, size: 0 }
        }
      }))
      if (isVideo) {
        addVideos(mediaFiles)
      } else {
        addImages(mediaFiles)
      }
    } catch {
      // user cancelled or error
    }
  }

  const showElectronPick = isElectron() && files.length < maxFiles

  return (
    <div
      {...getRootProps()}
      className={`relative cursor-pointer rounded-2xl border-2 border-dashed p-8 text-center transition-all duration-200 ${
        isDragOver
          ? isVideo
            ? 'border-primary bg-primary/5 shadow-lg shadow-primary/10'
            : 'border-green-400 bg-green-50 shadow-lg shadow-green-400/10'
          : isVideo
          ? 'border-primary/30 bg-primary/5 hover:border-primary hover:bg-primary/10'
          : 'border-green-400/30 bg-green-50 hover:border-green-400 hover:bg-green-100'
      }`}
    >
      <input {...getInputProps()} />

      <div className="flex flex-col items-center gap-4">
        {showElectronPick && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              handleElectronFilePick()
            }}
            className={`px-4 py-1.5 rounded-lg text-sm font-medium transition-colors ${
              isVideo
                ? 'bg-primary/20 text-primary hover:bg-primary/30'
                : 'bg-green-100 text-green-700 hover:bg-green-200'
            }`}
          >
            从本地选择文件
          </button>
        )}
        <div
          className={`w-16 h-16 rounded-full flex items-center justify-center ${
            isVideo ? 'bg-primary/20' : 'bg-green-100'
          }`}
        >
          {isVideo ? (
            <svg className="w-8 h-8 text-primary" fill="currentColor" viewBox="0 0 20 20">
              <path d="M2 6a2 2 0 012-2h6a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6zm12.553 1.106A1 1 0 0014 8v4a1 1 0 00.553.894l2 1A1 1 0 0018 13V7a1 1 0 00-1.447-.894l-2 1z" />
            </svg>
          ) : (
            <svg className="w-8 h-8 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
          )}
        </div>

        <div>
          <p className={`text-lg font-semibold ${isVideo ? 'text-primary' : 'text-green-600'}`}>
            上传{isVideo ? '视频' : '图片'}
          </p>
          <p className="text-sm text-gray-500 mt-1">
            {isVideo ? '支持 MP4、MOV、AVI 等格式' : '支持 JPG、PNG、WEBP 等格式'}
          </p>
          <p className="text-sm text-gray-500">
            {isVideo ? '最大 2GB，时长不超过 30 分钟' : `最多可上传 ${maxFiles} 张`}
          </p>
        </div>

        {files.length > 0 && (
          <div className="w-full mt-4">
            <p className="text-sm text-gray-600 mb-2">
              已选择 {files.length}/{maxFiles} 个文件
            </p>
            <div className="space-y-2">
              {files.map((file, index) => (
                <div key={index} className="flex items-center gap-2 bg-white/80 rounded-lg px-3 py-2 group">
                  {isVideo ? (
                    <svg className="w-4 h-4 text-primary flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M2 6a2 2 0 012-2h6a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
                    </svg>
                  ) : (
                    <img src={file.preview} alt="" className="w-8 h-8 rounded object-cover" />
                  )}
                  <span className="text-sm text-gray-600 truncate flex-1">{file.name}</span>
                  <span className="text-xs text-gray-400 flex-shrink-0">
                    {(file.size / 1024 / 1024).toFixed(1)}MB
                  </span>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation()
                      if (isVideo) removeVideo(index)
                      else removeImage(index)
                    }}
                    className="flex-shrink-0 w-5 h-5 rounded-full bg-gray-200 hover:bg-red-400 text-gray-400 hover:text-white flex items-center justify-center transition-colors opacity-0 group-hover:opacity-100"
                    title={`删除${isVideo ? '视频' : '图片'}`}
                  >
                    <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default UploadCard
