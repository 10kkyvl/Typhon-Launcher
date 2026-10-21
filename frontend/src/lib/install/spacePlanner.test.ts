import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SpaceInfo } from '../services/install';
import { createSpacePlanner, spaceDebounceMs } from './spacePlanner';

function info(required: number, free: number): SpaceInfo {
  return { requiredBytes: required, freeBytes: free, sameVolume: false, mode: 'copy', enough: free >= required };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function setup(fetch: (id: string, dest: string, mode: string) => Promise<SpaceInfo>) {
  const results: SpaceInfo[] = [];
  const errors: unknown[] = [];
  const pending = vi.fn();
  const planner = createSpacePlanner({
    fetch,
    onPending: pending,
    onResult: (value) => results.push(value),
    onError: (err) => errors.push(err),
  });
  return { planner, results, errors, pending };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('space planner', () => {
  it('asks again with the new folder once typing pauses, not on every keystroke', async () => {
    const fetch = vi.fn((_id: string, dest: string, _mode: string) => Promise.resolve(info(dest.length, 100)));
    const { planner, results } = setup(fetch);

    planner.schedule({ downloadId: 'd1', destination: 'D:\\G', mode: 'move' });
    vi.advanceTimersByTime(spaceDebounceMs - 1);
    planner.schedule({ downloadId: 'd1', destination: 'D:\\Games', mode: 'move' });
    vi.advanceTimersByTime(spaceDebounceMs - 1);
    expect(fetch).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith('d1', 'D:\\Games', 'move');
    expect(results).toEqual([info(8, 100)]);
  });

  it('asks at once for a picked folder and for a changed mode', async () => {
    const fetch = vi.fn(() => Promise.resolve(info(1, 2)));
    const { planner } = setup(fetch);

    planner.now({ downloadId: 'd1', destination: 'E:\\Picked', mode: 'copy' });
    expect(fetch).toHaveBeenCalledWith('d1', 'E:\\Picked', 'copy');
    planner.now({ downloadId: 'd1', destination: 'E:\\Picked', mode: 'move' });
    expect(fetch).toHaveBeenLastCalledWith('d1', 'E:\\Picked', 'move');
  });

  it('drops the answer for a folder the user has already left', async () => {
    const first = deferred<SpaceInfo>();
    const second = deferred<SpaceInfo>();
    const fetch = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { planner, results } = setup(fetch);

    planner.now({ downloadId: 'd1', destination: 'D:\\A', mode: 'copy' });
    planner.now({ downloadId: 'd1', destination: 'D:\\B', mode: 'copy' });
    second.resolve(info(5, 50));
    await vi.advanceTimersByTimeAsync(0);
    first.resolve(info(999, 1));
    await vi.advanceTimersByTimeAsync(0);

    expect(results).toEqual([info(5, 50)]);
  });

  it('reports a failure instead of keeping the previous numbers', async () => {
    const failure = new Error('no access');
    const { planner, results, errors } = setup(() => Promise.reject(failure));

    planner.now({ downloadId: 'd1', destination: '', mode: 'copy' });
    await vi.advanceTimersByTimeAsync(0);

    expect(results).toEqual([]);
    expect(errors).toEqual([failure]);
  });

  it('forgets a pending request and its answer once cancelled', async () => {
    const late = deferred<SpaceInfo>();
    const fetch = vi.fn(() => late.promise);
    const { planner, results } = setup(fetch);

    planner.schedule({ downloadId: 'd1', destination: 'D:\\A', mode: 'copy' });
    planner.cancel();
    await vi.advanceTimersByTimeAsync(spaceDebounceMs);
    expect(fetch).not.toHaveBeenCalled();

    planner.now({ downloadId: 'd1', destination: 'D:\\A', mode: 'copy' });
    planner.cancel();
    late.resolve(info(1, 1));
    await vi.advanceTimersByTimeAsync(0);
    expect(results).toEqual([]);
  });
});
