import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
// What the dialog asks Go to drop (`discard` a fetched torrent, `cancel` a fetch), and the refusal Go
// answers with, if any. The service functions are plain, not spies: a spy handles the promise it
// returns (to record how it settled), which would hide a rejection the dialog leaves unhandled.
const cleanup = vi.hoisted(() => ({
  discard: vi.fn<(hash: string) => void>(),
  cancel: vi.fn<(source: string) => void>(),
  refuse: { discard: null as Error | null, cancel: null as Error | null },
}));
vi.mock('../services/downloads', async (actual) => ({
  ...(await actual<typeof import('../services/downloads')>()),
  fetchMetadata: vi.fn(),
  discardMetadata: (hash: string) => {
    cleanup.discard(hash);
    return cleanup.refuse.discard ? Promise.reject(cleanup.refuse.discard) : Promise.resolve();
  },
  cancelFetchMetadata: (source: string) => {
    cleanup.cancel(source);
    return cleanup.refuse.cancel ? Promise.reject(cleanup.refuse.cancel) : Promise.resolve();
  },
  startDownloadFrom: vi.fn(async () => ({})),
}));
vi.mock('../services/system', async (actual) => ({
  ...(await actual<typeof import('../services/system')>()),
  appInfo: vi.fn(async () => ({ version: '1', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' })),
}));
vi.mock('../services/settings', async (actual) => ({
  ...(await actual<typeof import('../services/settings')>()),
  selectFolder: vi.fn(async () => ''),
}));
vi.mock('../stores/settings', async (actual) => {
  const { writable } = await import('svelte/store');
  return {
    ...(await actual<typeof import('../stores/settings')>()),
    settings: writable({ libraryPath: '/games', downloadsPath: '/downloads', autoInstall: false, elevateAhead: false }),
  };
});

import { get } from 'svelte/store';
import { locale, msg } from '../i18n';
import { installErrorText } from '../install/installErrors';
import {
  fetchMetadata,
  startDownloadFrom,
  type Download,
  type DownloadOrigin,
  type TorrentInfo,
} from '../services/downloads';
import { selectFolder } from '../services/settings';
import { toasts } from '../stores/toasts';
import { renderDom } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { sentToGo } from '../testing/gameDialogsTesting';
import { description } from '../testing/hints';
import AddDownloadModal from './AddDownloadModal.svelte';

const file = (path: string, size: number) => ({ path, size, selected: true, bytesDone: 0 });
const TORRENT: TorrentInfo = {
  infoHash: 'abc',
  name: 'Celeste',
  totalBytes: 3_000_000,
  files: [file('Celeste/setup.exe', 2_000_000), file('Celeste/readme.txt', 1_000_000)],
};

beforeEach(() => {
  locale.set('en');
  vi.mocked(fetchMetadata).mockResolvedValue(TORRENT);
  vi.mocked(startDownloadFrom).mockClear();
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

async function files() {
  const view = renderDom(AddDownloadModal, { open: true, initialSource: 'magnet:?xt=urn:btih:abc' });
  await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
  return view;
}

describe('AddDownloadModal files', () => {
  it('lists every file as a checkbox named by its path and described by its size, all ticked', async () => {
    await files();
    const setup = screen.getByRole('checkbox', { name: 'Celeste/setup.exe' });
    const readme = screen.getByRole('checkbox', { name: 'Celeste/readme.txt' });
    expect([setup, readme].map((box) => box.getAttribute('aria-checked'))).toEqual(['true', 'true']);
    expect(description(setup)).toMatch(/^2(?:[.,]0)?\s?MB$/);
  });

  it('Space unticks a file, Enter does not, and only the ticked files are started', async () => {
    const { user } = await files();
    const readme = screen.getByRole('checkbox', { name: 'Celeste/readme.txt' });
    readme.focus();

    await user.keyboard(' ');
    await user.keyboard('{Enter}');
    expect(readme.getAttribute('aria-checked')).toBe('false');

    await user.click(screen.getByRole('button', { name: 'Start download' }));

    await waitFor(() => expect(startDownloadFrom).toHaveBeenCalledOnce());
    expect(vi.mocked(startDownloadFrom).mock.calls[0].slice(0, 3)).toEqual(['abc', '/downloads', [0]]);
  });

  it('the install options are checkboxes with their descriptions; rights ahead appears with install for an installer', async () => {
    const { user } = await files();
    const install = screen.getByRole('checkbox', { name: 'Install after the download' });
    expect(install.getAttribute('aria-checked')).toBe('false');
    expect(description(install)).toBe('The default comes from settings; here it applies to this download only');
    expect(screen.queryByRole('checkbox', { name: 'Confirm administrator rights now' })).toBeNull();

    await user.click(install);

    expect(install.getAttribute('aria-checked')).toBe('true');
    const rights = await screen.findByRole('checkbox', { name: 'Confirm administrator rights now' });
    await user.click(rights);
    expect(rights.getAttribute('aria-checked')).toBe('true');
  });

  it('names the folder field by its label, and keeps the focus on "start", busy, while the download starts', async () => {
    let started!: () => void;
    vi.mocked(startDownloadFrom).mockReturnValueOnce(new Promise((resolve) => (started = () => resolve({} as never))));
    const { user } = await files();
    const folder = screen.getByRole('group', { name: 'Destination folder' });
    expect(within(folder).getByText('/downloads')).toBeTruthy();
    expect(screen.queryByRole('textbox', { name: 'Destination folder' })).toBeNull();
    const start = screen.getByRole('button', { name: 'Start download' });
    start.focus();

    await user.keyboard('{Enter}');

    expect(start.getAttribute('aria-busy')).toBe('true');
    expect(start).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(start);
    started();
  });
});

const MAGNET = 'magnet:?xt=urn:btih:abc';
const NOTHING_WAITING = 'typhon:download.metadata_required: сначала получите метаданные торрента: typhon:download.no_client: клиент заменён';
const FETCH_FAILED = 'typhon:download.no_metadata: метаданные не пришли';
const GAME_ORIGIN: DownloadOrigin = { gameId: 'g1', sourceId: 's1', distributionId: 'd1', releaseId: 'r1', title: 'Celeste' };

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

// Answers queued by one case (a fetch the dialog never made) must not reach the next.
function freshMocks() {
  vi.mocked(fetchMetadata).mockReset();
  vi.mocked(fetchMetadata).mockResolvedValue(TORRENT);
  vi.mocked(startDownloadFrom).mockReset();
  vi.mocked(startDownloadFrom).mockResolvedValue({} as Download);
  cleanup.discard.mockClear();
  cleanup.cancel.mockClear();
  cleanup.refuse.discard = null;
  cleanup.refuse.cancel = null;
}

const startButton = () => screen.getByRole('button', { name: msg('modals.addDownloadStart') });
const fileBox = (path: string) => screen.getByRole('checkbox', { name: path });

describe('AddDownloadModal: a Start that finds nothing waiting in Go', () => {
  beforeEach(() => {
    freshMocks();
    vi.mocked(selectFolder).mockResolvedValue('/elsewhere');
  });

  afterEach(() => {
    toasts.set([]);
  });

  // Untick the readme, choose another folder, turn on the install: the choices a fetch must not lose.
  async function choose(props: { origin?: DownloadOrigin } = {}) {
    const view = renderDom(AddDownloadModal, { open: true, initialSource: MAGNET, ...props });
    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    await view.user.click(fileBox('Celeste/readme.txt'));
    await view.user.click(screen.getByRole('button', { name: msg('modals.addDownloadBrowse') }));
    await waitFor(() => expect(within(screen.getByRole('group', { name: msg('modals.addDownloadDestinationLabel') })).getByText('/elsewhere')).toBeTruthy());
    await view.user.click(screen.getByRole('checkbox', { name: msg('modals.addDownloadAutoInstallLabel') }));
    return view;
  }

  it('fetches the same source once, shows the loading step, and lands on the files with the choices kept, Start focused', async () => {
    const refetch = deferred<TorrentInfo>();
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockReturnValueOnce(refetch.promise);
    startButton().focus();

    await user.keyboard('{Enter}');

    const loading = await screen.findByText(msg('modals.addDownloadFetchingMetadata'));
    expect(loading.closest('[role="status"]')).not.toBeNull();
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
    expect(vi.mocked(fetchMetadata).mock.calls[1]).toEqual([MAGNET]);
    const held = document.activeElement;
    expect(held === document.body || !held?.isConnected).toBe(false);
    expect(screen.getByRole('dialog').closest('[tabindex]')?.contains(held)).toBe(true);

    refetch.resolve(TORRENT);

    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    expect(fileBox('Celeste/setup.exe').getAttribute('aria-checked')).toBe('true');
    expect(fileBox('Celeste/readme.txt').getAttribute('aria-checked')).toBe('false');
    expect(within(screen.getByRole('group', { name: msg('modals.addDownloadDestinationLabel') })).getByText('/elsewhere')).toBeTruthy();
    expect(screen.getByRole('checkbox', { name: msg('modals.addDownloadAutoInstallLabel') }).getAttribute('aria-checked')).toBe('true');
    const note = screen.getByText(msg('modals.addDownloadRestartNeeded'));
    expect(note.closest('.notice')).not.toBeNull();
    expect(get(toasts)).toHaveLength(0);
    await waitFor(() => expect(document.activeElement).toBe(startButton()));
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
  });

  it('keeps the choices by path when the fetch brings the files back in another order', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockResolvedValueOnce({ ...TORRENT, files: [...TORRENT.files].reverse() });

    await user.click(startButton());
    await screen.findByText(msg('modals.addDownloadRestartNeeded'));

    expect(fileBox('Celeste/readme.txt').getAttribute('aria-checked')).toBe('false');
    expect(fileBox('Celeste/setup.exe').getAttribute('aria-checked')).toBe('true');
    await user.click(startButton());
    await waitFor(() => expect(startDownloadFrom).toHaveBeenCalledTimes(2));
    expect(vi.mocked(startDownloadFrom).mock.calls[1][2]).toEqual([1]);
  });

  it('leaves the focus where the player moved it while the files were fetched', async () => {
    const refetch = deferred<TorrentInfo>();
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockReturnValueOnce(refetch.promise);
    await user.click(startButton());
    await screen.findByText(msg('modals.addDownloadFetchingMetadata'));
    const close = screen.getByRole('button', { name: msg('common.close') });
    close.focus();

    refetch.resolve(TORRENT);

    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    for (let i = 0; i < 10; i += 1) await Promise.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(close);
  });

  it('sends the second Start with the kept choices and the origin, and closes when it succeeds', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose({ origin: GAME_ORIGIN });
    await user.click(startButton());
    await waitFor(() => expect(screen.getByText(msg('modals.addDownloadRestartNeeded'))).toBeTruthy());

    await user.click(startButton());

    await waitFor(() => expect(startDownloadFrom).toHaveBeenCalledTimes(2));
    const [first, second] = vi.mocked(startDownloadFrom).mock.calls;
    expect(second).toEqual(first);
    expect(second).toEqual(['abc', '/elsewhere', [0], { ...GAME_ORIGIN, autoInstall: true, elevateAhead: false }]);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(cleanup.discard).not.toHaveBeenCalled();
  });

  it('when that fetch fails too: the source step, the failure by its code with a Retry, and no fetch of its own', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockRejectedValueOnce(new Error(FETCH_FAILED)).mockReturnValueOnce(new Promise(() => {}));

    await user.click(startButton());

    const alert = await screen.findByRole('alert');
    const notice = alert.closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');
    expect(alert.textContent).toContain(msg('modals.addDownloadRefetchFailed'));
    expect(alert.textContent).toContain(installErrorText(new Error(FETCH_FAILED)));
    expect(within(notice).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(screen.getByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') })).toHaveProperty('value', MAGNET);
    for (let i = 0; i < 10; i += 1) await Promise.resolve();
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
  });

  it('Retry fetches the source again with the choices kept, and focuses Start', async () => {
    const again = deferred<TorrentInfo>();
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockRejectedValueOnce(new Error(FETCH_FAILED)).mockReturnValueOnce(again.promise);
    await user.click(startButton());
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(vi.mocked(fetchMetadata).mock.calls[2]).toEqual([MAGNET]);

    again.resolve(TORRENT);

    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    expect(notice.isConnected).toBe(false);
    expect(fileBox('Celeste/readme.txt').getAttribute('aria-checked')).toBe('false');
    expect(within(screen.getByRole('group', { name: msg('modals.addDownloadDestinationLabel') })).getByText('/elsewhere')).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(startButton()));
  });

  // Retry fetches the remembered source whatever the field says now; closing must cancel that fetch,
  // or Go keeps it and its reserved hash until the metadata timeout.
  it('closing during a Retry cancels the source being fetched, not the edited field', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockRejectedValueOnce(new Error(FETCH_FAILED)).mockReturnValueOnce(new Promise(() => {}));
    await user.click(startButton());
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const field = screen.getByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') });
    await user.clear(field);
    await user.type(field, 'magnet:?xt=urn:btih:other');

    await user.click(retry);
    await waitFor(() => expect(fetchMetadata).toHaveBeenCalledTimes(3));
    expect(vi.mocked(fetchMetadata).mock.calls[2]).toEqual([MAGNET]);
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(cleanup.cancel).toHaveBeenCalledExactlyOnceWith(MAGNET);
  });

  it('keeps the notice when the Retry fails again, and says it again', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error(NOTHING_WAITING));
    const { user } = await choose();
    vi.mocked(fetchMetadata).mockRejectedValue(new Error(FETCH_FAILED));
    await user.click(startButton());
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const words = notice.querySelector('.message')?.textContent?.replace(/\s+/g, ' ').trim() ?? '';
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(said()).toBe(words));
    expect(words).not.toBe('');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(fetchMetadata).toHaveBeenCalledTimes(3);
  });
});

describe('AddDownloadModal: a fetch the player asked for that fails', () => {
  const OTHER = 'magnet:?xt=urn:btih:other';

  beforeEach(() => {
    freshMocks();
  });

  afterEach(() => {
    toasts.set([]);
  });

  // Continue on a typed magnet; the dialog fails the fetch with `first`.
  async function failContinue(first: Error) {
    vi.mocked(fetchMetadata).mockRejectedValueOnce(first);
    const view = renderDom(AddDownloadModal, { open: true });
    const field = await screen.findByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') });
    await view.user.type(field, MAGNET);
    await view.user.click(screen.getByRole('button', { name: msg('common.continue') }));
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    return { ...view, retry, notice: retry.closest('.notice') as HTMLElement };
  }

  it('says why on the source step, in the dialog, with a Retry, and raises no toast', async () => {
    const { notice } = await failContinue(new Error(FETCH_FAILED));

    const alert = within(notice).getByRole('alert');
    expect(notice.getAttribute('data-tone')).toBe('danger');
    expect(alert.textContent).toContain(installErrorText(new Error(FETCH_FAILED)));
    expect(screen.getByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') })).toHaveProperty('value', MAGNET);
    expect(get(toasts)).toHaveLength(0);
    expect(fetchMetadata).toHaveBeenCalledOnce();
  });

  it('titles the failure as a failed fetch, not as the fetch again after a Start', async () => {
    const { notice } = await failContinue(new Error(FETCH_FAILED));

    const alert = within(notice).getByRole('alert');
    expect(notice.querySelector('.title')?.textContent?.trim()).toBe(msg('modals.addDownloadFetchFailed'));
    expect(alert.textContent).toContain(msg('modals.addDownloadFetchFailed'));
    expect(alert.textContent).not.toContain(msg('modals.addDownloadRefetchFailed'));
  });

  it('opened with a source (the game page), says a failed fetch the same way', async () => {
    vi.mocked(fetchMetadata).mockRejectedValueOnce(new Error(FETCH_FAILED));
    renderDom(AddDownloadModal, { open: true, initialSource: MAGNET, origin: GAME_ORIGIN });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(installErrorText(new Error(FETCH_FAILED)));
    expect(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(get(toasts)).toHaveLength(0);
  });

  it('Retry fetches the failed source again, keeps the notice while it runs, and focuses Start on the files', async () => {
    const again = deferred<TorrentInfo>();
    const { user, retry, notice } = await failContinue(new Error(FETCH_FAILED));
    vi.mocked(fetchMetadata).mockReturnValueOnce(again.promise);
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(notice.textContent).toContain(installErrorText(new Error(FETCH_FAILED)));
    expect(document.activeElement).toBe(retry);
    expect(vi.mocked(fetchMetadata).mock.calls[1]).toEqual([MAGNET]);

    again.resolve(TORRENT);

    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    expect(notice.isConnected).toBe(false);
    expect(fileBox('Celeste/readme.txt').getAttribute('aria-checked')).toBe('true');
    await waitFor(() => expect(document.activeElement).toBe(startButton()));
  });

  it('keeps the notice when the Retry fails again, says it again, and keeps focus on Retry', async () => {
    const { user, retry, notice } = await failContinue(new Error(FETCH_FAILED));
    vi.mocked(fetchMetadata).mockRejectedValue(new Error(FETCH_FAILED));
    const words = notice.querySelector('.message')?.textContent?.replace(/\s+/g, ' ').trim() ?? '';
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(said()).toBe(words));
    expect(words).toContain(installErrorText(new Error(FETCH_FAILED)));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
    expect(get(toasts)).toHaveLength(0);
  });

  it('a new Continue clears the old failure and fetches what the field holds', async () => {
    const { user } = await failContinue(new Error(FETCH_FAILED));
    vi.mocked(fetchMetadata).mockReturnValueOnce(new Promise(() => {}));
    const field = screen.getByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') });
    await user.clear(field);
    await user.type(field, OTHER);

    await user.click(screen.getByRole('button', { name: msg('common.continue') }));

    await screen.findByText(msg('modals.addDownloadFetchingMetadata'));
    expect(screen.queryByRole('alert')).toBeNull();
    expect(vi.mocked(fetchMetadata).mock.calls[1]).toEqual([OTHER]);
  });
});

describe('AddDownloadModal: any other Start failure', () => {
  beforeEach(() => {
    freshMocks();
  });

  afterEach(() => {
    toasts.set([]);
  });

  it('keeps the files step with Start working: a second press is sent and can succeed', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error('typhon:download.not_enough_space: мало места'));
    const { user } = await files();

    await user.click(startButton());

    const folder = screen.getByRole('group', { name: msg('modals.addDownloadDestinationLabel') });
    const alert = await within(folder).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(installErrorText(new Error('typhon:download.not_enough_space')));
    expect(get(toasts)).toEqual([]);
    expect(startButton()).toHaveProperty('disabled', false);
    expect(startButton().getAttribute('aria-busy')).not.toBe('true');
    expect(fetchMetadata).toHaveBeenCalledTimes(1);

    await user.click(startButton());

    await waitFor(() => expect(startDownloadFrom).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});

type Proc = { on(event: 'unhandledRejection', fn: (reason: unknown) => void): void; off(event: 'unhandledRejection', fn: (reason: unknown) => void): void };
const proc = (globalThis as unknown as { process: Proc }).process;

// Rejections nobody handled, collected until the end of the case; a macrotask lets Node report them.
function unhandled() {
  const seen: unknown[] = [];
  const note = (reason: unknown) => void seen.push(reason);
  proc.on('unhandledRejection', note);
  onTestFinished(() => proc.off('unhandledRejection', note));
  return async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
    return seen;
  };
}

describe('AddDownloadModal: a cleanup Go refuses', () => {
  const DISCARD_FAILED = new Error('typhon:download.no_client: discard refused');
  const CANCEL_FAILED = new Error('typhon:download.no_client: cancel refused');

  beforeEach(() => {
    freshMocks();
  });

  afterEach(() => {
    toasts.set([]);
  });

  it('a discard refused on close is logged, not left unhandled, and the dialog closes', async () => {
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
    onTestFinished(() => errors.mockRestore());
    const after = unhandled();
    cleanup.refuse.discard = DISCARD_FAILED;
    const { user } = await files();

    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(cleanup.discard).toHaveBeenCalledWith('abc');
    expect(await after()).toEqual([]);
    expect(errors.mock.calls.some((call) => call.includes(DISCARD_FAILED))).toBe(true);
    expect(get(toasts)).toHaveLength(0);
  });

  it('a cancel refused on close during a fetch is logged, not left unhandled', async () => {
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
    onTestFinished(() => errors.mockRestore());
    const after = unhandled();
    cleanup.refuse.cancel = CANCEL_FAILED;
    vi.mocked(fetchMetadata).mockReturnValueOnce(new Promise(() => {}));
    const { user } = renderDom(AddDownloadModal, { open: true, initialSource: MAGNET });
    await screen.findByText(msg('modals.addDownloadFetchingMetadata'));

    await user.keyboard('{Escape}');

    expect(cleanup.cancel).toHaveBeenCalledWith(MAGNET);
    expect(await after()).toEqual([]);
    expect(errors.mock.calls.some((call) => call.includes(CANCEL_FAILED))).toBe(true);
  });

  it('a discard refused for a late answer is logged, not left unhandled', async () => {
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
    onTestFinished(() => errors.mockRestore());
    const after = unhandled();
    const fetching = deferred<TorrentInfo>();
    vi.mocked(fetchMetadata).mockReturnValueOnce(fetching.promise);
    cleanup.refuse.discard = DISCARD_FAILED;
    const { user } = renderDom(AddDownloadModal, { open: true, initialSource: MAGNET });
    await screen.findByText(msg('modals.addDownloadFetchingMetadata'));
    await user.keyboard('{Escape}');

    fetching.resolve(TORRENT);

    await waitFor(() => expect(cleanup.discard).toHaveBeenCalledWith('abc'));
    expect(await after()).toEqual([]);
    expect(errors.mock.calls.some((call) => call.includes(DISCARD_FAILED))).toBe(true);
  });
});

describe('AddDownloadModal: closing and starting', () => {
  beforeEach(() => {
    freshMocks();
  });

  it('closing during a fetch cancels it, discards what it brings later, and the late answer changes nothing', async () => {
    const fetching = deferred<TorrentInfo>();
    vi.mocked(fetchMetadata).mockReturnValueOnce(fetching.promise);
    const { user, rerender } = renderDom(AddDownloadModal, { open: true, initialSource: MAGNET });
    await screen.findByText(msg('modals.addDownloadFetchingMetadata'));

    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(cleanup.cancel).toHaveBeenCalledWith(MAGNET);

    vi.mocked(fetchMetadata).mockReturnValueOnce(new Promise(() => {}));
    await rerender({ open: true, initialSource: MAGNET });
    await screen.findByText(msg('modals.addDownloadFetchingMetadata'));

    fetching.resolve(TORRENT);

    await waitFor(() => expect(cleanup.discard).toHaveBeenCalledWith('abc'));
    expect(screen.queryByRole('checkbox', { name: 'Celeste/setup.exe' })).toBeNull();
    expect(screen.getByText(msg('modals.addDownloadFetchingMetadata'))).toBeTruthy();
  });

  it('closing on the files step discards the fetched torrent', async () => {
    const { user } = await files();

    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(cleanup.discard).toHaveBeenCalledWith('abc');
  });

  it('cannot be closed while it starts', async () => {
    const starting = deferred<Download>();
    vi.mocked(startDownloadFrom).mockReturnValueOnce(starting.promise);
    const { user } = await files();
    await user.click(startButton());

    await user.keyboard('{Escape}');

    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(cleanup.discard).not.toHaveBeenCalled();
    starting.resolve({} as Download);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});

describe('AddDownloadModal: the fields', () => {
  beforeEach(() => {
    freshMocks();
  });

  afterEach(() => {
    toasts.set([]);
  });

  const continueButton = () => screen.getByRole('button', { name: msg('common.continue') });

  it('sends nothing while a link is typed or pasted, and fetches once on Continue', async () => {
    const { user } = renderDom(AddDownloadModal, { open: true });
    const field = screen.getByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') });

    await user.click(field);
    await user.paste('magnet:?xt=urn:btih:');
    await user.type(field, 'abc');

    expect(fetchMetadata).not.toHaveBeenCalled();
    await user.click(continueButton());
    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    expect(vi.mocked(fetchMetadata).mock.calls).toEqual([[MAGNET]]);
  });

  it('holds Continue with a reason until the field holds a magnet link', async () => {
    const { user } = renderDom(AddDownloadModal, { open: true });
    const field = screen.getByRole('textbox', { name: msg('modals.addDownloadMagnetLabel') });
    expect(continueButton().getAttribute('aria-disabled')).toBe('true');
    expect(description(continueButton())).toBe(msg('modals.addDownloadNeedsMagnet'));

    await user.type(field, 'https://example.org/x.torrent');
    await user.click(continueButton());
    expect(fetchMetadata).not.toHaveBeenCalled();

    await user.clear(field);
    await user.type(field, MAGNET);
    expect(continueButton().getAttribute('aria-disabled')).toBeNull();
  });

  it('shows the destination as text on the path field, read on Browse', async () => {
    await files();
    const browse = screen.getByRole('button', { name: msg('modals.addDownloadBrowse') });
    expect(description(browse)).toContain('/downloads');
  });

  it('says a refusal that is not about the folder in the dialog, by its code', async () => {
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error('typhon:download.duplicate_download: уже есть'));
    const { user } = await files();
    startButton().focus();
    await user.keyboard('{Enter}');

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errInstall.downloadDuplicateDownload'));
    await waitFor(() => expect(document.activeElement).toBe(alert.closest('[data-tone="danger"]')));
    expect(get(toasts)).toEqual([]);

    // The torrent still waits in Go for a Start: closing the dialog now discards it.
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    expect(cleanup.discard).toHaveBeenCalledWith('abc');
  });

  it('holds Cancel and Browse with a reason while the download starts', async () => {
    const starting = deferred<Download>();
    vi.mocked(startDownloadFrom).mockReturnValueOnce(starting.promise);
    const { user } = await files();
    await user.click(startButton());

    for (const name of [msg('common.cancel'), msg('modals.addDownloadBrowse')]) {
      const button = screen.getByRole('button', { name });
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(description(button)).toContain(msg('modals.actionRunning'));
    }
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(cleanup.discard).not.toHaveBeenCalled();
    starting.resolve({} as Download);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});

describe('AddDownloadModal: a Start answered after the dialog moved on', () => {
  beforeEach(() => {
    freshMocks();
  });

  afterEach(() => {
    toasts.set([]);
  });

  async function startThenReopen() {
    const starting = deferred<Download>();
    vi.mocked(startDownloadFrom).mockReturnValueOnce(starting.promise);
    const view = await files();
    await view.user.click(startButton());
    await view.rerender({ open: false, initialSource: MAGNET });
    vi.mocked(fetchMetadata).mockResolvedValueOnce({ ...TORRENT, infoHash: 'def', name: 'Hades', files: [file('Hades/setup.exe', 5)] });
    await view.rerender({ open: true, initialSource: 'magnet:?xt=urn:btih:def' });
    await screen.findByRole('checkbox', { name: 'Hades/setup.exe' });
    // The torrent being started is the Start's: closing the dialog meanwhile does not discard it.
    expect(cleanup.discard).not.toHaveBeenCalledWith('abc');
    return { starting, view };
  }

  it('a late success leaves the new opening open, with its own torrent kept', async () => {
    const { starting, view } = await startThenReopen();
    cleanup.discard.mockClear();

    starting.resolve({} as Download);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(screen.getByRole('checkbox', { name: 'Hades/setup.exe' })).toBeTruthy();
    await view.user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    await waitFor(() => expect(cleanup.discard).toHaveBeenCalledWith('def'));
  });

  it('a late refusal is said once in a toast naming the torrent, not in the new opening', async () => {
    const { starting } = await startThenReopen();

    starting.reject(new Error('typhon:download.destination_permission: нет доступа'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.queryByRole('alert')).toBeNull();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [msg('modals.addDownloadRefusedFor', { title: 'Celeste', reason: msg('errInstall.downloadDestinationPermission') }), 'danger'],
    ]);
    expect(cleanup.discard).toHaveBeenCalledWith('abc');
  });
});

describe('AddDownloadModal: what a running Start sent is what the files step shows', () => {
  beforeEach(() => {
    freshMocks();
  });

  const shown = (box: HTMLElement) => box.getAttribute('aria-checked');
  const sentOptions = () =>
    vi.mocked(startDownloadFrom).mock.calls.map((call) => ({ files: call[2], ...(call[3] as { autoInstall?: boolean; elevateAhead?: boolean }) }));

  it('holds the file boxes and "Install after the download" with a reason while the Start runs', async () => {
    const starting = deferred<Download>();
    vi.mocked(startDownloadFrom).mockReturnValueOnce(starting.promise);
    const { user } = await files();
    await user.click(startButton());
    const install = screen.getByRole('checkbox', { name: msg('modals.addDownloadAutoInstallLabel') });
    const setup = fileBox('Celeste/setup.exe');

    await user.click(install);
    await user.click(setup);

    expect({ install: shown(install), setup: shown(setup) }).toEqual({ install: 'false', setup: 'true' });
    expect(sentOptions()).toEqual([{ files: [0, 1], autoInstall: false, elevateAhead: false }]);
    for (const box of [install, setup, fileBox('Celeste/readme.txt')]) {
      expect(box.getAttribute('aria-disabled')).toBe('true');
      expect(description(box)).toContain(msg('modals.actionRunning'));
    }
    starting.resolve({} as Download);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('holds "Confirm administrator rights now" while the Start runs, and shows what it sent', async () => {
    const starting = deferred<Download>();
    vi.mocked(startDownloadFrom).mockReturnValueOnce(starting.promise);
    const { user } = await files();
    await user.click(screen.getByRole('checkbox', { name: msg('modals.addDownloadAutoInstallLabel') }));
    const rights = await screen.findByRole('checkbox', { name: msg('modals.addDownloadElevateAheadLabel') });
    await user.click(rights);
    await user.click(startButton());

    await user.click(rights);

    expect(shown(rights)).toBe('true');
    expect(sentOptions()).toEqual([{ files: [0, 1], autoInstall: true, elevateAhead: true }]);
    expect(rights.getAttribute('aria-disabled')).toBe('true');
    starting.resolve({} as Download);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('holds Start with a reason while no file is ticked, and sends nothing for a press', async () => {
    const { user } = await files();
    await user.click(fileBox('Celeste/setup.exe'));
    await user.click(fileBox('Celeste/readme.txt'));
    const start = startButton();

    expect(start.getAttribute('aria-disabled')).toBe('true');
    expect(start).toHaveProperty('disabled', false);
    expect(description(start)).toContain(msg('modals.addDownloadNoFiles'));
    await user.click(start);
    expect(startDownloadFrom).not.toHaveBeenCalled();

    await user.click(fileBox('Celeste/readme.txt'));
    expect(start.getAttribute('aria-disabled')).toBeNull();
  });
});
