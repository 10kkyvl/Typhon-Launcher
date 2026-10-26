---
name: typhon-theme
description: Use when asked to make, restyle, recolor or fix a theme, color scheme or skin for the Typhon game launcher, or when a .typhontheme file is mentioned or Typhon refuses to import one.
---

# Typhon theme

## Overview

A Typhon theme is one JSON file with the `.typhontheme` extension. The user imports it in
Typhon: **Settings → Appearance → Import**. The launcher checks every field and refuses the
whole file on the first mistake, so the file has to match the shape below exactly. There is no
other format: no `colors` object, no `mode`, no `author`.

## The file

The file is exactly this object and nothing else (unknown keys are refused):

```json
{
  "version": 1,
  "theme": {
    "id": "<lowercase id>",
    "name": "<display name>",
    "base": "dark",
    "tokens": { "<token>": "<value>" },
    "css": "",
    "layout": { "sidebar": {}, "topbar": {}, "gamePage": {} }
  }
}
```

| Field | Rule |
|---|---|
| `version` | Always the number `1`. |
| `theme.id` | `^[a-z0-9][a-z0-9-]{0,63}$`, e.g. `ocean-night`. Not `dark`, `light` or `contrast` (those are built in). Re-importing a file with the same id replaces that theme. |
| `theme.name` | 1–64 characters shown in the theme list; any language. |
| `theme.base` | `"dark"` or `"light"`: which way the launcher derives the personal accent palette. Pick by the background lightness. |
| `theme.tokens` | Map of token name → value. Names only from [tokens.md](tokens.md). Do not include `--ui-scale` (it belongs to a separate setting and is refused). |
| `theme.css` | Optional extra CSS, normally `""`. Leave it empty unless the user asks for something tokens cannot do. Rules in [Custom CSS](#custom-css). |
| `theme.layout` | Optional: sidebar side and order, search position, top bar buttons, game page sections, what is hidden. Leave it out unless the user asks to move or hide something. Every id and rule is in [layout.md](layout.md). |

## Writing the tokens

1. **Always set all 24 color tokens** (groups Background, Surfaces, Text, Accent, Status in
   [tokens.md](tokens.md)). A token left out falls back to the built-in *dark* value, so a light
   theme with missing colors ends up half dark.
2. Size, font, animation and shadow tokens are optional. Leave them out unless the user asked for
   a different shape or feel; the defaults are listed in tokens.md. Exception: with
   `"base": "light"` also set the three shadows, because the defaults are heavy black ones made
   for a dark window:
   `--shadow-pop`: `0 0.8rem 2.8rem rgba(15, 23, 32, 0.12), 0 2px 0.8rem rgba(15, 23, 32, 0.08)`,
   `--shadow-modal`: `0 2rem 6rem rgba(15, 23, 32, 0.16)`,
   `--shadow-lift`: `0 1.2rem 2.8rem -0.8rem rgba(15, 23, 32, 0.22)` (tint them to the theme if you like).
3. Keep the surface ladder ordered by elevation: `--bg` → `--surface` → `--surface-2` →
   `--surface-3` → `--surface-4`, each a small step lighter (dark base) or darker (light base).
4. Keep text readable. Contrast ratio (WCAG) of `--text` on `--bg` and on `--surface-2` at least
   7:1, `--text-2` at least 4.5:1, `--text-3` at least 3:1. White text sits on `--accent` buttons,
   so `--accent` needs at least 3:1 against `#ffffff`. The launcher does not check contrast for
   you — compute it before writing the file.
5. Derive the accent family from one hue: `--accent-hover` a step lighter (dark base) or darker
   (light base); `--accent-subtle` the accent at 0.12–0.18 alpha; `--accent-ring` the accent at
   about 0.5 alpha; `--accent-text` a lighter (dark base) or darker (light base) tint readable on
   `--accent-subtle`. Same pattern for `--success`, `--warning`, `--danger` and their `-subtle`.
6. If the user has a personal accent color set in Typhon, it is painted over `--accent*`; mention
   this when they say the accent did not change.

## Values the launcher accepts

Every value is checked against its token's kind (column *Kind* in tokens.md). Any character
outside `# 0-9 A-Z a-z space . , % ( ) / + * -` is refused: no quotes, `;`, `{}`, `@`, `\`,
`url(`, `/*`. Values are at most 120 characters.

| Kind | Accepted | Example |
|---|---|---|
| color | `#rgb`, `#rgba`, `#rrggbb`, `#rrggbbaa`; `rgb()`/`rgba()`/`hsl()`/`hsla()` (commas `rgb(1, 2, 3)` or spaces `rgb(1 2 3 / 50%)`, never mixed, no trailing comma); `oklch()`, `oklab()`, `lab()`, `lch()`, `hwb()` (spaces only); `color(display-p3 1 0 0)`; `color-mix(in <space>, A, B 30%)` with percentages 0–100%; every CSS color name (`red`, `rebeccapurple`); `transparent`, `currentColor`; `var(--<color token>)` | `rgba(46, 196, 182, 0.16)` |
| length | `0`, or a number with `px`, `rem`, `em`, `%`, `vw`, `vh`, `vmin`, `vmax`, `ch`, `ex`, `dvh`/`svh`/`lvh`, `dvw`/`svw`/`lvw`, `pt`, `cm`, `mm` or `in` (write `5px`, not `5.px`). Never negative, except for the two `--tracking-*` tokens. At most 10000 for px/pt/cm/mm/in and 1000 for every other unit. `calc()` with spaces around `+` and `-` (a length times a number, a length divided by a non-zero number; length times length is refused), `min()`, `max()`, `clamp(a, b, c)`, `var(--<length token>)`. A bare number like `12` is refused | `0.8rem` |
| time | number with `ms` or `s`, not negative, at most 10s. A bare `300` is refused | `160ms` |
| ease | `linear`, `ease`, `ease-in`, `ease-out`, `ease-in-out`, `step-start`, `step-end`, `steps(n)` or `steps(n, jump-end)` (also `jump-start`, `jump-none`, `jump-both`, `start`, `end`), `cubic-bezier(x1, y1, x2, y2)` with x1 and x2 between 0 and 1; case does not matter; `var(--<ease token>)` | `cubic-bezier(0.2, 0.8, 0.2, 1)` |
| shadow | `none`, or comma-separated shadows: optional single `inset`, 2–4 lengths (may be negative), optionally one color of any form above; `var(--<shadow token>)` | `0 2rem 6rem rgba(0, 0, 0, 0.55)` |
| font | comma-separated family names of letters, digits, spaces and hyphens, no quotes; every word must start with a letter (or `-` as in `-apple-system`), so `Source Sans 3`, `Font Awesome 5 Free` and `3270 Nerd Font` are refused (invalid without quotes: the whole UI would fall back to serif). End with a generic family (`sans-serif`, `serif`, `monospace`, `system-ui`) | `Inter, Segoe UI, sans-serif` |

A `var(--token)` must name a token from tokens.md of the same kind and never the token it is
set on.

`1rem` is 11 px in Typhon (before the user's interface scale), so `1.5rem` text is about 16 px.

## Custom CSS

`theme.css` is at most 32 KB, braces balanced and nested at most 4 deep. Target window parts by
their `data-part` and `data-id` attributes (list in [layout.md](layout.md)), for example
`[data-part="sidebar-item"][data-id="library"]`. Ordering and hiding belong in `theme.layout`,
not in CSS.

A theme must never make the launcher load anything from outside, so the launcher refuses the
whole file (case and spaces before `(` do not matter, comments are ignored) when the CSS
contains:

- a backslash `\` anywhere: no CSS escapes, not even inside strings or comments;
- `url(`, `src(`, `image-set(` (and `-webkit-image-set(`), `image(`, `cross-fade(`, `attr(`;
- `@import`, `@font-face`, `@namespace`;
- `expression(`, `javascript:`, `behavior:`, `-moz-binding`, `</style`;
- `--ui-scale` (the interface scale belongs to its own setting).

Use gradients and colors instead of images, and fonts the user already has installed (by family
name in `--font`), not web fonts.

## Example

A complete dark theme ("night ocean": deep blue with a teal accent):

```json
{
  "version": 1,
  "theme": {
    "id": "night-ocean",
    "name": "Night ocean",
    "base": "dark",
    "tokens": {
      "--bg": "#06111c",
      "--bg-sidebar": "#06111c",
      "--surface": "#0a1826",
      "--surface-2": "#0e1f31",
      "--surface-3": "#14283d",
      "--surface-4": "#1b3349",
      "--border": "rgba(255, 255, 255, 0.07)",
      "--border-strong": "rgba(255, 255, 255, 0.12)",
      "--hover": "rgba(255, 255, 255, 0.045)",
      "--hover-strong": "rgba(255, 255, 255, 0.075)",
      "--text": "#e8f3f6",
      "--text-2": "#97b2c0",
      "--text-3": "#6d8a99",
      "--accent": "#13877d",
      "--accent-hover": "#18a194",
      "--accent-subtle": "rgba(19, 135, 125, 0.18)",
      "--accent-text": "#6fe0d3",
      "--accent-ring": "rgba(19, 135, 125, 0.5)",
      "--success": "#5cc28a",
      "--success-subtle": "rgba(92, 194, 138, 0.13)",
      "--warning": "#e0b25c",
      "--warning-subtle": "rgba(224, 178, 92, 0.13)",
      "--danger": "#e06c75",
      "--danger-subtle": "rgba(224, 108, 117, 0.12)"
    },
    "css": ""
  }
}
```

## Before handing the file over

- [ ] Top level has exactly `version` (= 1) and `theme`; `theme` has only `id`, `name`, `base`,
      `tokens`, `css` and, if the user asked to move or hide something, `layout`.
- [ ] Every `layout` id is in layout.md, none appears twice, `settings` is not hidden.
- [ ] `css` has no backslash and nothing from the refused list in Custom CSS.
- [ ] All 24 color tokens are present (plus the three shadows for a light base); every token name
      is in tokens.md; no `--ui-scale`.
- [ ] Every value matches its kind; no quotes anywhere inside values.
- [ ] Contrast targets from step 4 hold (state the ratios you computed).
- [ ] Saved with the `.typhontheme` extension, UTF-8.

Tell the user to import it in **Settings → Appearance → Import** and to pick it in the theme
list. If Typhon refuses the file:

- the message for a wrong value lists every offending token with its kind (for example
  `--radius-md (length), --dur (time)`); fix exactly those values;
- for any other refusal the message does not name the field, so check in this order: `version` is
  `1`; `id` matches the pattern; `base` is `dark` or `light`; every token name exists in
  tokens.md (and is not `--ui-scale`); no value contains a character outside the allowed set
  (quotes, `;`, `{}`, `url(`), and none is longer than 120 characters; `css` is empty;
- a refused layout names the field and the bad value (for example `sidebar.hidden: unknown id
  "settings"`); fix exactly that id against layout.md.
