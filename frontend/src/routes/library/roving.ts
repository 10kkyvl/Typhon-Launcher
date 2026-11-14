import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';
import { focusQuietly } from '../../lib/layers/focus';
import { itemOf } from './itemMenu';

const CONTROLS = 'a[href], button, input, select, textarea, [tabindex]';
// Set on a control this list took out of the Tab order, so it gives back only what it took.
const OFF = 'data-roving-off';

export interface RovingOptions {
  /** `grid`: the arrows move by the rows on screen; `list`: up and down. */
  layout: 'grid' | 'list';
  /** The id of the game at an index of the list. */
  idAt: (index: number) => string | undefined;
  /** The game that holds the list's Tab stop; the first one when it is not in the list. */
  current: () => string | undefined;
  /** The player moved to a game (arrows, focus, a press). */
  onmove: (id: string) => void;
  /**
   * Ctrl+Enter (Cmd+Enter on a Mac) on any control of an item: launch its game, as Play does. For
   * items that have no such key of their own (a card has).
   */
  onlaunch?: (id: string) => void;
}

function items(list: HTMLElement): HTMLElement[] {
  return [...list.children] as HTMLElement[];
}

/** An item's own controls in DOM order, those this list took out of the Tab order included. */
export function controlsOf(item: HTMLElement): HTMLElement[] {
  return [...item.querySelectorAll<HTMLElement>(CONTROLS)].filter((el) => el.hasAttribute(OFF) || el.tabIndex >= 0);
}

/**
 * Focuses `target` and brings its item into view by the least movement: the item scrolls only as far
 * as it has to, whole cards included. The scroller (the shell's `main`) sits below the top bar, so
 * nothing covers its top edge.
 */
export function revealFocus(target: HTMLElement, item: Element | null = null): void {
  focusQuietly(target);
  const box = item ?? target;
  if (typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}

/** The control an item is entered on: its first control (the title). */
export function primaryOf(item: HTMLElement): HTMLElement | undefined {
  return controlsOf(item)[0];
}

/** The index of the item one row below (`down`) or above, nearest to it across: by where the items are on screen. */
function rowNeighbour(all: HTMLElement[], from: number, down: boolean): number {
  const here = all[from].getBoundingClientRect();
  const middle = here.left + here.width / 2;
  const rects = all.map((item) => item.getBoundingClientRect());
  const tops = rects.map((rect) => rect.top).filter((top) => (down ? top > here.top + 1 : top < here.top - 1));
  if (tops.length === 0) return from;
  const row = down ? Math.min(...tops) : Math.max(...tops);
  let best = from;
  let distance = Infinity;
  rects.forEach((rect, index) => {
    if (Math.abs(rect.top - row) > 1) return;
    const gap = Math.abs(rect.left + rect.width / 2 - middle);
    if (gap < distance) {
      best = index;
      distance = gap;
    }
  });
  return best;
}

/**
 * A list of games as one Tab stop (WAI-ARIA roving tabindex over the items, not over single
 * controls): only the current item's own controls are in the Tab order, Tab goes through them and
 * then out of the list. The arrows move between items (by the rows on screen in a grid), Home and
 * End to the first and the last; the item reached is entered on its first control. Each item can
 * take focus from script (`tabindex="-1"`), so what hands focus to an item (the game menu after a
 * pick took a game away) lands on its first control.
 *
 * `refresh` sets the Tab order again after the list or the current item changed; added or replaced
 * controls are caught as they come.
 */
export function createRoving(options: RovingOptions): { attach: Attachment<HTMLElement>; refresh: () => void } {
  let list: HTMLElement | undefined;

  function stopIndex(all: HTMLElement[]): number {
    const current = options.current();
    const found = current === undefined ? -1 : all.findIndex((_, index) => options.idAt(index) === current);
    return found >= 0 ? found : 0;
  }

  function setItem(item: HTMLElement, stop: boolean) {
    if (item.getAttribute('tabindex') !== '-1') item.setAttribute('tabindex', '-1');
    for (const control of item.querySelectorAll<HTMLElement>(CONTROLS)) {
      if (stop) {
        if (control.hasAttribute(OFF)) {
          control.removeAttribute(OFF);
          control.removeAttribute('tabindex');
        }
      } else if (!control.hasAttribute(OFF) && control.tabIndex >= 0) {
        control.setAttribute(OFF, '');
        control.setAttribute('tabindex', '-1');
      }
    }
  }

  function refresh(only?: ReadonlySet<HTMLElement>) {
    if (!list) return;
    const all = items(list);
    const stop = stopIndex(all);
    all.forEach((item, index) => {
      if (!only || only.has(item)) setItem(item, index === stop);
    });
  }

  function moveTo(index: number) {
    if (!list) return;
    const item = items(list)[index];
    const id = options.idAt(index);
    if (!item || id === undefined) return;
    options.onmove(id);
    refresh();
    const target = primaryOf(item);
    if (target) revealFocus(target, item);
  }

  function onkeydown(event: KeyboardEvent) {
    if (!list || event.defaultPrevented) return;
    const item = itemOf(list, event.target);
    if (!item) return;
    const all = items(list);
    const from = all.indexOf(item);
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.altKey && !event.shiftKey && options.onlaunch) {
      event.preventDefault();
      const id = options.idAt(from);
      if (!event.repeat && id !== undefined) options.onlaunch(id);
      return;
    }
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    const last = all.length - 1;
    let to: number | null = null;
    switch (event.key) {
      case 'Home':
        to = 0;
        break;
      case 'End':
        to = last;
        break;
      case 'ArrowDown':
        to = options.layout === 'grid' ? rowNeighbour(all, from, true) : Math.min(last, from + 1);
        break;
      case 'ArrowUp':
        to = options.layout === 'grid' ? rowNeighbour(all, from, false) : Math.max(0, from - 1);
        break;
      case 'ArrowRight':
        if (options.layout === 'grid') to = Math.min(last, from + 1);
        break;
      case 'ArrowLeft':
        if (options.layout === 'grid') to = Math.max(0, from - 1);
        break;
    }
    if (to === null) return;
    event.preventDefault();
    if (to !== from || !item.contains(document.activeElement) || document.activeElement === item) moveTo(to);
  }

  // Focus that comes into an item makes it the current one; focus handed to the item itself goes on
  // to its first control.
  function onfocusin(event: FocusEvent) {
    if (!list) return;
    const item = itemOf(list, event.target);
    if (!item) return;
    const index = items(list).indexOf(item);
    const id = options.idAt(index);
    if (id !== undefined && id !== options.current()) {
      options.onmove(id);
      refresh();
    }
    if (event.target === item) {
      const target = primaryOf(item);
      if (target) revealFocus(target, item);
    }
  }

  // A press on a control (a right click included) makes its item current, so a menu opened on it
  // returns there. A press on the item's own box (its padding, the gap a cell reaches into) is no
  // choice of the player's: the item takes focus from script only, so it neither takes focus nor
  // moves the Tab stop.
  function onpointerdown(event: PointerEvent) {
    if (!list) return;
    const item = itemOf(list, event.target);
    if (item && item === event.target) return;
    const id = item ? options.idAt(items(list).indexOf(item)) : undefined;
    if (id !== undefined && id !== options.current()) {
      options.onmove(id);
      refresh();
    }
  }

  const attach: Attachment<HTMLElement> = (node) => {
    list = node;
    refresh();
    const observer = new MutationObserver((records) => {
      const touched = new Set<HTMLElement>();
      for (const record of records) {
        const item = record.target === node ? null : itemOf(node, record.target);
        if (item) touched.add(item);
        else for (const added of record.addedNodes) if (added instanceof HTMLElement && added.parentElement === node) touched.add(added);
      }
      if (touched.size > 0) refresh(touched);
    });
    observer.observe(node, { childList: true, subtree: true });
    const offs = [
      on(node, 'keydown', onkeydown),
      on(node, 'focusin', onfocusin),
      on(node, 'pointerdown', onpointerdown),
      on(node, 'mousedown', (event) => {
        if (itemOf(node, event.target) === event.target) event.preventDefault();
      }),
    ];
    return () => {
      observer.disconnect();
      offs.forEach((off) => off());
      if (list === node) list = undefined;
    };
  };

  return { attach, refresh: () => refresh() };
}
