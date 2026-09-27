import { describe, expect, it, vi } from 'vitest';
import { createBigPictureSession } from './session';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function setup(launch = vi.fn(async () => true)) {
  const onReturn = vi.fn(async (_id: string) => {});
  const onReturnError = vi.fn();
  const session = createBigPictureSession({ launch, onReturn, onReturnError });
  return { session, launch, onReturn, onReturnError };
}

describe('Big Picture launch lifecycle', () => {
  it('returns once to the game launched here after its actual session ends', async () => {
    const { session, onReturn } = setup();
    await session.launch('owned');
    session.observe(new Set());
    expect(onReturn).not.toHaveBeenCalled();
    session.observe(new Set(['owned', 'external']));
    session.observe(new Set(['external']));
    session.observe(new Set());
    expect(onReturn).toHaveBeenCalledExactlyOnceWith('owned');
  });

  it('handles started/stopped events arriving before the binding resolves', async () => {
    const pending = deferred<boolean>();
    const { session, onReturn } = setup(vi.fn(() => pending.promise));
    const started = session.launch('fast');
    session.observe(new Set(['fast']));
    session.observe(new Set());
    expect(onReturn).not.toHaveBeenCalled();
    pending.resolve(true);
    expect(await started).toBe(true);
    expect(onReturn).toHaveBeenCalledExactlyOnceWith('fast');
  });

  it('does not take focus for games launched elsewhere', () => {
    const { session, onReturn } = setup();
    session.observe(new Set(['external']));
    session.observe(new Set());
    expect(onReturn).not.toHaveBeenCalled();
  });

  it('ignores cancellation and permits a later retry', async () => {
    const { session, launch, onReturn } = setup();
    launch.mockResolvedValueOnce(false);
    expect(await session.launch('game')).toBe(false);
    session.observe(new Set(['game']));
    session.observe(new Set());
    expect(onReturn).not.toHaveBeenCalled();
    expect(await session.launch('game')).toBe(true);
    session.observe(new Set(['game']));
    session.observe(new Set());
    expect(onReturn).toHaveBeenCalledExactlyOnceWith('game');
  });

  it('drops a failed launch without swallowing its error and allows retry', async () => {
    const { session, launch, onReturn } = setup();
    launch.mockRejectedValueOnce(new Error('missing executable'));
    await expect(session.launch('game')).rejects.toThrow('missing executable');
    session.observe(new Set(['game']));
    session.observe(new Set());
    expect(onReturn).not.toHaveBeenCalled();
    await expect(session.launch('game')).resolves.toBe(true);
  });

  it('does not start duplicate launches while waiting for the native response', async () => {
    const pending = deferred<boolean>();
    const { session, launch } = setup(vi.fn(() => pending.promise));
    const started = session.launch('game');
    expect(await session.launch('game')).toBe(false);
    expect(launch).toHaveBeenCalledTimes(1);
    pending.resolve(true);
    await started;
  });

  it('does not restore after leaving Big Picture, including a pending response', async () => {
    const pending = deferred<boolean>();
    const { session, onReturn } = setup(vi.fn(() => pending.promise));
    const started = session.launch('game');
    session.observe(new Set(['game']));
    session.dispose();
    pending.resolve(true);
    expect(await started).toBe(false);
    session.observe(new Set());
    expect(onReturn).not.toHaveBeenCalled();
  });

  it('surfaces a failed window restore without retrying on every library event', async () => {
    const { session, onReturn, onReturnError } = setup();
    const error = new Error('focus unavailable');
    onReturn.mockRejectedValueOnce(error);
    await session.launch('game');
    session.observe(new Set(['game']));
    session.observe(new Set());
    await Promise.resolve();
    session.observe(new Set());
    expect(onReturn).toHaveBeenCalledTimes(1);
    expect(onReturnError).toHaveBeenCalledExactlyOnceWith(error);
  });
});
