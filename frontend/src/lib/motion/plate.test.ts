import { describe, expect, it, vi } from 'vitest';
import { afterEach, beforeEach } from 'vitest';
import { measure, partsThatMove, Plate, plateKeyframes, plateStep, scrollToShow, type Box } from './plate';

const library: Box = { x: 12, y: 80, w: 200, h: 40 };
const friends: Box = { x: 12, y: 200, w: 200, h: 40 };
const narrow: Box = { x: 10, y: 80, w: 44, h: 40 };

function element(box: Box | null) {
  return {
    offsetLeft: box?.x ?? 0,
    offsetTop: box?.y ?? 0,
    offsetWidth: box?.w ?? 0,
    offsetHeight: box?.h ?? 0,
    // Window coordinates of an island docked on the right: the plate must not use them.
    getBoundingClientRect: () => ({ left: 1700 + (box?.x ?? 0), top: 40 + (box?.y ?? 0), width: box?.w ?? 0, height: box?.h ?? 0 }),
  } as unknown as HTMLElement;
}

function plateElement() {
  const animate = vi.fn<(keyframes: Keyframe[], options?: KeyframeAnimationOptions) => { cancel: () => void }>(() => ({ cancel: vi.fn() }));
  return { el: { style: {} as Record<string, string>, animate } as unknown as HTMLElement & { animate: typeof animate }, animate };
}


describe('plateStep', () => {
  it('sits on the first item without sliding in', () => {
    expect(plateStep(null, library, { motion: true, instant: false })).toEqual({ rest: library, from: null });
  });

  it('slides from where it is shown to the new item', () => {
    expect(plateStep(library, friends, { motion: true, instant: false })).toEqual({ rest: friends, from: library });
  });

  it('jumps on a resize (window width, collapse to icons) instead of sliding', () => {
    expect(plateStep(library, narrow, { motion: true, instant: true })).toEqual({ rest: narrow, from: null });
  });

  it('hides when the active item is gone (hidden by a theme, a route with no item)', () => {
    expect(plateStep(library, null, { motion: true, instant: false })).toEqual({ rest: null, from: null });
  });

  it('never slides with animations off', () => {
    expect(plateStep(library, friends, { motion: false, instant: false })).toEqual({ rest: friends, from: null });
  });

  it('does not replay a slide onto the box it already holds', () => {
    expect(plateStep(library, { ...library }, { motion: true, instant: false })).toEqual({ rest: library, from: null });
  });
});

describe('plateKeyframes', () => {
  it('moves by transform only and ends at the resting place', () => {
    expect(plateKeyframes(library, { x: 12, y: 200, w: 100, h: 20 })).toEqual([
      { transform: 'translate(12px, 80px) scale(2, 2)' },
      { transform: 'translate(12px, 200px) scale(1, 1)' },
    ]);
  });
});

describe('measure', () => {
  it('reads the item inside the island, the same on the left and on the right side', () => {
    expect(measure(element(friends))).toEqual(friends);
  });

  it('has nothing to measure for a missing or collapsed item', () => {
    expect(measure(null)).toBeNull();
    expect(measure(element(null))).toBeNull();
  });
});

describe('Plate', () => {
  it('stays invisible until it has an item, then shows without a slide', () => {
    const { el, animate } = plateElement();
    const plate = new Plate(el, () => true);
    expect(el.style.opacity).toBe('0');
    plate.to(element(library));
    expect(el.style.opacity).toBe('1');
    expect(el.style.width).toBe('200px');
    expect(el.style.transform).toBe('translate(12px, 80px) scale(1, 1)');
    expect(animate).not.toHaveBeenCalled();
  });

  it('slides to the next item when motion is on', () => {
    const { el, animate } = plateElement();
    const plate = new Plate(el, () => true);
    plate.to(element(library));
    plate.to(element(friends));
    expect(el.style.transform).toBe('translate(12px, 200px) scale(1, 1)');
    expect(animate).toHaveBeenCalledOnce();
    expect(animate.mock.calls[0][0]).toEqual(plateKeyframes(library, friends));
  });

  it('only moves when motion is off', () => {
    const { el, animate } = plateElement();
    const plate = new Plate(el, () => false);
    plate.to(element(library));
    plate.to(element(friends));
    expect(el.style.transform).toBe('translate(12px, 200px) scale(1, 1)');
    expect(animate).not.toHaveBeenCalled();
  });

  it('hides when the item goes away', () => {
    const { el } = plateElement();
    const plate = new Plate(el, () => true);
    plate.to(element(library));
    plate.to(null);
    expect(el.style.opacity).toBe('0');
  });
});

// The island holds the brand, the scrolling nav, the bottom block and the status row; the plate sits in
// the island itself, the nav items in the nav.
const island = { offsetParent: null } as unknown as HTMLElement;

function scroller(top: number, scrollTop: number, height: number) {
  return { offsetLeft: 12, offsetTop: top, clientLeft: 0, clientTop: 0, scrollLeft: 0, scrollTop, clientHeight: height, offsetParent: island } as unknown as HTMLElement;
}

function item(parent: HTMLElement, top: number, height = 40) {
  return { offsetLeft: 0, offsetTop: top, offsetWidth: 200, offsetHeight: height, offsetParent: parent } as unknown as HTMLElement;
}

describe('measure inside the scrolling nav', () => {
  it('places an item of a scrolled nav where it shows', () => {
    expect(measure(item(scroller(100, 250, 400), 300), island)).toEqual({ x: 12, y: 150, w: 200, h: 40 });
  });

  it('keeps only the part of an item the nav shows', () => {
    expect(measure(item(scroller(100, 0, 400), 380), island)).toEqual({ x: 12, y: 480, w: 200, h: 20 });
    expect(measure(item(scroller(100, 310, 400), 300), island)).toEqual({ x: 12, y: 100, w: 200, h: 30 });
  });

  it('has nothing to show for an item scrolled out of the nav', () => {
    expect(measure(item(scroller(100, 0, 400), 420), island)).toBeNull();
    expect(measure(item(scroller(100, 400, 400), 300), island)).toBeNull();
  });

  it('measures a bottom-block item (settings, support) straight in the island', () => {
    expect(measure(item(island, 560), island)).toEqual({ x: 0, y: 560, w: 200, h: 40 });
  });
});

describe('scrollToShow', () => {
  it('scrolls the nav just enough to show the active item', () => {
    expect(scrollToShow(300, 40, 0, 200)).toBe(140);
    expect(scrollToShow(40, 40, 120, 200)).toBe(40);
    expect(scrollToShow(150, 40, 100, 200)).toBe(100);
  });
});

describe('Plate following the nav', () => {
  it('jumps with the scroll and never slides for it', () => {
    const { el, animate } = plateElement();
    Object.assign(el, { offsetParent: island });
    const plate = new Plate(el, () => true);
    const nav = scroller(100, 0, 400);
    plate.to(item(nav, 300));
    (nav as unknown as { scrollTop: number }).scrollTop = 120;
    plate.to(item(nav, 300), true);
    expect(el.style.transform).toBe('translate(12px, 280px) scale(1, 1)');
    expect(animate).not.toHaveBeenCalled();
  });

  it('lets a slide run when a re-measure finds the plate already in place', () => {
    const { el, animate } = plateElement();
    const plate = new Plate(el, () => true);
    plate.to(element(library));
    plate.to(element(friends));
    const slide = animate.mock.results[0].value as { cancel: ReturnType<typeof vi.fn> };
    plate.to(element(friends), true);
    expect(slide.cancel).not.toHaveBeenCalled();
  });
});

describe('Plate re-measuring', () => {
  const observed: Element[] = [];
  let fire: () => void = () => {};

  beforeEach(() => {
    observed.length = 0;
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: () => void) {
          fire = callback;
        }
        observe(el: Element) {
          observed.push(el);
        }
        disconnect() {}
      },
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it('watches every part whose size can move the active item, including the status row', () => {
    const status = { children: [] } as unknown as Element;
    const group = { children: [] } as unknown as Element;
    const nav = { children: [group] } as unknown as Element;
    const brand = { children: [] } as unknown as Element;
    const bottom = { children: [] } as unknown as Element;
    const aside = { children: [brand, nav, bottom, status] } as unknown as HTMLElement;
    const { el } = plateElement();
    new Plate(el, () => true).observe(partsThatMove(aside), () => null);
    expect(observed).toEqual([aside, brand, nav, bottom, status, group]);
  });

  it('re-places the plate without a slide when a watched part changes size', () => {
    const { el, animate } = plateElement();
    const plate = new Plate(el, () => true);
    let active = element(library);
    plate.observe([island], () => active);
    plate.to(active);
    active = element({ ...library, y: 64 });
    fire();
    expect(el.style.transform).toBe('translate(12px, 64px) scale(1, 1)');
    expect(animate).not.toHaveBeenCalled();
  });
});
