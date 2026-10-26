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
  at most 6 times. Items left out keep showing, appended in their default order:
  `library`, `catalog`, `installed`, `diskusage`, `-`, `downloads`, `sources`, `friends`,
  `activity`, `stats`, `history`, `-`, `lan`.
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

| Part | data-id |
|---|---|
| `app-shell` | no |
| `sidebar` | no |
| `sidebar-nav` | no |
| `sidebar-item` | yes |
| `sidebar-separator` | no |
| `sidebar-bottom` | no |
| `sidebar-account` | no |
| `topbar` | no |
| `topbar-history` | no |
| `topbar-search` | no |
| `topbar-actions` | no |
| `topbar-action` | yes |
| `window-controls` | no |
| `main` | no |
| `page` | no |
| `game-hero` | no |
| `game-actions` | no |
| `game-tabs` | no |
| `game-main` | no |
| `game-side` | no |
| `game-section` | yes |

Use `layout` to reorder or hide; use CSS only for looks. Hiding a part with CSS that `layout`
cannot hide (window controls, settings) leaves the user without a way back.
