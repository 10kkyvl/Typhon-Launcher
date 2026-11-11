import { anchorTo, scrolledUnder, type Placement } from '../utils/position';
import type { HoverIntent } from './hoverIntent';

/**
 * The DOM side of a hint (Tooltip, the hover card of a friend row): which events of its control feed
 * the hover intent, and what the shown hint does until it goes. The timing rules are hoverIntent.ts.
 */

export interface HintSource {
  intent: HoverIntent;
  /** The control the hint belongs to; read on every event, since the page may create it again. */
  control: () => HTMLElement | null;
}

/** A hint shows for a control on the page that is not disabled. */
export function usable(control: HTMLElement | null): boolean {
  return control !== null && control.isConnected && !control.matches(':disabled');
}

/**
 * Feeds `intent` from the control's events as they reach `node` (the control, or an element around
 * it): the pointer resting on it (touch aside), keyboard focus (`:focus-visible`), blur, and a press,
 * which hides the hint until the pointer and focus have left. Returns a cleanup function.
 */
export function hintTrigger(node: HTMLElement, { intent, control }: HintSource): () => void {
  const inControl = (target: EventTarget | null): boolean => target instanceof Node && Boolean(control()?.contains(target));

  const onPointerover = (event: PointerEvent) => {
    if (event.pointerType === 'touch' || !inControl(event.target) || !usable(control())) return;
    intent.hover();
  };
  const onPointerout = (event: PointerEvent) => {
    if (!inControl(event.target) || inControl(event.relatedTarget)) return;
    intent.unhover();
  };
  const onFocusin = (event: FocusEvent) => {
    const target = event.target;
    if (target instanceof HTMLElement && inControl(target) && target.matches(':focus-visible')) intent.focus();
  };
  const onFocusout = (event: FocusEvent) => {
    if (inControl(event.relatedTarget)) return;
    intent.blur();
  };
  const onPointerdown = (event: PointerEvent) => {
    if (inControl(event.target)) intent.escape();
  };

  node.addEventListener('pointerover', onPointerover);
  node.addEventListener('pointerout', onPointerout);
  node.addEventListener('focusin', onFocusin);
  node.addEventListener('focusout', onFocusout);
  node.addEventListener('pointerdown', onPointerdown);
  return () => {
    node.removeEventListener('pointerover', onPointerover);
    node.removeEventListener('pointerout', onPointerout);
    node.removeEventListener('focusin', onFocusin);
    node.removeEventListener('focusout', onFocusout);
    node.removeEventListener('pointerdown', onPointerdown);
  };
}

export interface HintBubbleOptions extends HintSource {
  placement: Placement;
  /** Used when the hint fits on neither side of `placement`'s axis. */
  fallback?: Placement;
  offset?: number;
  /** Where the control lives: the control removed from it, or turning disabled, hides the hint. */
  watch: HTMLElement | null;
}

/**
 * While a hint is shown (`node`, a layer on the stack): it stays next to the control (anchorTo), stays
 * while the pointer is over it, keeps focus on the control when pressed, and goes away on a scroll of
 * the page under the control, the window losing focus, the control scrolled out of sight, removed or
 * disabled. Returns a cleanup function.
 */
export function hintBubble(node: HTMLElement, options: HintBubbleOptions): () => void {
  const { intent, control, watch } = options;
  const stopAnchor = anchorTo(node, () => control(), {
    placement: options.placement,
    fallback: options.fallback,
    offset: options.offset ?? 6,
    onhide: () => intent.dismiss(),
  });
  const enter = () => intent.hover();
  const leave = () => intent.unhover();
  // A press on the hint must not take focus off the control it describes.
  const keepFocus = (event: MouseEvent) => event.preventDefault();
  const onScroll = (event: Event) => {
    if (scrolledUnder(event.target, control())) intent.dismiss();
  };
  const onBlur = () => intent.dismiss();
  const watcher = new MutationObserver(() => {
    if (!usable(control())) intent.dismiss();
  });
  if (watch) watcher.observe(watch, { subtree: true, childList: true, attributes: true, attributeFilter: ['disabled'] });
  node.addEventListener('pointerenter', enter);
  node.addEventListener('pointerleave', leave);
  node.addEventListener('mousedown', keepFocus);
  window.addEventListener('scroll', onScroll, true);
  window.addEventListener('blur', onBlur);
  return () => {
    stopAnchor();
    watcher.disconnect();
    node.removeEventListener('pointerenter', enter);
    node.removeEventListener('pointerleave', leave);
    node.removeEventListener('mousedown', keepFocus);
    window.removeEventListener('scroll', onScroll, true);
    window.removeEventListener('blur', onBlur);
  };
}
