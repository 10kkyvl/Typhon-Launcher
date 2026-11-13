import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { motionOn } from './motion';

// The animations setting puts `no-anim` on the document (stores/settings.ts); the system asks through
// `prefers-reduced-motion`. Both change while the window is open.
const settle = () => new Promise<void>((resolve) => queueMicrotask(resolve));

afterEach(() => {
  document.documentElement.classList.remove('no-anim');
  vi.unstubAllGlobals();
});

describe('motionOn', () => {
  it('follows the animations setting without a reload', async () => {
    const seen: boolean[] = [];
    const stop = motionOn.subscribe((on) => seen.push(on));
    document.documentElement.classList.add('no-anim');
    await settle();
    document.documentElement.classList.remove('no-anim');
    await settle();
    stop();
    expect(seen).toEqual([true, false, true]);
  });

  it('follows a reduced-motion request made while the window is open', () => {
    const query = new EventTarget() as EventTarget & { matches: boolean };
    query.matches = false;
    vi.stubGlobal('matchMedia', () => query);
    const seen: boolean[] = [];
    const stop = motionOn.subscribe((on) => seen.push(on));
    query.matches = true;
    query.dispatchEvent(new Event('change'));
    stop();
    expect(seen).toEqual([true, false]);
  });

  it('reads the document afresh for a reader that does not subscribe', () => {
    document.documentElement.classList.add('no-anim');
    expect(get(motionOn)).toBe(false);
  });
});
