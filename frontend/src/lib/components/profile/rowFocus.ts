import { rowFocus } from '../../utils/rowFocus';

/**
 * The profile blocks' rules for focus when a row leaves while focus is in it, or moves under it (a
 * reordered row is taken out of the page and put back, and Chromium drops the focus it held): the
 * shared row rule, the same control of the row now in its place, else of the row above.
 */

/** The attribute every row's controls carry with their kind. */
export const CONTROL_ATTR = 'data-row-control';

/** The attribute of each game of a collection's list in the block settings, with its catalog id. */
export const GAME_ATTR = 'data-collection-game';
/** The attribute of the place a game is added from, under the list. */
export const ADD_GAME_ATTR = 'data-collection-add';

function controlIn(row: HTMLElement, kind: string | null): HTMLElement | null {
  const marked = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`));
  return marked.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? row.querySelector<HTMLElement>('button, input');
}

/** With no game left in the list, the search for a game to add. */
export const gameRows = rowFocus({
  rowAttr: GAME_ATTR,
  controlAttr: CONTROL_ATTR,
  controlIn,
  fallback: () => document.querySelector<HTMLElement>(`[${ADD_GAME_ATTR}] input`),
});
