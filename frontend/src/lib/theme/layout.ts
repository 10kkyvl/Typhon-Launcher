export const SEPARATOR = '-';
export const MAX_SEPARATORS = 6;

export const SIDEBAR_SIDES = ['left', 'right'] as const;
export const SIDEBAR_ITEMS = [
  'library',
  'catalog',
  'installed',
  'diskusage',
  'downloads',
  'sources',
  'friends',
  'activity',
  'stats',
  'history',
  'lan',
] as const;
export const SIDEBAR_DEFAULT_ORDER: readonly string[] = [
  'library',
  'catalog',
  'installed',
  'diskusage',
  SEPARATOR,
  'downloads',
  'sources',
  'friends',
  'activity',
  'stats',
  'history',
  SEPARATOR,
  'lan',
];
export const SIDEBAR_HIDEABLE: readonly string[] = [...SIDEBAR_ITEMS, 'support'];

export const SEARCH_POSITIONS = ['left', 'center'] as const;
export const TOPBAR_ACTIONS = ['bigpicture', 'notifications', 'friends'] as const;

export const GAME_MAIN_SECTIONS = [
  'summary',
  'tags',
  'links',
  'update',
  'install',
  'requirements',
  'releases',
  'similar',
  'otherVersions',
  'notes',
  'steamExtras',
  'reviews',
] as const;
export const GAME_SIDE_SECTIONS = ['about', 'achievements', 'reviewSummary', 'friends'] as const;

export const LAYOUT_PARTS = [
  'app-shell',
  'sidebar',
  'sidebar-nav',
  'sidebar-item',
  'sidebar-separator',
  'sidebar-bottom',
  'sidebar-account',
  'topbar',
  'topbar-history',
  'topbar-search',
  'topbar-actions',
  'topbar-action',
  'window-controls',
  'main',
  'page',
  'game-hero',
  'game-actions',
  'game-tabs',
  'game-main',
  'game-side',
  'game-section',
] as const;
export const PARTS_WITH_ID = ['sidebar-item', 'topbar-action', 'game-section'] as const;

export type SidebarSide = (typeof SIDEBAR_SIDES)[number];
export type SidebarItem = (typeof SIDEBAR_ITEMS)[number];
export type SearchPosition = (typeof SEARCH_POSITIONS)[number];
export type TopbarAction = (typeof TOPBAR_ACTIONS)[number];
export type GameSection = (typeof GAME_MAIN_SECTIONS)[number] | (typeof GAME_SIDE_SECTIONS)[number];

type Ids = string[] | null;

export interface ThemeLayout {
  sidebar?: { side?: string; order?: Ids; hidden?: Ids } | null;
  topbar?: { search?: string; order?: Ids; hidden?: Ids } | null;
  gamePage?: { main?: Ids; side?: Ids; hidden?: Ids } | null;
}

export type SidebarEntry = { kind: 'item'; id: SidebarItem } | { kind: 'separator'; key: string };

const isSidebarItem = (id: string): id is SidebarItem => (SIDEBAR_ITEMS as readonly string[]).includes(id);
const isTopbarAction = (id: string): id is TopbarAction => (TOPBAR_ACTIONS as readonly string[]).includes(id);
const isMainSection = (id: string): boolean => (GAME_MAIN_SECTIONS as readonly string[]).includes(id);
const isSideSection = (id: string): boolean => (GAME_SIDE_SECTIONS as readonly string[]).includes(id);

export function sidebarSide(layout: ThemeLayout | null | undefined): SidebarSide {
  return layout?.sidebar?.side === 'right' ? 'right' : 'left';
}

export function supportHidden(layout: ThemeLayout | null | undefined): boolean {
  return (layout?.sidebar?.hidden ?? []).includes('support');
}

// Items a theme leaves out of its order keep working: they are appended in
// their default order, so a theme written before a new item existed still
// shows it.
export function sidebarEntries(layout: ThemeLayout | null | undefined): SidebarEntry[] {
  const custom = layout?.sidebar?.order;
  const source = custom && custom.length > 0 ? custom : SIDEBAR_DEFAULT_ORDER;
  const hidden = new Set(layout?.sidebar?.hidden ?? []);
  const seen = new Set<string>();
  const raw: (SidebarItem | typeof SEPARATOR)[] = [];
  for (const id of source) {
    if (id === SEPARATOR) {
      raw.push(SEPARATOR);
    } else if (isSidebarItem(id) && !seen.has(id)) {
      seen.add(id);
      raw.push(id);
    }
  }
  for (const id of SIDEBAR_ITEMS) {
    if (!seen.has(id)) raw.push(id);
  }
  const out: SidebarEntry[] = [];
  let separators = 0;
  for (const id of raw) {
    if (id === SEPARATOR) {
      const last = out.at(-1);
      if (last && last.kind === 'item' && separators < MAX_SEPARATORS) {
        separators += 1;
        out.push({ kind: 'separator', key: `sep-${separators}` });
      }
    } else if (!hidden.has(id)) {
      out.push({ kind: 'item', id });
    }
  }
  while (out.at(-1)?.kind === 'separator') out.pop();
  return out;
}

export function searchPosition(layout: ThemeLayout | null | undefined): SearchPosition {
  return layout?.topbar?.search === 'center' ? 'center' : 'left';
}

export function topbarActions(layout: ThemeLayout | null | undefined): TopbarAction[] {
  const hidden = new Set(layout?.topbar?.hidden ?? []);
  const seen = new Set<string>();
  const out: TopbarAction[] = [];
  for (const id of [...(layout?.topbar?.order ?? []), ...TOPBAR_ACTIONS]) {
    if (!isTopbarAction(id) || seen.has(id)) continue;
    seen.add(id);
    if (!hidden.has(id)) out.push(id);
  }
  return out;
}

// A section may be moved to the other column; one the theme does not place
// stays in its default column after the placed ones. A section in `keep` is
// shown even when the theme hides it: the page's main button acts on it.
export function gameSections(
  layout: ThemeLayout | null | undefined,
  keep: readonly string[] = [],
): { main: GameSection[]; side: GameSection[] } {
  const hidden = new Set((layout?.gamePage?.hidden ?? []).filter((id) => !keep.includes(id)));
  const seen = new Set<string>();
  const known = (id: string): id is GameSection => isMainSection(id) || isSideSection(id);
  const place = (ids: readonly string[] | null | undefined): GameSection[] => {
    const out: GameSection[] = [];
    for (const id of ids ?? []) {
      if (!known(id) || seen.has(id)) continue;
      seen.add(id);
      out.push(id);
    }
    return out;
  };
  const main = place(layout?.gamePage?.main);
  const side = place(layout?.gamePage?.side);
  main.push(...place(GAME_MAIN_SECTIONS));
  side.push(...place(GAME_SIDE_SECTIONS));
  return { main: main.filter((id) => !hidden.has(id)), side: side.filter((id) => !hidden.has(id)) };
}
