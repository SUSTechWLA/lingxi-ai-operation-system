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
}

export interface TaskResponse {
  taskId: string
  message: string
}
