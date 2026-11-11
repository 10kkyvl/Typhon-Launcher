import { flushSync } from 'svelte';
import type { Attachment } from 'svelte/attachments';
import { get, writable } from 'svelte/store';
import { menuButtonHandlers, type MenuFocus, type MenuTriggerProps } from '../components/menu';
import { contextMenuTrigger, type ContextPlace } from '../utils/contextmenu';
import { libraryGames } from './library';

/** The game menu is one menu in the window; its id is what a menu button's `aria-controls` names. */
export const GAME_MENU_ID = 'game-menu';

/** The game menu while it is open: which game, where it opens and how. */
export interface GameMenuState {
  gameId: string;
  /** At the pointer (a right-click), or next to an element (the focused control, a menu button). */
  at: ContextPlace;
  /** Focus as it opens: the menu itself (by pointer), its first or last item (from the keyboard). */
  focus: MenuFocus;
  /** Id of the menu button it was opened from (a Catalog card's "…"), else null. */
  trigger: string | null;
  /**
   * The game's element it was opened on (its card or row, the menu button): where focus goes after a
   * dialog from the menu, or near it when the game is gone.
   */
  source: HTMLElement | null;
}

export const gameMenu = writable<GameMenuState | null>(null);

/** Opens the menu of a game in the library; false (nothing opens) for any other id. Each call is a new opening. */
export function openGameMenu(gameId: string, opening: Omit<GameMenuState, 'gameId'>): boolean {
  if (!get(libraryGames).some((game) => game.id === gameId)) return false;
  gameMenu.set({ gameId, ...opening });
  return true;
}

export function closeGameMenu() {
  gameMenu.set(null);
}

/**
 * `{@attach gameContextMenu(id)}` on a game's card or row: a right-click opens the game menu at the
 * pointer, Shift+F10 or the ContextMenu key next to the focused control. A game the library does not
 * have leaves the browser alone.
 */
export function gameContextMenu(gameId: string): Attachment<HTMLElement> {
  return contextMenuTrigger(({ at, focus }, source) => openGameMenu(gameId, { at, focus, trigger: null, source }));
}

/**
 * Props of a menu button that opens the game menu next to itself (a Catalog card's "…"): spread them
 * on the button. `id` must be unique in the page; `state` is the current `$gameMenu`, so the button
 * shows whether its menu is open.
 */
export function gameMenuButton(gameId: string, id: string, state: GameMenuState | null): MenuTriggerProps {
  const open = state?.trigger === id;
  const button = () => document.getElementById(id);
  return {
    id,
    'aria-haspopup': 'menu',
    'aria-expanded': open,
    'aria-controls': open ? GAME_MENU_ID : undefined,
    ...menuButtonHandlers({
      open: () => get(gameMenu)?.trigger === id,
      show: (focus) => {
        openGameMenu(gameId, { at: { anchor: button }, focus, trigger: id, source: button() });
      },
      // Only from the button's handlers: unmounted now, focus is back on the button when this returns.
      close: () => {
        if (get(gameMenu) === null) return;
        closeGameMenu();
        flushSync();
      },
    }),
  };
}
