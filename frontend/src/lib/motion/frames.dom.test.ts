import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { runFrames } from './frames';

// jsdom has no Web Animations and lays nothing out: what can be proven here is when `draw` is called,
// with which clock, and that nothing is called with motion off. What a frame looks like is the caller's
// pure function (components/updateShowTimeline.ts), tested on its own.

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'setTimeout', 'clearTimeout'] });
  vi.setSystemTime(1_700_000_000_000);
});

afterEach(() => {
  document.documentElement.classList.remove('no-anim');
  vi.unstubAllGlobals();
});

describe('runFrames', () => {
  it('draws on every animation frame with the wall clock until stopped', () => {
    const seen: number[] = [];
    const stop = runFrames((now) => {
      seen.push(now);
    });
    expect(stop).toBeTypeOf('function');
    vi.advanceTimersByTime(50);
    stop?.();
    const count = seen.length;
    vi.advanceTimersByTime(50);

    expect(count).toBeGreaterThanOrEqual(2);
    expect(seen.length).toBe(count);
    expect(seen[0]).toBeGreaterThanOrEqual(1_700_000_000_000);
    expect(seen.every((now, i) => i === 0 || now >= seen[i - 1])).toBe(true);
  });

  it('stops when the drawing says it is done', () => {
    let calls = 0;
    runFrames(() => ++calls < 3);
    vi.advanceTimersByTime(200);
    expect(calls).toBe(3);
  });

  it('draws nothing with the animations setting off', () => {
    document.documentElement.classList.add('no-anim');
    const draw = vi.fn();
    expect(runFrames(draw)).toBeNull();
    vi.advanceTimersByTime(100);
    expect(draw).not.toHaveBeenCalled();
  });

  it('draws nothing when the system asks for reduced motion', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true }));
    const draw = vi.fn();
    expect(runFrames(draw)).toBeNull();
    vi.advanceTimersByTime(100);
    expect(draw).not.toHaveBeenCalled();
  });

  it('degrades to nothing on a host without animation frames', () => {
    vi.stubGlobal('requestAnimationFrame', undefined);
    const draw = vi.fn();
    expect(runFrames(draw)).toBeNull();
    expect(draw).not.toHaveBeenCalled();
  });
});
