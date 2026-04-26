import { create } from 'zustand'
import { MediaFile, Platform } from '../utils/types'

interface AppState {
  title: string
  description: string
  keywords: string
  videos: MediaFile[]
  images: MediaFile[]
  platforms: Platform[]
  isPublishing: boolean

  setTitle: (title: string) => void
  setDescription: (description: string) => void
  setKeywords: (keywords: string) => void
  addVideos: (files: MediaFile[]) => void
  removeVideo: (index: number) => void
  addImages: (files: MediaFile[]) => void
  removeImage: (index: number) => void
  togglePlatform: (id: string) => void
  toggleAllPlatforms: (enabled: boolean) => void
  clearAll: () => void
  setIsPublishing: (isPublishing: boolean) => void
  getSelectedPlatforms: () => string[]
}

const defaultPlatforms: Platform[] = [
  { id: 'douyin', name: '抖音', icon: 'douyin', enabled: true, status: 'available' },
  { id: 'kuaishou', name: '快手', icon: 'kuaishou', enabled: false, status: 'developing' },
  { id: 'shipinhao', name: '视频号', icon: 'shipinhao', enabled: false, status: 'developing' },
  { id: 'xiaohongshu', name: '小红书', icon: 'xiaohongshu', enabled: true, status: 'available' },
  { id: 'bilibili', name: 'B站', icon: 'bilibili', enabled: false, status: 'developing' },
  { id: 'weibo', name: '微博', icon: 'weibo', enabled: false, status: 'developing' },
  { id: 'toutiao', name: '今日头条', icon: 'toutiao', enabled: false, status: 'developing' },
  { id: 'baijiahao', name: '百家号', icon: 'baijiahao', enabled: false, status: 'developing' },
  { id: 'zhihu', name: '知乎', icon: 'zhihu', enabled: false, status: 'developing' },
  { id: 'gongzhonghao', name: '公众号', icon: 'gongzhonghao', enabled: false, status: 'developing' },
]

export const useAppStore = create<AppState>((set, get) => ({
  title: '',
  description: '',
  keywords: '',
  videos: [],
  images: [],
  platforms: defaultPlatforms,
  isPublishing: false,

  setTitle: (title) => set({ title }),
  setDescription: (description) => set({ description }),
  setKeywords: (keywords) => set({ keywords }),

  addVideos: (files) =>
    set((state) => ({
      videos: [...state.videos, ...files].slice(0, 5),
    })),
  removeVideo: (index) =>
    set((state) => ({
      videos: state.videos.filter((_, i) => i !== index),
    })),

  addImages: (files) =>
    set((state) => ({
      images: [...state.images, ...files].slice(0, 9),
    })),
  removeImage: (index) =>
    set((state) => ({
      images: state.images.filter((_, i) => i !== index),
    })),

  togglePlatform: (id) =>
    set((state) => ({
      platforms: state.platforms.map((p) =>
        p.id === id ? { ...p, enabled: !p.enabled } : p
      ),
    })),

  toggleAllPlatforms: (enabled) =>
    set((state) => ({
      platforms: state.platforms.map((p) => ({
        ...p,
        enabled: p.status === 'available' ? enabled : false,
      })),
    })),

  clearAll: () =>
    set({
      title: '',
      description: '',
      keywords: '',
      videos: [],
      images: [],
    }),

  setIsPublishing: (isPublishing) => set({ isPublishing }),

  getSelectedPlatforms: () => {
    const { platforms } = get()
    return platforms.filter((p) => p.enabled).map((p) => p.id)
  },
}))
