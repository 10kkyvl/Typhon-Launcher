export type BigPictureDirection = 'up' | 'down' | 'left' | 'right';

interface FocusableElement {
  disabled?: boolean;
  hidden?: boolean;
  isContentEditable?: boolean;
  tagName?: string;
  ownerDocument?: Document | null;
  parentElement?: FocusableElement | null;
  style?: {
    display?: string;
    visibility?: string;
  } | null;
  getAttribute?: (name: string) => string | null;
  getBoundingClientRect?: () => {
    left: number;
    top: number;
    right: number;
    bottom: number;
    width: number;
    height: number;
  };
  getClientRects?: () => ArrayLike<unknown>;
  focus?: (options?: FocusOptions) => void;
  scrollIntoView?: (options?: ScrollIntoViewOptions) => void;
}

interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
  centerX: number;
  centerY: number;
}

const EPSILON = 0.001;

function getAttribute(element: FocusableElement, name: string): string | null {
  try {
    return typeof element.getAttribute === 'function' ? element.getAttribute(name) : null;
  } catch {
    return null;
  }
}

function isTrueAttribute(element: FocusableElement, name: string): boolean {
  const value = getAttribute(element, name);
  return value !== null && value !== 'false';
}

function isHiddenSelf(element: FocusableElement): boolean {
  if (element.hidden === true || isTrueAttribute(element, 'hidden') || isTrueAttribute(element, 'aria-hidden')) {
    return true;
  }

  const style = element.style;
  if (style && (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse')) {
    return true;
  }

  // `getComputedStyle` is deliberately looked up at call time: the module is
  // also imported by Node-side tests where the DOM globals do not exist.
  const ownerDocument = element.ownerDocument as (Document & {
    defaultView?: Window | null;
  }) | null | undefined;
  const defaultView = ownerDocument?.defaultView;
  try {
    const getComputedStyle = defaultView?.getComputedStyle ??
      (typeof globalThis.getComputedStyle === 'function' ? globalThis.getComputedStyle : undefined);
    if (getComputedStyle) {
      const computed = getComputedStyle.call(defaultView ?? globalThis, element as unknown as Element);
      if (computed.display === 'none' || computed.visibility === 'hidden' || computed.visibility === 'collapse') {
        return true;
      }
    }
  } catch {
    // A minimal fake element may expose an incomplete style object. Explicit
    // hidden/aria-hidden attributes above still provide deterministic behavior.
  }

  try {
    // Unlike getBoundingClientRect(), an empty client-rect list is a reliable
    // hidden/detached signal in a real browser and in focused DOM test doubles.
    // Do not require it: jsdom-like hosts often omit layout APIs entirely.
    if (typeof element.getClientRects === 'function' && element.getClientRects().length === 0) return true;
  } catch {
    // Ignore incomplete test doubles.
  }

  return false;
}

function isHidden(element: FocusableElement): boolean {
  const seen = new Set<FocusableElement>();
  let current: FocusableElement | null | undefined = element;
  while (current && !seen.has(current)) {
    seen.add(current);
    if (isHiddenSelf(current) || isTrueAttribute(current, 'inert')) return true;
    current = current.parentElement;
  }
  return false;
}

function isDisabled(element: FocusableElement): boolean {
  return element.disabled === true || isTrueAttribute(element, 'disabled') || isTrueAttribute(element, 'aria-disabled');
}

function isFocusable(element: FocusableElement): boolean {
  const tagName = element.tagName;
  if (typeof tagName === 'string' && tagName.toLowerCase() !== 'button') return false;
  return !isDisabled(element) && !isHidden(element) && typeof element.focus === 'function';
}

function readRect(element: FocusableElement): Rect {
  let left = 0;
  let top = 0;
  let right = 0;
  let bottom = 0;

  try {
    if (typeof element.getBoundingClientRect === 'function') {
      const value = element.getBoundingClientRect();
      left = Number.isFinite(value.left) ? value.left : 0;
      top = Number.isFinite(value.top) ? value.top : 0;
      right = Number.isFinite(value.right) ? value.right : left + (Number.isFinite(value.width) ? value.width : 0);
      bottom = Number.isFinite(value.bottom) ? value.bottom : top + (Number.isFinite(value.height) ? value.height : 0);
    } else {
      const candidate = element as FocusableElement & {
        offsetLeft?: number;
        offsetTop?: number;
        offsetWidth?: number;
        offsetHeight?: number;
      };
      left = Number.isFinite(candidate.offsetLeft) ? candidate.offsetLeft! : 0;
      top = Number.isFinite(candidate.offsetTop) ? candidate.offsetTop! : 0;
      right = left + (Number.isFinite(candidate.offsetWidth) ? candidate.offsetWidth! : 0);
      bottom = top + (Number.isFinite(candidate.offsetHeight) ? candidate.offsetHeight! : 0);
    }
  } catch {
    // Keep a zero rectangle for a detached or intentionally minimal fake node.
  }

  const width = Math.max(0, right - left);
  const height = Math.max(0, bottom - top);
  return {
    left,
    top,
    right: Math.max(right, left),
    bottom: Math.max(bottom, top),
    width,
    height,
    centerX: left + width / 2,
    centerY: top + height / 2,
  };
}

function overlaps(startA: number, endA: number, startB: number, endB: number): boolean {
  return Math.min(endA, endB) - Math.max(startA, startB) > EPSILON;
}

function directionCandidate(direction: BigPictureDirection, current: Rect, candidate: Rect): boolean {
  switch (direction) {
    case 'up':
      return candidate.centerY < current.centerY - EPSILON && candidate.bottom <= current.top + EPSILON;
    case 'down':
      return candidate.centerY > current.centerY + EPSILON && candidate.top >= current.bottom - EPSILON;
    case 'left':
      return candidate.centerX < current.centerX - EPSILON;
    case 'right':
      return candidate.centerX > current.centerX + EPSILON;
  }
}

function primaryDistance(direction: BigPictureDirection, current: Rect, candidate: Rect): number {
  switch (direction) {
    case 'up':
      return Math.max(0, current.top - candidate.bottom);
    case 'down':
      return Math.max(0, candidate.top - current.bottom);
    case 'left':
      return Math.max(0, current.left - candidate.right);
    case 'right':
      return Math.max(0, candidate.left - current.right);
  }
}

function crossDistance(direction: BigPictureDirection, current: Rect, candidate: Rect): number {
  switch (direction) {
    case 'up':
    case 'down':
      return Math.abs(candidate.centerX - current.centerX);
    case 'left':
    case 'right':
      return Math.abs(candidate.centerY - current.centerY);
  }
}

function isAligned(direction: BigPictureDirection, current: Rect, candidate: Rect): boolean {
  if (direction === 'up' || direction === 'down') {
    return overlaps(current.left, current.right, candidate.left, candidate.right);
  }
  return overlaps(current.top, current.bottom, candidate.top, candidate.bottom);
}

function sortByDocumentOrder(elements: readonly FocusableElement[], rects: readonly Rect[]): FocusableElement[] {
  return elements
    .map((element, index) => ({ element, rect: rects[index], index }))
    .sort((a, b) => {
      const row = a.rect.centerY - b.rect.centerY;
      if (Math.abs(row) > EPSILON) return row;
      const column = a.rect.centerX - b.rect.centerX;
      if (Math.abs(column) > EPSILON) return column;
      return a.index - b.index;
    })
    .map(({ element }) => element);
}

const pendingReveals = new WeakMap<Document, object>();

/** Focus without a browser jump, then reveal after the selected game's layout updates. */
export function focusControl(element?: FocusableElement): void {
  if (!element || !isFocusable(element)) return;
  try {
    element.focus?.({ preventScroll: true });
  } catch {
    // Older WebViews and tiny test doubles may not accept FocusOptions.
    element.focus?.();
  }

  const document = element.ownerDocument;
  const view = document?.defaultView;
  const request = {};
  if (document) pendingReveals.set(document, request);

  const reveal = () => {
    // A newer key press or a dialog may already have moved focus elsewhere.
    if (document && (document.activeElement !== element || pendingReveals.get(document) !== request)) return;
    if (document) pendingReveals.delete(document);
    const reducedMotion = view?.matchMedia?.('(prefers-reduced-motion: reduce)').matches ||
      document?.documentElement?.classList?.contains('no-anim');
    try {
      element.scrollIntoView?.({ block: 'nearest', inline: 'nearest', behavior: reducedMotion ? 'instant' : 'smooth' });
    } catch {
      element.scrollIntoView?.();
    }
  };

  if (view?.requestAnimationFrame) view.requestAnimationFrame(reveal);
  else reveal();
}

function activeElementFor(root: FocusableElement): FocusableElement | null {
  const ownerDocument = root.ownerDocument as Document | null | undefined;
  const active = ownerDocument?.activeElement as FocusableElement | null | undefined;
  if (active) return active;

  // The fallback is useful for the deliberately small DOM hosts used by
  // integration tests and for a WebView while its document is initializing.
  try {
    const globalDocument = typeof document !== 'undefined' ? document : undefined;
    return (globalDocument?.activeElement as FocusableElement | null | undefined) ?? null;
  } catch {
    return null;
  }
}

function findCurrent(elements: readonly FocusableElement[], active: FocusableElement | null): number {
  if (!active) return -1;
  const exact = elements.indexOf(active);
  if (exact >= 0) return exact;

  // A nested icon/span can be the active element in a custom focus host. Walk
  // its ancestors so the nearest Big Picture control remains the anchor.
  let parent = active.parentElement ?? null;
  while (parent) {
    const index = elements.indexOf(parent);
    if (index >= 0) return index;
    parent = parent.parentElement ?? null;
  }
  return -1;
}

function candidatesFor(
  direction: BigPictureDirection,
  current: Rect,
  elements: readonly FocusableElement[],
  rects: readonly Rect[],
  row?: string,
): Array<{ element: FocusableElement; rect: Rect; index: number; aligned: boolean; primary: number; cross: number }> {
  return elements
    .map((element, index) => ({
      element,
      rect: rects[index],
      index,
      aligned: isAligned(direction, current, rects[index]),
      primary: primaryDistance(direction, current, rects[index]),
      cross: crossDistance(direction, current, rects[index]),
    }))
    .filter(({ element, rect }) => row
      ? getAttribute(element, 'data-bp-row') === row &&
        (direction === 'up' || direction === 'down' || directionCandidate(direction, current, rect))
      : directionCandidate(direction, current, rect));
}

/**
 * Move focus among the visible, enabled controls marked with
 * `[data-bp-focus]` inside `root`.
 *
 * Vertical movement visits the adjacent row before choosing its nearest card.
 * Horizontal movement stays within the current row. At a
 * boundary the function leaves focus where it is and returns `false`; this
 * lets the shell decide whether a direction should scroll or do nothing.
 */
export function moveFocus(root: HTMLElement, direction: BigPictureDirection): boolean {
  if (!root || typeof root.querySelectorAll !== 'function') return false;

  let queried: ArrayLike<FocusableElement> | null = null;
  try {
    queried = root.querySelectorAll('[data-bp-focus]') as unknown as ArrayLike<FocusableElement>;
  } catch {
    return false;
  }

  const elements = Array.from(queried ?? []).filter(isFocusable);
  if (elements.length === 0) return false;

  const rects = elements.map(readRect);
  const currentIndex = findCurrent(elements, activeElementFor(root as unknown as FocusableElement));

  if (currentIndex < 0) {
    const ordered = sortByDocumentOrder(elements, rects);
    const target = direction === 'up' || direction === 'left' ? ordered.at(-1) : ordered[0];
    if (!target) return false;
    focusControl(target);
    return true;
  }

  const current = rects[currentIndex];
  const vertical = direction === 'up' || direction === 'down';
  // A page may combine an explicitly ordered shelf with a geometric toolbar.
  // Use row ordering only when it covers the entire scope, otherwise those
  // unlabelled controls would be unreachable from the shelf.
  const orderedRows = elements.every((element) => getAttribute(element, 'data-bp-row'));
  let row = orderedRows ? getAttribute(elements[currentIndex], 'data-bp-row') ?? undefined : undefined;
  if (row && vertical) {
    // Shelf order stays stable while its scroller moves past the fixed header.
    const rows = [...new Set(elements.map((element) => getAttribute(element, 'data-bp-row')).filter(Boolean))];
    row = rows[rows.indexOf(row) + (direction === 'down' ? 1 : -1)] ?? undefined;
    if (!row) return false;
  }
  const candidates = candidatesFor(direction, current, elements, rects, row);
  if (candidates.length === 0) return false;

  const nearest = candidates.reduce((a, b) => a.primary <= b.primary ? a : b);
  // Alignment is only a preference within the next row, never a reason to
  // skip a short shelf. Overlap keeps unequal-height buttons in the same row.
  const pool = row ? candidates : vertical
    ? candidates.filter(({ rect }) => overlaps(rect.top, rect.bottom, nearest.rect.top, nearest.rect.bottom))
    : candidates.filter((candidate) => candidate.aligned);
  pool.sort((a, b) => {
    if (vertical) {
      const alignment = Number(b.aligned) - Number(a.aligned);
      if (alignment !== 0) return alignment;
      const cross = a.cross - b.cross;
      if (Math.abs(cross) > EPSILON) return cross;
    }
    const primary = a.primary - b.primary;
    if (Math.abs(primary) > EPSILON) return primary;
    const cross = a.cross - b.cross;
    if (Math.abs(cross) > EPSILON) return cross;
    return a.index - b.index;
  });

  const target = pool[0]?.element;
  if (!target || target === elements[currentIndex]) return false;
  focusControl(target);
  return true;
}
