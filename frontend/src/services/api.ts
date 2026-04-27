import axios from 'axios'
import { ApiResponse, TaskResponse, AIGenerateData, AIPolishData, TraceData } from '../utils/types'
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
