import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createAutosave, noteLength } from './notes';

describe('noteLength', () => {
  it('counts code points, not UTF-16 units', () => {
    expect(noteLength('')).toBe(0);
    expect(noteLength('abc')).toBe(3);
    expect(noteLength('a😀б')).toBe(3);
  });
});

describe('createAutosave', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('saves only the last text after the delay', async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    const auto = createAutosave(save, 800);
    auto.schedule('a');
    await vi.advanceTimersByTimeAsync(500);
    auto.schedule('ab');
    await vi.advanceTimersByTimeAsync(799);
    expect(save).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith('ab');
    expect(auto.pending()).toBe(false);
  });

  it('flush saves the pending text immediately and cancels the timer', async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    const auto = createAutosave(save, 800);
    auto.schedule('x');
    await auto.flush();
    expect(save).toHaveBeenCalledWith('x');
    await vi.advanceTimersByTimeAsync(2000);
    expect(save).toHaveBeenCalledTimes(1);
  });

  it('flush without pending text saves nothing', async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    const auto = createAutosave(save, 800);
    await auto.flush();
    expect(save).not.toHaveBeenCalled();
  });

  it('runs saves one after another', async () => {
    const order: string[] = [];
    let release: () => void = () => {};
    const save = vi.fn((text: string) => {
      order.push(`start:${text}`);
      return new Promise<void>((resolve) => {
        release = () => {
          order.push(`end:${text}`);
          resolve();
        };
      });
    });
    const auto = createAutosave(save, 800);
    auto.schedule('one');
    const first = auto.flush();
    auto.schedule('two');
    const second = auto.flush();
    await Promise.resolve();
    expect(order).toEqual(['start:one']);
    release();
    await first;
    await vi.advanceTimersByTimeAsync(0);
    expect(order).toEqual(['start:one', 'end:one', 'start:two']);
    release();
    await second;
  });

  it('surfaces a save failure to the flush caller', async () => {
    const auto = createAutosave(() => Promise.reject(new Error('boom')), 800);
    auto.schedule('x');
    await expect(auto.flush()).rejects.toThrow('boom');
  });

  it('keeps the text of a failed save for the next flush', async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error('disk full')).mockResolvedValue(undefined);
    const auto = createAutosave(save, 800);
    auto.schedule('note');
    await vi.advanceTimersByTimeAsync(800);
    expect(auto.pending()).toBe(true);
    await auto.flush();
    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenLastCalledWith('note');
    expect(auto.pending()).toBe(false);
  });

  it('a newer text replaces the failed one', async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error('disk full')).mockResolvedValue(undefined);
    const auto = createAutosave(save, 800);
    auto.schedule('old');
    const failed = auto.flush();
    auto.schedule('new');
    await expect(failed).rejects.toThrow('disk full');
    await auto.flush();
    expect(save).toHaveBeenLastCalledWith('new');
    expect(save).toHaveBeenCalledTimes(2);
  });
});
