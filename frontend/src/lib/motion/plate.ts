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
  return { rest: next, from: same(shown, next) ? null : shown };
}

export function plateKeyframes(from: Box, to: Box): Keyframe[] {
  return [{ transform: `${place(from)} scale(${from.w / to.w}, ${from.h / to.h})` }, { transform: plateRest(to) }];
}

const same = (a: Box, b: Box) => a.x === b.x && a.y === b.y && a.w === b.w && a.h === b.h;

// Offsets inside the plate's container (the island), so the plate needs nothing per side. An item
// of a positioned scroller inside it (the nav) is shifted by the scroll and cut to what the
// scroller shows; an item scrolled out has no box.
export function measure(target: HTMLElement | null | undefined, container?: Element | null): Box | null {
  if (!target) return null;
  const w = target.offsetWidth;
  const h = target.offsetHeight;
  if (!w || !h) return null;
  const parent = target.offsetParent as HTMLElement | null;
  if (!parent || parent === container) return { x: target.offsetLeft, y: target.offsetTop, w, h };
  const left = parent.offsetLeft + parent.clientLeft;
  const top = parent.offsetTop + parent.clientTop;
  const y = top + target.offsetTop - parent.scrollTop;
  const shownTop = Math.max(y, top);
  const shownBottom = Math.min(y + h, top + parent.clientHeight);
  if (shownBottom - shownTop < 1) return null;
  return { x: left + target.offsetLeft - parent.scrollLeft, y: shownTop, w, h: shownBottom - shownTop };
}

// The scroll position that shows an item of a scroller with the least movement.
export function scrollToShow(itemTop: number, itemHeight: number, scrollTop: number, viewHeight: number): number {
  if (itemTop < scrollTop) return itemTop;
  if (itemTop + itemHeight > scrollTop + viewHeight) return itemTop + itemHeight - viewHeight;
  return scrollTop;
}

// The container, its children and theirs: whatever changes size among them can move the active item.
export function partsThatMove(container: Element): Element[] {
  const children = [...container.children];
  return [container, ...children, ...children.flatMap((child) => [...child.children])];
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
  private unfollow: (() => void) | null = null;

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
    const next = measure(target, this.el.offsetParent);
    // A re-measure that finds the plate in place leaves a running slide alone.
    if (instant && next && this.box && same(next, this.box)) return;
    const step = plateStep(this.shown(), next, { motion: this.motion(), instant });
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

  // Re-places without a slide whenever one of the parts changes size (window width, collapse to
  // icons, a status row growing or shrinking, fonts).
  observe(parts: readonly Element[], current: () => HTMLElement | null | undefined): void {
    if (typeof ResizeObserver !== 'function') return;
    this.ro?.disconnect();
    const ro = new ResizeObserver(() => this.to(current(), true));
    for (const part of parts) ro.observe(part);
    this.ro = ro;
  }

  // Keeps the plate on the active item while the scroller it lives in scrolls.
  follow(scroller: HTMLElement, current: () => HTMLElement | null | undefined): void {
    this.unfollow?.();
    const onScroll = () => this.to(current(), true);
    scroller.addEventListener('scroll', onScroll, { passive: true });
    this.unfollow = () => scroller.removeEventListener('scroll', onScroll);
  }

  destroy(): void {
    this.ro?.disconnect();
    this.unfollow?.();
    this.unfollow = null;
    this.slide?.cancel();
    this.slide = null;
  }
}
