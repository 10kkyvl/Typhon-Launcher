import { afterEach, describe, expect, it, vi } from 'vitest';
import { anchorTo, windowInsets, type Rect } from './position';

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
