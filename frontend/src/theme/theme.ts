export type ThemeMode = 'system' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

export const THEME_STORAGE_KEY = 'tangying.creator.theme.v1'

export function normalizeThemeMode(value: unknown): ThemeMode {
  return value === 'light' || value === 'dark' || value === 'system' ? value : 'system'
}

export function resolveTheme(mode: ThemeMode, prefersDark: boolean): ResolvedTheme {
  return mode === 'system' ? (prefersDark ? 'dark' : 'light') : mode
}

export function readThemeMode(storage: Pick<Storage, 'getItem'> = window.localStorage): ThemeMode {
  return normalizeThemeMode(storage.getItem(THEME_STORAGE_KEY))
}

export function persistThemeMode(
  mode: ThemeMode,
  storage: Pick<Storage, 'setItem'> = window.localStorage,
): void {
  storage.setItem(THEME_STORAGE_KEY, mode)
}

function syncRootTheme(
  mode: ThemeMode,
  root: HTMLElement,
  media: Pick<MediaQueryList, 'matches'>,
): void {
  const resolved = resolveTheme(mode, media.matches)
  root.dataset.theme = resolved
  root.style.colorScheme = resolved
}

export function initializeTheme(options: {
  storage?: Pick<Storage, 'getItem'>
  root?: HTMLElement
  media?: Pick<MediaQueryList, 'matches'>
} = {}): ThemeMode {
  const mode = readThemeMode(options.storage)
  const root = options.root ?? document.documentElement
  const media = options.media ?? window.matchMedia('(prefers-color-scheme: dark)')
  syncRootTheme(mode, root, media)
  return mode
}

export function applyTheme(mode: ThemeMode, options: {
  root?: HTMLElement
  media?: MediaQueryList
} = {}): () => void {
  const root = options.root ?? document.documentElement
  const media = options.media ?? window.matchMedia('(prefers-color-scheme: dark)')
  const sync = () => syncRootTheme(mode, root, media)
  sync()
  if (mode !== 'system') return () => undefined
  media.addEventListener('change', sync)
  return () => media.removeEventListener('change', sync)
}
