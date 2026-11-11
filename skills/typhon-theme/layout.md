# Typhon theme layout

Every id the optional `theme.layout` object may use, and the `data-part` hooks custom CSS can
target. The launcher refuses the whole file on an unknown id, an id used twice, or a wrong
value, so copy ids exactly (case matters).

```json
"layout": {
  "sidebar":  { "side": "right", "order": ["library", "-", "downloads"], "hidden": ["stats"] },
  "topbar":   { "search": "center", "order": ["friends", "notifications"], "hidden": ["bigpicture"] },
  "gamePage": { "main": ["summary", "notes"], "side": ["achievements", "about"], "hidden": ["reviews"] }
}
```

Every block and every field is optional; leave out what the user did not ask to change. A
layout with nothing in it is dropped, so a theme without layout wishes has no `layout` key.

## Sidebar

- `side`: `left` (default) or `right`.
- `order`: sidebar items top to bottom, each at most once, plus the separator `-` (a thin line)
  at most 6 times. Without `order` the sidebar shows the default order `library`, `catalog`,
  `downloads`, `friends`, `activity`, `stats`, `-`, `installed`, `diskusage`, `lan`, `sources`,
  `history`. Items an `order` leaves out keep showing, appended after it in the order of the
  table below.
- `hidden`: items to hide, each at most once. Settings can never be hidden and has no id.

| Id | What it is | In `order` | In `hidden` |
|---|---|---|---|
| `library` | Library | yes | yes |
| `catalog` | All games | yes | yes |
| `installed` | Installed | yes | yes |
| `diskusage` | Disk usage | yes | yes |
| `downloads` | Downloads | yes | yes |
| `sources` | Sources | yes | yes |
| `friends` | Friends | yes | yes |
| `activity` | Activity | yes | yes |
| `stats` | Statistics | yes | yes |
| `history` | History | yes | yes |
| `lan` | Local network | yes | yes |
| `support` | Support, at the bottom | no | yes |

## Top bar

- `search`: `left` (default) or `center`.
- `order`: action buttons left to right, each at most once; buttons left out are appended in the
  default order `bigpicture`, `notifications`, `friends`.
- `hidden`: buttons to hide, each at most once.

| Id | What it is |
|---|---|
| `bigpicture` | Big Picture mode button |
| `notifications` | Notifications |
| `friends` | Friends list |

## Game page

- `main`: sections of the wide column top to bottom; `side`: sections of the narrow column.
  Any section may go to either column, but each id appears at most once across both lists.
  Sections left out stay in their default column after the placed ones.
- `hidden`: sections to hide, each at most once.

| Id | What it is | Default column |
|---|---|---|
| `summary` | Description | main |
| `tags` | Tags | main |
| `links` | Links | main |
| `update` | Update | main |
| `install` | Installation | main |
| `requirements` | System requirements | main |
| `releases` | Releases | main |
| `similar` | Similar releases | main |
| `otherVersions` | Other versions | main |
| `notes` | Notes | main |
| `steamExtras` | Steam extras | main |
| `reviews` | Reviews | main |
| `about` | About the game | side |
| `achievements` | Achievements | side |
| `reviewSummary` | Rating | side |
| `friends` | Friends who play it | side |

## Parts for custom CSS

Elements of the window carry `data-part="<name>"`. The parts marked *data-id* also carry
`data-id="<id>"` with the ids from the tables above (sidebar item, top bar button, game
section). Select them with attribute selectors in `theme.css`:

```css
[data-part="sidebar-item"][data-id="library"] { font-weight: 600; }
[data-part="game-section"][data-id="notes"] { border: 1px solid var(--accent-ring); }
```

The account chip (avatar, name, the account menu) sits in the top bar since theme contract 2:
its part is `topbar-account`. In a theme with `"contract": 2` write `topbar-account`; the 1.x
name `sidebar-account` matches nothing there. Typhon renames it by itself only in themes without
`contract` (1.x themes), inside `[data-part=…]` selectors whose value is the whole old name.

| Part | data-id |
|---|---|
| `app-shell` | no |
| `sidebar` | no |
| `sidebar-nav` | no |
| `sidebar-item` | yes |
| `sidebar-separator` | no |
| `sidebar-bottom` | no |
| `topbar` | no |
| `topbar-history` | no |
| `topbar-search` | no |
| `topbar-actions` | no |
| `topbar-action` | yes |
| `topbar-account` | no |
| `window-controls` | no |
| `main` | no |
| `page` | no |
| `game-hero` | no |
| `game-actions` | no |
| `game-tabs` | no |
| `game-main` | no |
| `game-side` | no |
| `game-section` | yes |
| `dialog` | no |
| `menu` | no |

Dialogs and menus are drawn above the window, outside the part that opened them: select their
panel as `[data-part="dialog"]` or `[data-part="menu"]`, not as a descendant of the page or the
button they came from. A menu marks the side it opened on in `data-placement` (`bottom-start`,
`top-end` and so on).

Use `layout` to reorder or hide; use CSS only for looks. Hiding a part with CSS that `layout`
cannot hide (window controls, settings) leaves the user without a way back.
