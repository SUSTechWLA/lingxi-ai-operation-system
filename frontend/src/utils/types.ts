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
  body?: string
  keywords?: string[]
}

export interface AIPolishData {
  content: string
  taskId: string
  traceUrl: string
}

export interface PolishSubmitData {
  taskId: string
  nodeId: string
  message: string
  traceUrl: string
}

export interface PolishQueryData {
  taskId: string
  nodeId: string
  status: string
  content?: string
  error?: string
  traceUrl: string
}

export interface MediaAsset {
  id: string
  userId: string
  originalName: string
  mimeType: string
  size: number
  minioPath: string
  tags: string[]
  embeddingId?: string
  createdAt: string
  updatedAt: string
  url?: string
}

export interface MediaListResponse {
  items: MediaAsset[]
  total: number
  offset: number
  limit: number
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

// Chat types for conversational AI generation
export interface Suggestion {
  text: string
  type: string
}

export interface ChatGeneratedFields {
  title?: string
  description?: string
  body?: string
  keywords?: string[]
}

export interface ChatGenerateResponse {
  session_id: string
  reply: string
  suggestions?: Suggestion[]
  fields?: ChatGeneratedFields
}

export interface ChatReviseResponse {
  reply: string
  fields: ChatGeneratedFields
  task_id: string
}

export interface ChatMessageItem {
  role: 'user' | 'assistant'
  content: string
  suggestions?: Suggestion[]
  fields?: ChatGeneratedFields
}

export type ContentType = 'image' | 'video' | null

