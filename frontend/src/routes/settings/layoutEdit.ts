import {
  GAME_MAIN_SECTIONS,
  GAME_SIDE_SECTIONS,
  MAX_SEPARATORS,
  SEPARATOR,
  SIDEBAR_DEFAULT_ORDER,
  SIDEBAR_HIDEABLE,
  TOPBAR_ACTIONS,
  gameSections,
  searchPosition,
  sidebarEntries,
  sidebarSide,
  topbarActions,
  type SearchPosition,
  type SidebarSide,
  type ThemeLayout,
} from '../../lib/theme/layout';

export type ListKey = 'sidebar' | 'topbar' | 'main' | 'side';

export interface EditState {
  sidebarSide: SidebarSide;
  sidebar: string[];
  sidebarHidden: string[];
  search: SearchPosition;
  topbar: string[];
  topbarHidden: string[];
  main: string[];
  side: string[];
  gameHidden: string[];
}

const same = (a: readonly string[], b: readonly string[]) => a.length === b.length && a.every((id, i) => id === b[i]);

export function toState(layout: ThemeLayout | null | undefined): EditState {
  const sidebar = sidebarEntries({ sidebar: { order: layout?.sidebar?.order } }).map((e) =>
    e.kind === 'item' ? e.id : SEPARATOR,
  );
  const game = gameSections({ gamePage: { main: layout?.gamePage?.main, side: layout?.gamePage?.side } });
  const gameIds = new Set<string>([...game.main, ...game.side]);
  return {
    sidebarSide: sidebarSide(layout),
    sidebar,
    sidebarHidden: (layout?.sidebar?.hidden ?? []).filter((id) => SIDEBAR_HIDEABLE.includes(id)),
    search: searchPosition(layout),
    topbar: topbarActions({ topbar: { order: layout?.topbar?.order } }),
    topbarHidden: (layout?.topbar?.hidden ?? []).filter((id) => (TOPBAR_ACTIONS as readonly string[]).includes(id)),
    main: game.main,
    side: game.side,
    gameHidden: (layout?.gamePage?.hidden ?? []).filter((id) => gameIds.has(id)),
  };
}

export function fromState(state: EditState): ThemeLayout | undefined {
  const out: ThemeLayout = {};

  const sidebar: NonNullable<ThemeLayout['sidebar']> = {};
  if (state.sidebarSide !== 'left') sidebar.side = state.sidebarSide;
  if (!same(state.sidebar, SIDEBAR_DEFAULT_ORDER)) sidebar.order = [...state.sidebar];
  if (state.sidebarHidden.length > 0) sidebar.hidden = [...state.sidebarHidden];
  if (Object.keys(sidebar).length > 0) out.sidebar = sidebar;

  const topbar: NonNullable<ThemeLayout['topbar']> = {};
  if (state.search !== 'left') topbar.search = state.search;
  if (!same(state.topbar, TOPBAR_ACTIONS)) topbar.order = [...state.topbar];
  if (state.topbarHidden.length > 0) topbar.hidden = [...state.topbarHidden];
  if (Object.keys(topbar).length > 0) out.topbar = topbar;

  const gamePage: NonNullable<ThemeLayout['gamePage']> = {};
  if (!same(state.main, GAME_MAIN_SECTIONS) || !same(state.side, GAME_SIDE_SECTIONS)) {
    gamePage.main = [...state.main];
    gamePage.side = [...state.side];
  }
  if (state.gameHidden.length > 0) gamePage.hidden = [...state.gameHidden];
  if (Object.keys(gamePage).length > 0) out.gamePage = gamePage;

  return Object.keys(out).length > 0 ? out : undefined;
}

export function listOf(state: EditState, key: ListKey): string[] {
  return state[key];
}

function withList(state: EditState, key: ListKey, list: string[]): EditState {
  return { ...state, [key]: list };
}

export function moveIndex(state: EditState, key: ListKey, from: number, to: number): EditState {
  const list = [...state[key]];
  if (from < 0 || from >= list.length || to < 0 || to >= list.length || from === to) return state;
  const [item] = list.splice(from, 1);
  list.splice(to, 0, item);
  return withList(state, key, list);
}

export function step(state: EditState, key: ListKey, index: number, delta: -1 | 1): EditState {
  return moveIndex(state, key, index, index + delta);
}

export function separatorCount(state: EditState): number {
  return state.sidebar.filter((id) => id === SEPARATOR).length;
}

export function canAddSeparator(state: EditState): boolean {
  return separatorCount(state) < MAX_SEPARATORS;
}

export function addSeparator(state: EditState): EditState {
  if (!canAddSeparator(state)) return state;
  const list = [...state.sidebar];
  list.splice(Math.max(list.length - 1, 0), 0, SEPARATOR);
  return withList(state, 'sidebar', list);
}

export function removeSeparator(state: EditState, index: number): EditState {
  if (state.sidebar[index] !== SEPARATOR) return state;
  const list = [...state.sidebar];
  list.splice(index, 1);
  return withList(state, 'sidebar', list);
}

export function toggleHidden(state: EditState, scope: 'sidebar' | 'topbar' | 'game', id: string): EditState {
  const field = scope === 'sidebar' ? 'sidebarHidden' : scope === 'topbar' ? 'topbarHidden' : 'gameHidden';
  const current = state[field];
  const next = current.includes(id) ? current.filter((x) => x !== id) : [...current, id];
  return { ...state, [field]: next };
}

export function isHidden(state: EditState, scope: 'sidebar' | 'topbar' | 'game', id: string): boolean {
  const field = scope === 'sidebar' ? 'sidebarHidden' : scope === 'topbar' ? 'topbarHidden' : 'gameHidden';
  return state[field].includes(id);
}

export function moveToColumn(
  state: EditState,
  from: 'main' | 'side',
  index: number,
  to: 'main' | 'side',
  at?: number,
): EditState {
  if (from === to) return at === undefined ? state : moveIndex(state, from, index, Math.min(at, state[from].length - 1));
  const source = [...state[from]];
  if (index < 0 || index >= source.length) return state;
  const [item] = source.splice(index, 1);
  const target = [...state[to]];
  target.splice(at === undefined ? target.length : Math.min(Math.max(at, 0), target.length), 0, item);
  return { ...state, [from]: source, [to]: target };
}

export function dropOn(
  state: EditState,
  from: { key: ListKey; index: number },
  to: { key: ListKey; index?: number },
): EditState {
  if (from.key === to.key) {
    const last = state[to.key].length - 1;
    return moveIndex(state, to.key, from.index, to.index === undefined ? last : to.index);
  }
  const isGame = (k: ListKey): k is 'main' | 'side' => k === 'main' || k === 'side';
  if (isGame(from.key) && isGame(to.key)) return moveToColumn(state, from.key, from.index, to.key, to.index);
  return state;
}
