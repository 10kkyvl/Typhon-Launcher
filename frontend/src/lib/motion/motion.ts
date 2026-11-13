import { on } from 'svelte/events';
import { readable, type Readable } from 'svelte/store';

/**
 * The one answer to "may the UI move?": the `no-anim` class that the animations setting puts on the
 * document, and the system reduced-motion request. JS-driven motion asks here; CSS animations follow
 * the same two switches through styles/global.css.
 */

const REDUCED = '(prefers-reduced-motion: reduce)';

export function motionAllowed(root: { classList: { contains(name: string): boolean } }, reducedMotion: boolean): boolean {
  return !root.classList.contains('no-anim') && !reducedMotion;
}

export function documentMotion(): boolean {
  if (typeof document === 'undefined') return false;
  const reduced = typeof matchMedia === 'function' && matchMedia(REDUCED).matches;
  return motionAllowed(document.documentElement, reduced);
}

/**
 * `documentMotion()` for what has to follow a change while the window is open (the live island hands
 * its messages to the toast stack when motion goes off): the animations setting toggling `no-anim`,
 * and the system's reduced-motion request changing.
 */
export const motionOn: Readable<boolean> = readable(documentMotion(), (set) => {
  if (typeof document === 'undefined') return;
  const update = () => set(documentMotion());
  update();
  const watcher = typeof MutationObserver === 'function' ? new MutationObserver(update) : null;
  watcher?.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  const query = typeof matchMedia === 'function' ? matchMedia(REDUCED) : null;
  const off = query ? on(query, 'change', update) : undefined;
  return () => {
    watcher?.disconnect();
    off?.();
  };
});

export function tokenMs(name: string, fallback: number): number {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return fallback;
  const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const n = parseFloat(raw);
  if (!Number.isFinite(n)) return fallback;
  return raw.endsWith('ms') ? n : raw.endsWith('s') ? n * 1000 : fallback;
}

export function tokenEase(name: string, fallback: string): string {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return fallback;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}
