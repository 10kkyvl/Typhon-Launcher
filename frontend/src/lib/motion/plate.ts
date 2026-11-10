/**
 * One indicator that moves between the selected items of a list (the sidebar plate). It is a single
 * absolutely positioned element (`left: 0; top: 0; transform-origin: 0 0`) laid out at the target's
 * size and placed by transform. A new selection plays one short slide from where the indicator is on
 * screen, so a selection made mid-slide continues from there instead of jumping. The first placement
 * and resizes never slide; with animations off (the `no-anim` class or a reduced-motion request)
 * nothing plays and the indicator just sits on the target.
 */

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface PlateStep {
  rest: Box | null;
  from: Box | null;
}

const place = (b: Box) => `translate(${b.x}px, ${b.y}px)`;

export function plateRest(b: Box): string {
  return `${place(b)} scale(1, 1)`;
}

export function plateStep(shown: Box | null, next: Box | null, opts: { motion: boolean; instant: boolean }): PlateStep {
  if (!next) return { rest: null, from: null };
  if (!shown || opts.instant || !opts.motion) return { rest: next, from: null };
  const same = shown.x === next.x && shown.y === next.y && shown.w === next.w && shown.h === next.h;
  return { rest: next, from: same ? null : shown };
}

export function plateKeyframes(from: Box, to: Box): Keyframe[] {
  return [{ transform: `${place(from)} scale(${from.w / to.w}, ${from.h / to.h})` }, { transform: plateRest(to) }];
}

// Offsets inside the island, so the plate needs nothing per side.
export function measure(target: HTMLElement | null | undefined): Box | null {
  if (!target) return null;
  const w = target.offsetWidth;
  const h = target.offsetHeight;
  if (!w || !h) return null;
  return { x: target.offsetLeft, y: target.offsetTop, w, h };
}

export function motionAllowed(root: { classList: { contains(name: string): boolean } }, reducedMotion: boolean): boolean {
  return !root.classList.contains('no-anim') && !reducedMotion;
}

function documentMotion(): boolean {
  if (typeof document === 'undefined') return false;
  const reduced = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  return motionAllowed(document.documentElement, reduced);
}

function tokenMs(name: string, fallback: number): number {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return fallback;
  const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const n = parseFloat(raw);
  if (!Number.isFinite(n)) return fallback;
  return raw.endsWith('ms') ? n : raw.endsWith('s') ? n * 1000 : fallback;
}

function tokenEase(name: string, fallback: string): string {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return fallback;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

export class Plate {
  private box: Box | null = null;
  private slide: Animation | null = null;
  private ro: ResizeObserver | null = null;

  constructor(
    private readonly el: HTMLElement,
    private readonly motion: () => boolean = documentMotion,
  ) {
    el.style.opacity = '0';
  }

  // Where the plate is on screen right now, mid-slide included.
  private shown(): Box | null {
    if (!this.box) return null;
    if (!this.slide || typeof DOMMatrix !== 'function' || typeof getComputedStyle !== 'function') return this.box;
    const m = new DOMMatrix(getComputedStyle(this.el).transform);
    return { x: m.e, y: m.f, w: this.box.w * m.a, h: this.box.h * m.d };
  }

  to(target: HTMLElement | null | undefined, instant = false): void {
    const step = plateStep(this.shown(), measure(target), { motion: this.motion(), instant });
    this.slide?.cancel();
    this.slide = null;
    this.box = step.rest;
    if (!step.rest) {
      this.el.style.opacity = '0';
      return;
    }
    this.el.style.width = `${step.rest.w}px`;
    this.el.style.height = `${step.rest.h}px`;
    this.el.style.transform = plateRest(step.rest);
    this.el.style.opacity = '1';
    if (!step.from || typeof this.el.animate !== 'function') return;
    const slide = this.el.animate(plateKeyframes(step.from, step.rest), {
      duration: tokenMs('--motion-fast', 150),
      easing: tokenEase('--ease-out', 'ease-out'),
    });
    this.slide = slide;
    slide.onfinish = () => {
      if (this.slide === slide) this.slide = null;
    };
  }

  observe(container: HTMLElement, current: () => HTMLElement | null | undefined): void {
    if (typeof ResizeObserver !== 'function') return;
    this.ro?.disconnect();
    this.ro = new ResizeObserver(() => this.to(current(), true));
    this.ro.observe(container);
  }

  destroy(): void {
    this.ro?.disconnect();
    this.slide?.cancel();
    this.slide = null;
  }
}
