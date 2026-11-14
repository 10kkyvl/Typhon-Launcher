import { render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { cloudResolveConflict, loadCloudStatus, type CloudConflict, type CloudGameStatus, type CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict, cloudConflictTarget, cloudStatus, conflictsOnPage, fetchCloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { dateTime } from '../utils/format';
import CloudConflictBanner from './CloudConflictBanner.svelte';
import CloudConflictModal from './CloudConflictModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudResolveConflict: vi.fn(),
  loadCloudStatus: vi.fn(),
}));

const conflict: CloudConflict = {
  localDigest: 'a',
  localUpdatedAt: '2026-11-01T10:00:00Z',
  localSize: 10,
  remoteDigest: 'b',
  remoteUpdatedAt: '2026-11-02T12:00:00Z',
  remoteSize: 20,
  remoteDevice: 'Deck',
};

const game = (patch: Partial<CloudGameStatus> = {}): CloudGameStatus => ({ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '', conflict, ...patch });
const statusWith = (...games: CloudGameStatus[]): CloudStatus => ({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games });

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

let app: HTMLDivElement;
let heading: HTMLHeadingElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  heading = document.createElement('h1');
  heading.tabIndex = -1;
  heading.textContent = 'Hades';
  app.append(heading);
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
  vi.mocked(loadCloudStatus).mockReturnValue(new Promise(() => {}));
});

afterEach(() => {
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  vi.mocked(cloudResolveConflict).mockReset();
  vi.mocked(loadCloudStatus).mockReset();
  document.body.replaceChildren();
});

function renderBanner(gameId: string | undefined = 'g1') {
  return render(CloudConflictBanner, { target: app, props: { gameId, fallbackFocus: () => heading } });
}

const banner = () => screen.queryByRole('status');
const action = () => screen.getByRole('button', { name: msg('cloudsaves.resolve') });

describe('CloudConflictBanner: when it shows', () => {
  it('says the conflict with both dates and the cloud copy’s device', () => {
    cloudStatus.set(statusWith(game()));
    renderBanner();

    const notice = banner()?.closest('[data-tone]') as HTMLElement;
    expect(notice.dataset.tone).toBe('warning');
    expect(banner()?.textContent?.trim()).toBe(
      msg('cloudsaves.bannerWhenDevice', { local: dateTime(new Date(conflict.localUpdatedAt)), remote: dateTime(new Date(conflict.remoteUpdatedAt)), device: 'Deck' }),
    );
  });

  it('says the conflict in general words when the status has no details and none are loading', () => {
    cloudStatus.set(statusWith(game({ conflict: undefined })));
    renderBanner();

    expect(banner()?.textContent?.trim()).toBe(msg('cloudsaves.conflictTitle'));
  });

  it('shows nothing for a game without a conflict, another game’s conflict, or a page the library does not hold', () => {
    cloudStatus.set(statusWith(game({ state: 'synced', conflict: undefined }), game({ gameId: 'g2' })));
    renderBanner('g1');
    expect(banner()).toBeNull();
    document.body.replaceChildren(app);

    renderBanner(undefined);
    expect(banner()).toBeNull();
  });

  it('leaves the game to the page while it shows, and gives it back when it goes', async () => {
    cloudStatus.set(statusWith(game()));
    renderBanner();
    await tick();
    expect(get(conflictsOnPage).has('g1')).toBe(true);

    cloudStatus.set(statusWith(game({ state: 'synced', conflict: undefined })));
    await tick();

    expect(get(conflictsOnPage).has('g1')).toBe(false);
  });
});

describe('CloudConflictBanner: the status loading again', () => {
  it('waits for the details an event lacks, then comes once with them, and a later load says nothing new', async () => {
    cloudStatus.set(statusWith(game({ state: 'synced', conflict: undefined })));
    renderBanner();
    const first = deferred<CloudStatus>();
    vi.mocked(loadCloudStatus).mockReturnValueOnce(first.promise);

    // As initCloudSaves does it: the event, then a load for its details.
    cloudStatus.set(statusWith(game({ conflict: undefined })));
    const loading = fetchCloudStatus();
    await tick();
    expect(banner()).toBeNull();

    first.resolve(statusWith(game()));
    await loading;
    await tick();
    const region = banner();
    const words = region?.textContent;
    expect(words).toContain('Deck');

    const second = deferred<CloudStatus>();
    vi.mocked(loadCloudStatus).mockReturnValueOnce(second.promise);
    const again = fetchCloudStatus();
    await tick();
    second.resolve(statusWith(game()));
    await again;
    await tick();

    expect(banner()).toBe(region);
    expect(banner()?.textContent).toBe(words);
  });

  it('stays up while a load runs for a conflict it already shows without details', async () => {
    cloudStatus.set(statusWith(game({ conflict: undefined })));
    renderBanner();
    const region = banner();
    expect(region).not.toBeNull();
    vi.mocked(loadCloudStatus).mockReturnValueOnce(new Promise(() => {}));

    void fetchCloudStatus();
    await tick();

    expect(banner()).toBe(region);
  });
});

describe('CloudConflictBanner: its action and the dialog', () => {
  it('opens the conflict dialog for this game, and a dialog closed without a choice gives focus back to the action', async () => {
    cloudStatus.set(statusWith(game()));
    renderBanner();
    render(CloudConflictModal, { target: app });
    const keys = user();
    action().focus();

    await keys.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    expect(get(cloudConflictTarget)).toEqual({ gameId: 'g1', title: 'Hades' });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictLater') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(banner()).not.toBeNull();
    expect(document.activeElement).toBe(action());
  });

  it('goes as soon as a choice is applied, though the status still lists the conflict, and focus moves to the page heading', async () => {
    vi.mocked(cloudResolveConflict).mockResolvedValue();
    cloudStatus.set(statusWith(game()));
    renderBanner();
    render(CloudConflictModal, { target: app });
    const keys = user();
    action().focus();
    await keys.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(get(cloudStatus)?.games[0].state).toBe('conflict');
    expect(banner()).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });
});
