import type { Attachment } from 'svelte/attachments';
import { portal } from '../utils/portal';
import { focusableIn, focusQuietly, initialFocus, nextFocusTarget } from './focus';
import { LayerStack, type Closed } from './stack';

/**
 * The window's layer stack in the DOM. A layer is a portalled root (a dialog's overlay, the image
 * viewer) registered here while it is open. The stack:
 *
 * - makes the app content inert while a modal layer is open: every element child of `#app` except
 *   the parts marked `data-layer-keep`: the title bar (the window can still be moved and closed) and
 *   the toast regions (a toast raised from a dialog is seen and its action pressed), including children mounted later (`{#key $locale}`, Big Picture swapping the shell);
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
const outsidePress = new Map<HTMLElement, 'close' | 'swallow'>();

/**
 * Options of a layer: the extension point for every kind of layer, built-in or from a plugin.
 *
 * - A dialog (Modal, Lightbox) is modal: it makes everything under it inert, takes focus on open
 *   (`initialFocus`), traps Tab and returns focus on close (`returnFocus` when its opener is gone).
 * - A menu, a select's listbox, a popover or a tooltip is non-modal (`modal: false`): it inerts
 *   nothing and takes focus only when given `initialFocus`. Opened from inside a dialog and portalled
 *   out of it, it names its `owner` (the trigger, or any element inside the owning layer): it then
 *   counts as part of that dialog for the Tab trap (its controls follow the trigger in Tab order), for
 *   where focus may return, and for presses. A press outside it and the layers it owns closes it
 *   (`outsidePress`, default `'close'`; `'swallow'` also eats that press so it does not reach what is
 *   under it; `false` keeps it open). The press that opened it never closes it.
 * - Escape (and Big Picture's back) closes only the top layer, if `dismissible()` allows; an element
 *   inside a layer can claim Escape first with `preventDefault` or `stopPropagation`; a held Escape
 *   (auto-repeat) closes nothing more.
 */
export interface LayerOptions {
  /** A modal layer inerts everything under it and traps Tab. Default true. */
  modal?: boolean;
  /** Read when Escape (or Big Picture's back, or an outside press) arrives. Default: always. */
  dismissible?: () => boolean;
  /** The user asked to close the layer (Escape, back, an outside press). */
  dismiss: () => void;
  /** An element or a selector inside the layer to focus on open; default per `initialFocus` in focus.ts. */
  initialFocus?: () => HTMLElement | string | null | undefined;
  /** Where focus goes on close when the element that opened the layer is gone. */
  returnFocus?: () => HTMLElement | null | undefined;
  /** Non-modal: an element inside the layer this one belongs to (usually the trigger). */
  owner?: () => HTMLElement | null | undefined;
  /** Non-modal: what a press outside this layer and the layers it owns does. Default 'close'. */
  outsidePress?: 'close' | 'swallow' | false;
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
  // Capture: an Escape outside every layer and a press outside a menu are seen before anything below.
  if (on) {
    window.addEventListener('keydown', onWindowKeydown, true);
    window.addEventListener('pointerdown', onWindowPointerdown, true);
  } else {
    window.removeEventListener('keydown', onWindowKeydown, true);
    window.removeEventListener('pointerdown', onWindowPointerdown, true);
  }
}

function isEscape(event: KeyboardEvent): boolean {
  return event.key === 'Escape' && !event.isComposing;
}

function onWindowKeydown(event: KeyboardEvent): void {
  if (!isEscape(event)) return;
  if (event.target instanceof Node && stack.holding(event.target as HTMLElement)) return;
  event.preventDefault();
  event.stopPropagation();
  if (!event.repeat) dismissTopLayer();
}

function onWindowPointerdown(event: PointerEvent): void {
  if (!(event.target instanceof HTMLElement)) return;
  const swallow = outsidePress.get(stack.top() as HTMLElement) === 'swallow';
  const leaving = leavingSet();
  const dismissal = stack.pressOutside(event.target);
  if (!dismissal) return;
  if (swallow) {
    event.preventDefault();
    event.stopPropagation();
    window.addEventListener('click', (click) => click.stopPropagation(), { capture: true, once: true });
  }
  apply(dismissal.closed, dismissal.dismiss, leaving);
}

// The top layer and the layers it owns: where focus may be when it closes.
function leavingSet(): HTMLElement[] {
  const top = stack.top();
  return top ? [top, ...stack.owned(top).map((layer) => layer.element)] : [];
}

function settle(closed: Closed<HTMLElement, HTMLElement> | null, leaving: HTMLElement[]): void {
  if (!closed) return;
  const active = document.activeElement;
  const lost = !active || active === document.body || !active.isConnected || leaving.some((el) => el.contains(active));
  if (!lost) return;
  if (closed.focus) focusQuietly(closed.focus);
  else if (closed.enter) enterLayer(closed.enter);
}

function apply(closed: Closed<HTMLElement, HTMLElement> | null, dismiss: (() => void) | null, leaving: HTMLElement[]): void {
  if (!dismiss) return;
  sync();
  settle(closed, leaving);
  dismiss();
}

/** Escape's effect: closes the top layer if it may close. False when no layer is open. */
export function dismissTopLayer(): boolean {
  const leaving = leavingSet();
  const { consumed, closed, dismiss } = stack.dismissTop();
  apply(closed, dismiss, leaving);
  return consumed;
}

/** The root element of the top layer, or null when none is open. */
export function topLayer(): HTMLElement | null {
  return stack.top();
}

// Tab order of a trap: the layer's controls, each owned layer's controls right after its trigger.
function trapList(root: HTMLElement): HTMLElement[] {
  const list = focusableIn(root);
  for (const layer of stack.owned(root)) {
    const at = layer.opener ? list.indexOf(layer.opener) : -1;
    list.splice(at < 0 ? list.length : at + 1, 0, ...focusableIn(layer.element));
  }
  return list;
}

function moveFocus(root: HTMLElement, backwards: boolean): void {
  const list = trapList(root);
  const active = document.activeElement as HTMLElement | null;
  const next = nextFocusTarget(list, active && list.includes(active) ? active : null, backwards);
  focusQuietly(next ?? root);
}

/** Moves focus to the next (previous) tabbable element of the top layer, wrapping at the ends. */
export function cycleFocus(backwards: boolean): void {
  const top = stack.top();
  if (top) moveFocus(top, backwards);
}

/** Puts focus where the layer would on open (focus fell out of it, or never got in). */
export function enterLayer(element: HTMLElement): void {
  focusQuietly(initialFocus(element));
}

/** Registers `element` as an open layer; the returned function closes it. */
export function openLayer(element: HTMLElement, options: LayerOptions): () => void {
  const modal = options.modal ?? true;
  const active = document.activeElement;
  const owner = modal ? null : options.owner?.();
  const id = stack.open({
    element,
    modal,
    dismissible: options.dismissible ?? (() => true),
    dismiss: options.dismiss,
    opener: active instanceof HTMLElement && active !== document.body ? active : null,
    returnFocus: options.returnFocus,
    owner: owner ? stack.holding(owner) : null,
    outsidePress: !modal && options.outsidePress !== false,
  });
  if (!modal && options.outsidePress) outsidePress.set(element, options.outsidePress);
  if (!element.hasAttribute('tabindex')) element.setAttribute('tabindex', '-1');

  function onKeydown(event: KeyboardEvent) {
    if (!stack.has(id)) return;
    if (isEscape(event)) {
      event.stopPropagation();
      if (event.defaultPrevented) return;
      event.preventDefault();
      if (!event.repeat) dismissTopLayer();
      return;
    }
    const trap = event.key === 'Tab' && !event.defaultPrevented ? stack.trapFor(id) : null;
    if (trap) {
      event.preventDefault();
      moveFocus(trap, event.shiftKey);
    }
  }

  element.addEventListener('keydown', onKeydown);
  sync();
  const focus = options.initialFocus?.();
  if (modal || focus) focusQuietly(initialFocus(element, focus));

  return () => {
    element.removeEventListener('keydown', onKeydown);
    outsidePress.delete(element);
    const leaving = [element, ...stack.owned(element).map((layer) => layer.element)];
    const closed = stack.close(id);
    sync();
    settle(closed, leaving);
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
