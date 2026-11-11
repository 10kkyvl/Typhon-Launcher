/**
 * The one answer to "may the UI move?": the `no-anim` class that the animations setting puts on the
 * document, and the system reduced-motion request. JS-driven motion asks here; CSS animations follow
 * the same two switches through styles/global.css.
 */

export function motionAllowed(root: { classList: { contains(name: string): boolean } }, reducedMotion: boolean): boolean {
  return !root.classList.contains('no-anim') && !reducedMotion;
}

export function documentMotion(): boolean {
  if (typeof document === 'undefined') return false;
  const reduced = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  return motionAllowed(document.documentElement, reduced);
}

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
