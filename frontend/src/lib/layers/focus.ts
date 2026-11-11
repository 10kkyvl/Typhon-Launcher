const FOCUSABLE = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'iframe',
  'audio[controls]',
  'video[controls]',
  '[contenteditable]:not([contenteditable="false"])',
  '[tabindex]',
].join(',');

function shown(el: HTMLElement): boolean {
  if (el.closest('[hidden], [inert]')) return false;
  // checkVisibility needs layout; a host without it (jsdom) skips the check.
  return typeof el.checkVisibility === 'function' ? el.checkVisibility({ visibilityProperty: true }) : true;
}

function tabbable(el: HTMLElement): boolean {
  if (!el.matches(FOCUSABLE) || el.tabIndex < 0) return false;
  if ((el as HTMLButtonElement).disabled) return false;
  // Only the checked radio of a group is in the tab order.
  if (el instanceof HTMLInputElement && el.type === 'radio' && el.name && !el.checked) {
    const scope = el.form ?? el.ownerDocument;
    const group = scope.querySelectorAll<HTMLInputElement>('input[type="radio"]');
    if (Array.from(group).some((radio) => radio.name === el.name && radio.checked)) return false;
  }
  return shown(el);
}

/** Tabbable elements inside `root`, in DOM order. */
export function focusableIn(root: ParentNode): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(tabbable);
}

/** Where Tab (or Shift+Tab) lands inside a trap: wraps at both ends; from outside it enters at the first (last). */
export function nextFocusTarget<T>(list: readonly T[], current: T | null, backwards: boolean): T | null {
  if (list.length === 0) return null;
  const index = current === null ? -1 : list.indexOf(current);
  if (index === -1) return backwards ? (list.at(-1) as T) : list[0];
  return list[(index + (backwards ? -1 : 1) + list.length) % list.length];
}

/**
 * The element a layer focuses when it opens: the caller's choice (an element or a selector inside the
 * layer), else a `[data-autofocus]` element or the first tabbable inside one, else the first tabbable
 * outside `[data-skip-autofocus]` (a dialog's close button), else the layer itself.
 */
export function initialFocus(root: HTMLElement, choice?: HTMLElement | string | null): HTMLElement {
  if (choice instanceof HTMLElement && root.contains(choice)) return choice;
  const picked = typeof choice === 'string' ? root.querySelector<HTMLElement>(choice) : null;
  const marked = picked ?? root.querySelector<HTMLElement>('[data-autofocus]');
  // The marked element may be a `display: contents` wrapper around a control: it has no box of its own.
  const target = marked && (tabbable(marked) ? marked : focusableIn(marked)[0]);
  if (target) return target;
  return focusableIn(root).find((el) => !el.closest('[data-skip-autofocus]')) ?? root;
}

export function focusQuietly(el: HTMLElement): void {
  el.focus({ preventScroll: true });
}
