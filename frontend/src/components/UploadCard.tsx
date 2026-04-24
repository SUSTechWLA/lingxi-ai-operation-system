import React, { useCallback, useState } from 'react'
import { useDropzone } from 'react-dropzone'
import { useAppStore } from '../stores/appStore'
import { MediaFile } from '../utils/types'

interface UploadCardProps {
  type: 'video' | 'image'
}

const UploadCard: React.FC<UploadCardProps> = ({ type }) => {
  const [isDragOver, setIsDragOver] = useState(false)
  const addVideos = useAppStore((state) => state.addVideos)
  const addImages = useAppStore((state) => state.addImages)
  const videos = useAppStore((state) => state.videos)
  const images = useAppStore((state) => state.images)

  const isVideo = type === 'video'
  const files = isVideo ? videos : images
  const maxFiles = isVideo ? 5 : 9
  const accept = isVideo
    ? { 'video/*': ['.mp4', '.mov', '.avi'] }
    : { 'image/*': ['.jpg', '.jpeg', '.png', '.webp'] }

  const createMediaFile = (file: File): MediaFile => ({
    file,
    preview: isVideo ? undefined : URL.createObjectURL(file),
    name: file.name,
    size: file.size,
  })

  const onDrop = useCallback(
    (acceptedFiles: File[]) => {
      const mediaFiles = acceptedFiles.map(createMediaFile)
      if (isVideo) {
        addVideos(mediaFiles)
      } else {
        addImages(mediaFiles)
      }
    },
    [isVideo, addVideos, addImages]
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
                <div key={index} className="flex items-center gap-2 bg-white/80 rounded-lg px-3 py-2">
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
