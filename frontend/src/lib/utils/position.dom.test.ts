import { afterEach, describe, expect, it, vi } from 'vitest';
import { anchorTo, placeAtPoint, windowInsets, type Rect } from './position';

function box(rect: Rect): DOMRect {
  return { ...rect, x: rect.left, y: rect.top, right: rect.left + rect.width, bottom: rect.top + rect.height, toJSON: () => rect } as DOMRect;
}

afterEach(() => {
  document.documentElement.removeAttribute('style');
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('windowInsets', () => {
  it('keeps the title bar clear at the top and the popup edge on every side, in px of the current rem', () => {
    const root = document.documentElement;
    root.style.fontSize = '10px';
    root.style.setProperty('--titlebar-h', '4rem');
    root.style.setProperty('--popup-edge', '0.8rem');

    expect(windowInsets()).toEqual({ top: 48, right: 8, bottom: 8, left: 8 });
  });
});

describe('anchorTo', () => {
  it('places the element fixed next to its anchor and follows the anchor when the page scrolls', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {});
    const anchor = document.createElement('button');
    const floating = document.createElement('div');
    const scroller = document.createElement('div');
    scroller.append(anchor);
    document.body.append(scroller, floating);
    let anchorRect: Rect = { left: 100, top: 100, width: 40, height: 30 };
    anchor.getBoundingClientRect = () => box(anchorRect);
    floating.getBoundingClientRect = () => box({ left: 0, top: 0, width: 120, height: 80 });

    const stop = anchorTo(floating, anchor, { placement: 'bottom-end', offset: 4, padding: 8 });
    expect(floating.style.position).toBe('fixed');
    expect([floating.style.left, floating.style.top]).toEqual(['20px', '134px']);
    expect(floating.dataset.placement).toBe('bottom-end');

    anchorRect = { ...anchorRect, top: 40 };
    scroller.dispatchEvent(new Event('scroll'));
    for (const frame of frames.splice(0)) frame(0);
    expect(floating.style.top).toBe('74px');

    stop();
    anchorRect = { ...anchorRect, top: 10 };
    window.dispatchEvent(new Event('resize'));
    expect(frames).toEqual([]);
  });

  it('does not re-place when the floating element itself (or anything in it) scrolls', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {});
    const anchor = document.createElement('button');
    const floating = document.createElement('div');
    const inner = document.createElement('div');
    floating.append(inner);
    document.body.append(anchor, floating);

    const stop = anchorTo(floating, anchor, { padding: 8 });
    floating.dispatchEvent(new Event('scroll'));
    inner.dispatchEvent(new Event('scroll'));
    expect(frames).toHaveLength(0);

    document.dispatchEvent(new Event('scroll'));
    expect(frames).toHaveLength(1);
    stop();
  });

  it('reports a hidden anchor: scrolled out of its scroll container, then out of the window', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {});
    const scroller = document.createElement('div');
    scroller.style.overflow = 'auto';
    const anchor = document.createElement('button');
    const floating = document.createElement('div');
    scroller.append(anchor);
    document.body.append(scroller, floating);
    scroller.getBoundingClientRect = () => box({ left: 0, top: 100, width: 400, height: 300 });
    let anchorRect: Rect = { left: 20, top: 150, width: 40, height: 30 };
    anchor.getBoundingClientRect = () => box(anchorRect);
    const onhide = vi.fn();
    const scrolled = (rect: Rect) => {
      anchorRect = rect;
      scroller.dispatchEvent(new Event('scroll'));
      for (const frame of frames.splice(0)) frame(0);
    };

    const stop = anchorTo(floating, () => anchor, { padding: 8, onhide });
    expect(onhide).not.toHaveBeenCalled();

    scrolled({ ...anchorRect, top: 110 });
    expect(onhide).not.toHaveBeenCalled();
    scrolled({ ...anchorRect, top: 60 });
    expect(onhide).toHaveBeenCalledTimes(1);

    scroller.getBoundingClientRect = () => box({ left: 0, top: -400, width: 400, height: 300 });
    onhide.mockClear();
    scrolled({ ...anchorRect, top: -380 });
    expect(onhide).toHaveBeenCalledTimes(1);
    stop();
  });

  it('does not count a clipping ancestor above a fixed box the anchor sits in', () => {
    const clipped = document.createElement('div');
    clipped.style.overflow = 'hidden';
    const dock = document.createElement('div');
    dock.style.position = 'fixed';
    const anchor = document.createElement('button');
    const floating = document.createElement('div');
    dock.append(anchor);
    clipped.append(dock);
    document.body.append(clipped, floating);
    clipped.getBoundingClientRect = () => box({ left: 0, top: 0, width: 10, height: 10 });
    anchor.getBoundingClientRect = () => box({ left: 300, top: 300, width: 40, height: 30 });
    const onhide = vi.fn();

    const stop = anchorTo(floating, anchor, { padding: 8, onhide });

    expect(onhide).not.toHaveBeenCalled();
    stop();
  });

  it('keeps the scroll position of a height-limited element when it is placed again', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb));
    vi.stubGlobal('cancelAnimationFrame', () => {});
    const anchor = document.createElement('button');
    const floating = document.createElement('div');
    document.body.append(anchor, floating);
    anchor.getBoundingClientRect = () => box({ left: 100, top: 100, width: 40, height: 30 });
    // A browser clamps scrollTop to 0 while the element is laid out without its max-height.
    let scrollTop = 0;
    Object.defineProperty(floating, 'scrollTop', {
      configurable: true,
      get: () => scrollTop,
      set: (value: number) => (scrollTop = value),
    });
    floating.getBoundingClientRect = () => {
      if (floating.style.maxHeight === '') scrollTop = 0;
      return box({ left: 0, top: 0, width: 120, height: 2000 });
    };

    const stop = anchorTo(floating, anchor, { padding: 8 });
    expect(floating.style.maxHeight).not.toBe('');
    floating.scrollTop = 120;

    window.dispatchEvent(new Event('resize'));
    for (const frame of frames.splice(0)) frame(0);

    expect(floating.scrollTop).toBe(120);
    stop();
  });
});

describe('placeAtPoint', () => {
  function atPoint() {
    const floating = document.createElement('div');
    const inner = document.createElement('div');
    const page = document.createElement('div');
    floating.append(inner);
    document.body.append(page, floating);
    floating.getBoundingClientRect = () => box({ left: 0, top: 0, width: 200, height: 300 });
    return { floating, inner, page };
  }

  it('puts the corner at the point, below the title bar and flipped at the window edges', () => {
    const root = document.documentElement;
    root.style.fontSize = '10px';
    root.style.setProperty('--titlebar-h', '4rem');
    root.style.setProperty('--popup-edge', '0.8rem');
    const { floating } = atPoint();

    const stop = placeAtPoint(floating, 120, 10);
    expect(floating.style.position).toBe('fixed');
    expect([floating.style.left, floating.style.top]).toEqual(['120px', '48px']);
    stop();

    const flipped = placeAtPoint(floating, window.innerWidth - 20, window.innerHeight - 20);
    expect([floating.style.left, floating.style.top]).toEqual([`${window.innerWidth - 220}px`, `${window.innerHeight - 320}px`]);
    flipped();
  });

  it.each([
    ['the page scrolls', (page: HTMLElement) => page.dispatchEvent(new Event('scroll', { bubbles: false }))],
    ['the window resizes', () => window.dispatchEvent(new Event('resize'))],
    ['the window loses focus', () => window.dispatchEvent(new Event('blur'))],
  ])('reports that the point no longer holds when %s', (_case, happen) => {
    const { floating, page } = atPoint();
    const onleave = vi.fn();

    const stop = placeAtPoint(floating, 50, 60, { onleave });
    expect(onleave).not.toHaveBeenCalled();
    happen(page);

    expect(onleave).toHaveBeenCalledTimes(1);
    stop();
  });

  it('stays when its own content scrolls, and listens to nothing once stopped', () => {
    const { floating, inner, page } = atPoint();
    const onleave = vi.fn();

    const stop = placeAtPoint(floating, 50, 60, { onleave });
    floating.dispatchEvent(new Event('scroll'));
    inner.dispatchEvent(new Event('scroll'));
    expect(onleave).not.toHaveBeenCalled();

    stop();
    page.dispatchEvent(new Event('scroll'));
    window.dispatchEvent(new Event('resize'));
    window.dispatchEvent(new Event('blur'));
    expect(onleave).not.toHaveBeenCalled();
  });
});
