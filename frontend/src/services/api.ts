import axios from 'axios'
import { ApiResponse, TaskResponse, AIGenerateData, AIPolishData, TraceData, MediaListResponse, MediaAsset, ChatGenerateResponse, ChatReviseResponse } from '../utils/types'
import { isElectron } from '../utils/electron'

const API_BASE = isElectron() ? 'http://localhost:8080/api' : '/api'

const api = axios.create({
  baseURL: API_BASE,
  timeout: 30000,
})

export const publishContent = async (
  title: string,
  description: string,
  keywords: string,
  platforms: string[],
  videoFiles: File[],
  imageFiles: File[]
): Promise<TaskResponse> => {
  const formData = new FormData()
  formData.append('title', title)
  formData.append('description', description)
  formData.append('keywords', keywords)
  formData.append('platforms', JSON.stringify(platforms))

  videoFiles.forEach((file) => {
    formData.append('videos', file)
  })

  imageFiles.forEach((file) => {
    formData.append('images', file)
  })

  const response = await api.post<ApiResponse<TaskResponse>>('/publish', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  })

  return response.data.data
}

export const aiGenerateContent = async (
  prompt: string,
  signal?: AbortSignal
): Promise<AIGenerateData> => {
  const response = await api.post<ApiResponse<AIGenerateData>>('/ai/generate', { prompt }, { signal })
  return response.data.data
}

export const aiGenerateFromMedia = async (
  prompt: string,
  images: File[],
  videos: File[],
  signal?: AbortSignal
): Promise<AIGenerateData> => {
  const formData = new FormData()
  formData.append('prompt', prompt)
  images.forEach((file) => formData.append('images', file))
  videos.forEach((file) => formData.append('videos', file))

  const response = await api.post<ApiResponse<AIGenerateData>>('/ai/generate-from-media', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    signal,
  })
  return response.data.data
}

export const aiPolishText = async (
  text: string,
  type: 'title' | 'description',
  signal?: AbortSignal
): Promise<AIPolishData> => {
  const response = await api.post<ApiResponse<AIPolishData>>('/ai/polish', { text, type }, { signal })
  return response.data.data
}

export const fetchTrace = async (taskId: string): Promise<TraceData> => {
  const response = await api.get<ApiResponse<TraceData>>(`/trace/${taskId}`)
  return response.data.data
}

export const fetchRecentTrace = async (): Promise<TraceData> => {
  const response = await api.get<ApiResponse<TraceData>>('/trace/recent')
  return response.data.data
}

// Chat API - Conversational AI generation
export const chatGenerate = async (
  message: string,
  currentContext?: {
    title?: string
    description?: string
    body?: string
    keywords?: string[]
    media_count?: number
  },
  sessionId?: string,
  signal?: AbortSignal
): Promise<ChatGenerateResponse> => {
  const response = await api.post<ApiResponse<ChatGenerateResponse>>('/chat/generate', {
    session_id: sessionId,
    message,
    current_context: currentContext || {},
  }, { signal })
  return response.data.data
}

export const chatRevise = async (
  message: string,
  currentFields: {
    title: string
    description: string
    keywords: string[]
  },
  signal?: AbortSignal
): Promise<ChatReviseResponse> => {
  const response = await api.post<ApiResponse<ChatReviseResponse>>('/chat/revise', {
    message,
    current_fields: currentFields,
  }, { signal })
  return response.data.data
}

// Media management API
export const fetchMediaList = async (
  offset = 0,
  limit = 20,
  tag?: string
): Promise<MediaListResponse> => {
  const params: Record<string, string | number> = { offset, limit }
  if (tag) params.tag = tag
  const response = await api.get<ApiResponse<MediaListResponse>>('/media/list', { params })
  return response.data.data
}

export const fetchMediaDetail = async (id: string): Promise<MediaAsset> => {
  const response = await api.get<ApiResponse<MediaAsset>>(`/media/${id}`)
  return response.data.data
}

export const updateMediaTags = async (id: string, tags: string[]): Promise<MediaAsset> => {
  const response = await api.put<ApiResponse<MediaAsset>>(`/media/${id}/tags`, { tags })
  return response.data.data
}

export const batchProcessMedia = async (
  mediaIds: string[],
  action: 'analyze' | 'generate'
): Promise<{ taskIds: string[] }> => {
  const response = await api.post<ApiResponse<{ taskIds: string[] }>>('/media/batch-process', {
    media_ids: mediaIds,
    action,
  })
  return response.data.data
}
