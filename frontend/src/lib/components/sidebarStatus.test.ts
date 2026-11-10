import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/router', async (actual) => ({ ...(await actual<typeof import('../stores/router')>()), navigate: vi.fn() }));
vi.mock('../stores/cloudsaves', async (actual) => ({ ...(await actual<typeof import('../stores/cloudsaves')>()), openCloudConflict: vi.fn() }));
vi.mock('../stores/selfupdate', async (actual) => ({
  ...(await actual<typeof import('../stores/selfupdate')>()),
  requestDownload: vi.fn(async () => {}),
  retryFailed: vi.fn(async () => {}),
}));

import type { CloudGameStatus, CloudState, CloudStatus } from '../services/cloudsaves';
import type { SelfUpdateStatus } from '../services/selfupdate';
import { navigate } from '../stores/router';
import { openCloudConflict } from '../stores/cloudsaves';
import { requestDownload, retryFailed } from '../stores/selfupdate';
import { cloudRow, openCloudRow, runUpdateRow, updateRow } from './sidebarStatus';

const games = [
  { id: 'g1', title: 'Hollow' },
  { id: 'g2', title: 'Celeste' },
  { id: 'g3', title: 'Tunic' },
];

function game(gameId: string, state: CloudState): CloudGameStatus {
  return { gameId, state, code: '', message: '', at: '', syncedAt: '' };
}

function cloud(over: Partial<CloudStatus> = {}): CloudStatus {
  return { enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [], ...over };
}

function update(over: Partial<SelfUpdateStatus> = {}): SelfUpdateStatus {
  return { state: 'idle', currentVersion: '1.4.1', ...over };
}

beforeEach(() => vi.clearAllMocks());

describe('cloudRow', () => {
  it.each([
    { name: 'not reported yet', status: null, offline: false, want: { kind: 'none' } },
    { name: 'turned off', status: cloud({ enabled: false, games: [game('g1', 'conflict')] }), offline: false, want: { kind: 'off' } },
    { name: 'no account', status: cloud({ signedIn: false }), offline: false, want: { kind: 'signedOut' } },
    { name: 'everything synced', status: cloud({ games: [game('g1', 'synced'), game('g2', 'skipped')] }), offline: false, want: { kind: 'synced' } },
    { name: 'a game waits', status: cloud({ games: [game('g1', 'synced'), game('g2', 'pending')] }), offline: false, want: { kind: 'pending' } },
    { name: 'the whole cloud waits', status: cloud({ pending: true }), offline: false, want: { kind: 'pending' } },
    { name: 'offline: neither synced nor syncing is true', status: cloud({ games: [game('g1', 'pending')] }), offline: true, want: { kind: 'offline' } },
    {
      name: 'a failure outranks waiting and offline',
      status: cloud({ games: [game('g1', 'pending'), game('g2', 'failed')] }),
      offline: true,
      want: { kind: 'failed', gameId: 'g2', title: 'Celeste' },
    },
    {
      name: 'a conflict outranks a failure',
      status: cloud({ games: [game('g2', 'failed'), game('g3', 'conflict'), game('g1', 'pending')] }),
      offline: false,
      want: { kind: 'conflict', gameId: 'g3', title: 'Tunic', count: 1 },
    },
    {
      name: 'several conflicts open the first one and count them',
      status: cloud({ games: [game('g1', 'conflict'), game('g2', 'synced'), game('g3', 'conflict')] }),
      offline: false,
      want: { kind: 'conflict', gameId: 'g1', title: 'Hollow', count: 2 },
    },
    {
      name: 'a game the library no longer has keeps its id as the title',
      status: cloud({ games: [game('gone', 'failed')] }),
      offline: false,
      want: { kind: 'failed', gameId: 'gone', title: 'gone' },
    },
  ])('$name', ({ status, offline, want }) => {
    expect(cloudRow(status, offline, games)).toEqual(want);
  });
});

describe('updateRow', () => {
  it.each([
    { name: 'no version known', status: update({ currentVersion: '' }), view: 'idle', downloading: false, want: { kind: 'none' } },
    { name: 'up to date', status: update(), view: 'idle', downloading: false, want: { kind: 'version', version: '1.4.1' } },
    { name: 'checking', status: update({ state: 'checking' }), view: 'checking', downloading: false, want: { kind: 'version', version: '1.4.1' } },
    {
      name: 'a failed check is not an update failure',
      status: update({ error: 'offline', errorCode: 'check' }),
      view: 'failed',
      downloading: false,
      want: { kind: 'version', version: '1.4.1' },
    },
    {
      name: 'an update is available',
      status: update({ state: 'available', availableVersion: '1.5.0' }),
      view: 'available',
      downloading: false,
      want: { kind: 'available', version: '1.5.0' },
    },
    {
      name: 'downloading shows the share done',
      status: update({ state: 'downloading', availableVersion: '1.5.0', totalBytes: 400, downloadedBytes: 100 }),
      view: 'downloading',
      downloading: true,
      want: { kind: 'downloading', version: '1.5.0', percent: 25 },
    },
    {
      name: 'a download just asked for counts as downloading',
      status: update({ state: 'available', availableVersion: '1.5.0' }),
      view: 'available',
      downloading: true,
      want: { kind: 'downloading', version: '1.5.0', percent: 0 },
    },
    {
      name: 'downloaded and ready to restart',
      status: update({ state: 'ready', availableVersion: '1.5.0' }),
      view: 'ready',
      downloading: false,
      want: { kind: 'ready', version: '1.5.0' },
    },
    {
      name: 'the download of an available update failed',
      status: update({ state: 'available', availableVersion: '1.5.0', error: 'x', errorCode: 'download' }),
      view: 'failed',
      downloading: false,
      want: { kind: 'failed', version: '1.5.0' },
    },
    {
      name: 'installing keeps the current version until the restart',
      status: update({ state: 'applying', availableVersion: '1.5.0' }),
      view: 'applying',
      downloading: false,
      want: { kind: 'version', version: '1.4.1' },
    },
  ])('$name', ({ status, view, downloading, want }) => {
    expect(updateRow(status, view, downloading, null)).toEqual(want);
  });

  it('prefers the live progress event to the last status', () => {
    const status = update({ state: 'downloading', availableVersion: '1.5.0', totalBytes: 400, downloadedBytes: 100 });
    expect(updateRow(status, 'downloading', false, { version: '1.5.0', totalBytes: 400, downloadedBytes: 300 })).toEqual({
      kind: 'downloading',
      version: '1.5.0',
      percent: 75,
    });
  });
});

describe('status row actions', () => {
  it('opens the cloud settings when the cloud is off or has no account', () => {
    openCloudRow({ kind: 'off' });
    openCloudRow({ kind: 'signedOut' });
    expect(vi.mocked(navigate).mock.calls).toEqual([
      ['settings', { tab: 'downloads' }],
      ['settings', { tab: 'downloads' }],
    ]);
  });

  it('opens the conflict dialog of the first conflicted game', () => {
    openCloudRow({ kind: 'conflict', gameId: 'g3', title: 'Tunic', count: 2 });
    expect(openCloudConflict).toHaveBeenCalledWith('g3');
    expect(navigate).not.toHaveBeenCalled();
  });

  it('opens the page of the game that failed to sync', () => {
    openCloudRow({ kind: 'failed', gameId: 'g2', title: 'Celeste' });
    expect(navigate).toHaveBeenCalledWith('game', { id: 'g2' });
  });

  it('does nothing for the quiet states', () => {
    for (const kind of ['none', 'offline', 'pending', 'synced'] as const) openCloudRow({ kind });
    expect(navigate).not.toHaveBeenCalled();
    expect(openCloudConflict).not.toHaveBeenCalled();
  });

  it('starts the download the update banner starts', async () => {
    const ask = vi.fn();
    await runUpdateRow({ kind: 'available', version: '1.5.0' }, ask);
    expect(requestDownload).toHaveBeenCalledOnce();
    expect(ask).not.toHaveBeenCalled();
  });

  it('asks before restarting into a downloaded update', async () => {
    const ask = vi.fn();
    await runUpdateRow({ kind: 'ready', version: '1.5.0' }, ask);
    expect(ask).toHaveBeenCalledOnce();
    expect(requestDownload).not.toHaveBeenCalled();
  });

  it('retries a failed update the way the banner does', async () => {
    await runUpdateRow({ kind: 'failed', version: '1.5.0' }, vi.fn());
    expect(retryFailed).toHaveBeenCalledOnce();
  });

  it('starts nothing from the plain version or a running download', async () => {
    const ask = vi.fn();
    await runUpdateRow({ kind: 'version', version: '1.4.1' }, ask);
    await runUpdateRow({ kind: 'downloading', version: '1.5.0', percent: 10 }, ask);
    expect(requestDownload).not.toHaveBeenCalled();
    expect(retryFailed).not.toHaveBeenCalled();
    expect(ask).not.toHaveBeenCalled();
  });
});
