import { derived } from 'svelte/store';
import {
  gameSections,
  searchPosition,
  sidebarEntries,
  sidebarSide,
  supportHidden,
  topbarActions,
  type GameSection,
  type SearchPosition,
  type SidebarEntry,
  type SidebarItem,
  type SidebarSide,
  type TopbarAction,
} from '../theme/layout';
import { activeTheme } from './theme';

export type NavBlock = { kind: 'group'; ids: SidebarItem[] } | { kind: 'separator'; key: string };

export function navBlocks(entries: SidebarEntry[], lanShown: boolean): NavBlock[] {
  const out: NavBlock[] = [];
  let pendingKey: string | null = null;
  for (const entry of entries) {
    if (entry.kind === 'separator') {
      if (out.length > 0) pendingKey = entry.key;
      continue;
    }
    if (entry.id === 'lan' && !lanShown) continue;
    if (pendingKey !== null) {
      out.push({ kind: 'separator', key: pendingKey });
      pendingKey = null;
    }
    const last = out.at(-1);
    if (last && last.kind === 'group') last.ids.push(entry.id);
    else out.push({ kind: 'group', ids: [entry.id] });
  }
  return out;
}

export interface ThemeLayoutView {
  side: SidebarSide;
  supportHidden: boolean;
  entries: SidebarEntry[];
  search: SearchPosition;
  actions: TopbarAction[];
  sections: { main: GameSection[]; side: GameSection[] };
}

export const themeLayout = derived(activeTheme, ($theme): ThemeLayoutView => {
  const layout = $theme?.layout;
  return {
    side: sidebarSide(layout),
    supportHidden: supportHidden(layout),
    entries: sidebarEntries(layout),
    search: searchPosition(layout),
    actions: topbarActions(layout),
    sections: gameSections(layout),
  };
});
