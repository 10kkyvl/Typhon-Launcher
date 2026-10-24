# Typhon theme tokens

Every token a `.typhontheme` file may set. *Default* is the built-in dark value a missing token falls back to. Color groups (Background … Status colors) must all be set; the rest are optional.

## Background

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--bg` | color | Window background: The main background behind everything in the window | `#090d17` |
| `--bg-sidebar` | color | Sidebar background: The left panel with the launcher sections | `#090d17` |

## Surfaces and borders

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--surface` | color | Panels and fields: Background of nested panels, search fields and switches | `#0f1522` |
| `--surface-2` | color | Cards and dialogs: Background of cards, dialogs and input fields | `#131a29` |
| `--surface-3` | color | Buttons and tiles: Background of secondary buttons, tiles, menus and cover placeholders | `#1a2233` |
| `--surface-4` | color | Pressed and floating elements: Buttons and tabs on hover, notifications and tooltips | `#222b3f` |
| `--border` | color | Borders: Thin outlines of cards and lists | `rgba(255, 255, 255, 0.07)` |
| `--border-strong` | color | Borders: prominent: Outlines of buttons, menus and input fields | `rgba(255, 255, 255, 0.12)` |
| `--hover` | color | Hover highlight: Light highlight of list rows and sidebar items | `rgba(255, 255, 255, 0.045)` |
| `--hover-strong` | color | Hover highlight: strong: Highlight of menu items and transparent buttons | `rgba(255, 255, 255, 0.075)` |

## Text

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--text` | color | Main text: Headings and regular text | `#eef2f6` |
| `--text-2` | color | Secondary text: Descriptions, captions and inactive items | `#9aa5b8` |
| `--text-3` | color | Muted text: Hints, explanations and placeholder text in fields | `#647089` |

## Accent

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--accent` | color | Accent: buttons and highlights: Main buttons, selected items and indicators. The personal accent from settings is applied on top | `#4f6bff` |
| `--accent-hover` | color | Accent on hover: Main button color while the pointer is over it | `#6480ff` |
| `--accent-subtle` | color | Accent: soft background: Background of selected filters, marked reactions and tags | `rgba(79, 107, 255, 0.16)` |
| `--accent-text` | color | Accent: text and links: Links and text on the soft accent background | `#93a4ff` |
| `--accent-ring` | color | Accent: outline and glow: Outline of a focused field and the glow of the main button | `rgba(79, 107, 255, 0.5)` |

## Status colors

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--success` | color | Success: Color of successful operations and positive badges | `#65b883` |
| `--success-subtle` | color | Success: background: Background of success badges | `rgba(101, 184, 131, 0.13)` |
| `--warning` | color | Warning: Color of warnings and things that need attention | `#d8a45d` |
| `--warning-subtle` | color | Warning: background: Background of warning badges | `rgba(216, 164, 93, 0.13)` |
| `--danger` | color | Error: Color of errors and dangerous actions such as deleting | `#d96969` |
| `--danger-subtle` | color | Error: background: Background of error badges | `rgba(217, 105, 105, 0.12)` |

## Sizes

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--radius-xs` | length | Corner radius: smallest: Small tags and compact elements | `0.4rem` |
| `--radius-sm` | length | Corner radius: small: Small buttons and auxiliary elements | `0.6rem` |
| `--radius-md` | length | Corner radius: medium: Buttons and input fields | `0.8rem` |
| `--radius-lg` | length | Corner radius: large: Cards | `1.2rem` |
| `--radius-xl` | length | Corner radius: largest: Dialogs, large banners and pill filters | `1.6rem` |
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
| `--page-x` | length | Page side margins: Distance from the content to the left and right edges of the window | `3.6rem` |
| `--page-max` | length | Maximum page width: Content does not stretch wider than this | `232rem` |
| `--prose-max` | length | Text block width: Maximum width of long text such as descriptions and documents | `76rem` |
| `--sidebar-w` | length | Sidebar width: Width of the left panel with the sections | `23.2rem` |
| `--topbar-h` | length | Top bar height: Height of the strip with the section title | `5.6rem` |
| `--font-xs` | length | Text size: tiny: Captions and explanations | `1.3rem` |
| `--font-sm` | length | Text size: small: Secondary text and button labels | `1.4rem` |
| `--font-md` | length | Text size: regular: Main interface text | `1.5rem` |
| `--font-lg` | length | Text size: large: Titles of cards and dialogs | `1.7rem` |
| `--font-xl` | length | Text size: extra large: Titles of profile blocks, shelves and documents | `1.9rem` |
| `--font-title` | length | Page title size: Section titles and large numbers in statistics | `3.2rem` |
| `--font-hero` | length | Largest heading size: Not used anywhere in the interface right now | `clamp(3.6rem, 2.6vw, 4.4rem)` |
| `--tracking-title` | length | Letter spacing: titles: Letter density in page titles; a negative value tightens it | `-0.024em` |
| `--tracking-heading` | length | Letter spacing: headings: Letter density in card names and headings | `-0.012em` |
| `--control-sm` | length | Control height: small: Compact buttons, filters and tabs | `3.2rem` |
| `--control-md` | length | Control height: regular: Buttons and input fields | `3.8rem` |
| `--control-lg` | length | Control height: large: Large buttons such as "Play" | `4.4rem` |

## Font, animations and shadows

| Token | Kind | What it paints | Default |
|---|---|---|---|
| `--font-sans` | font | Interface font: A comma-separated list of fonts; the first installed one is used | `Inter Tight Variable, Inter, Segoe UI, system-ui, sans-serif` |
| `--ease` | ease | Animation easing: Curve of regular transitions: hover, panels appearing | `cubic-bezier(0.2, 0.8, 0.2, 1)` |
| `--ease-spring` | ease | Easing: springy: Curve of springy animations: avatars and buttons on game cards | `cubic-bezier(0.34, 1.4, 0.64, 1)` |
| `--dur` | time | Animation duration: regular: A typical transition on hover and selection | `160ms` |
| `--dur-fast` | time | Animation duration: fast: Small transitions of buttons and menus | `120ms` |
| `--dur-panel` | time | Animation duration: panels: Showing and hiding panels and lists | `180ms` |
| `--dur-slow` | time | Animation duration: slow: Slow effects: game covers, avatars appearing | `320ms` |
| `--shadow-pop` | shadow | Shadow: menus and tooltips: Shadow of drop-down menus, tooltips and notifications | `0 0.8rem 2.8rem rgba(0, 0, 0, 0.45), 0 2px 0.8rem rgba(0, 0, 0, 0.3)` |
| `--shadow-modal` | shadow | Shadow: dialogs: Shadow of dialog windows and full-screen viewers | `0 2rem 6rem rgba(0, 0, 0, 0.55)` |
| `--shadow-lift` | shadow | Shadow: raised card: Shadow of a game cover on hover | `0 1.2rem 2.8rem -0.8rem rgba(0, 0, 0, 0.6)` |
