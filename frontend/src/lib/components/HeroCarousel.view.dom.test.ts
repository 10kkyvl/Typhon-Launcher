import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ambientState } from '../stores/ambient';
import { renderDom } from '../testing/dom';
import HeroCarousel, { ADVANCE_MS, type HeroSlide } from './HeroCarousel.svelte';

const slide = (id: string): HeroSlide => ({ id, title: `Game ${id}`, art: {} });
const shown = () => screen.getByRole('heading', { level: 2 }).textContent?.trim();

interface Watch {
  callback: IntersectionObserverCallback;
  options: IntersectionObserverInit | undefined;
  targets: Element[];
  disconnected: boolean;
}

let watches: Watch[] = [];

/** Reports the carousel as `ratio` in view, the way the browser does when a threshold is crossed. */
async function inView(ratio: number) {
  const watch = watches.at(-1);
  expect(watch, 'the carousel is observed').toBeDefined();
  const { callback, targets } = watch as Watch;
  const entries = targets.map((target) => ({ target, intersectionRatio: ratio, isIntersecting: ratio > 0 }) as IntersectionObserverEntry);
  callback(entries, {} as IntersectionObserver);
  await vi.advanceTimersByTimeAsync(0);
}

beforeEach(() => {
  watches = [];
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      private readonly watch: Watch;
      constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
        this.watch = { callback, options, targets: [], disconnected: false };
        watches.push(this.watch);
      }
      observe(target: Element) {
        this.watch.targets.push(target);
      }
      unobserve() {}
      disconnect() {
        this.watch.disconnected = true;
      }
      takeRecords() {
        return [];
      }
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.style.removeProperty('overflow-y');
});

function setup() {
  vi.useFakeTimers();
  return renderDom(HeroCarousel, { slides: [slide('a'), slide('b'), slide('c')], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn(), ambient: true });
}

describe('HeroCarousel: out of view', () => {
  it('holds its timer, and with it the window, once scrolled mostly out of view', async () => {
    setup();
    await inView(1);
    await inView(0.2);

    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);

    expect(shown()).toBe('Game a');
    expect(get(ambientState).resting).toBe('a');
  });

  it('runs out the time left once back in view', async () => {
    setup();
    await vi.advanceTimersByTimeAsync(3000);
    await inView(0.1);
    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);

    await inView(0.5);
    await vi.advanceTimersByTimeAsync(ADVANCE_MS - 3000 - 1);
    expect(shown()).toBe('Game a');
    await vi.advanceTimersByTimeAsync(1);
    expect(shown()).toBe('Game b');
    expect(get(ambientState).resting).toBe('b');
  });

  it('does not flip between the thresholds: running stays running, held stays held', async () => {
    setup();
    await inView(1);
    await inView(0.3);
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(shown()).toBe('Game b');

    await inView(0.24);
    await inView(0.3);
    await inView(0.39);
    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);
    expect(shown()).toBe('Game b');

    await inView(0.4);
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(shown()).toBe('Game c');
  });

  it('is observed against the scroller it sits in, and no longer once it is gone', async () => {
    document.body.style.overflowY = 'auto';
    const { unmount } = setup();
    const watch = watches.at(-1) as Watch;
    expect(watch.options?.root).toBe(document.body);
    expect(watch.targets).toEqual([screen.getByRole('region')]);
    unmount();
    expect(watch.disconnected).toBe(true);
  });

  it('runs where the browser cannot tell what is in view', async () => {
    vi.stubGlobal('IntersectionObserver', undefined);
    setup();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(shown()).toBe('Game b');
  });
});
