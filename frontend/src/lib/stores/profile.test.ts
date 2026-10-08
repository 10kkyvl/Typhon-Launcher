import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { ProfileSnapshot } from '../services/profile';
import type { CurrentUser } from '../services/account';

const handlers: Record<string, () => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: () => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));

const fetchSnapshot = vi.fn();

vi.mock('../services/profile', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  getProfileSnapshot: fetchSnapshot,
}));

function snapshot(marker: string): ProfileSnapshot {
  return { marker } as unknown as ProfileSnapshot;
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

function user(showcase: string[]): CurrentUser {
  return { id: 'u1', profile: { showcase } } as unknown as CurrentUser;
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./profile');
  const account = await import('./user');
  return { store, currentUser: account.currentUser };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

describe('profile snapshot', () => {
  it('keeps the newest answer when an older request finishes last', async () => {
    const { store } = await load();
    const slow = deferred<ProfileSnapshot>();
    fetchSnapshot.mockReturnValueOnce(slow.promise).mockResolvedValueOnce(snapshot('new'));

    const first = store.refreshProfile();
    await store.refreshProfile();
    slow.resolve(snapshot('old'));
    await first;

    expect((get(store.profileSnapshot) as unknown as { marker: string }).marker).toBe('new');
  });

  it('ignores a failure of a request that was already superseded', async () => {
    const { store } = await load();
    const slow = deferred<ProfileSnapshot>();
    fetchSnapshot.mockReturnValueOnce(slow.promise).mockResolvedValueOnce(snapshot('new'));

    const first = store.refreshProfile();
    await store.refreshProfile();
    slow.reject(new Error('late failure'));
    await first;

    expect((get(store.profileSnapshot) as unknown as { marker: string }).marker).toBe('new');
    expect(console.error).not.toHaveBeenCalled();
  });

  it('keeps what it had when a refresh fails', async () => {
    const { store } = await load();
    fetchSnapshot.mockResolvedValueOnce(snapshot('kept'));
    await store.refreshProfile();
    fetchSnapshot.mockRejectedValueOnce(new Error('offline'));

    await store.refreshProfile();

    expect((get(store.profileSnapshot) as unknown as { marker: string }).marker).toBe('kept');
  });
});

describe('profile refresh triggers', () => {
  it('reloads on library, session and play log events', async () => {
    const { store } = await load();
    fetchSnapshot.mockResolvedValue(snapshot('x'));
    store.initProfile();
    await Promise.resolve();
    fetchSnapshot.mockClear();

    for (const name of ['library:updated', 'game:started', 'game:stopped', 'playlog:recorded']) {
      handlers[name]();
    }

    expect(fetchSnapshot).toHaveBeenCalledTimes(4);
  });

  it('subscribes only once however often it is initialised', async () => {
    const { store } = await load();
    fetchSnapshot.mockResolvedValue(snapshot('x'));
    const runtime = await import('@wailsio/runtime');

    store.initProfile();
    store.initProfile();

    expect(vi.mocked(runtime.Events.On)).toHaveBeenCalledTimes(4);
  });

  it('reloads when the showcase changes and not when something else about the user does', async () => {
    const { store, currentUser } = await load();
    fetchSnapshot.mockResolvedValue(snapshot('x'));
    currentUser.set(user(['a']));
    store.initProfile();
    await Promise.resolve();
    fetchSnapshot.mockClear();

    currentUser.set({ ...user(['a']), id: 'u1', displayName: 'Renamed' } as unknown as CurrentUser);
    expect(fetchSnapshot).not.toHaveBeenCalled();

    currentUser.set(user(['a', 'b']));
    expect(fetchSnapshot).toHaveBeenCalledTimes(1);
  });
});
