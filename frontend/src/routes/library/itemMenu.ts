import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';
import { openGameMenu } from '../../lib/stores/gameMenu';
import { contextMenuTrigger } from '../../lib/utils/contextmenu';

/** The item of `list` (one of its children) that holds `target`, or null. */
export function itemOf(list: HTMLElement, target: EventTarget | null | undefined): HTMLElement | null {
  let node = target instanceof Element ? target : null;
  while (node && node.parentElement !== list) node = node.parentElement;
  return node instanceof HTMLElement ? node : null;
}

/**
 * On the list of games: a right click on an item that has no game menu (a watched game, a download
 * not installed yet) does not open the browser's menu either. An item that has its own menu (a game
 * card) has answered the event before it gets here.
 */
export const noBrowserMenu: Attachment<HTMLElement> = (list) =>
  on(list, 'contextmenu', (event) => {
    if (!event.defaultPrevented && itemOf(list, event.target)) event.preventDefault();
  });

/**
 * On a list whose items carry no menu of their own (the list view's rows): a right click on an item
 * opens its game's menu at the pointer, Shift+F10 or the ContextMenu key next to the focused control;
 * the item is where focus goes back to when the menu closes. `idAt` names the game of the item at an
 * index.
 */
export function rowMenus(idAt: (index: number) => string | undefined): Attachment<HTMLElement> {
  return (list) => {
    let pointed: EventTarget | null = null;
    const offPointed = on(list, 'contextmenu', (event) => (pointed = event.target), { capture: true });
    const offTrigger = contextMenuTrigger(({ at, focus }) => {
      const from = 'anchor' in at ? (typeof at.anchor === 'function' ? at.anchor() : at.anchor) : pointed;
      const item = itemOf(list, from);
      const id = item ? idAt([...list.children].indexOf(item)) : undefined;
      return Boolean(item && id && openGameMenu(id, { at, focus, trigger: null, source: item }));
    })(list);
    return () => {
      offPointed();
      if (typeof offTrigger === 'function') offTrigger();
    };
  };
}
