import { describe, expect, it, vi } from 'vitest';
import { LookReader } from './lookReader';
import type { BackdropLook } from './pixels';

const look = (dim: number): BackdropLook => ({ saturate: 0.75, page: [14, 15, 17], dim, limit: { max: 0.03 } });

function harness(read: () => BackdropLook | null) {
  const frames = new Map<number, () => void>();
  let next = 0;
  const apply = vi.fn();
  const readSpy = vi.fn(read);
  const reader = new LookReader(
    readSpy,
    (step) => {
      frames.set(++next, step);
      return next;
    },
    (id) => {
      frames.delete(id);
    },
    apply,
  );
  const tick = () => {
    const pending = [...frames.values()];
    frames.clear();
    for (const step of pending) step();
  };
  return { reader, apply, read: readSpy, frames, tick };
}

describe('reading the backdrop look from the window', () => {
  it('reads once per frame however often the theme moves within it, and the last state wins', () => {
    let dim = 0.5;
    const h = harness(() => look(dim));
    for (let i = 0; i < 20; i++) {
      dim = 0.5 + i / 100;
      h.reader.request();
    }
    expect(h.read).not.toHaveBeenCalled();
    h.tick();
    expect(h.read).toHaveBeenCalledTimes(1);
    expect(h.apply).toHaveBeenCalledTimes(1);
    expect(h.apply.mock.calls[0][0].dim).toBe(0.69);
  });

  it('passes nothing on when the look did not change, so no fade restarts', () => {
    const h = harness(() => look(0.6));
    h.reader.request();
    h.tick();
    h.reader.request();
    h.tick();
    expect(h.read).toHaveBeenCalledTimes(2);
    expect(h.apply).toHaveBeenCalledTimes(1);
  });

  it('passes a change on, and a theme without a wash as null', () => {
    let current: BackdropLook | null = look(0.6);
    const h = harness(() => current);
    h.reader.request();
    h.tick();
    current = null;
    h.reader.request();
    h.tick();
    expect(h.apply.mock.calls.map(([value, key]) => [value, key])).toEqual([
      [look(0.6), JSON.stringify(look(0.6))],
      [null, ''],
    ]);
  });

  it('reads nothing once disposed', () => {
    const h = harness(() => look(0.6));
    h.reader.request();
    h.reader.dispose();
    expect(h.frames.size).toBe(0);
    h.reader.request();
    expect(h.frames.size).toBe(0);
  });
});
