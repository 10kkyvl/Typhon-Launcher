# Typhon themes

How themes work in Typhon 2.0, what changed since 1.x, and what happens to a theme made earlier.
The full token list, the value rules and the layout options are in the skill for AI agents:
[`skills/typhon-theme`](skills/typhon-theme/SKILL.md) (the same files "Save skill…" writes from
Settings → Appearance).

## In short

- A theme is one `.typhontheme` file: JSON with file version `1` and a `theme` object.
- `theme` must carry `"contract": 2`. It says the values were written for 2.0, where `1rem` is
  10 pixels.
- Typhon reads a file without `contract` as a 1.x theme and converts it to the new contract.
- There are four built-in themes: dark, light, high contrast and Blush. Their ids (`dark`,
  `light`, `contrast`, `blush`) are reserved.

## What changed in 2.0

| What | 1.x | 2.0 |
|---|---|---|
| Root size | `1rem` = 11 px | `1rem` = 10 px |
| Fonts | Inter Tight | Onest for the interface, JetBrains Mono for paths and code; both ship with the launcher |
| New tokens | — | `--font-mono`, `--accent-bright`, `--island-fill`, `--island-gap`, `--radius-control`, `--ambient-dim`, `--motion-backdrop` |
| Accent | one colour for everything | four roles, see below |
| Built-in themes | three | four, Blush added |
| Defaults | blue accent, navy background, 6–18 px radii | teal accent, graphite background, 8–24 px radii, the size scale of the 2.0 design |

### Accent roles

| Token | Paints | Requirement |
|---|---|---|
| `--accent`, `--accent-hover` | the fill of main buttons and filled badges, always behind a label | the label reads at 4.5:1 or better |
| `--accent-text` | accent-coloured text and icons, links | 4.5:1 or better on the page and surfaces |
| `--accent-ring` | the focus outline and the glow of the main button | 3:1 or better on every surface |
| `--accent-bright` | indicators: progress, switches, checkboxes, selection borders, the tab underline | 3:1 or better on every surface |
| `--accent-subtle` | the soft background of selected filters and tags | — |

The "Readability" panel in the theme editor shows these pairs and warns; it never blocks a save.
The personal accent from Settings is applied on top of the theme and derives every role itself.

The top bar's search field and button groups are rounded by `--radius-control` (`999px`, pills);
to get square controls back, set it to a small length such as `0.8rem`.

Behind the main window's interface a blurred picture of the game's art shows. It is always dimmed
toward the page colour `--bg`; a theme sets only how strongly: `--ambient-dim` is a number from 0
to 1, the share of the page colour laid over the art (`0.6` in the dark theme, `0.74` in the light
ones, `1` turns the art off, as High contrast does). The launcher limits the art's brightness
itself so text on it reads at AA or better. Status colours are text only on surfaces; on the art
they are marks and keep 3:1 or better. When the faint text `--text-3` leaves the art too little
room, as in the built-in light theme, the art is not drawn. How fast the art flows from one game
to the next is `--motion-backdrop`.

## What happens to a 1.x theme

Typhon converts such a theme to the new contract whenever it reads one: from its own storage at
startup, from a file, from pasted text, or from a watched file.

1. The values tell which built-in theme yours was made from (dark, light or high contrast) and
   which launcher version it was copied from.
2. A value you never changed is replaced with today's value of that built-in theme. This is how
   your theme gets the new fonts, radii and colours.
3. A value you changed stays. In lengths and shadows every `rem` number is multiplied by 1.1, so
   the size in pixels does not change.
4. New tokens are added from the same built-in theme. If you set your own `--accent`, it also
   becomes `--accent-bright`: in 1.x the accent painted the indicators too. Likewise your own
   `--bg-sidebar` becomes `--island-fill`, the fill of the floating sidebar.
5. In your custom CSS, renamed window parts get their new names, see below.
6. The theme is marked `"contract": 2` and is not converted again.

The stored file is not rewritten at startup: the new form reaches the disk the first time a theme
is saved or switched.

### Window part names

Tokens, `data-part` names and layout ids are part of the theme format: they are not removed or
renamed without converting older themes. 2.0 renames one part: the account block moved to the top
bar, and `sidebar-account` became `topbar-account`. In a 1.x theme a selector
`[data-part="sidebar-account"]` (any quoting, any operator, as long as the value is the whole name)
is rewritten to the new name on read; the rest of the CSS stays as it is, and comments and strings
are not touched. In a theme with `"contract": 2` write `topbar-account` yourself.

2.0 adds six parts: `dialog` (a dialog's panel), `menu` (a dropdown menu), `listbox` (the list
of a select field), `tooltip` (the hint next to a button or a field), `popover` (a small panel
next to a button: the notifications, a reaction picker) and `toast` (a short message in the corner
of the window). All but the toast are drawn above the window, outside the part that opened them. A selector such as `[data-part="topbar-account"] .menu` no longer finds
the account menu: write `[data-part="menu"]`. The notifications panel is no longer inside
`[data-part="topbar-action"][data-id="notifications"]`: write
`[data-part="popover"][data-id="notifications"]`.

A count on a control (unread messages, friend requests: in the sidebar, the top bar and the chat) is
the part `count-badge`; on a button's corner it has the class `corner`, on an accent fill the class
`on-accent`.

### Limits

- **Sizes in custom CSS are not converted.** `rem` sizes inside the `css` field come out about
  9 % smaller. Fix them by hand: multiply by 1.1. Only window part names are converted.
- **A partial theme is compared with the last 1.x release only.** In a theme with just a few
  tokens every one of them counts as yours, unless it equals the 1.4 preset.
- **Typhon 1.x does not open 2.0 theme files.** It refuses a file with the `contract` field.
- **Downgrading to 1.x and coming back.** The old launcher reads the 2.0 theme storage but drops
  the mark when it saves. Typhon 2.0 recognises a theme that holds the new tokens and does not
  convert it twice. A partial theme without the new tokens is converted again: its lengths grow
  by 10 %.
- **A theme from a later version.** A file whose `contract` is above 2 is refused with "This
  theme was made for a newer version of Typhon".

## Big Picture and the overlay

- Big Picture is always dark. It takes neutral and status colours from the built-in dark theme.
  A dark theme's accent is kept as it is when it reads on the dark background; for a light theme
  and for a personal accent it is recomputed for the dark background with the same hue.
- Sizes in Big Picture and the overlay do not follow the theme: the type scale, spacing and
  control heights are fixed there. The theme's colours, fonts and radii still apply.

## Making your own theme

1. Settings → Appearance: pick a theme; its editor opens below with a preview and the
   "Readability" panel. Changes to a built-in theme are saved as a new theme.
2. Or ask an AI agent: "Save skill…" writes the format description to disk, the agent writes a
   `.typhontheme` file, and you import it with "Import" or "Paste text".
3. An imported file is watched: edits to it show up in the launcher right away.

A theme id is lowercase Latin letters, digits and hyphens, up to 64 characters. If it equals a
built-in id or is already taken, Typhon appends a number.
