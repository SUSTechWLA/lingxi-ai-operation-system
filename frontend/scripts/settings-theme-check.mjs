import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const temp = await mkdtemp(join(tmpdir(), 'settings-theme-'))
const bundle = join(temp, 'theme.mjs')
const settingsSource = await readFile(new URL('../src/pages/SettingsPage.tsx', import.meta.url), 'utf8')
const startSource = await readFile(new URL('../src/features/creator-studio/StartCreationPage.tsx', import.meta.url), 'utf8')
const authSource = await readFile(new URL('../src/components/AuthScreen.tsx', import.meta.url), 'utf8')
const directorSource = await readFile(new URL('../src/pages/DirectorStudioPage.tsx', import.meta.url), 'utf8')
const cssSource = await readFile(new URL('../src/index.css', import.meta.url), 'utf8')
const tailwindSource = await readFile(new URL('../tailwind.config.js', import.meta.url), 'utf8')

function readRgbToken(cssBlock, tokenName) {
  const match = cssBlock.match(new RegExp(`--color-${tokenName}:\\s*(\\d+)\\s+(\\d+)\\s+(\\d+)\\s*;`))
  assert.ok(match, `missing --color-${tokenName} token`)
  return match.slice(1).map(Number)
}

function relativeLuminance(rgb) {
  const [red, green, blue] = rgb.map((channel) => {
    const value = channel / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  })
  return (0.2126 * red) + (0.7152 * green) + (0.0722 * blue)
}

function contrastRatio(foreground, background) {
  const lighter = Math.max(relativeLuminance(foreground), relativeLuminance(background))
  const darker = Math.min(relativeLuminance(foreground), relativeLuminance(background))
  return (lighter + 0.05) / (darker + 0.05)
}

try {
  await build({
    entryPoints: [new URL('../src/theme/theme.ts', import.meta.url).pathname],
    outfile: bundle,
    bundle: true,
    format: 'esm',
    platform: 'node',
  })

  const theme = await import(pathToFileURL(bundle))
  assert.equal(theme.normalizeThemeMode('dark'), 'dark')
  assert.equal(theme.normalizeThemeMode('invalid'), 'system')
  assert.equal(theme.resolveTheme('system', true), 'dark')
  assert.equal(theme.resolveTheme('light', true), 'light')

  const values = new Map()
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  }
  theme.persistThemeMode('dark', storage)
  assert.equal(theme.readThemeMode(storage), 'dark')

  const root = { dataset: {}, style: {} }
  let listener
  const media = {
    matches: true,
    addEventListener: (_event, callback) => { listener = callback },
    removeEventListener: (_event, callback) => {
      if (listener === callback) listener = undefined
    },
  }
  const dispose = theme.applyTheme('system', { root, media })
  assert.equal(root.dataset.theme, 'dark')
  assert.equal(root.style.colorScheme, 'dark')
  media.matches = false
  listener()
  assert.equal(root.dataset.theme, 'light')
  dispose()
  assert.equal(listener, undefined)

  for (const label of ['文本生成', '图片生成', '视频生成', '外观']) {
    assert.match(settingsSource, new RegExp(label))
  }
  assert.match(settingsSource, /fetchModelProviderSettings/)
  assert.match(settingsSource, /saveModelProviderSettings/)
  assert.match(settingsSource, /useTheme/)
  assert.match(settingsSource, /跟随系统/)
  assert.match(settingsSource, /仅在本机持久化/)
  assert.match(settingsSource, /随本次已认证生成请求传输给云端编排/)
  assert.doesNotMatch(settingsSource, /不上传云端/)
  assert.match(settingsSource, /clearApiKey:\s*true/)
  assert.match(settingsSource, /清除已保存密钥/)
  assert.match(settingsSource, /aria-controls=\{settingsPanelId\(row\.id\)\}/)
  assert.match(settingsSource, /aria-labelledby=\{settingsTabId\(activeTab\)\}/)
  assert.match(settingsSource, /tabIndex=\{activeTab === row\.id \? 0 : -1\}/)
  assert.match(settingsSource, /onKeyDown=\{\(event\) => handleSettingsTabKeyDown\(event, index\)\}/)
  assert.match(startSource, /IP 口播视频/)
  assert.match(startSource, /智能补充素材/)
  for (const internalTerm of ['每个 Shot 都按三层设计', 'A-roll', 'HyperFrames', 'AIGC 丰富层']) {
    assert.doesNotMatch(startSource, new RegExp(internalTerm))
  }
  assert.match(cssSource, /:root\s*\{[\s\S]*--color-background:/)
  assert.match(cssSource, /\[data-theme=['"]dark['"]\]\s*\{[\s\S]*--color-background:/)
  assert.doesNotMatch(cssSource, /#FFF6D6|#2B1606/)
  assert.match(tailwindSource, /rgb\(var\(--color-background\) \/ <alpha-value>\)/)
  assert.match(tailwindSource, /rgb\(var\(--color-ink\) \/ <alpha-value>\)/)

  const lightThemeMatch = cssSource.match(/:root\s*\{([\s\S]*?)\}/)
  const darkThemeMatch = cssSource.match(/\[data-theme=['"]dark['"]\]\s*\{([\s\S]*?)\}/)
  assert.ok(lightThemeMatch, 'missing light theme token block')
  assert.ok(darkThemeMatch, 'missing dark theme token block')
  const contrastFailures = []
  for (const [themeName, cssBlock] of [['light', lightThemeMatch[1]], ['dark', darkThemeMatch[1]]]) {
    for (const [backgroundToken, foregroundToken] of [['primary', 'on-primary'], ['primary-dark', 'on-primary-dark']]) {
      const ratio = contrastRatio(readRgbToken(cssBlock, foregroundToken), readRgbToken(cssBlock, backgroundToken))
      if (ratio < 4.5) contrastFailures.push(`${themeName} ${foregroundToken} on ${backgroundToken} is ${ratio.toFixed(2)}:1; expected at least 4.5:1`)
    }
  }

  assert.match(tailwindSource, /on-primary[^\n]*--color-on-primary/)
  assert.match(tailwindSource, /on-primary-dark[^\n]*--color-on-primary-dark/)
  const accentControlSource = `${authSource}\n${directorSource}\n${settingsSource}`
  assert.doesNotMatch(accentControlSource, /bg-primary(?:-dark)?[^'"\n]*text-white|text-white[^'"\n]*bg-primary(?:-dark)?/)

  const copyButtonStart = settingsSource.indexOf('function SettingsCopyButton')
  const copyButtonEnd = settingsSource.indexOf('async function copyToClipboard', copyButtonStart)
  assert.notEqual(copyButtonStart, -1, 'missing SettingsCopyButton')
  assert.notEqual(copyButtonEnd, -1, 'missing SettingsCopyButton boundary')
  const copyButtonSource = settingsSource.slice(copyButtonStart, copyButtonEnd)
  if (/\bbg-white\b/.test(copyButtonSource)) contrastFailures.push('SettingsCopyButton hard-codes bg-white')

  assert.deepEqual(contrastFailures, [])
} finally {
  await rm(temp, { recursive: true, force: true })
}

console.log('settings and theme checks passed')
