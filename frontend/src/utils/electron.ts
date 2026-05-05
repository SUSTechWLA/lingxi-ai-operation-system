import type { ElectronAPI } from './electron.d'

export function isElectron(): boolean {
  return !!(window as any).electronAPI?.isElectron
}

export function getElectronAPI(): ElectronAPI | null {
  if (isElectron()) {
    return (window as any).electronAPI as ElectronAPI
  }
  return null
}
