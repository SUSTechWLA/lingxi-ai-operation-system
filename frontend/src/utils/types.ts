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
  taskId: string
  traceUrl: string
}

export interface TraceNode {
  id: string
  taskId: string
  type: string
  name: string
  status: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  errorMessage?: string
  retryCount: number
  createdAt: string
}

export interface TraceData {
  task: {
    taskId: string
    status: string
    input?: Record<string, unknown>
    output?: Record<string, unknown>
    createdAt: string
    nodes: TraceNode[]
  }
  contexts: Array<{
    id: number
    contextType: string
    taskId: string
    nodeId: string
    message: string
    metadata?: Record<string, unknown>
    createdAt: string
  }>
}

