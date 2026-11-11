import type { Attachment } from 'svelte/attachments';
import type { MenuFocus } from '../components/menu';

/** Where a context menu opens: at the pointer, or next to an element (the focused control, a menu button). */
export type ContextPlace = { x: number; y: number } | { anchor: HTMLElement | (() => HTMLElement | null | undefined) };

/** A context menu asked for: by pointer at its position, focus on the menu; from the keyboard next to the focused control, focus on the first item. */
export interface ContextRequest {
  at: ContextPlace;
  focus: MenuFocus;
}

// A keyboard request the browser may follow with a contextmenu event of its own (the ContextMenu key,
// Shift+F10 on Windows): that event is the same request, already answered.
const ECHO_MS = 500;

function isMenuKey(event: KeyboardEvent): boolean {
  if (event.isComposing || event.ctrlKey || event.metaKey || event.altKey) return false;
  return event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey);
}

function focusedIn(node: HTMLElement): HTMLElement {
  const active = document.activeElement;
  return active instanceof HTMLElement && node.contains(active) ? active : node;
}

/**
 * `{@attach contextMenuTrigger(open)}` on what has a context menu (a game's card or row). A right-click
 * on it asks for the menu at the pointer; Shift+F10 or the ContextMenu key with focus inside it asks
 * for the menu next to the focused control, which is where focus returns when the menu closes. A
 * contextmenu event without a pointer position (sent for the keyboard, at 0,0) is a keyboard request
 * too. `open` gets the request and this element and returns false when there is no menu for it: the
 * browser's own menu and the key are then left alone. A held key asks once.
 */
export function contextMenuTrigger(open: (request: ContextRequest, source: HTMLElement) => boolean): Attachment<HTMLElement> {
  return (node) => {
    let keyedAt = -Infinity;

    function onKeydown(event: KeyboardEvent) {
      if (!isMenuKey(event) || event.repeat) return;
      const anchor = event.target instanceof HTMLElement ? event.target : focusedIn(node);
      if (!open({ at: { anchor }, focus: 'first' }, node)) return;
      event.preventDefault();
      keyedAt = Date.now();
    }

    function onContextmenu(event: MouseEvent) {
      if (Date.now() - keyedAt < ECHO_MS) {
        event.preventDefault();
        return;
      }
      const keyboard = event.button !== 2 && event.clientX === 0 && event.clientY === 0;
      const request: ContextRequest = keyboard
        ? { at: { anchor: focusedIn(node) }, focus: 'first' }
        : { at: { x: event.clientX, y: event.clientY }, focus: 'menu' };
      if (!open(request, node)) return;
      event.preventDefault();
      event.stopPropagation();
      // The menu comes from this element and focus returns here when it closes, not to a control
      // elsewhere that held focus before the right-click (WebKit moves no focus on one).
      const active = document.activeElement;
      if (!keyboard && active instanceof HTMLElement && active !== document.body && !node.contains(active)) active.blur();
    }

    node.addEventListener('keydown', onKeydown);
    node.addEventListener('contextmenu', onContextmenu);
    return () => {
      node.removeEventListener('keydown', onKeydown);
      node.removeEventListener('contextmenu', onContextmenu);
    };
  };
}
