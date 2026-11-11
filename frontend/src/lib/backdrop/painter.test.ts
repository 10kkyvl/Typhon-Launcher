import { describe, expect, it, vi } from 'vitest';
import { BackdropPainter, type PainterDeps } from './painter';
import type { BackdropLook } from './pixels';

const LOOK: BackdropLook = { saturate: 0.75, dim: [14, 15, 17, 0.6], limit: { max: 0.03 } };
const KEY = 'dark';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const solid = (v: number) => Uint8ClampedArray.from([v, v, v, 255, v, v, v, 255]);

function harness(pictures: Record<string, () => Promise<Uint8ClampedArray | null>>) {
  const screen = new Uint8ClampedArray(8);
  const frames = new Map<number, (now: number) => void>();
  let nextFrame = 0;
  const deps: PainterDeps = {
    picture: vi.fn((url: string) => (pictures[url] ?? (() => Promise.resolve(null)))()),
    put: vi.fn(),
    frame: vi.fn((step: (now: number) => void) => {
      frames.set(++nextFrame, step);
      return nextFrame;
    }),
    cancelFrame: vi.fn((id: number) => {
      frames.delete(id);
    }),
    now: () => 0,
  };
  const painter = new BackdropPainter(screen, deps);
  // Runs the pending frame at `now`; the step may ask for the next one.
  const tick = (now: number) => {
    const [id, step] = [...frames][0] ?? [];
    expect(step, 'a frame is pending').toBeDefined();
    frames.delete(id as number);
    (step as (now: number) => void)(now);
  };
  return { painter, screen, deps, frames, tick };
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('the backdrop painter', () => {
  it('swaps at once when motion is off', async () => {
    const h = harness({ a: () => Promise.resolve(solid(40)) });
    await h.painter.show('a', LOOK, KEY, 0);
    expect([...h.screen]).toEqual([...solid(40)]);
    expect(h.deps.frame).not.toHaveBeenCalled();
    expect(h.deps.put).toHaveBeenCalledTimes(1);
  });

  it('cross-fades over the given time and stops asking for frames once it is done', async () => {
    const h = harness({ a: () => Promise.resolve(solid(40)), b: () => Promise.resolve(solid(200)) });
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('b', LOOK, KEY, 600);
    expect(h.deps.frame).toHaveBeenCalledTimes(1);
    h.tick(300);
    expect(h.screen[0]).toBeGreaterThan(40);
    expect(h.screen[0]).toBeLessThan(200);
    h.tick(600);
    expect([...h.screen]).toEqual([...solid(200)]);
    expect(h.frames.size).toBe(0);
  });

  it('the newest target wins: a late picture for an older one is never drawn', async () => {
    const slow = deferred<Uint8ClampedArray | null>();
    const h = harness({ old: () => slow.promise, new: () => Promise.resolve(solid(90)) });
    const first = h.painter.show('old', LOOK, KEY, 0);
    await h.painter.show('new', LOOK, KEY, 0);
    slow.resolve(solid(250));
    await first;
    expect([...h.screen]).toEqual([...solid(90)]);
  });

  it('a target that comes back to what is drawn still cancels the load in flight', async () => {
    const slow = deferred<Uint8ClampedArray | null>();
    const h = harness({ a: () => Promise.resolve(solid(40)), b: () => slow.promise });
    await h.painter.show('a', LOOK, KEY, 0);
    const toB = h.painter.show('b', LOOK, KEY, 0);
    await h.painter.show('a', LOOK, KEY, 0);
    slow.resolve(solid(250));
    await toB;
    expect([...h.screen]).toEqual([...solid(40)]);
  });

  it('keeps the previous wash when a picture fails, and does not try it again', async () => {
    const h = harness({ a: () => Promise.resolve(solid(40)), broken: () => Promise.resolve(null) });
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('broken', LOOK, KEY, 0);
    expect([...h.screen]).toEqual([...solid(40)]);
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('broken', LOOK, KEY, 0);
    await h.painter.show('broken', LOOK, KEY, 0);
    expect(vi.mocked(h.deps.picture).mock.calls.filter(([url]) => url === 'broken')).toHaveLength(1);
  });

  it('treats a rejected load as a failed one', async () => {
    const h = harness({ a: () => Promise.resolve(solid(40)), bad: () => Promise.reject(new Error('decode')) });
    await h.painter.show('a', LOOK, KEY, 0);
    await expect(h.painter.show('bad', LOOK, KEY, 0)).resolves.toBeUndefined();
    expect([...h.screen]).toEqual([...solid(40)]);
  });

  it('fades to the plain page when there is no art or no look', async () => {
    const h = harness({ a: () => Promise.resolve(solid(40)) });
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('', LOOK, KEY, 0);
    expect([...h.screen]).toEqual([0, 0, 0, 0, 0, 0, 0, 0]);
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('a', null, '', 0);
    expect([...h.screen]).toEqual([0, 0, 0, 0, 0, 0, 0, 0]);
    expect(vi.mocked(h.deps.picture).mock.calls).toHaveLength(1);
  });

  it('bakes again for a new look, and does nothing for the same target and look', async () => {
    const h = harness({ a: () => Promise.resolve(solid(40)) });
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('a', LOOK, KEY, 0);
    expect(h.deps.put).toHaveBeenCalledTimes(1);
    await h.painter.show('a', { ...LOOK, saturate: 0.5 }, 'other', 0);
    expect(h.deps.picture).toHaveBeenCalledTimes(2);
    expect(vi.mocked(h.deps.picture).mock.calls[1][1].saturate).toBe(0.5);
  });

  it('a new target in the middle of a fade starts from what is on screen', async () => {
    const h = harness({ a: () => Promise.resolve(solid(0)), b: () => Promise.resolve(solid(200)), c: () => Promise.resolve(solid(100)) });
    await h.painter.show('a', LOOK, KEY, 0);
    await h.painter.show('b', LOOK, KEY, 600);
    h.tick(300);
    const mid = h.screen[0];
    expect(mid).toBeGreaterThan(0);
    await h.painter.show('c', LOOK, KEY, 600);
    expect(h.frames.size).toBe(1);
    h.tick(0);
    expect(h.screen[0]).toBe(mid);
    h.tick(600);
    expect(h.screen[0]).toBe(100);
  });

  it('draws nothing more after it is disposed', async () => {
    const slow = deferred<Uint8ClampedArray | null>();
    const h = harness({ a: () => slow.promise });
    const pending = h.painter.show('a', LOOK, KEY, 0);
    h.painter.dispose();
    slow.resolve(solid(40));
    await pending;
    await flush();
    expect(h.deps.put).not.toHaveBeenCalled();
  });
});
