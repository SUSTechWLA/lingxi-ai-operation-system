# Horizontal Historical Shot Review Design

## Purpose

Turn the completed-project Shot review into a practical video review desk. A user should be able to switch shots quickly, read the selected shot without narrow text columns, and inspect all retained media without large unused areas.

## Approved direction

Use a horizontal Shot filmstrip above a full-width review dossier.

```text
+------------------------------------------------------------------+
| Historical Shot review                              5 shots       |
| [Shot 1] [Shot 2] [Shot 3] [Shot 4] [Shot 5]  -> horizontal     |
+------------------------------------------------------------------+
| Shot title and duration                                             |
|                                                                    |
| Narration summary                                    [Show all]    |
|                                                                    |
| Camera             | Composition       | Lighting | Transition    |
|                                                                    |
| IP A-roll           | Text layer        | Enrichment layer        |
| short summary       | short summary     | short summary           |
| [Show details]      | [Show details]    | [Show details]          |
|                                                                    |
| Video / reference images / narration media                         |
+------------------------------------------------------------------+
```

The filmstrip is the signature element: it should feel like a compact editing timeline, not a generic sidebar or tab bar.

## Visual system

- Keep the existing warm creator-studio palette and typography so this remains part of the same product.
- Use the existing primary orange only for the active Shot, focus state, and small structural labels.
- Keep card backgrounds quiet and use spacing, rules, and typographic hierarchy instead of additional decoration.
- Use a full-width content measure while keeping individual prose blocks at a comfortable reading width.

## Components and layout

### Historical Shot selector

- `ShotReviewQueue` remains the selector component but receives a historical-filmstrip presentation.
- The heading and Shot buttons share one full-width top card.
- Shot buttons lay out horizontally with a practical minimum width and horizontal overflow when space is limited.
- Each button contains only the Shot number, content availability summary, and selected/history status.
- The active button exposes `aria-current="true"`; keyboard focus remains visible.

### Historical dossier

- `HistoricalShotInspector` fills the complete row below the selector.
- The title and duration stay in a compact header.
- Narration uses a readable block with a four-line visual limit by default. A user-controlled disclosure reveals the exact full text without changing data.
- Camera, composition, lighting, and transition use a responsive two- or four-column information grid.
- The three visual layers use equal-width cards on desktop. Each card shows a concise preview and an explicit disclosure for the complete retained description.
- Long machine-style identifiers must wrap safely and cannot force narrow columns or horizontal page overflow.
- Media occupies a separate full-width grid below the text review. Images retain click-to-zoom behavior; video and audio retain the existing players.

### Responsive behavior

- Desktop and tablet: selector above the dossier; filmstrip scrolls horizontally if necessary.
- Narrow screens: Shot buttons remain horizontally scrollable, detail grids collapse to one column, and disclosure controls remain reachable.
- No breakpoint may restore the historical left sidebar.

## Data flow

No artifact schema or API changes are required. The existing historical Shot projection remains authoritative. The change is presentation-only:

1. `ProjectWorkspacePage` identifies historical mode and places the selector above the detail.
2. `ShotReviewQueue` renders the horizontal selector.
3. `ShotInspector` renders the selected dossier and its disclosures.
4. Existing selection state reloads the appropriate historical artifact content.

## Empty and failure states

- Missing reference images continue to state that no independent reference image is available.
- Missing registered media continues to show the existing truthful player error and regeneration direction.
- A partial artifact load preserves all readable content and shows the existing partial-read notice.
- Collapsing long text never discards or rewrites the retained historical content.

## Verification

- Add static contract checks proving historical mode receives a filmstrip modifier and does not use the sidebar grid.
- Add checks for accessible current-shot state and long-text disclosures.
- Run Creator Studio checks, lint, TypeScript/Vite build, and relevant backend artifact tests.
- Open the real completed project `vp-1b8ceb41` in the packaged desktop client and verify:
  - five Shot choices appear in one horizontal top strip;
  - the selected dossier spans the available width;
  - narration and layer descriptions do not form narrow vertical text walls;
  - full text remains available through disclosure;
  - actual video, image, and audio states remain visible;
  - the layout remains usable at a narrow window width.

## Scope boundary

This change does not alter historical artifacts, regenerate media, change the current active-project Shot workflow, or redesign unrelated Creator Studio steps.
