import type { Attachment } from 'svelte/attachments';
import { portal } from '../utils/portal';
import { focusableIn, focusQuietly, initialFocus, nextFocusTarget } from './focus';
import { LayerStack, type Closed } from './stack';

/**
 * The window's layer stack in the DOM. A layer is a portalled root (a dialog's overlay, the image
 * viewer) registered here while it is open. The stack:
 *
 * - makes the app content inert while a modal layer is open: every element child of `#app` except
 *   the window chrome marked `data-layer-keep` (the title bar stays usable to move and close the
 *   window), including children mounted later (`{#key $locale}`, Big Picture swapping the shell);
 *   and every layer under the topmost modal one. It clears only the `inert` it set itself;
 * - answers Escape for the top layer only: a key pressed inside any layer, or anywhere outside them,
 *   closes the top one (if it may close) and goes no further, so pages and Big Picture never see it;
 *   an element inside a layer can claim Escape first with `preventDefault` or `stopPropagation`;
 * - moves focus into a modal layer when it opens, keeps Tab inside the topmost modal layer, and
 *   returns focus when a layer closes (see LayerStack.close).
 *
 * State is per document: the overlay window runs the same bundle in its own document and module
 * instance, so nothing is shared across windows.
 */

const APP_ROOT_ID = 'app';
const KEEP = 'data-layer-keep';

const stack = new LayerStack<HTMLElement, HTMLElement>({
  contains: (element, target) => element.contains(target),
  alive: (target) => target.isConnected,
});
const inerted = new Set<HTMLElement>();
let appObserver: MutationObserver | null = null;
let listening = false;

export interface LayerOptions {
  /** A modal layer inerts everything under it and traps Tab. Default true. */
  modal?: boolean;
  /** Read when Escape (or Big Picture's back) arrives. Default: always. */
  dismissible?: () => boolean;
  /** The user asked to close the layer (Escape, back). */
  dismiss: () => void;
  /** An element or a selector inside the layer to focus on open; default per `initialFocus` in focus.ts. */
  initialFocus?: () => HTMLElement | string | null | undefined;
  /** Where focus goes on close when the element that opened the layer is gone. */
  returnFocus?: () => HTMLElement | null | undefined;
}

function appContent(): HTMLElement[] {
  const root = document.getElementById(APP_ROOT_ID);
  if (!root) return [];
  return [...root.children].filter((el): el is HTMLElement => el instanceof HTMLElement && !el.hasAttribute(KEEP));
}

function sync(): void {
  const want = stack.inert();
  const desired = new Set([...(want.app ? appContent() : []), ...want.layers]);
  for (const el of inerted) {
    if (desired.has(el)) continue;
    el.removeAttribute('inert');
    inerted.delete(el);
  }
  for (const el of desired) {
    if (el.hasAttribute('inert')) continue;
    el.setAttribute('inert', '');
    inerted.add(el);
  }
  watchApp(want.app);
  listen(stack.size > 0);
}

function watchApp(on: boolean): void {
  if (!on) {
    appObserver?.disconnect();
    appObserver = null;
    return;
  }
  const root = document.getElementById(APP_ROOT_ID);
  if (appObserver || !root || typeof MutationObserver !== 'function') return;
  appObserver = new MutationObserver(sync);
  appObserver.observe(root, { childList: true });
}

function listen(on: boolean): void {
  if (on === listening) return;
  listening = on;
  if (on) window.addEventListener('keydown', onWindowKeydown, true);
  else window.removeEventListener('keydown', onWindowKeydown, true);
}

function isEscape(event: KeyboardEvent): boolean {
  return event.key === 'Escape' && !event.isComposing;
}

function onWindowKeydown(event: KeyboardEvent): void {
  if (!isEscape(event)) return;
  if (event.target instanceof Node && stack.holding(event.target as HTMLElement)) return;
  event.preventDefault();
  event.stopPropagation();
  dismissTopLayer();
}

function settle(closed: Closed<HTMLElement, HTMLElement> | null, leaving: HTMLElement | null): void {
  if (!closed) return;
  const active = document.activeElement;
  const lost = !active || active === document.body || !active.isConnected || (leaving?.contains(active) ?? false);
  if (!lost) return;
  if (closed.focus) focusQuietly(closed.focus);
  else if (closed.enter) enterLayer(closed.enter);
}

/** Escape's effect: closes the top layer if it may close. False when no layer is open. */
export function dismissTopLayer(): boolean {
  const leaving = stack.top();
  const { consumed, closed, dismiss } = stack.dismissTop();
  if (!dismiss) return consumed;
  sync();
  settle(closed, leaving);
  dismiss();
  return consumed;
}

/** The root element of the top layer, or null when none is open. */
export function topLayer(): HTMLElement | null {
  return stack.top();
}

/** Moves focus to the next (previous) tabbable element of the top layer, wrapping at the ends. */
export function cycleFocus(backwards: boolean): void {
  const top = stack.top();
  if (!top) return;
  const list = focusableIn(top);
  const active = document.activeElement as HTMLElement | null;
  const next = nextFocusTarget(list, active && list.includes(active) ? active : null, backwards);
  focusQuietly(next ?? top);
}

/** Puts focus where the layer would on open (focus fell out of it, or never got in). */
export function enterLayer(element: HTMLElement): void {
  focusQuietly(initialFocus(element));
}

/** Registers `element` as an open layer; the returned function closes it. */
export function openLayer(element: HTMLElement, options: LayerOptions): () => void {
  const modal = options.modal ?? true;
  const active = document.activeElement;
  const id = stack.open({
    element,
    modal,
    dismissible: options.dismissible ?? (() => true),
    dismiss: options.dismiss,
    opener: active instanceof HTMLElement && active !== document.body ? active : null,
    returnFocus: options.returnFocus,
  });
  if (!element.hasAttribute('tabindex')) element.setAttribute('tabindex', '-1');

  function onKeydown(event: KeyboardEvent) {
    if (!stack.has(id)) return;
    if (isEscape(event)) {
      event.stopPropagation();
      if (event.defaultPrevented) return;
      event.preventDefault();
      dismissTopLayer();
      return;
    }
    if (event.key === 'Tab' && !event.defaultPrevented && stack.isTopModal(id)) {
      const list = focusableIn(element);
      const current = document.activeElement as HTMLElement | null;
      const next = nextFocusTarget(list, current && list.includes(current) ? current : null, event.shiftKey);
      event.preventDefault();
      focusQuietly(next ?? element);
    }
  }

  element.addEventListener('keydown', onKeydown);
  sync();
  if (modal) focusQuietly(initialFocus(element, options.initialFocus?.()));

  return () => {
    element.removeEventListener('keydown', onKeydown);
    const closed = stack.close(id);
    sync();
    settle(closed, element);
  };
}

/** `{@attach layer({...})}`: portals the element and keeps it registered while it is mounted. */
export function layer(options: LayerOptions): Attachment<HTMLElement> {
  return (node) => {
    const unportal = portal(node);
    const close = openLayer(node, options);
    return () => {
      close();
      unportal?.();
    };
  };
}
