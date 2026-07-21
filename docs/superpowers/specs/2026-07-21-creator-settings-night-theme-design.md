# Creator settings and night theme design

## Context

The production Creator Shell currently exposes only creation and video-library routes. Model provider configuration still exists in `DesktopPage` and the Local Agent API, but the production app no longer has a route or menu entry that users can reach. The start-creation page also exposes internal Shot-layer terminology that belongs to orchestration rather than the creator-facing interface.

## Goals

- Restore a production-accessible settings page from the user-avatar menu.
- Let users configure text, image, and video generation independently.
- Keep API credentials local and preserve the existing Local Agent storage contract.
- Add persisted `system`, `light`, and `dark` appearance modes.
- Remove Shot-layer implementation details from the start-creation page while preserving the backend `shot_visual_layers_v1` workflow unchanged.

## User experience

### Navigation

The avatar menu adds a `设置` item. Selecting it navigates to `#/settings`, closes the menu, and keeps settings out of the primary creation navigation. The settings page provides an explicit return path to creation.

### Settings page

The page uses four tabs:

1. `文本生成`
2. `图片生成`
3. `视频生成`
4. `外观`

Each model tab edits its own OpenAI-compatible base URL, model name, and API key. Saved-key previews remain masked. Empty API-key input preserves an existing key. Saving writes all three provider configurations through the existing Local Agent endpoint and shows clear loading, success, and error feedback.

Video settings also contain the existing Dreamina/JiMeng setup controls because those tools provide video-generation capability. Service diagnostics and implementation-level commands remain secondary information rather than the page's primary focus.

The appearance tab presents three mutually exclusive modes: follow system, light, and dark. A choice applies immediately and persists locally.

### Start creation

The start page removes the banner that explains IP A-roll, HyperFrames, and AIGC layers. User-facing options use outcome language:

- `IP 口播视频` instead of `三层口播（IP + 文字特效 + AIGC）`
- `影视短片` instead of `影视化 Shot（三层画面设计）`
- `智能补充素材` instead of `AIGC 丰富层`
- `仅使用本地素材` instead of explaining disabled AIGC execution

The submitted values remain `talking_head`, `cinematic_story`, `auto`, and `disabled`. Backend planning and compilation therefore continue to create the same three-layer Shot contract without exposing that contract on the creation homepage.

## Architecture

### Routing and shell

- Extend the creator route union with a `settings` page and parse `#/settings` as a production route.
- Render a dedicated creator settings page from `CreatorShell`.
- Add the avatar-menu action without adding a third primary navigation button.

### Configuration

- Reuse `fetchModelProviderSettings`, `mergeModelProviderSettings`, and `saveModelProviderSettings`.
- Extract or adapt the existing provider and JiMeng controls so they can render inside the Creator Shell without duplicating persistence logic.
- Keep provider secrets in the existing Local Agent data directory. No model credentials are added to cloud project payloads except the existing per-run client provider handoff.

### Theme

- Add a small theme module with `system | light | dark` as the public type.
- Persist the selected mode under one versioned local-storage key.
- Resolve `system` through `prefers-color-scheme` and listen for operating-system changes only while system mode is selected.
- Apply the resolved theme through `document.documentElement.dataset.theme` and update `color-scheme`.
- Define shared light and dark color tokens as CSS custom properties. Tailwind semantic colors and creator-specific CSS consume those tokens so the complete production UI changes together.
- Initialize the stored theme before React renders to avoid a light-theme flash when the user selected dark mode.

## Data flow

1. App startup reads the stored appearance mode, resolves it, and applies the root theme.
2. The settings page reads model metadata from the Local Agent without exposing raw saved keys.
3. The user edits one or more capability tabs and saves.
4. The Local Agent validates and persists the same three provider records it already supports.
5. A creation run calls the existing provider handoff and backend plan compiler.
6. Backend Shot planning continues to emit `ip_aroll`, `hyperframes_text`, and `aigc_enrichment` layers regardless of whether the homepage describes them.

## Error and accessibility behavior

- Route fallbacks continue to return users to the creation page.
- Settings load and save failures stay visible in the page and do not erase current form values.
- Theme controls use radio semantics, visible keyboard focus, and text labels in addition to icons.
- Tabs use tab/list semantics and remain usable by keyboard.
- Dark mode maintains readable contrast for backgrounds, fields, borders, notices, dialogs, and focus indicators.
- Reduced-motion preferences continue to suppress nonessential transitions.

## Testing

- Route tests cover `#/settings` and invalid-route fallback.
- Creator contract checks assert the start page no longer renders internal Shot-layer wording while request values remain unchanged.
- Theme tests cover stored-mode normalization, system resolution, persistence, and application to the document root.
- Settings checks cover all three capability labels, existing Local Agent persistence functions, and avatar-menu navigation.
- The complete frontend creator/director checks, ESLint, TypeScript/Vite build, Electron tests, and desktop package build run before delivery.

## Out of scope

- Changing the backend `shot_visual_layers_v1` schema or composition policy.
- Moving credentials to the cloud.
- Adding new model providers or billing logic.
- Exposing developer trace, agent roles, or internal layer diagnostics on the start page.
