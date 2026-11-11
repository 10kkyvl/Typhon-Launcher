/**
 * Anchored positioning for every popup: menus, listboxes, tooltips, popovers, a context menu at the
 * pointer.
 *
 * Pure maths first (`computePosition`, `positionAtPoint`), unit tested; `anchorTo` and `placeAtPoint`
 * wire it to the DOM with `position: fixed`. Nothing may extend past the window's usable area: the
 * floating element flips to the side with more room, then clamps inside it, and gets a max size (it
 * scrolls) when it is bigger than the space it has. The usable area is the window minus the title bar
 * at the top (`--titlebar-h`) and `--popup-edge` on every side (`windowInsets`).
 */

export type Side = 'top' | 'bottom' | 'left' | 'right';
export type Align = 'start' | 'center' | 'end';
export type Placement = Side | `${Side}-start` | `${Side}-end`;

export interface Rect {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface Size {
  width: number;
  height: number;
}

/** Distance a floating element keeps from each window edge, px. */
export interface Insets {
  top: number;
  right: number;
  bottom: number;
  left: number;
}

export interface PositionOptions {
  placement?: Placement;
  /** Gap between anchor and floating element, px. */
  offset?: number;
  /** Minimum distance to the viewport edges, px: one number for all four or one per side. */
  padding?: number | Insets;
  viewport: Size;
}

export interface PositionResult {
  x: number;
  y: number;
  placement: Placement;
  /** Set when the floating element must shrink (and scroll) to fit. */
  maxWidth?: number;
  maxHeight?: number;
}

const OPPOSITE: Record<Side, Side> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' };
const DEFAULT_PADDING = 8;

export function parsePlacement(placement: Placement): [Side, Align] {
  const [side, align] = placement.split('-') as [Side, 'start' | 'end' | undefined];
  return [side, align ?? 'center'];
}

function join(side: Side, align: Align): Placement {
  return (align === 'center' ? side : `${side}-${align}`) as Placement;
}

function insets(padding: number | Insets | undefined): Insets {
  if (typeof padding === 'object') return padding;
  const all = padding ?? DEFAULT_PADDING;
  return { top: all, right: all, bottom: all, left: all };
}

export function clamp(value: number, min: number, max: number): number {
  if (max < min) return min;
  return Math.min(Math.max(value, min), max);
}

function spaceOn(side: Side, anchor: Rect, viewport: Size, offset: number, pad: Insets): number {
  switch (side) {
    case 'top':
      return anchor.top - offset - pad.top;
    case 'bottom':
      return viewport.height - (anchor.top + anchor.height) - offset - pad.bottom;
    case 'left':
      return anchor.left - offset - pad.left;
    case 'right':
      return viewport.width - (anchor.left + anchor.width) - offset - pad.right;
  }
}

function alignOn(start: number, length: number, size: number, align: Align): number {
  if (align === 'start') return start;
  if (align === 'end') return start + length - size;
  return start + (length - size) / 2;
}

/** Position of a floating box next to an anchor rect, flipped and clamped into the viewport. */
export function computePosition(anchor: Rect, floating: Size, options: PositionOptions): PositionResult {
  const { placement = 'bottom-start', offset = 4, viewport } = options;
  const pad = insets(options.padding);
  let [side, align] = parsePlacement(placement);
  const vertical = side === 'top' || side === 'bottom';
  const need = vertical ? floating.height : floating.width;

  // Flip when the preferred side is too small and the opposite one has more room.
  const here = spaceOn(side, anchor, viewport, offset, pad);
  const there = spaceOn(OPPOSITE[side], anchor, viewport, offset, pad);
  if (need > here && there > here) side = OPPOSITE[side];
  const space = Math.max(0, spaceOn(side, anchor, viewport, offset, pad));

  const availWidth = Math.max(0, viewport.width - pad.left - pad.right);
  const availHeight = Math.max(0, viewport.height - pad.top - pad.bottom);
  const result: PositionResult = { x: 0, y: 0, placement: join(side, align) };

  let width = floating.width;
  let height = floating.height;
  if (vertical) {
    if (height > space) height = result.maxHeight = space;
    if (width > availWidth) width = result.maxWidth = availWidth;
  } else {
    if (width > space) width = result.maxWidth = space;
    if (height > availHeight) height = result.maxHeight = availHeight;
  }

  if (vertical) {
    result.y = side === 'bottom' ? anchor.top + anchor.height + offset : anchor.top - offset - height;
    result.x = alignOn(anchor.left, anchor.width, width, align);
  } else {
    result.x = side === 'right' ? anchor.left + anchor.width + offset : anchor.left - offset - width;
    result.y = alignOn(anchor.top, anchor.height, height, align);
  }

  result.x = clamp(result.x, pad.left, viewport.width - pad.right - width);
  result.y = clamp(result.y, pad.top, viewport.height - pad.bottom - height);
  return result;
}

/**
 * Position for a context menu opened at a pointer. The menu's corner sits at the point; near the right
 * or bottom edge it opens towards the other side, and it is clamped so no part leaves the usable area.
 */
export function positionAtPoint(
  x: number,
  y: number,
  floating: Size,
  options: { viewport: Size; padding?: number | Insets },
): PositionResult {
  const { viewport } = options;
  const pad = insets(options.padding);
  const result: PositionResult = { x, y, placement: 'bottom-start' };
  let { width, height } = floating;
  const availWidth = viewport.width - pad.left - pad.right;
  const availHeight = viewport.height - pad.top - pad.bottom;
  if (width > availWidth) width = result.maxWidth = availWidth;
  if (height > availHeight) height = result.maxHeight = availHeight;

  if (x + width > viewport.width - pad.right && x - width >= pad.left) result.x = x - width;
  if (y + height > viewport.height - pad.bottom && y - height >= pad.top) {
    result.y = y - height;
    result.placement = 'top-start';
  }
  result.x = clamp(result.x, pad.left, viewport.width - pad.right - width);
  result.y = clamp(result.y, pad.top, viewport.height - pad.bottom - height);
  return result;
}

/**
 * The part of an anchor still on screen: `rect` cut by each clipping ancestor's box (its scroll
 * containers) and by the window. Null when nothing is left; an empty rect that lies inside them (no
 * layout yet, a zero-size anchor) still counts as shown.
 */
export function visiblePart(rect: Rect, clips: readonly Rect[], viewport: Size): Rect | null {
  let left = Math.max(rect.left, 0);
  let top = Math.max(rect.top, 0);
  let right = Math.min(rect.left + rect.width, viewport.width);
  let bottom = Math.min(rect.top + rect.height, viewport.height);
  for (const clip of clips) {
    left = Math.max(left, clip.left);
    top = Math.max(top, clip.top);
    right = Math.min(right, clip.left + clip.width);
    bottom = Math.min(bottom, clip.top + clip.height);
  }
  if (right < left || bottom < top) return null;
  return { left, top, width: right - left, height: bottom - top };
}

// The boxes of the ancestors that clip `element` (any overflow but visible), up to the first fixed
// box on the way, which its ancestors' overflow does not clip.
function clipRects(element: Element): Rect[] {
  const clips: Rect[] = [];
  for (let child = element, parent = element.parentElement; parent && parent !== document.documentElement; child = parent, parent = parent.parentElement) {
    if (getComputedStyle(child).position === 'fixed') break;
    const style = getComputedStyle(parent);
    const clipping = [style.overflow, style.overflowX, style.overflowY].some((value) => value !== '' && value !== 'visible');
    if (clipping) clips.push(parent.getBoundingClientRect());
  }
  return clips;
}

/** Nothing of `element` is on screen: scrolled out of the window or clipped away by a scroll container. */
export function outOfSight(element: Element): boolean {
  return visiblePart(element.getBoundingClientRect(), clipRects(element), viewportSize()) === null;
}

/** A CSS length token (`4rem`, `8px`) in px; `fallback` for anything else (a calc, an unset token). */
export function lengthPx(raw: string, remPx: number, fallback: number): number {
  const value = raw.trim();
  const n = parseFloat(value);
  if (!Number.isFinite(n)) return fallback;
  if (value.endsWith('rem')) return n * remPx;
  if (value.endsWith('px')) return n;
  return fallback;
}

/** The window's usable area for popups: the title bar on top, `--popup-edge` on every side. */
export function windowInsets(): Insets {
  const root = getComputedStyle(document.documentElement);
  const remPx = parseFloat(root.fontSize) || 16;
  const edge = lengthPx(root.getPropertyValue('--popup-edge'), remPx, DEFAULT_PADDING);
  const titlebar = lengthPx(root.getPropertyValue('--titlebar-h'), remPx, 0);
  return { top: titlebar + edge, right: edge, bottom: edge, left: edge };
}

export function viewportSize(): Size {
  const root = document.documentElement;
  return { width: root.clientWidth || window.innerWidth, height: window.innerHeight };
}

function natural(node: HTMLElement): Size {
  // Measure without the limits from a previous pass so it can grow back. The scroll position this
  // loses is put back by keepScroll.
  const { maxWidth, maxHeight } = node.style;
  node.style.maxWidth = '';
  node.style.maxHeight = '';
  const rect = node.getBoundingClientRect();
  node.style.maxWidth = maxWidth;
  node.style.maxHeight = maxHeight;
  return { width: rect.width, height: rect.height };
}

// Measuring a height-limited element without its limit resets its scroll position; the user's place
// in a long list must survive a re-place.
function keepScroll<T>(node: HTMLElement, place: () => T): T {
  const { scrollTop, scrollLeft } = node;
  const result = place();
  if (node.scrollTop !== scrollTop) node.scrollTop = scrollTop;
  if (node.scrollLeft !== scrollLeft) node.scrollLeft = scrollLeft;
  return result;
}

function apply(node: HTMLElement, result: PositionResult): void {
  node.style.left = `${Math.round(result.x)}px`;
  node.style.top = `${Math.round(result.y)}px`;
  node.style.maxWidth = result.maxWidth !== undefined ? `${Math.floor(result.maxWidth)}px` : '';
  node.style.maxHeight = result.maxHeight !== undefined ? `${Math.floor(result.maxHeight)}px` : '';
  node.dataset.placement = result.placement;
}

export interface AnchorOptions {
  placement?: Placement;
  offset?: number;
  /** Default: `windowInsets()`, read on every placement (the zoom setting changes the rem). */
  padding?: number | Insets;
  /** Make the floating element at least as wide as the anchor (listboxes). */
  matchWidth?: boolean;
  /** Called after each placement. */
  onplace?: (result: PositionResult) => void;
  /**
   * Called on a placement that finds the anchor no longer shown: out of the window, or clipped away
   * by a scroll container it sits in. An anchored popup closes then rather than float detached from it.
   */
  onhide?: () => void;
}

/** What a popup is anchored to: an element, or a getter for one (read on every placement) or for a rect. */
export type Anchor = HTMLElement | (() => HTMLElement | Rect | null | undefined);

/**
 * Keeps a `position: fixed` element next to an anchor while the page scrolls (any scrolling ancestor)
 * or the window resizes, and when either element changes size. Sets `data-placement` to the side it
 * ended up on. A getter that returns nothing (the anchor element is being re-created) keeps the last
 * place. Returns a cleanup function.
 */
export function anchorTo(node: HTMLElement, anchor: Anchor, options: AnchorOptions = {}): () => void {
  node.style.position = 'fixed';
  node.style.left = '0px';
  node.style.top = '0px';

  let frame = 0;
  let observed: Element | null = null;
  let rect: Rect = { left: 0, top: 0, width: 0, height: 0 };
  const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(() => schedule()) : null;
  observer?.observe(node);

  const place = () => {
    frame = 0;
    const target = typeof anchor === 'function' ? anchor() : anchor;
    let hidden = false;
    if (target instanceof Element) {
      rect = target.getBoundingClientRect();
      hidden = visiblePart(rect, clipRects(target), viewportSize()) === null;
      if (target !== observed) {
        if (observed) observer?.unobserve(observed);
        observer?.observe(target);
        observed = target;
      }
    } else if (target) {
      rect = target;
      hidden = visiblePart(rect, [], viewportSize()) === null;
    }
    if (options.matchWidth) node.style.minWidth = `${Math.round(rect.width)}px`;
    const result = keepScroll(node, () => {
      const placed = computePosition(rect, natural(node), {
        placement: options.placement,
        offset: options.offset,
        padding: options.padding ?? windowInsets(),
        viewport: viewportSize(),
      });
      apply(node, placed);
      return placed;
    });
    options.onplace?.(result);
    if (hidden) options.onhide?.();
  };
  function schedule() {
    if (!frame) frame = requestAnimationFrame(place);
  }
  // The page scrolling moves the anchor; the floating element scrolling its own content does not.
  const onScroll = (event: Event) => {
    if (event.target instanceof Node && node.contains(event.target)) return;
    schedule();
  };

  place();
  window.addEventListener('resize', schedule);
  window.addEventListener('scroll', onScroll, true);

  return () => {
    cancelAnimationFrame(frame);
    window.removeEventListener('resize', schedule);
    window.removeEventListener('scroll', onScroll, true);
    observer?.disconnect();
  };
}

/** Places a `position: fixed` element at a point (context menu). Returns a cleanup function. */
export function placeAtPoint(node: HTMLElement, x: number, y: number, padding?: number | Insets): () => void {
  node.style.position = 'fixed';
  const place = () =>
    keepScroll(node, () =>
      apply(node, positionAtPoint(x, y, natural(node), { viewport: viewportSize(), padding: padding ?? windowInsets() })),
    );
  place();
  const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(() => place()) : null;
  observer?.observe(node);
  return () => observer?.disconnect();
}
