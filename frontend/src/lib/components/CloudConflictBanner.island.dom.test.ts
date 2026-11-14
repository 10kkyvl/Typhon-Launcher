import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { loadCloudStatus, type CloudConflict, type CloudGameStatus, type CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict, cloudStatus, fetchCloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { liveIsland, liveIslandMessages } from '../stores/liveIsland';
import { resetHistory } from '../stores/router';
import { holdToasts, toasts } from '../stores/toasts';
import IslandShellHarness from '../testing/IslandShellHarness.svelte';
import CloudConflictBanner from './CloudConflictBanner.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  loadCloudStatus: vi.fn(),
}));

// What the island says is session memory: every test gets copies and a game of its own.
let copiesMade = 0;
const copies = (): CloudConflict => {
  const id = `banner-copy-${++copiesMade}`;
  return { localDigest: id, localUpdatedAt: '2026-11-01T10:00:00Z', localSize: 10, remoteDigest: `remote-${id}`, remoteUpdatedAt: '2026-11-02T10:00:00Z', remoteSize: 20, remoteDevice: 'Deck' };
};
const game = (gameId: string, state: CloudGameStatus['state'], conflict?: CloudConflict): CloudGameStatus => ({ gameId, state, code: '', message: '', at: '', syncedAt: '', conflict });
const cloud = (...games: CloudGameStatus[]): CloudStatus => ({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games });

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

let app: HTMLDivElement;
const offs: (() => void)[] = [];

// Every value the island's messages and the toasts take, so that a message said and taken back counts.
function record(): string[] {
  const said: string[] = [];
  offs.push(liveIslandMessages.subscribe((list) => said.push(...list.map((m) => m.id))));
  offs.push(toasts.subscribe((list) => said.push(...list.map((t) => `toast:${t.message}`))));
  return said;
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  toasts.set([]);
  holdToasts(false);
  liveIsland.clear();
  closeCloudConflict();
  cloudStatus.set(cloud());
  libraryGames.set(['Hades', 'Celeste', 'Tunic', 'Outer Wilds'].map((title, i) => ({ id: `g${i + 1}`, title }) as LibraryGame));
  resetHistory();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  render(IslandShellHarness, { target: app });
});

afterEach(() => {
  for (const off of offs.splice(0)) off();
  document.body.replaceChildren();
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  liveIsland.clear();
  resetHistory();
  vi.mocked(loadCloudStatus).mockReset();
  vi.unstubAllGlobals();
});

describe('a conflict of the page\'s game reported by an event', () => {
  it('is said by the banner alone: the island and the toasts never say it, before or after the details load', async () => {
    render(CloudConflictBanner, { target: app, props: { gameId: 'g1' } });
    await tick();
    const said = record();
    const details = deferred<CloudStatus>();
    vi.mocked(loadCloudStatus).mockReturnValueOnce(details.promise);

    // As initCloudSaves does it: the event's status, then a load for the details.
    cloudStatus.set(cloud(game('g1', 'conflict')));
    const loading = fetchCloudStatus();
    await tick();
    details.resolve(cloud(game('g1', 'conflict', copies())));
    await loading;
    await tick();

    expect(app.querySelector('[data-tone="warning"]')?.textContent).toContain('Deck');
    expect(said.filter((id) => id.includes('save-conflict') || id.startsWith('toast:'))).toEqual([]);
  });
});

describe('the page holding its game', () => {
  it('silences nothing of another game: a conflict of another game is said in the island', async () => {
    render(CloudConflictBanner, { target: app, props: { gameId: 'g1' } });
    await tick();

    cloudStatus.set(cloud(game('g2', 'conflict', copies())));
    await tick();

    expect(get(liveIslandMessages).map((m) => m.id)).toEqual(['save-conflict:g2']);
  });

  it('says a conflict the banner never showed once the page lets the game go', async () => {
    const view = render(CloudConflictBanner, { target: app, props: { gameId: 'g3' } });
    await tick();
    vi.mocked(loadCloudStatus).mockReturnValueOnce(new Promise(() => {}));
    cloudStatus.set(cloud(game('g3', 'conflict')));
    void fetchCloudStatus();
    await tick();
    expect(app.querySelector('[data-tone="warning"]')).toBeNull();
    expect(get(liveIslandMessages)).toEqual([]);

    view.unmount();
    await tick();

    expect(get(liveIslandMessages).map((m) => m.id)).toEqual(['save-conflict:g3']);
  });

  it('takes a conflict the banner showed as seen: nothing is said once the page lets the game go', async () => {
    cloudStatus.set(cloud(game('g4', 'conflict', copies())));
    const view = render(CloudConflictBanner, { target: app, props: { gameId: 'g4' } });
    await tick();
    expect(app.querySelector('[data-tone="warning"]')).not.toBeNull();

    view.unmount();
    await tick();

    expect(get(liveIslandMessages)).toEqual([]);
  });
});
