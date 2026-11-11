import { untrack } from 'svelte';
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

// Focus targets and press targets are any Element: a press often lands on an icon's <svg> or <path>.
const stack = new LayerStack<HTMLElement, Element>({
  contains: (element, target) => element.contains(target),
  holds: (anchor, target) => anchor.contains(target),
  alive: (target) => target.isConnected,
});
const inerted = new Set<HTMLElement>();
let appObserver: MutationObserver | null = null;
let listening = false;
// Presses that closed a layer: the scrim under it must not take the same press as its own.
const closingPresses = new WeakSet<Event>();

/**
 * Options of a layer: the extension point for every kind of layer, built-in or from a plugin.
 *
 * - Paint order. Every layer root, modal or not, is portalled to the end of <body> when it opens and
 *   sits on `--z-layer` (z-index.test.ts holds this), so the layer opened last paints on top, as the
 *   stack orders them.
 * - A dialog (Modal, Lightbox) is modal: it makes everything under it inert (the app content and every
 *   layer under it), takes focus on open (`initialFocus`), traps Tab and returns focus on close
 *   (`returnFocus` when its opener is gone). It owns the keyboard: `topModalLayer()` is the dialog that
 *   key handlers and Big Picture act on, whatever non-modal layer sits above it. A dialog that opens
 *   first closes every non-modal layer open at that moment (menus, tooltips, a listbox of a dialog
 *   under it), each that `dismissible()` allows; otherwise they would be left inert under or over
 *   its scrim. A non-modal layer opened afterwards from inside the dialog stays. A dialog opened from
 *   an item of such a menu adopts the menu's trigger as where focus returns when it closes (the item
 *   is gone by then), ahead of its own `returnFocus`, which is used when the trigger is gone too.
 * - A menu, a select's listbox, a popover or a tooltip is non-modal (`modal: false`): it inerts nothing
 *   and takes focus only when given `initialFocus` (only then is its root made focusable). Its `owner`
 *   is the element it was opened from (the trigger), in a dialog or on the bare page; the getter is read
 *   each time it is needed, so a trigger re-created while the layer is open (a re-keyed row) still
 *   counts. Option getters are read untracked: a re-render does not reopen the layer. Inside a dialog
 *   it counts as part of that dialog for the Tab trap (its controls follow the trigger in Tab order)
 *   and for where focus may return.
 * - Presses. A press (on any element, an icon's <svg> or <path> included) outside the top non-modal
 *   layer closes it, and keeps closing the non-modal layers under it until one holds the press: a press
 *   inside a layer, on its trigger or in a layer it owns belongs to that layer. So a press in a parent
 *   menu closes its submenu only, a press outside both closes both, and a press on a trigger is the
 *   trigger's own handler to decide (no close-then-reopen). `outsidePress: false` keeps a layer open
 *   (and when it takes no focus either, makes it passive: see below).
 *   The press still reaches what is under it, but a dialog's scrim does not take a press that closed a
 *   layer as its own (`pressClosedLayer`): one press closes the menu, the next one the dialog. The
 *   press that opened a layer never closes it.
 * - Escape (and Big Picture's back) closes only the top layer, if `dismissible()` allows; an element
 *   inside a layer can claim Escape first with `preventDefault` or `stopPropagation`; a held Escape
 *   (auto-repeat) closes nothing more.
 * - Passive layers. A non-modal layer with neither `initialFocus` nor outside presses
 *   (`outsidePress: false`) is passive: a hint (Tooltip, the hover card of a friend row). It never
 *   stands in the way of the layers under it. A press passes over it to them (a press outside a menu
 *   closes the menu though a hint shows above it); a press on the hint itself closes nothing. It
 *   records no opener and never moves focus when it closes. A layer under it closes as if it were the
 *   top one, so focus goes where that layer's close puts it. `topActiveLayer()` skips it. Escape still
 *   closes the top layer first, a hint included.
 */
export interface LayerOptions {
  /** A modal layer inerts everything under it, takes focus and traps Tab. Default true. */
  modal?: boolean;
  /** Read when Escape, Big Picture's back, an outside press or a dialog opening would close it. Default: always. */
  dismissible?: () => boolean;
  /** The layer is being closed by the user or by a dialog opening over it: unmount it. */
  dismiss: () => void;
  /** An element or a selector inside the layer to focus on open; default per `initialFocus` in focus.ts. */
  initialFocus?: () => HTMLElement | string | null | undefined;
  /** Where focus goes on close when the element that opened the layer is gone. */
  returnFocus?: () => HTMLElement | null | undefined;
  /** Non-modal: the element it was opened from (its trigger), in a dialog or on the bare page. */
  owner?: () => HTMLElement | null | undefined;
  /** Non-modal: a press outside it, its trigger and the layers it owns closes it. Default true. */
  outsidePress?: boolean;
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
  if (event.target instanceof Element && stack.holding(event.target)) return;
  event.preventDefault();
  event.stopPropagation();
  if (!event.repeat) dismissTopLayer();
}

function onWindowPointerdown(event: PointerEvent): void {
  if (!(event.target instanceof Element)) return;
  const before = stack.elements();
  const closed = stack.pressOutside(event.target);
  if (closed.length === 0) return;
  const open = new Set(stack.elements());
  const leaving = before.filter((element) => !open.has(element));
  closingPresses.add(event);
  for (const dismissal of closed) apply(dismissal.closed, dismissal.dismiss, leaving);
}

/** This press closed a menu or another non-modal layer: a dialog's scrim must not also close the dialog. */
export function pressClosedLayer(event: Event): boolean {
  return closingPresses.has(event);
}

// The top layer and the layers it owns: where focus may be when it closes.
function leavingSet(): HTMLElement[] {
  const top = stack.top();
  return top ? [top, ...stack.owned(top).map((layer) => layer.element)] : [];
}

function settle(closed: Closed<HTMLElement, Element> | null, leaving: HTMLElement[]): void {
  if (!closed) return;
  const active = document.activeElement;
  const lost = !active || active === document.body || !active.isConnected || leaving.some((el) => el.contains(active));
  if (!lost) return;
  if (closed.focus) focusQuietly(closed.focus);
  else if (closed.enter) enterLayer(closed.enter);
}

function apply(closed: Closed<HTMLElement, Element> | null, dismiss: (() => void) | null, leaving: HTMLElement[]): void {
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

/** The root element of the top layer, modal or not (a tooltip counts), or null when none is open. */
export function topLayer(): HTMLElement | null {
  return stack.top();
}

/** `target` is inside the layer `root` or inside a layer it owns, at any depth (a listbox, its submenu). */
export function withinLayer(root: HTMLElement, target: Element): boolean {
  return stack.within(root, target);
}

/** The root of the topmost layer that is not passive (the hints above it aside), modal or not, or null. */
export function topActiveLayer(): HTMLElement | null {
  return stack.topActive();
}

/** The root of the topmost modal layer (the dialog that owns the keyboard and the Tab trap), or null. */
export function topModalLayer(): HTMLElement | null {
  return stack.topModal();
}

// Tab order of a trap: the layer's controls, each owned layer's controls right after its trigger.
function trapList(root: HTMLElement): HTMLElement[] {
  const list = focusableIn(root);
  for (const layer of stack.owned(root)) {
    const at = layer.opener instanceof HTMLElement ? list.indexOf(layer.opener) : -1;
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

/** Moves focus to the next (previous) tabbable element of `root` and the layers it owns, wrapping at the ends. */
export function cycleFocus(root: HTMLElement, backwards: boolean): void {
  moveFocus(root, backwards);
}

/** Puts focus where the layer would on open (focus fell out of it, or never got in). */
export function enterLayer(element: HTMLElement): void {
  focusQuietly(initialFocus(element));
}

/** Registers `element` as an open layer; the returned function closes it. */
export function openLayer(element: HTMLElement, options: LayerOptions): () => void {
  const modal = options.modal ?? true;
  const passive = !modal && options.outsidePress === false && !options.initialFocus;
  const active = document.activeElement;
  // Read on every use, not once: the trigger may be re-created while the layer is open.
  const trigger = (): HTMLElement | null => (modal ? null : (options.owner?.() ?? null));
  // A passive layer never had focus to give back.
  const opener = !passive && active instanceof HTMLElement && active !== document.body ? active : null;
  const dismissed = modal ? stack.dismissNonModal(opener) : { dismiss: [], homes: [] };
  for (const dismiss of dismissed.dismiss) dismiss();
  const id = stack.open({
    element,
    modal,
    dismissible: options.dismissible ?? (() => true),
    dismiss: options.dismiss,
    opener,
    adopted: dismissed.homes,
    returnFocus: options.returnFocus,
    owner: () => {
      const element = trigger();
      return element ? stack.holding(element) : null;
    },
    anchor: trigger,
    outsidePress: !modal && options.outsidePress !== false,
    passive,
  });
  const focus = options.initialFocus?.();
  const takesFocus = modal || Boolean(focus);
  // Only a layer that takes focus can hold it itself (a click on its bare background keeps it inside).
  if (takesFocus && !element.hasAttribute('tabindex')) element.setAttribute('tabindex', '-1');

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
  if (takesFocus) focusQuietly(initialFocus(element, focus));

  return () => {
    element.removeEventListener('keydown', onKeydown);
    const leaving = [element, ...stack.owned(element).map((layer) => layer.element)];
    const closed = stack.close(id);
    sync();
    settle(closed, leaving);
  };
}

/**
 * `{@attach layer({...})}`: portals the element and keeps it registered while it is mounted. The
 * option getters are read untracked: state they read changing later (a re-rendered trigger, a new
 * `initialFocus`) must not close and reopen the layer, which would move focus and reset its opener.
 */
export function layer(options: LayerOptions): Attachment<HTMLElement> {
  return (node) => {
    const unportal = portal(node);
    const close = untrack(() => openLayer(node, options));
    return () => {
      close();
      unportal?.();
    };
  };
}
