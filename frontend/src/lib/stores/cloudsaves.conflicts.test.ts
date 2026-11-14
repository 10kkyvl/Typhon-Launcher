import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { CloudConflict, CloudEvent, CloudGameStatus, CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';

vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));

let load: () => Promise<CloudStatus> = async () => status();
let emit: (event: CloudEvent) => void = () => {};

vi.mock('../services/cloudsaves', () => ({
  cloudAllowLaunchOnce: vi.fn(async () => {}),
  loadCloudStatus: vi.fn(() => load()),
  onCloudEvent: vi.fn((handler: (event: CloudEvent) => void) => {
    emit = handler;
    return vi.fn();
  }),
}));

vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => []),
  getRunningGames: vi.fn(async () => []),
}));

const copies = (local: string, remote: string): CloudConflict => ({
  localDigest: local,
  localUpdatedAt: '2026-11-10T10:00:00Z',
  localSize: 10,
  remoteDigest: remote,
  remoteUpdatedAt: '2026-11-11T10:00:00Z',
  remoteSize: 20,
  remoteDevice: 'DESK',
});

const inConflict = (conflict?: CloudConflict, gameId = 'g1'): CloudGameStatus => ({ gameId, state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict });
const synced = (gameId = 'g1'): CloudGameStatus => ({ gameId, state: 'synced', code: '', message: '', at: '', syncedAt: '' });

function status(...games: CloudGameStatus[]): CloudStatus {
  return { enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

async function init(first: CloudStatus) {
  vi.resetModules();
  load = async () => first;
  const store = await import('./cloudsaves');
  const library = await import('./library');
  library.libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
  store.initCloudSaves();
  await Promise.resolve();
  await Promise.resolve();
  return store;
}

// The choice applied in the dialog, as CloudConflictModal does it: the dialog goes, then the status loads.
async function resolveInDialog(store: Awaited<ReturnType<typeof init>>, gameId: string, after: CloudStatus) {
  store.openCloudConflict(gameId);
  const target = get(store.cloudConflictTarget);
  if (!target) throw new Error('the dialog did not open');
  store.finishCloudConflict(target);
  load = async () => after;
  return store.refreshCloudStatus();
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('the conflicts the player has not settled', () => {
  it('lists each game in conflict with its details, or none yet', async () => {
    const store = await init(status(inConflict(copies('a', 'b')), inConflict(undefined, 'g2')));

    const open = get(store.unsettledConflicts);
    expect([...open.keys()]).toEqual(['g1', 'g2']);
    expect(open.get('g1')?.remoteDevice).toBe('DESK');
    expect(open.get('g2')).toBeUndefined();
  });

  it('drops a conflict as soon as its choice is applied, before the status says so', async () => {
    const store = await init(status(inConflict(copies('a', 'b'))));
    store.openCloudConflict('g1');
    const target = get(store.cloudConflictTarget);
    if (!target) throw new Error('the dialog did not open');

    store.finishCloudConflict(target);

    expect(get(store.cloudStatus)?.games[0].state).toBe('conflict');
    expect(get(store.unsettledConflicts).has('g1')).toBe(false);
  });

  it('keeps a conflict played over once on this computer: it stays recorded', async () => {
    const store = await init(status(inConflict(copies('a', 'b'))));
    const asked = store.askToResolveConflict('g1');
    const target = get(store.cloudConflictTarget);
    if (!asked || !target) throw new Error('the dialog did not open');

    await store.allowConflictLaunch(target);

    expect(await asked).toBe(true);
    expect(get(store.unsettledConflicts).has('g1')).toBe(true);
  });

  it('lists a new conflict of the game found by a later load', async () => {
    const store = await init(status(inConflict(copies('a', 'b'))));
    await resolveInDialog(store, 'g1', status(synced()));
    expect(get(store.unsettledConflicts).has('g1')).toBe(false);

    load = async () => status(inConflict(copies('c', 'd')));
    await store.refreshCloudStatus();

    expect(get(store.unsettledConflicts).get('g1')?.localDigest).toBe('c');
  });

  it('does not bring a settled conflict back from a load that started before the choice', async () => {
    const store = await init(status(inConflict(copies('a', 'b'))));
    const stale = deferred<CloudStatus>();
    load = () => stale.promise;
    const loading = store.refreshCloudStatus();

    store.openCloudConflict('g1');
    const target = get(store.cloudConflictTarget);
    if (!target) throw new Error('the dialog did not open');
    store.finishCloudConflict(target);
    stale.resolve(status(inConflict(copies('a', 'b'))));
    await loading;

    expect(get(store.unsettledConflicts).has('g1')).toBe(false);
  });

  it('forgets the choice once the game leaves the conflict, so that a later conflict with the same copies counts', async () => {
    const store = await init(status(inConflict(copies('a', 'b'))));
    store.openCloudConflict('g1');
    const target = get(store.cloudConflictTarget);
    if (!target) throw new Error('the dialog did not open');
    store.finishCloudConflict(target);

    emit({ gameId: 'g1', status: 'synced', code: '', message: '', usedBytes: 0, quotaBytes: 0 });
    emit({ gameId: 'g1', status: 'conflict', code: '', message: '', usedBytes: 0, quotaBytes: 0 });

    expect(get(store.unsettledConflicts).has('g1')).toBe(true);
  });

  it('says while the status loads', async () => {
    const store = await init(status());
    const answer = deferred<CloudStatus>();
    load = () => answer.promise;

    const loading = store.refreshCloudStatus();
    expect(get(store.cloudStatusLoading)).toBe(true);
    answer.resolve(status());
    await loading;

    expect(get(store.cloudStatusLoading)).toBe(false);
  });
});

describe('conflicts the game page shows', () => {
  it('holds a game while any part of the page shows its conflict', async () => {
    const store = await init(status());

    const first = store.showConflictOnPage('g1');
    const second = store.showConflictOnPage('g1');
    expect(get(store.conflictsOnPage).has('g1')).toBe(true);
    first();
    expect(get(store.conflictsOnPage).has('g1')).toBe(true);
    second();

    expect(get(store.conflictsOnPage).has('g1')).toBe(false);
  });
});
