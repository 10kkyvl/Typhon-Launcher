import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Window: {} }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/router', async (actual) => ({ ...(await actual<typeof import('../stores/router')>()), navigate: vi.fn() }));
vi.mock('../stores/cloudsaves', async (actual) => ({ ...(await actual<typeof import('../stores/cloudsaves')>()), openCloudConflict: vi.fn() }));
vi.mock('../stores/user', async (actual) => ({ ...(await actual<typeof import('../stores/user')>()), leaveGuest: vi.fn(async () => {}) }));
vi.mock('../stores/toasts', () => ({ toast: vi.fn() }));
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
import { leaveGuest } from '../stores/user';
import { toast } from '../stores/toasts';
import { accountErrorText } from '../services/accountMessages';
import { msg } from '../i18n';
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

const online = { offline: false, guest: false };

describe('cloudRow', () => {
  it.each([
    { name: 'not reported yet', status: null, who: online, want: { kind: 'none' } },
    { name: 'turned off', status: cloud({ enabled: false, games: [game('g1', 'conflict')] }), who: online, want: { kind: 'off' } },
    { name: 'a guest has no cloud account: sign in', status: cloud({ signedIn: false }), who: { offline: false, guest: true }, want: { kind: 'signIn' } },
    {
      name: 'signed in to the launcher, but the cloud has no token: nothing here can fix it',
      status: cloud({ signedIn: false, error: 'cloud.token' }),
      who: online,
      want: { kind: 'account', code: 'cloud.token' },
    },
    { name: 'everything synced', status: cloud({ games: [game('g1', 'synced'), game('g2', 'skipped')] }), who: online, want: { kind: 'synced' } },
    { name: 'a game waits', status: cloud({ games: [game('g1', 'synced'), game('g2', 'pending')] }), who: online, want: { kind: 'pending' } },
    { name: 'the whole cloud waits', status: cloud({ pending: true }), who: online, want: { kind: 'pending' } },
    {
      name: 'offline: neither synced nor syncing is true',
      status: cloud({ games: [game('g1', 'pending')] }),
      who: { offline: true, guest: false },
      want: { kind: 'offline' },
    },
    {
      name: 'a failed pass is never shown as synced',
      status: cloud({ error: 'cloud.server', games: [game('g1', 'synced')] }),
      who: online,
      want: { kind: 'error', code: 'cloud.server' },
    },
    {
      name: 'a failed pass outranks offline and waiting',
      status: cloud({ error: 'cloud.network', pending: true }),
      who: { offline: true, guest: false },
      want: { kind: 'error', code: 'cloud.network' },
    },
    {
      name: 'a game failure outranks a failed pass, waiting and offline',
      status: cloud({ error: 'cloud.server', games: [game('g1', 'pending'), game('g2', 'failed')] }),
      who: { offline: true, guest: false },
      want: { kind: 'failed', gameId: 'g2', title: 'Celeste' },
    },
    {
      name: 'a conflict outranks a failure',
      status: cloud({ games: [game('g2', 'failed'), game('g3', 'conflict'), game('g1', 'pending')] }),
      who: online,
      want: { kind: 'conflict', gameId: 'g3', title: 'Tunic', count: 1 },
    },
    {
      name: 'several conflicts open the first one and count them',
      status: cloud({ games: [game('g1', 'conflict'), game('g2', 'synced'), game('g3', 'conflict')] }),
      who: online,
      want: { kind: 'conflict', gameId: 'g1', title: 'Hollow', count: 2 },
    },
    {
      name: 'a conflict of a game the library no longer has is neither counted nor picked',
      status: cloud({ games: [game('a-gone', 'conflict'), game('g3', 'conflict')] }),
      who: online,
      want: { kind: 'conflict', gameId: 'g3', title: 'Tunic', count: 1 },
    },
    {
      name: 'with only gone games in conflict or failed, the next state shows',
      status: cloud({ games: [game('gone', 'conflict'), game('gone-2', 'failed'), game('g1', 'pending')] }),
      who: online,
      want: { kind: 'pending' },
    },
    {
      name: 'a failure of a gone game gives way to a failure that can be opened',
      status: cloud({ games: [game('a-gone', 'failed'), game('g2', 'failed')] }),
      who: online,
      want: { kind: 'failed', gameId: 'g2', title: 'Celeste' },
    },
  ])('$name', ({ status, who, want }) => {
    expect(cloudRow(status, who, games)).toEqual(want);
  });

  // The live island says these games' conflicts at the moment; the row says each conflict in one place.
  it.each([
    {
      name: 'the one conflict the island says: the row does not say it, nor that the saves are synced',
      status: cloud({ games: [game('g1', 'conflict'), game('g2', 'synced')] }),
      aside: ['g1'],
      want: { kind: 'none' },
    },
    {
      name: 'two conflicts, one in the island: the row names the other and counts it alone',
      status: cloud({ games: [game('g1', 'conflict'), game('g3', 'conflict')] }),
      aside: ['g1'],
      want: { kind: 'conflict', gameId: 'g3', title: 'Tunic', count: 1 },
    },
    {
      name: 'a failure still shows while the island says the conflict',
      status: cloud({ games: [game('g1', 'conflict'), game('g2', 'failed')] }),
      aside: ['g1'],
      want: { kind: 'failed', gameId: 'g2', title: 'Celeste' },
    },
    {
      name: 'a failed pass still shows while the island says the conflict',
      status: cloud({ error: 'cloud.server', games: [game('g1', 'conflict')] }),
      aside: ['g1'],
      want: { kind: 'error', code: 'cloud.server' },
    },
    {
      name: 'the island saying nothing leaves the row as it was',
      status: cloud({ games: [game('g1', 'conflict'), game('g3', 'conflict')] }),
      aside: [],
      want: { kind: 'conflict', gameId: 'g1', title: 'Hollow', count: 2 },
    },
  ])('$name', ({ status, aside, want }) => {
    expect(cloudRow(status, online, games, new Set(aside))).toEqual(want);
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
  it('opens the cloud settings when the cloud is off', async () => {
    await openCloudRow({ kind: 'off' });
    expect(navigate).toHaveBeenCalledWith('settings', { tab: 'downloads' });
  });

  it('signs a guest in the way the account menu does', async () => {
    await openCloudRow({ kind: 'signIn' });
    expect(leaveGuest).toHaveBeenCalledOnce();
    expect(navigate).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });

  it('reports a failed sign-in with the account menu text', async () => {
    vi.mocked(leaveGuest).mockRejectedValueOnce(new Error('boom'));
    await openCloudRow({ kind: 'signIn' });
    expect(toast).toHaveBeenCalledWith(accountErrorText(new Error('boom'), msg('ui.signOutFailed')), 'danger');
  });

  it('opens the conflict dialog of the first conflicted game', async () => {
    await openCloudRow({ kind: 'conflict', gameId: 'g3', title: 'Tunic', count: 2 });
    expect(openCloudConflict).toHaveBeenCalledWith('g3');
    expect(navigate).not.toHaveBeenCalled();
  });

  it('opens the page of the game that failed to sync', async () => {
    await openCloudRow({ kind: 'failed', gameId: 'g2', title: 'Celeste' });
    expect(navigate).toHaveBeenCalledWith('game', { id: 'g2' });
  });

  it('does nothing for the states it cannot fix', async () => {
    for (const kind of ['none', 'offline', 'pending', 'synced'] as const) await openCloudRow({ kind });
    await openCloudRow({ kind: 'account', code: 'cloud.token' });
    await openCloudRow({ kind: 'error', code: 'cloud.server' });
    expect(leaveGuest).not.toHaveBeenCalled();
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
