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
const cssSource = await readFile(new URL('../src/index.css', import.meta.url), 'utf8')
const tailwindSource = await readFile(new URL('../tailwind.config.js', import.meta.url), 'utf8')

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
  assert.match(settingsSource, /仅保存在本机/)
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
} finally {
  await rm(temp, { recursive: true, force: true })
}

console.log('settings and theme checks passed')
