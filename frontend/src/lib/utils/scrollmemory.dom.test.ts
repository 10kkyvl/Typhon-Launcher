import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { resetHistory, route, stashRoute } from '../stores/router';
import { scrollmemory } from './scrollmemory';

// jsdom lays nothing out: the scroller is given a position and a ceiling (how far its content lets it
// scroll), which the test raises as a loading page grows. Setting the position sends `scroll`, as a
// browser does when it moves.
class Scroller {
  top = 0;
  ceiling = Number.POSITIVE_INFINITY;
  readonly node = document.createElement('div');
  constructor() {
    Object.defineProperty(this.node, 'scrollTop', {
      configurable: true,
      get: () => this.top,
      set: (value: number) => {
        const next = Math.max(0, Math.min(value, this.ceiling));
        if (next === this.top) return;
        this.top = next;
        this.node.dispatchEvent(new Event('scroll'));
      },
    });
    document.body.append(this.node);
  }
}

const frame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

let destroy: (() => void) | undefined;

beforeEach(() => {
  resetHistory();
});

afterEach(() => {
  destroy?.();
  destroy = undefined;
  document.body.replaceChildren();
});

function attach(scroller: Scroller, onposition: (top: number) => void) {
  destroy = scrollmemory(scroller.node, onposition).destroy;
}

describe('the position a page is at, as scrollmemory reports it', () => {
  it('a new page reports the top at once, then where the user scrolls, once a frame', async () => {
    const seen = vi.fn();
    const scroller = new Scroller();
    attach(scroller, seen);
    expect(seen.mock.calls).toEqual([[0]]);
    scroller.node.scrollTop = 40;
    scroller.node.scrollTop = 80;
    await frame();
    expect(seen.mock.calls).toEqual([[0], [80]]);
  });

  it('a page returned to reports the position being restored first, holds it while the page grows, and the real one once there', async () => {
    stashRoute(get(route).key, 'scroll', 300);
    const seen = vi.fn();
    const scroller = new Scroller();
    scroller.ceiling = 120;
    attach(scroller, seen);
    expect(seen.mock.calls).toEqual([[300]]);
    await frame();
    scroller.ceiling = 200;
    await frame();
    // Short of the target while the content loads: nothing reported, the stage keeps the restored value.
    expect(scroller.top).toBe(200);
    expect(seen.mock.calls).toEqual([[300]]);
    scroller.ceiling = 1000;
    await frame();
    await frame();
    expect(scroller.top).toBe(300);
    expect(seen.mock.calls).toEqual([[300], [300]]);
  });

  it('a user who takes over during the restore gets the real position from then on', async () => {
    stashRoute(get(route).key, 'scroll', 300);
    const seen = vi.fn();
    const scroller = new Scroller();
    scroller.ceiling = 120;
    attach(scroller, seen);
    await frame();
    scroller.node.dispatchEvent(new Event('wheel'));
    expect(seen.mock.calls).toEqual([[300], [120]]);
    scroller.ceiling = 1000;
    scroller.node.scrollTop = 160;
    await frame();
    expect(seen.mock.calls).toEqual([[300], [120], [160]]);
  });

  it('still remembers the position for the way back without anyone listening', async () => {
    const scroller = new Scroller();
    destroy = scrollmemory(scroller.node).destroy;
    scroller.node.scrollTop = 90;
    const key = get(route).key;
    destroy();
    destroy = undefined;
    const again = new Scroller();
    again.ceiling = 1000;
    const seen = vi.fn();
    attach(again, seen);
    expect(seen.mock.calls[0]).toEqual([90]);
    await frame();
    expect(again.top).toBe(90);
    expect(get(route).key).toBe(key);
  });
});
