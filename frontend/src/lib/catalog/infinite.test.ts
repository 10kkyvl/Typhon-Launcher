import { afterEach, describe, expect, it, vi } from 'vitest';
import { maxAutoPages, shouldLoadMore, watchSentinel } from './infinite';

const idle = { hasMore: true, loading: false, failed: false, pagesSinceScroll: 0 };

describe('shouldLoadMore', () => {
  it('loads when more pages exist and nothing is in flight', () => {
    expect(shouldLoadMore(idle)).toBe(true);
  });

  it.each([
    ['no more pages', { hasMore: false }],
    ['a request in flight', { loading: true }],
    ['a failed load', { failed: true }],
    ['the auto-page cap reached', { pagesSinceScroll: maxAutoPages }],
  ])('stays idle on %s', (_name, patch) => {
    expect(shouldLoadMore({ ...idle, ...patch })).toBe(false);
  });
});

describe('watchSentinel', () => {
  type Callback = (entries: { isIntersecting: boolean }[]) => void;
  let callback: Callback = () => {};
  const disconnect = vi.fn();

  function install() {
    vi.stubGlobal('IntersectionObserver', class {
      constructor(cb: Callback) { callback = cb; }
      observe() {}
      disconnect() { disconnect(); }
    });
    vi.stubGlobal('getComputedStyle', () => ({ overflowY: 'visible' }));
  }

  afterEach(() => {
    vi.unstubAllGlobals();
    disconnect.mockClear();
  });

  const node = { parentElement: null } as unknown as HTMLElement;

  it('requests the next page once when the sentinel is visible', () => {
    install();
    const load = vi.fn();
    watchSentinel(node, load);
    callback([{ isIntersecting: true }]);
    callback([{ isIntersecting: true }]);
    expect(load).toHaveBeenCalledTimes(1);
    expect(disconnect).toHaveBeenCalled();
  });

  it('ignores a sentinel that is out of view', () => {
    install();
    const load = vi.fn();
    watchSentinel(node, load);
    callback([{ isIntersecting: false }]);
    expect(load).not.toHaveBeenCalled();
  });

  it('does nothing without IntersectionObserver', () => {
    const load = vi.fn();
    expect(() => watchSentinel(node, load)())
      .not.toThrow();
    expect(load).not.toHaveBeenCalled();
  });
});
