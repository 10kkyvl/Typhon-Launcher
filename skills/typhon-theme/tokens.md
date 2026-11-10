# Typhon theme tokens

Every token a `.typhontheme` file may set. *Default* is the built-in dark value a missing token falls back to. Color groups (Background … Status colors) must all be set; the rest are optional.

## Background

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--bg` | color | Window background: The main background behind everything in the window | `#0e0f11` |
| `--bg-sidebar` | color | Avatar ring: The ring around the status dot of the avatar on the sidebar island; set it to the colour `--island-fill` shows over `--bg` | `#0b0c0e` |
| `--island-fill` | color | Islands: fill: Fill of the floating panels of the interface; translucent, the window shows through it. Text on it has to stay readable over black and over white as well as over `--bg` | `rgba(11, 12, 14, 0.88)` |

## Surfaces and borders

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--surface` | color | Panels and fields: Background of nested panels, search fields and switches | `#16181b` |
| `--surface-2` | color | Cards and dialogs: Background of cards, dialogs and input fields | `#1e2024` |
| `--surface-3` | color | Buttons and tiles: Background of secondary buttons, tiles, menus and cover placeholders | `#25282c` |
| `--surface-4` | color | Pressed and floating elements: Buttons and tabs on hover, notifications and tooltips | `#2c2f34` |
| `--border` | color | Borders: Thin outlines of cards and lists | `rgba(255, 255, 255, 0.07)` |
| `--border-strong` | color | Borders: prominent: Outlines of buttons, menus and input fields | `rgba(255, 255, 255, 0.13)` |
| `--hover` | color | Hover highlight: Light highlight of list rows and sidebar items | `rgba(255, 255, 255, 0.045)` |
| `--hover-strong` | color | Hover highlight: strong: Highlight of menu items and transparent buttons | `rgba(255, 255, 255, 0.08)` |

## Text

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--text` | color | Main text: Headings and regular text | `#edeef0` |
| `--text-2` | color | Secondary text: Descriptions, captions and inactive items | `#b6b9bf` |
| `--text-3` | color | Muted text: Hints, explanations and placeholder text in fields | `#93979e` |

## Accent

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--accent` | color | Accent: buttons and highlights: Fill of main buttons and filled badges, always behind a label. The personal accent from settings is applied on top | `#0b7285` |
| `--accent-hover` | color | Accent on hover: Main button color while the pointer is over it | `#0d7a8f` |
| `--accent-subtle` | color | Accent: soft background: Background of selected filters, marked reactions and tags | `rgba(44, 207, 232, 0.16)` |
| `--accent-text` | color | Accent: text and links: Links and text on the soft accent background | `#8be9f7` |
| `--accent-ring` | color | Accent: outline and glow: Outline of a focused field and the glow of the main button | `rgba(44, 207, 232, 0.75)` |
| `--accent-bright` | color | Accent: indicators: Progress bars, switches, checkboxes, selection borders and markers: it has to stand out on every surface | `#2ccfe8` |

## Status colors

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--success` | color | Success: Color of successful operations and positive badges | `#55cf8d` |
| `--success-subtle` | color | Success: background: Background of success badges | `rgba(85, 207, 141, 0.14)` |
| `--warning` | color | Warning: Color of warnings and things that need attention | `#e0a85a` |
| `--warning-subtle` | color | Warning: background: Background of warning badges | `rgba(224, 168, 90, 0.14)` |
| `--danger` | color | Error: Color of errors and dangerous actions such as deleting | `#ef7070` |
| `--danger-subtle` | color | Error: background: Background of error badges | `rgba(239, 112, 112, 0.13)` |

## Sizes

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--radius-xs` | length | Corner radius: smallest: Small tags and compact elements | `0.4rem` |
| `--radius-sm` | length | Corner radius: small: Small buttons and auxiliary elements | `0.8rem` |
| `--radius-md` | length | Corner radius: medium: Buttons and input fields | `1.2rem` |
| `--radius-lg` | length | Corner radius: large: Cards | `1.8rem` |
| `--radius-xl` | length | Corner radius: largest: Dialogs, large banners and pill filters | `2.4rem` |
| `--radius-control` | length | Corner radius: controls: The search field and the button groups of the top bar; `999px` draws them as pills, a value like `0.8rem` brings square corners back | `999px` |
| `--cut` | length | Sidebar marker corner radius: Corners of the indicator bar next to the active sidebar item | `0.2rem` |
| `--space-1` | length | Spacing 1: The smallest gap between elements | `0.4rem` |
| `--space-2` | length | Spacing 2: A small gap between elements | `0.8rem` |
| `--space-3` | length | Spacing 3: The gap between close blocks | `1.2rem` |
| `--space-4` | length | Spacing 4: The usual gap between blocks | `1.6rem` |
| `--space-5` | length | Spacing 5: Inner padding of cards | `2rem` |
| `--space-6` | length | Spacing 6: A large gap between blocks | `2.4rem` |
| `--space-8` | length | Spacing 8: Padding of large blocks and empty states | `3.2rem` |
| `--space-10` | length | Spacing 10: Bottom padding of a page and the gap between sections | `4rem` |
| `--space-12` | length | Spacing 12: Side margins of a page in a very wide window | `4.8rem` |
| `--page-x` | length | Page side margins: Distance from the content to the left and right edges of the window | `3.2rem` |
| `--page-max` | length | Maximum page width: Content does not stretch wider than this | `192rem` |
| `--prose-max` | length | Text block width: Maximum width of long text such as descriptions and documents | `72rem` |
| `--sidebar-w` | length | Sidebar width: Width of the left panel with the sections | `23.2rem` |
| `--island-gap` | length | Islands: margin: Distance from the floating panels to the window edges | `0.8rem` |
| `--topbar-h` | length | Top bar height: Height of the strip with the section title | `6.4rem` |
| `--font-xs` | length | Text size: tiny: Captions and explanations | `1.2rem` |
| `--font-sm` | length | Text size: small: Secondary text and button labels | `1.3rem` |
| `--font-md` | length | Text size: regular: Main interface text | `1.4rem` |
| `--font-lg` | length | Text size: large: Titles of cards and dialogs | `1.6rem` |
| `--font-xl` | length | Text size: extra large: Titles of profile blocks, shelves and documents | `2rem` |
| `--font-title` | length | Page title size: Section titles and large numbers in statistics | `2.8rem` |
| `--font-hero` | length | Largest heading size: Not used anywhere in the interface right now | `clamp(2.4rem, 2.2vw, 3.6rem)` |
| `--tracking-title` | length | Letter spacing: titles: Letter density in page titles; a negative value tightens it | `-0.02em` |
| `--tracking-heading` | length | Letter spacing: headings: Letter density in card names and headings | `-0.01em` |
| `--control-sm` | length | Control height: small: Compact buttons, filters and tabs | `3.2rem` |
| `--control-md` | length | Control height: regular: Buttons and input fields | `3.6rem` |
| `--control-lg` | length | Control height: large: Large buttons such as "Play" | `4.4rem` |

## Font, animations and shadows

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--font-sans` | font | Interface font: A comma-separated list of fonts; the first installed one is used | `Onest Variable, Onest, Segoe UI, system-ui, sans-serif` |
| `--font-mono` | font | Monospace font: Paths, identifiers and other technical text; a comma-separated list, the first installed one is used | `JetBrains Mono Variable, JetBrains Mono, ui-monospace, SF Mono, Consolas, monospace` |
| `--ease` | ease | Animation easing: Curve of regular transitions: hover, panels appearing | `cubic-bezier(0.16, 1, 0.3, 1)` |
| `--ease-spring` | ease | Easing: springy: Curve of springy animations: avatars and buttons on game cards | `cubic-bezier(0.34, 1.36, 0.64, 1)` |
| `--dur` | time | Animation duration: regular: A typical transition on hover and selection | `150ms` |
| `--dur-fast` | time | Animation duration: fast: Small transitions of buttons and menus | `150ms` |
| `--dur-panel` | time | Animation duration: panels: Showing and hiding panels and lists | `200ms` |
| `--dur-slow` | time | Animation duration: slow: Slow effects: game covers, avatars appearing | `450ms` |
| `--shadow-pop` | shadow | Shadow: menus and tooltips: Shadow of drop-down menus, tooltips and notifications | `0 0.8rem 2.4rem rgba(0, 0, 0, 0.5), 0 1px 0 rgba(255, 255, 255, 0.04) inset` |
| `--shadow-modal` | shadow | Shadow: dialogs: Shadow of dialog windows and full-screen viewers | `0 2.4rem 6rem rgba(0, 0, 0, 0.6)` |
| `--shadow-lift` | shadow | Shadow: raised card: Shadow of a game cover on hover | `0 1.2rem 2.8rem -0.8rem rgba(0, 0, 0, 0.65)` |
