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
  the live app (desktop and Big Picture). An SSR snapshot does not prove clicks or effects.
