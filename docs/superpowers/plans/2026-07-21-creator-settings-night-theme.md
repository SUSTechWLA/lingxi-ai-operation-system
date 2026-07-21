# Creator Settings and Night Theme Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore user-accessible text, image, and video generation settings from the avatar menu, add a persisted night theme, and remove internal Shot-layer terminology from the creation homepage without changing backend orchestration.

**Architecture:** Add a production creator settings route that reuses the existing Local Agent provider APIs and refactors the current desktop settings UI into a shared `SettingsPage`. Add a small pure theme core plus a React provider that owns `system | light | dark` state and applies semantic CSS tokens before the first render. Keep all submitted production-route and AIGC-policy values unchanged so the backend still compiles `shot_visual_layers_v1`.

**Tech Stack:** React 18, TypeScript, Vite, Tailwind CSS, Electron, Local Agent HTTP APIs, Node assertion/esbuild checks.

## Global Constraints

- The settings entry lives in the user-avatar menu, not the primary creator navigation.
- Text, image, and video generation settings remain independent and are stored only by the Local Agent.
- Appearance supports exactly `system`, `light`, and `dark` and persists locally.
- The start-creation homepage must not describe IP A-roll, HyperFrames, AIGC layers, or the three-layer Shot contract.
- Backend `shot_visual_layers_v1`, `talking_head`, `cinematic_story`, `auto`, and `disabled` values remain unchanged.
- No new model provider, cloud credential storage, billing feature, or developer diagnostics are added.

---

## File structure

- Create `frontend/src/theme/theme.ts`: pure mode normalization, resolution, persistence, root application, and system-theme subscription.
- Create `frontend/src/theme/ThemeProvider.tsx`: app-wide theme state and context.
- Create `frontend/scripts/settings-theme-check.mjs`: executable theme and source-contract regression tests.
- Rename `frontend/src/pages/DesktopPage.tsx` to `frontend/src/pages/SettingsPage.tsx`: shared settings UI used by creator and developer shells.
- Modify `frontend/src/main.tsx`: apply the stored theme before React renders and install the provider.
- Modify `frontend/src/creatorRoutes.ts`: add the production `#/settings` route.
- Modify `frontend/src/features/creator-studio/CreatorShell.tsx`: add avatar-menu navigation and render shared settings.
- Modify `frontend/src/features/creator-studio/StartCreationPage.tsx`: remove layer copy while retaining backend values.
- Modify `frontend/src/pages/DirectorStudioPage.tsx`: update the shared settings import.
- Modify `frontend/src/index.css` and `frontend/tailwind.config.js`: use theme-aware semantic color tokens.
- Modify `frontend/package.json`: run the new settings/theme check in the normal test surface.

---

### Task 1: Theme behavior core

**Files:**
- Create: `frontend/scripts/settings-theme-check.mjs`
- Create: `frontend/src/theme/theme.ts`
- Create: `frontend/src/theme/ThemeProvider.tsx`
- Modify: `frontend/src/main.tsx`
- Modify: `frontend/package.json`

**Interfaces:**
- Produces: `ThemeMode = 'system' | 'light' | 'dark'`
- Produces: `normalizeThemeMode(value: unknown): ThemeMode`
- Produces: `resolveTheme(mode: ThemeMode, prefersDark: boolean): 'light' | 'dark'`
- Produces: `readThemeMode(storage?: Pick<Storage, 'getItem'>): ThemeMode`
- Produces: `persistThemeMode(mode: ThemeMode, storage?: Pick<Storage, 'setItem'>): void`
- Produces: `initializeTheme(options?): ThemeMode`
- Produces: `applyTheme(mode: ThemeMode, options?): () => void`
- Produces: `ThemeProvider` and `useTheme(): { mode: ThemeMode; setMode(mode: ThemeMode): void }`

- [ ] **Step 1: Write the failing theme check**

Create `frontend/scripts/settings-theme-check.mjs` with an esbuild bundle of `src/theme/theme.ts` and assertions:

```js
import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const temp = await mkdtemp(join(tmpdir(), 'settings-theme-'))
const bundle = join(temp, 'theme.mjs')
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
  const storage = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) }
  theme.persistThemeMode('dark', storage)
  assert.equal(theme.readThemeMode(storage), 'dark')
  const root = { dataset: {}, style: {} }
  let listener
  const media = {
    matches: true,
    addEventListener: (_event, callback) => { listener = callback },
    removeEventListener: (_event, callback) => { if (listener === callback) listener = undefined },
  }
  const dispose = theme.applyTheme('system', { root, media })
  assert.equal(root.dataset.theme, 'dark')
  assert.equal(root.style.colorScheme, 'dark')
  media.matches = false
  listener()
  assert.equal(root.dataset.theme, 'light')
  dispose()
  assert.equal(listener, undefined)
} finally {
  await rm(temp, { recursive: true, force: true })
}
```

Add `"test:settings": "node scripts/settings-theme-check.mjs"` to `frontend/package.json`.

- [ ] **Step 2: Run the check and verify RED**

Run: `cd frontend && npm run test:settings`

Expected: FAIL because `src/theme/theme.ts` does not exist.

- [ ] **Step 3: Implement the pure theme core**

Create `frontend/src/theme/theme.ts`:

```ts
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

export function persistThemeMode(mode: ThemeMode, storage: Pick<Storage, 'setItem'> = window.localStorage): void {
  storage.setItem(THEME_STORAGE_KEY, mode)
}

function syncRootTheme(mode: ThemeMode, root: HTMLElement, media: Pick<MediaQueryList, 'matches'>): void {
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
```

- [ ] **Step 4: Verify the pure checks pass**

Run: `cd frontend && npm run test:settings`

Expected: PASS with exit code 0.

- [ ] **Step 5: Add the React theme owner and pre-render initialization**

Create `frontend/src/theme/ThemeProvider.tsx` with a context that initializes from `readThemeMode`, calls `applyTheme` in an effect, persists on `setMode`, and throws a clear error when `useTheme` is called outside the provider. Update `main.tsx` to call `initializeTheme()` before `createRoot` and wrap `<App />` in `<ThemeProvider>`. `initializeTheme` applies once without installing a listener; the provider owns the only live system-theme listener.

Core provider shape:

```tsx
const ThemeContext = createContext<ThemeContextValue | null>(null)

export function ThemeProvider({ children }: PropsWithChildren) {
  const [mode, setModeState] = useState<ThemeMode>(() => readThemeMode())
  useEffect(() => applyTheme(mode), [mode])
  const setMode = useCallback((next: ThemeMode) => {
    persistThemeMode(next)
    setModeState(next)
  }, [])
  return <ThemeContext.Provider value={{ mode, setMode }}>{children}</ThemeContext.Provider>
}
```

- [ ] **Step 6: Run theme check, creator checks, and build**

Run: `cd frontend && npm run test:settings && npm run test:creator && npm run build`

Expected: all commands exit 0.

- [ ] **Step 7: Commit the theme behavior**

```bash
git add frontend/package.json frontend/scripts/settings-theme-check.mjs frontend/src/theme frontend/src/main.tsx
git commit -m "feat: add persisted creator theme modes"
```

---

### Task 2: Shared generation settings route

**Files:**
- Rename: `frontend/src/pages/DesktopPage.tsx` to `frontend/src/pages/SettingsPage.tsx`
- Modify: `frontend/src/pages/SettingsPage.tsx`
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`
- Modify: `frontend/src/features/creator-studio/CreatorShell.tsx`
- Modify: `frontend/src/creatorRoutes.ts`
- Modify: `frontend/scripts/director-studio-logic-check.mjs`
- Modify: `frontend/scripts/settings-theme-check.mjs`

**Interfaces:**
- Consumes: `useTheme()` from Task 1.
- Consumes: existing Local Agent provider and JiMeng functions from `frontend/src/services/localAgent.ts`.
- Produces: `SettingsPage({ variant, onBack })`, where `variant?: 'creator' | 'developer'` and `onBack?: () => void`.
- Produces: creator route `{ kind: 'creator'; page: 'settings' }` and avatar-menu navigation to `#/settings`.

- [ ] **Step 1: Add failing settings source assertions**

Extend `settings-theme-check.mjs` to read `SettingsPage.tsx` and assert:

```js
// Change the fs/promises import to include readFile, then define these beside the bundle path.
const settingsSource = await readFile(new URL('../src/pages/SettingsPage.tsx', import.meta.url), 'utf8')
const startSource = await readFile(new URL('../src/features/creator-studio/StartCreationPage.tsx', import.meta.url), 'utf8')
const cssSource = await readFile(new URL('../src/index.css', import.meta.url), 'utf8')
const tailwindSource = await readFile(new URL('../tailwind.config.js', import.meta.url), 'utf8')

for (const label of ['文本生成', '图片生成', '视频生成', '外观']) {
  assert.match(settingsSource, new RegExp(label))
}
assert.match(settingsSource, /fetchModelProviderSettings/)
assert.match(settingsSource, /saveModelProviderSettings/)
assert.match(settingsSource, /useTheme/)
assert.match(settingsSource, /跟随系统/)
assert.match(settingsSource, /仅保存在本机/)
```

Add these assertions to `frontend/scripts/director-studio-logic-check.mjs`:

```js
assert.deepEqual(parseAppRoute('#/settings', false), { kind: 'creator', page: 'settings' })
assert.match(creatorShellSource, />设置</)
assert.match(creatorShellSource, /onNavigate\('#\/settings'\)/)
assert.doesNotMatch(creatorShellSource.match(/<nav[\s\S]*?<\/nav>/)?.[0] || '', />设置</)
```

- [ ] **Step 2: Run the settings check and verify RED**

Run: `cd frontend && npm run test:settings && npm run test:director`

Expected: FAIL because `SettingsPage.tsx`, appearance controls, `#/settings`, and the avatar-menu action do not exist.

- [ ] **Step 3: Rename and adapt the existing settings page**

Move `DesktopPage.tsx` to `SettingsPage.tsx`. Preserve the existing provider state, masked-key handling, Local Agent save call, and JiMeng setup/login behavior. Add:

```ts
type SettingsTab = ModelCapability | 'appearance'
interface SettingsPageProps {
  variant?: 'creator' | 'developer'
  onBack?: () => void
}
```

Use four labels mapped to the existing capability IDs plus `appearance`. Render model fields only for capability tabs, JiMeng controls only for `text_to_video`, and appearance radio cards only for `appearance`.

Appearance control shape:

```tsx
const { mode, setMode } = useTheme()
const appearanceModes = [
  { id: 'system', label: '跟随系统', detail: '自动匹配 macOS 外观' },
  { id: 'light', label: '浅色', detail: '始终使用明亮界面' },
  { id: 'dark', label: '深色', detail: '始终使用夜间界面' },
] as const
```

Use `role="radiogroup"` and real radio inputs. Keep all model fields controlled and keep the existing save feedback.

- [ ] **Step 4: Add the route and update both consumers**

Extend `CreatorPageRoute` in `frontend/src/creatorRoutes.ts`:

```ts
| { kind: 'creator'; page: 'settings'; shouldReplace?: true }
```

Add parsing before project-step parsing:

```ts
if (decoded.length === 1 && decoded[0] === 'settings') return { kind: 'creator', page: 'settings' }
```

Update `DirectorStudioPage.tsx` to import `SettingsPage` and render `<SettingsPage variant="developer" />`. Update `CreatorShell.tsx` to import the same page and render `<SettingsPage variant="creator" onBack={() => onNavigate('#/create')} />`.

Add a `设置` menu item before connection status:

```tsx
<button type="button" role="menuitem" onClick={() => {
  closeTransientUi()
  onNavigate('#/settings')
}}>设置</button>
```

- [ ] **Step 5: Run settings, director, lint, and build checks**

Run: `cd frontend && npm run test:settings && npm run test:director && npm run lint && npm run build`

Expected: all commands exit 0.

- [ ] **Step 6: Commit the shared settings page**

```bash
git add frontend/src/pages frontend/src/creatorRoutes.ts frontend/src/features/creator-studio/CreatorShell.tsx frontend/scripts/director-studio-logic-check.mjs frontend/scripts/settings-theme-check.mjs
git commit -m "feat: restore creator generation settings"
```

---

### Task 3: Hide internal Shot structure on the creation homepage

**Files:**
- Modify: `frontend/scripts/settings-theme-check.mjs`
- Modify: `frontend/src/features/creator-studio/StartCreationPage.tsx`
- Modify: `frontend/src/index.css`

**Interfaces:**
- Preserves: `CreatorProductionRoute` values `talking_head | cinematic_story`.
- Preserves: `CreatorAIGCPolicy` values `auto | disabled`.
- Removes: creator homepage copy describing three internal visual layers.

- [ ] **Step 1: Add failing homepage copy assertions**

Read `StartCreationPage.tsx` in `settings-theme-check.mjs` and add:

```js
for (const forbidden of ['每个 Shot 都按三层设计', 'IP A-roll', 'HyperFrames', 'AIGC 丰富层', '三层口播', '影视化 Shot']) {
  assert.doesNotMatch(startSource, new RegExp(forbidden))
}
for (const expected of ['IP 口播视频', '影视短片', '智能补充素材', '仅使用本地素材']) {
  assert.match(startSource, new RegExp(expected))
}
assert.match(startSource, /value: 'talking_head'/)
assert.match(startSource, /value: 'cinematic_story'/)
assert.match(startSource, /value: 'auto'/)
assert.match(startSource, /value: 'disabled'/)
```

- [ ] **Step 2: Run the check and verify RED**

Run: `cd frontend && npm run test:settings`

Expected: FAIL on the current layer banner and internal option labels.

- [ ] **Step 3: Replace only presentation copy**

Delete the `creator-layer-contract` block. Change option labels while keeping values:

```ts
const productionRouteOptions = [
  { value: 'talking_head', label: 'IP 口播视频' },
  { value: 'cinematic_story', label: '影视短片' },
]
const aigcPolicyOptions = [
  { value: 'auto', label: '智能补充素材' },
  { value: 'disabled', label: '仅使用本地素材' },
]
```

Change the form label from `AIGC 丰富层` to `补充素材`. Remove unused `.creator-layer-contract` CSS.

- [ ] **Step 4: Verify frontend copy and backend request contract**

Run: `cd frontend && npm run test:settings && npm run test:creator`

Expected: source-copy assertions pass and creator request assertions still show `visualLayerContract: 'shot_visual_layers_v1'` with all designed layers.

- [ ] **Step 5: Commit homepage simplification**

```bash
git add frontend/scripts/settings-theme-check.mjs frontend/src/features/creator-studio/StartCreationPage.tsx frontend/src/index.css
git commit -m "refactor: keep shot layers behind the creator workflow"
```

---

### Task 4: Complete semantic dark-theme styling

**Files:**
- Modify: `frontend/tailwind.config.js`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/settings-theme-check.mjs`

**Interfaces:**
- Consumes: root `data-theme="light|dark"` from Task 1.
- Produces: semantic RGB-channel variables used by Tailwind and handwritten CSS.

- [ ] **Step 1: Add failing token assertions**

Extend `settings-theme-check.mjs`:

```js
assert.match(cssSource, /:root[\s\S]*--color-background:/)
assert.match(cssSource, /\[data-theme=['"]dark['"]\][\s\S]*--color-background:/)
assert.match(tailwindSource, /rgb\(var\(--color-background\) \/ <alpha-value>\)/)
assert.match(cssSource, /color-scheme:/)
```

- [ ] **Step 2: Run the check and verify RED**

Run: `cd frontend && npm run test:settings`

Expected: FAIL because semantic theme variables are not defined.

- [ ] **Step 3: Define light and dark tokens**

At the start of `index.css`, define RGB-channel variables:

```css
:root {
  color-scheme: light;
  --color-primary: 232 148 18;
  --color-primary-soft: 255 233 168;
  --color-primary-light: 255 214 90;
  --color-primary-dark: 43 22 6;
  --color-background: 255 246 214;
  --color-background-mist: 255 233 168;
  --color-background-card: 255 253 246;
  --color-ink: 43 22 6;
  --color-ink-muted: 111 77 36;
  --color-ink-soft: 152 114 58;
  --color-line: 232 207 134;
  --color-success: 31 157 98;
  --color-danger: 180 66 34;
}

[data-theme='dark'] {
  color-scheme: dark;
  --color-primary: 245 171 64;
  --color-primary-soft: 70 48 18;
  --color-primary-light: 248 198 91;
  --color-primary-dark: 24 16 10;
  --color-background: 22 17 12;
  --color-background-mist: 39 29 18;
  --color-background-card: 31 25 19;
  --color-ink: 249 239 220;
  --color-ink-muted: 210 188 154;
  --color-ink-soft: 169 147 116;
  --color-line: 76 61 43;
  --color-success: 69 190 127;
  --color-danger: 244 126 96;
}
```

- [ ] **Step 4: Point Tailwind semantic colors at the tokens**

Replace hard-coded semantic colors in `tailwind.config.js` with values such as:

```js
DEFAULT: 'rgb(var(--color-background) / <alpha-value>)'
```

Use the matching variable for primary, background, ink, line, and success keys. Keep semantic class names unchanged.

- [ ] **Step 5: Convert creator CSS surfaces to semantic variables**

Apply this replacement map throughout the creator, settings, director surface, and markdown styles in `index.css`:

```text
#FFF6D6 -> rgb(var(--color-background))
#FFFDF6 -> rgb(var(--color-background-card))
#FFE9A8 -> rgb(var(--color-background-mist))
#FFF0BE -> rgb(var(--color-background-mist))
#2B1606 -> rgb(var(--color-ink))
#3A1C06 -> rgb(var(--color-ink))
#6F4D24 and #7B5B2B -> rgb(var(--color-ink-muted))
#6F3A08 -> rgb(var(--color-ink-muted))
#98723A -> rgb(var(--color-ink-soft))
#E8CF86 -> rgb(var(--color-line))
#E89412 -> rgb(var(--color-primary))
#B76512 and #8B4A12 -> rgb(var(--color-primary))
#FFD65A -> rgb(var(--color-primary-light))
#B44222 -> rgb(var(--color-danger))
#1F9D62 -> rgb(var(--color-success))
```

For existing `rgba(...)` shadows, replace the palette-derived RGB triplet with the corresponding CSS variable form, for example `rgba(43, 22, 6, 0.08)` becomes `rgb(var(--color-ink) / 0.08)`. Preserve the intentional media-preview black background. Add `.settings-page` field, tab, radio-card, notice, and back-button rules using the same tokens rather than new literal colors.

- [ ] **Step 6: Run theme checks, lint, and build**

Run: `cd frontend && npm run test:settings && npm run lint && npm run build`

Expected: all commands exit 0 and Vite emits the production bundle.

- [ ] **Step 7: Commit theme styling**

```bash
git add frontend/tailwind.config.js frontend/src/index.css frontend/scripts/settings-theme-check.mjs
git commit -m "feat: add complete night theme styling"
```

---

### Task 5: End-to-end verification and documentation

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Verifies all preceding tasks together.

- [ ] **Step 1: Update user documentation**

Add this current-version bullet to `README.md`:

```markdown
- 生产版用户可从头像菜单进入“设置”，分别管理文本、图片、视频生成接口和跟随系统/浅色/深色外观；开始创作页只展示创作选择，Shot 三层编排继续由后端自动完成。
```

Add these entries below `## Unreleased` in `CHANGELOG.md`:

```markdown
### Added

- Added production Creator settings for local text, image, and video model providers plus persisted system, light, and dark appearance modes.

### Changed

- Moved the settings entry into the user-avatar menu and removed internal three-layer Shot terminology from the start-creation page while preserving backend orchestration.
```

- [ ] **Step 2: Run the complete frontend verification suite**

Run:

```bash
cd frontend
npm run test:settings
npm run test:creator
npm run test:director
npm run lint
npm run build
node --test electron/*.test.cjs
```

Expected: every command exits 0 with zero failed tests.

- [ ] **Step 3: Build the desktop package**

Run: `cd frontend && npm run electron:build`

Expected: exit 0 and a `Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg` in `frontend/release/`.

- [ ] **Step 4: Inspect the packaged UI**

Open the packaged app and verify:

1. Avatar → 设置 opens the settings route.
2. Text, image, video, and appearance tabs render.
3. Selecting dark mode changes the whole creator surface and survives reload.
4. Start creation contains no three-layer explanation or internal layer names.
5. Creating a request still submits the same production-route and AIGC-policy values.

- [ ] **Step 5: Check diff and commit documentation**

Run: `git diff --check && git status --short`

Then:

```bash
git add README.md CHANGELOG.md
git commit -m "docs: document creator settings and appearance"
```
