import axios from 'axios'
import { TaskResponse } from '../utils/types'

const api = axios.create({
  baseURL: '/api',
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

  const response = await api.post<TaskResponse>('/publish', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  })

  return response.data
}

export const aiGenerateContent = async (
  prompt: string
): Promise<{ title: string; description: string }> => {
  const response = await api.post('/ai/generate', { prompt })
  return response.data
}

export const aiPolishText = async (
  text: string,
  type: 'title' | 'description'
): Promise<string> => {
  const response = await api.post('/ai/polish', { text, type })
  return response.data.content
}
