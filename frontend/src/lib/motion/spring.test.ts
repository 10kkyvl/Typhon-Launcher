import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SPRINGS, SpringValue } from './spring';

describe('SpringValue', () => {
  let now = 0;
  let queue = new Map<number, FrameRequestCallback>();
  let next = 1;
  const frame = (ms = 16) => {
    now += ms;
    const run = [...queue.values()];
    queue = new Map();
    for (const cb of run) cb(now);
  };
  const on = () => true;
  const off = () => false;

  beforeEach(() => {
    now = 0;
    queue = new Map();
    next = 1;
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      const id = next++;
      queue.set(id, cb);
      return id;
    });
    vi.stubGlobal('cancelAnimationFrame', (id: number) => queue.delete(id));
    vi.spyOn(performance, 'now').mockImplementation(() => now);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('settles on the target and asks for no more frames', () => {
    const seen: number[] = [];
    const s = new SpringValue(0, (v) => seen.push(v), 'smooth', on);
    s.set(100);
    for (let i = 0; i < 120 && queue.size; i++) frame();
    expect(s.value).toBe(100);
    expect(seen.at(-1)).toBe(100);
    expect(queue.size).toBe(0);
  });

  it('overshoots a little with bouncy and never with smooth', () => {
    const peak = (preset: keyof typeof SPRINGS) => {
      let top = 0;
      const s = new SpringValue(0, (v) => (top = Math.max(top, v)), preset, on);
      s.set(100);
      for (let i = 0; i < 200 && queue.size; i++) frame();
      return top;
    };
    expect(peak('smooth')).toBeLessThanOrEqual(100);
    expect(peak('bouncy')).toBeGreaterThan(101);
    expect(peak('bouncy')).toBeLessThan(110);
  });

  it('keeps its velocity when retargeted mid-flight (no jerk)', () => {
    const s = new SpringValue(0, () => {}, 'smooth', on);
    s.set(100);
    for (let i = 0; i < 6; i++) frame();
    expect(s.velocity).toBeGreaterThan(0);
    const at = s.value;
    s.set(0);
    expect(s.value).toBe(at);
    frame(2);
    // Right after the turn it still moves the old way: the new target brakes it, it does not reset it.
    expect(s.value).toBeGreaterThan(at);
    expect(queue.size).toBe(1);
  });

  it('with motion off jumps to the target at once and asks for no frame', () => {
    const seen: number[] = [];
    const s = new SpringValue(0, (v) => seen.push(v), 'bouncy', off);
    s.set(240);
    expect(seen).toEqual([240]);
    expect(s.value).toBe(240);
    expect(queue.size).toBe(0);
  });

  it('follows the motion setting at each set, without being made again', () => {
    let motion = true;
    const s = new SpringValue(0, () => {}, 'smooth', () => motion);
    s.set(100);
    expect(queue.size).toBe(1);
    motion = false;
    s.set(50);
    expect(s.value).toBe(50);
    expect(queue.size).toBe(0);
  });

  it('jumps when told to, whatever the motion setting', () => {
    const s = new SpringValue(0, () => {}, 'smooth', on);
    s.set(80, { instant: true });
    expect(s.value).toBe(80);
    expect(queue.size).toBe(0);
  });

  it('stops when its owner goes: no frame is left and none reports a value', () => {
    const seen: number[] = [];
    const s = new SpringValue(0, (v) => seen.push(v), 'smooth', on);
    s.set(100);
    frame();
    const count = seen.length;
    // A frame callback the browser already took (cancelling it is too late) must not report either.
    const late = [...queue.values()];
    s.stop();
    expect(queue.size).toBe(0);
    for (const cb of late) cb(now + 16);
    expect(seen.length).toBe(count);
  });
});
