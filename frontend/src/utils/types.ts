export interface MediaFile {
  file: File
  preview?: string
  name: string
  size: number
}

export interface PublishData {
  title: string
  description: string
  keywords: string
  videos: MediaFile[]
  images: MediaFile[]
  platforms: string[]
}

export interface Platform {
  id: string
  name: string
  icon: string
  enabled: boolean
  status: 'available' | 'developing'
}

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

export interface TaskResponse {
  taskId: string
  message: string
}

export interface AIGenerateData {
  title: string
  description: string
}

export interface AIPolishData {
  content: string
}
