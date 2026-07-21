import { useCallback, useEffect, useMemo, useState, type PropsWithChildren } from 'react'
import { applyTheme, persistThemeMode, readThemeMode, type ThemeMode } from './theme'
import { ThemeContext } from './ThemeContext'

export function ThemeProvider({ children }: PropsWithChildren) {
  const [mode, setModeState] = useState<ThemeMode>(() => readThemeMode())

  useEffect(() => applyTheme(mode), [mode])

  const setMode = useCallback((nextMode: ThemeMode) => {
    persistThemeMode(nextMode)
    setModeState(nextMode)
  }, [])

  const value = useMemo(() => ({ mode, setMode }), [mode, setMode])

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
