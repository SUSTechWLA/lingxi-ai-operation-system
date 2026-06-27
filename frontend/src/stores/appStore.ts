import { create } from 'zustand'
import { MediaFile, Platform, ContentType } from '../utils/types'

interface AppState {
  title: string
  description: string
  keywords: string
  body: string
  videos: MediaFile[]
  images: MediaFile[]
  cover: MediaFile | null
  platforms: Platform[]
  isPublishing: boolean
  contentType: ContentType
  aiLoadingMessage: string | null

  setTitle: (title: string) => void
  setDescription: (description: string) => void
  setKeywords: (keywords: string) => void
  setBody: (body: string) => void
  addVideos: (files: MediaFile[]) => void
  removeVideo: (index: number) => void
  addImages: (files: MediaFile[]) => void
  removeImage: (index: number) => void
  setCover: (file: MediaFile | null) => void
  togglePlatform: (id: string) => void
  toggleAllPlatforms: (enabled: boolean) => void
  clearAll: () => void
  setIsPublishing: (isPublishing: boolean) => void
  setContentType: (type: ContentType) => void
  clearMedia: () => void
  setAILoadingMessage: (msg: string | null) => void
  applyFields: (fields: Partial<Pick<AppState, 'title' | 'description' | 'keywords' | 'body'>>) => void
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
  body: '',
  videos: [],
  images: [],
  cover: null,
  platforms: defaultPlatforms,
  isPublishing: false,
  contentType: null,
  aiLoadingMessage: null,

  setTitle: (title) => set({ title }),
  setDescription: (description) => set({ description }),
  setKeywords: (keywords) => set({ keywords }),
  setBody: (body) => set({ body }),

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
  setCover: (cover) => set({ cover }),

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
      body: '',
      videos: [],
      images: [],
      cover: null,
    }),

  setIsPublishing: (isPublishing) => set({ isPublishing }),
  setContentType: (contentType) => set({ contentType }),
  clearMedia: () => set({ videos: [], images: [], cover: null }),
  setAILoadingMessage: (aiLoadingMessage) => set({ aiLoadingMessage }),

  applyFields: (fields) => set((state) => ({
    title: fields.title ?? state.title,
    description: fields.description ?? state.description,
    keywords: fields.keywords ?? state.keywords,
    body: fields.body ?? state.body,
  })),

  getSelectedPlatforms: () => {
    const { platforms } = get()
    return platforms.filter((p) => p.enabled).map((p) => p.id)
  },
}))
