---
paths:
  - "frontend/src/**"
---

# Svelte 5 / TS: layer rules

- Layers: `lib/services/*` wrap bindings behind the `inWails` guard (reads return an empty value,
  writes throw; one policy per file); `lib/stores/*` subscribe to events and hold state; components
  and `routes/` never import `bindings/` directly.
- Store init: `Events.On(...)` first, then `await list()`; every `init*` returns a disposer.
- Event names and payloads go through constants and types, not `'x:y'` literals or an unchecked
  `event.data as X`. A new event: Go constant → `RegisterEvent` → TS constant and type → subscription.
- Errors from Go arrive as `typhon:<code>`; the text comes from the domain i18n table. Parsing
  Russian prose from Go is forbidden; if a new code is needed, add it in Go and in the table together.
- All UI strings go through i18n (`ru` is primary, `en` mirrors it; `catalogGuard` keeps parity).
  Confirmations use `ConfirmModal`, not `window.confirm`.
- One game lives in three stores (`downloads`, `installations`, `libraryGames`); join them through
  shared selectors in `lib/`, not a hand-written `.find` in a component. Desktop and Big Picture share
  one behaviour implementation; divergent copies (such as `ownsDownload`) are forbidden.
- A component over ~600 lines or with more than one responsibility is split into a child
  component/controller in a separate, behaviour-free commit before the change that needs it.
- Checks: `npm run check`, `npx vitest run <file>`; for screen behaviour, before/after screenshots in
  the live app (desktop and Big Picture).

## Built to last

The frontend is rebuilt to stay for years and to be extended by themes, later by plugins, in one style.

- A port or rebuild is hunted first (`typhon-hunter` on the old code). The brief lists the defects
  found; the diff closes each or names it as deferred. Defects outside the ported part are reported,
  not fixed (AGENTS.md: no unrelated refactors). Behaviour users rely on is kept (Hard rule 4).
- Interactive components follow the WAI-ARIA pattern of their kind (dialog, menu, tabs, combobox,
  switch, tooltip). Behaviour is proven in `*.dom.test.ts` (jsdom, Testing Library and user-event via
  `lib/testing/dom.ts`): click, typing, Tab order, Escape, focus trap and return, ARIA state. An SSR
  test proves markup only; neither replaces the live run for Wails integration.
- Colours, radii, shadows, fonts, durations and z-index come from tokens (`styles/tokens.css`
  themable, `styles/shell.css` fixed). A literal in a component is a defect; do not add exceptions to
  the guards in `styles/*.test.ts`.
- Theme tokens, `data-part` names and `layout.json` ids are public API for themes. A new one is added
  to every holder (`tokens.css`, `internal/theme/tokens.go` and `presets.go`, `layout.json`, the theme
  skill) in one commit; a rename or removal keeps existing themes working through
  `internal/theme/contract.go` and is documented in `THEMING.md`.
- New routes, nav items, top-bar actions, game-page blocks, settings sections and backdrop claims
  (`claimAmbient` in `lib/stores/ambient.ts`) are one registry entry plus one component, never an `if`
  chain in a shared component. Where no registry exists yet, the first such feature creates it.
- Before writing a button, menu, dialog, tab, switch, select, progress bar or notice, look for the base
  component in `lib/components/` (`Page.svelte` is the page container). A missing variant is added to
  the base component; no local copies, no `:global` reach-in.
- `$derived` before `$effect`; every effect and listener cleans up.
