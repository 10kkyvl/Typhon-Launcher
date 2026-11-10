import { describe, expect, it, vi } from 'vitest';
import { measure, motionAllowed, Plate, plateKeyframes, plateStep, type Box } from './plate';

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

const root = (classes: string[] = []) => ({ classList: { contains: (name: string) => classes.includes(name) } });

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

describe('motionAllowed', () => {
  it('follows the animations setting and the system reduced-motion request', () => {
    expect(motionAllowed(root(), false)).toBe(true);
    expect(motionAllowed(root(['no-anim']), false)).toBe(false);
    expect(motionAllowed(root(), true)).toBe(false);
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
