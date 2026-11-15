import { screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  removeDownload: vi.fn(),
}));
vi.mock('../../lib/services/settings', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/settings')>()),
  openFolder: vi.fn(),
}));
vi.mock('../../lib/services/install', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/install')>()),
  retryInstall: vi.fn(),
}));
vi.mock('../../lib/services/library', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/library')>()),
  playGame: vi.fn(),
}));

import { libraryGames, runningGames } from '../../lib/stores/library';
import { locale } from '../../lib/i18n';
import { installErrorText } from '../../lib/install/installErrors';
import { removeDownload, type Download } from '../../lib/services/downloads';
import { retryInstall, type Installation } from '../../lib/services/install';
import { playGame, type LibraryGame } from '../../lib/services/library';
import { openFolder } from '../../lib/services/settings';
import { downloads } from '../../lib/stores/downloads';
import { installations } from '../../lib/stores/install';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize, clockTime, longDate, speedBytes } from '../../lib/utils/format';
import CompletedSection from './CompletedSection.svelte';

const MB = 1024 * 1024;
// 18:06 on 28 October 2026, local time.
const NOW = new Date(2026, 9, 28, 18, 6);

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'completed',
    progress: 1,
    downloaded: 1000 * MB,
    total: 1000 * MB,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-10-28T10:00:00Z',
    completedAt: new Date(2026, 9, 28, 15, 0).toISOString(),
    error: '',
    origin: { gameId: 'g1' },
    ...patch,
  };
}

function installation(patch: Partial<Installation> = {}): Installation {
  return {
    id: 'i1',
    downloadId: 'd1',
    gameId: 'lib1',
    name: 'Game One',
    type: 'archive_zip',
    status: 'extracting',
    mode: 'move',
    sourcePath: '',
    contentRoot: '',
    destination: '',
    installerPath: '',
    workingDir: '',
    archivePath: '',
    progress: 0.42,
    currentFile: '',
    bytesDone: 42,
    bytesTotal: 100,
    executable: '',
    candidates: null,
    detectedVersion: '',
    versionSource: '',
    startedAt: '2026-10-28T15:01:00Z',
    completedAt: null,
    error: '',
    engine: '',
    silent: false,
    ...patch,
  };
}

/** Installations that finished, one per download id: rows that need nothing more from the screen. */
function installed(...ids: string[]): Installation[] {
  return ids.map((id) => installation({ id: `i-${id}`, downloadId: id, status: 'completed' }));
}

function libraryGame(patch: Partial<LibraryGame> = {}): LibraryGame {
  return { id: 'lib1', title: 'Game One', executable: 'game.exe', uninstalled: false, tags: [], ...patch } as unknown as LibraryGame;
}

let heading: HTMLElement;

/** The two completed sections as the page mounts them, "Installing" above "Completed", with one set of handlers. */
function show(list: Download[], installs: Installation[] = [], props: { onannounce?: (text: string) => void } = {}) {
  downloads.set(list);
  installations.set(installs);
  const onopen = vi.fn();
  const oninstall = vi.fn();
  const installing = renderDom(CompletedSection, { part: 'installing', onopen, oninstall, ...props });
  const done = renderDom(CompletedSection, { part: 'completed', onopen, oninstall, ...props });
  const unmount = () => {
    installing.unmount();
    done.unmount();
  };
  return { user: done.user, unmount, onopen, oninstall };
}

/** The row of a download, found by its title. */
function row(name: string): HTMLElement {
  const item = screen.getByText(name, { selector: '.title, .title *' }).closest('li');
  if (!item) throw new Error(`no row for ${name}`);
  return item;
}

const more = (name: string) => within(row(name)).getByRole('button', { name: `More actions for “${name}”` });
const section = (name: string) => screen.getByRole('region', { name: new RegExp(`^${name}`) });

function goRemoves(id: string) {
  downloads.update((list) => list.filter((d) => d.id !== id));
}

function deferred() {
  let resolve: () => void = () => {};
  const promise = new Promise<void>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

async function pickFromMenu(view: ReturnType<typeof show>, name: string, item: string) {
  const button = more(name);
  button.focus();
  await view.user.keyboard('{Enter}');
  const entry = screen.getByRole('menuitem', { name: item });
  entry.focus();
  await view.user.keyboard('{Enter}');
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(NOW);
  locale.set('en');
  toasts.set([]);
  libraryGames.set([]);
  runningGames.set(new Set());
  heading = document.createElement('h1');
  heading.tabIndex = -1;
  heading.textContent = 'Downloads';
  document.body.append(heading);
  for (const fn of [removeDownload, openFolder, retryInstall, playGame]) {
    vi.mocked(fn).mockReset();
    vi.mocked(fn).mockResolvedValue(undefined);
  }
});

afterEach(() => {
  vi.useRealTimers();
  downloads.set([]);
  installations.set([]);
  libraryGames.set([]);
  heading.remove();
  resetHistory();
});

describe('completed downloads: the two parts', () => {
  it('lists what is installing or waits for the player under "Installing", the rest under "Completed"', () => {
    show(
      [
        download({ id: 'a', name: 'Extracting One' }),
        download({ id: 'b', name: 'Waiting One' }),
        download({ id: 'c', name: 'Ready One' }),
        download({ id: 'd', name: 'Installed One' }),
      ],
      [
        installation({ id: 'ia', downloadId: 'a' }),
        installation({ id: 'ib', downloadId: 'b', status: 'waiting_for_user' }),
        installation({ id: 'id', downloadId: 'd', status: 'completed' }),
      ],
    );

    const installing = section('Installing');
    const done = section('Completed');
    expect(within(installing).getByRole('heading', { level: 2 }).textContent).toMatch(/Installing\s*2/);
    expect(within(done).getByRole('heading', { level: 2 }).textContent).toMatch(/Completed\s*2/);
    expect(within(installing).getAllByRole('listitem').map((li) => li.querySelector('.title')?.textContent?.trim())).toEqual([
      'Extracting One',
      'Waiting One',
    ]);
    expect(within(done).getAllByRole('listitem').map((li) => li.querySelector('.title')?.textContent?.trim())).toEqual([
      'Ready One',
      'Installed One',
    ]);
  });
});

describe('completed downloads: each state of a row', () => {
  it('installing: the stage and the percent in words, a bar named after the download, Details opens the install dialog', async () => {
    const view = show([download()], [installation()]);
    const own = row('Game One');

    expect(own.textContent).toContain('Extracting, 42%');
    const bar = within(own).getByRole('progressbar', { name: 'Installing “Game One”' });
    expect(bar.getAttribute('aria-valuenow')).toBe('42');
    await view.user.click(within(own).getByRole('button', { name: 'Installation details for “Game One”' }));
    expect(view.oninstall).toHaveBeenCalledWith('d1');
  });

  it('installing with no known total: the bar draws an unknown amount and the row claims no percent', () => {
    show([download()], [installation({ status: 'installing', bytesTotal: 0, silent: true })]);
    const own = row('Game One');

    const bar = within(own).getByRole('progressbar');
    expect(bar.getAttribute('aria-busy')).toBe('true');
    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(own.textContent).not.toContain('%');
  });

  it('waiting for the player: says so, and Continue leads to the install dialog', async () => {
    const view = show([download()], [installation({ status: 'waiting_for_user' })]);
    const own = row('Game One');

    expect(own.textContent).toContain('The installer is waiting for your choice');
    await view.user.click(within(own).getByRole('button', { name: 'Continue installing “Game One”' }));
    expect(view.oninstall).toHaveBeenCalledWith('d1');
  });

  it('failed: what failed by its code, and Retry asks Go to install again', async () => {
    const view = show([download()], [installation({ status: 'failed', error: 'typhon:install.no_free_space: no room' })]);
    const own = row('Game One');

    expect(own.textContent).toContain('Installation failed:');
    expect(own.textContent).not.toContain('typhon:');
    expect(own.textContent).not.toContain('no room');
    await view.user.click(within(own).getByRole('button', { name: 'Retry installing “Game One”' }));
    expect(retryInstall).toHaveBeenCalledWith('i1');
  });

  it('a failed retry is said by its code and leaves the row as Go has it', async () => {
    vi.mocked(retryInstall).mockRejectedValueOnce(new Error('typhon:install.no_free_space: no room'));
    const view = show([download()], [installation({ status: 'failed', error: 'typhon:install.no_free_space' })]);
    const retry = within(row('Game One')).getByRole('button', { name: 'Retry installing “Game One”' });
    retry.focus();

    await view.user.keyboard('{Enter}');

    await waitFor(() => expect(get(toasts)).toHaveLength(1));
    expect(get(toasts)[0].kind).toBe('danger');
    expect(get(toasts)[0].message).not.toContain('typhon:');
    expect(document.activeElement).toBe(retry);
  });

  it('a retry pressed twice sends one request, the button busy and focused while it runs', async () => {
    const gate = deferred();
    vi.mocked(retryInstall).mockImplementation(() => gate.promise);
    const view = show([download()], [installation({ status: 'failed' })]);
    const retry = within(row('Game One')).getByRole('button', { name: 'Retry installing “Game One”' });
    retry.focus();

    await view.user.keyboard('{Enter}');
    await view.user.keyboard('{Enter}');

    expect(retryInstall).toHaveBeenCalledTimes(1);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    gate.resolve();
    await waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
  });

  it('a Retry that moves the row to "Installing" takes focus along, to the row’s Details there', async () => {
    let installLater = () => {};
    vi.mocked(retryInstall).mockImplementation(async (id) => {
      installLater = () => installations.update((list) => list.map((i) => (i.id === id ? { ...i, status: 'extracting' } : i)));
    });
    const view = show(
      [download(), download({ id: 'd2', name: 'Game Two' })],
      [installation({ status: 'failed' })],
    );
    const retry = within(row('Game One')).getByRole('button', { name: 'Retry installing “Game One”' });
    retry.focus();

    await view.user.keyboard('{Enter}');
    await waitFor(() => expect(retryInstall).toHaveBeenCalled());
    installLater();

    await waitFor(() => expect(within(section('Installing')).queryByText('Game One')).not.toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(within(section('Installing')).getByRole('button', { name: 'Installation details for “Game One”' })),
    );
  });

  it('cancelled and interrupted say which, and offer to install again', async () => {
    const view = show(
      [download(), download({ id: 'd2', name: 'Game Two' })],
      [installation({ status: 'cancelled' }), installation({ id: 'i2', downloadId: 'd2', status: 'interrupted' })],
    );

    expect(row('Game One').textContent).toContain('Installation cancelled');
    expect(row('Game Two').textContent).toContain('Installation interrupted');
    await view.user.click(within(row('Game Two')).getByRole('button', { name: 'Install “Game Two” again' }));
    expect(view.oninstall).toHaveBeenCalledWith('d2');
  });

  it('ready: Install opens the install dialog', async () => {
    const view = show([download()]);

    await view.user.click(within(row('Game One')).getByRole('button', { name: 'Install “Game One”' }));
    expect(view.oninstall).toHaveBeenCalledWith('d1');
  });

  it('an update the updates apply offers no Install', () => {
    show([download({ origin: { gameId: 'g1', purpose: 'update' } })]);

    expect(within(row('Game One')).queryByRole('button', { name: /Install/ })).toBeNull();
  });

  it('update and repair files say what they are, by the download’s purpose, and the title leads to the game', async () => {
    const view = show([
      download({ origin: { gameId: 'g1', purpose: 'update' } }),
      download({ id: 'd2', name: 'Game Two', origin: { gameId: 'g2', purpose: 'repair' } }),
    ]);

    expect(row('Game One').querySelector('.state')?.textContent?.trim()).toBe('Update files for the game');
    expect(row('Game Two').querySelector('.state')?.textContent?.trim()).toBe('Repair files for the game');
    await view.user.click(within(row('Game One')).getByRole('button', { name: 'Game One' }));
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
  });

  it('installed: says so with the time, and Play launches the library game through the shared launch', async () => {
    libraryGames.set([libraryGame()]);
    const view = show([download()], [installation({ status: 'completed', completedAt: new Date(2026, 9, 28, 15, 30).toISOString() })]);
    const own = row('Game One');

    expect(own.textContent).toContain('Installed');
    expect(own.textContent).toContain(`installed today, ${clockTime(new Date(2026, 9, 28, 15, 30))}`);
    await view.user.click(within(own).getByRole('button', { name: 'Play “Game One”' }));
    expect(playGame).toHaveBeenCalledWith('lib1');
  });

  it('Play pressed twice launches once, busy and focused while the launch runs', async () => {
    const gate = deferred();
    vi.mocked(playGame).mockImplementation(() => gate.promise);
    libraryGames.set([libraryGame()]);
    const view = show([download()], [installation({ status: 'completed' })]);
    const play = within(row('Game One')).getByRole('button', { name: 'Play “Game One”' });
    play.focus();

    await view.user.keyboard('{Enter}');
    await view.user.keyboard('{Enter}');

    expect(playGame).toHaveBeenCalledTimes(1);
    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(play);
    gate.resolve();
    await waitFor(() => expect(play.hasAttribute('aria-busy')).toBe(false));
  });

  it('offers no Play for a game that runs, has no executable or is not in the library', async () => {
    libraryGames.set([libraryGame()]);
    runningGames.set(new Set(['lib1']));
    show([download()], [installation({ status: 'completed' })]);
    expect(row('Game One').textContent).toContain('Game running');
    expect(within(row('Game One')).queryByRole('button', { name: /Play/ })).toBeNull();

    runningGames.set(new Set());
    libraryGames.set([libraryGame({ executable: '' })]);
    await tick();
    expect(row('Game One').textContent).not.toContain('Game running');
    expect(within(row('Game One')).queryByRole('button', { name: /Play/ })).toBeNull();

    libraryGames.set([]);
    await tick();
    expect(within(row('Game One')).queryByRole('button', { name: /Play/ })).toBeNull();

    // The same row offers Play once the game can run: the checks above saw the page updated.
    libraryGames.set([libraryGame()]);
    await waitFor(() => expect(within(row('Game One')).getByRole('button', { name: 'Play “Game One”' })).toBeTruthy());
  });

  it('seeding: says so, with the upload speed while it moves', () => {
    show([download({ seeding: true, uploadSpeed: MB }), download({ id: 'd2', name: 'Game Two', seeding: true })]);

    expect(row('Game One').textContent).toContain(`Seeding, ${speedBytes(MB)}`);
    expect(row('Game Two').textContent).toContain('Seeding');
    expect(row('Game Two').textContent).not.toContain(speedBytes(0));
  });

  it('the size and when it finished in words; the exact time for assistive tech and on hover, never a native title', async () => {
    const view = show([download()]);
    const own = row('Game One');
    const finished = new Date(2026, 9, 28, 15, 0);
    const exact = `Downloaded on ${longDate(finished)} at ${clockTime(finished)}`;

    expect(own.textContent).toContain(bytesSize(1000 * MB));
    const when = own.querySelector('time');
    expect(when?.getAttribute('datetime')).toBe(finished.toISOString());
    expect(when?.querySelector('[aria-hidden="true"]')?.textContent).toBe(`today, ${clockTime(finished)}`);
    expect(when?.querySelector('.sr-only')?.textContent).toBe(exact);
    expect(document.querySelector('[title]')).toBeNull();

    await view.user.hover(when as HTMLElement);
    await waitFor(() => expect(document.querySelector('[data-part="tooltip"]')?.textContent?.trim()).toBe(exact));
  });

  it('the title and the cover lead to the game page; nothing in the row is a role=button wrapper', () => {
    show([download()]);
    const own = row('Game One');

    expect(within(own).getByRole('button', { name: 'Game One' }).classList.contains('text-link')).toBe(true);
    expect(own.querySelector('[role="button"]')).toBeNull();
    expect(own.hasAttribute('tabindex')).toBe(false);
  });
});

describe('completed downloads: the row menu', () => {
  it('names its download, offers Details, Open folder and Remove from list, and nothing named Cancel', async () => {
    const view = show([download(), download({ id: 'd2', name: 'Game Two', seeding: true })]);

    await view.user.click(more('Game One'));
    expect(screen.getAllByRole('menuitem').map((item) => item.textContent?.trim())).toEqual([
      'Details',
      'Open folder',
      'Remove from list (files stay)',
    ]);
    await view.user.keyboard('{Escape}');

    await view.user.click(more('Game Two'));
    const items = screen.getAllByRole('menuitem').map((item) => item.textContent?.trim() ?? '');
    expect(items).toContain('Remove from list and stop seeding (files stay)');
    expect(items.some((item) => /cancel/i.test(item))).toBe(false);
  });

  it('Details opens the download details; Open folder opens its folder and says so when it cannot', async () => {
    const view = show([download()]);

    await pickFromMenu(view, 'Game One', 'Details');
    expect(view.onopen).toHaveBeenCalledWith('d1');

    await pickFromMenu(view, 'Game One', 'Open folder');
    expect(openFolder).toHaveBeenCalledWith('C:\\Games\\d1');

    vi.mocked(openFolder).mockRejectedValueOnce(new Error('gone'));
    await pickFromMenu(view, 'Game One', 'Open folder');
    await waitFor(() => expect(get(toasts).map((t) => t.message)).toContain('Folder unavailable'));
  });

  it('a failed install keeps its install dialog one press away in the menu', async () => {
    const view = show([download()], [installation({ status: 'failed' })]);

    await pickFromMenu(view, 'Game One', 'Installation window');
    expect(view.oninstall).toHaveBeenCalledWith('d1');
  });
});

describe('completed downloads: Remove from list', () => {
  it('asks Go to remove, says so, and hands focus to the row now in its place', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const onannounce = vi.fn();
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], installed('d1', 'd2'), { onannounce });

    await pickFromMenu(view, 'Game One', 'Remove from list (files stay)');

    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(screen.queryByText('Game One')).toBeNull());
    expect(removeDownload).toHaveBeenCalledWith('d1');
    expect(onannounce).toHaveBeenCalledWith('“Game One” removed from the list');
    await waitFor(() => expect(document.activeElement).toBe(more('Game Two')));
  });

  it('the last row hands focus to the one above, the only one to the heading the page gives', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], installed('d1', 'd2'));

    await pickFromMenu(view, 'Game Two', 'Remove from list (files stay)');
    await waitFor(() => expect(document.activeElement).toBe(more('Game One')));

    await pickFromMenu(view, 'Game One', 'Remove from list (files stay)');
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it('a row leaving its section with another section still there hands focus to its own section’s heading', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const view = show(
      [download(), download({ id: 'd2', name: 'Game Two' })],
      [...installed('d1'), installation({ id: 'i2', downloadId: 'd2', status: 'waiting_for_user' })],
    );

    await pickFromMenu(view, 'Game One', 'Remove from list (files stay)');

    await waitFor(() => expect(screen.queryByText('Game One')).toBeNull());
    expect(document.activeElement).toBe(heading);
    expect(section('Installing')).toBeTruthy();
  });

  it('hands focus on also when Go answers before the row leaves', async () => {
    let removeLater = () => {};
    vi.mocked(removeDownload).mockImplementation(async (id) => {
      removeLater = () => goRemoves(id);
    });
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], installed('d1', 'd2'));

    await pickFromMenu(view, 'Game One', 'Remove from list (files stay)');
    await waitFor(() => expect(removeDownload).toHaveBeenCalled());
    removeLater();

    await waitFor(() => expect(screen.queryByText('Game One')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(more('Game Two')));
  });

  it('a removal Go refuses is said by its code; the row stays and keeps focus on its menu button', async () => {
    vi.mocked(removeDownload).mockRejectedValueOnce(new Error('typhon:download.not_found: gone'));
    const onannounce = vi.fn();
    const view = show([download()], installed('d1'), { onannounce });

    await pickFromMenu(view, 'Game One', 'Remove from list (files stay)');

    await waitFor(() => expect(get(toasts)).toHaveLength(1));
    expect(get(toasts)[0].message).not.toContain('typhon:');
    expect(row('Game One')).toBeTruthy();
    expect(document.activeElement).toBe(more('Game One'));
    expect(onannounce).not.toHaveBeenCalled();
  });
});

describe('completed downloads: Remove from list says what it takes', () => {
  const removeItem = 'Remove from list (files stay)';
  const menuItems = () => screen.getAllByRole('menuitem').map((item) => item.textContent?.trim());

  it('is not offered while the installation runs or waits for the player', async () => {
    const view = show(
      [download(), download({ id: 'd2', name: 'Game Two' })],
      [installation(), installation({ id: 'i2', downloadId: 'd2', status: 'waiting_for_user' })],
    );

    for (const name of ['Game One', 'Game Two']) {
      await view.user.click(more(name));
      expect(menuItems(), name).toEqual(['Details', 'Open folder']);
      await view.user.keyboard('{Escape}');
    }
  });

  it.each<[string, Partial<Installation> | null]>([
    ['ready', null],
    ['install failed', { status: 'failed', error: 'typhon:install.no_free_space' }],
    ['install cancelled', { status: 'cancelled' }],
    ['install interrupted', { status: 'interrupted' }],
  ])('%s: asks first, naming the download and what goes; Keep sends nothing', async (_kind, install) => {
    const view = show([download()], install ? [installation(install)] : []);

    await pickFromMenu(view, 'Game One', removeItem);

    const dialog = await screen.findByRole('dialog', { name: 'Remove “Game One” from the list?' });
    expect(dialog.textContent).toContain(
      'The download’s record goes and its files stay on disk. The game can no longer be installed from this screen.',
    );
    expect(dialog.textContent).not.toContain('Seeding stops too.');
    expect(removeDownload).not.toHaveBeenCalled();
    await view.user.click(within(dialog).getByRole('button', { name: 'Keep it' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(removeDownload).not.toHaveBeenCalled();
    expect(row('Game One')).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(more('Game One')));
  });

  it('a confirmed removal sends Go’s Remove, says so, and focus goes to the row now in its place', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const onannounce = vi.fn();
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], [], { onannounce });

    await pickFromMenu(view, 'Game One', removeItem);
    const dialog = await screen.findByRole('dialog');
    await view.user.click(within(dialog).getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(removeDownload).mock.calls).toEqual([['d1']]);
    expect(onannounce).toHaveBeenCalledWith('“Game One” removed from the list');
    await waitFor(() => expect(document.activeElement).toBe(more('Game Two')));
  });

  it('a removal Go refuses stays in the dialog with the reason by its code', async () => {
    vi.mocked(removeDownload).mockRejectedValueOnce(new Error('typhon:download.not_found: gone'));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const view = show([download()]);

    await pickFromMenu(view, 'Game One', removeItem);
    const dialog = await screen.findByRole('dialog');
    await view.user.click(within(dialog).getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).not.toBe(''));
    expect(dialog.textContent).not.toContain('typhon:');
    expect(screen.getByRole('dialog')).toBe(dialog);
    expect(get(toasts)).toEqual([]);
  });

  it('a seeding download that can still be installed: the dialog says seeding stops too', async () => {
    const view = show([download({ seeding: true })]);

    await pickFromMenu(view, 'Game One', 'Remove from list and stop seeding (files stay)');

    const dialog = await screen.findByRole('dialog');
    expect(dialog.textContent).toContain('Seeding stops too.');
  });

  it('an installed download and update or repair files go at once, with no dialog', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const view = show(
      [download(), download({ id: 'd2', name: 'Game Two', origin: { gameId: 'g1', purpose: 'update' } })],
      installed('d1'),
    );

    await pickFromMenu(view, 'Game One', removeItem);
    await pickFromMenu(view, 'Game Two', removeItem);

    await waitFor(() => expect(vi.mocked(removeDownload).mock.calls).toEqual([['d1'], ['d2']]));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('the dialog closes when its download leaves the list by Go’s doing; nothing is sent', async () => {
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })]);

    await pickFromMenu(view, 'Game One', removeItem);
    await screen.findByRole('dialog');
    installations.set([installation()]);

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(removeDownload).not.toHaveBeenCalled();
    // Back in "Completed" once its install stops: the old question does not come back with it.
    installations.set([installation({ status: 'cancelled' })]);
    await waitFor(() => expect(row('Game One').closest('section')).toBe(section('Completed')));
    await new Promise((done) => setTimeout(done, 0));
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});

describe('completed downloads: Clear the list', () => {
  const clearButton = () => within(section('Completed')).getByRole('button', { name: 'Clear the list' });

  it('removes only what needs nothing more from the screen, one request each, and says how many went and how many could not', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => {
      if (id === 'f') throw new Error('typhon:download.not_found: gone');
      goRemoves(id);
    });
    const onannounce = vi.fn();
    const view = show(
      [
        download({ id: 'a', name: 'Ready One' }),
        download({ id: 'b', name: 'Installed One' }),
        download({ id: 'c', name: 'Seeding One', seeding: true }),
        download({ id: 'd', name: 'Installing One' }),
        download({ id: 'e', name: 'Waiting One' }),
        download({ id: 'f', name: 'Refused One' }),
        download({ id: 'g', name: 'Failed One' }),
        download({ id: 'h', name: 'Stopped One' }),
        download({ id: 'u', name: 'Update One', origin: { gameId: 'g1', purpose: 'update' } }),
        download({ id: 'r', name: 'Repair One', origin: { gameId: 'g1', purpose: 'repair' } }),
      ],
      [
        installation({ id: 'ib', downloadId: 'b', status: 'completed' }),
        installation({ id: 'ic', downloadId: 'c', status: 'completed' }),
        installation({ id: 'id', downloadId: 'd', status: 'installing' }),
        installation({ id: 'ie', downloadId: 'e', status: 'waiting_for_user' }),
        installation({ id: 'if', downloadId: 'f', status: 'completed' }),
        installation({ id: 'ig', downloadId: 'g', status: 'failed' }),
        installation({ id: 'ih', downloadId: 'h', status: 'cancelled' }),
      ],
      { onannounce },
    );
    const clear = clearButton();
    clear.focus();

    await view.user.keyboard('{Enter}');

    await waitFor(() => expect(onannounce).toHaveBeenCalledTimes(1));
    expect(vi.mocked(removeDownload).mock.calls.map(([id]) => id).sort()).toEqual(['b', 'f', 'r', 'u']);
    expect(onannounce).toHaveBeenCalledWith('Removed 3 downloads from the list. 1 could not be removed');
    for (const name of ['Ready One', 'Seeding One', 'Installing One', 'Waiting One', 'Refused One', 'Failed One', 'Stopped One']) {
      expect(row(name)).toBeTruthy();
    }
    for (const name of ['Installed One', 'Update One', 'Repair One']) expect(screen.queryByText(name)).toBeNull();
    // Said on screen once, by the section, not once per refusal by the store.
    expect(get(toasts).map((t) => [t.kind, t.message])).toEqual([
      ['danger', `1 download could not be removed from the list: ${installErrorText(new Error('typhon:download.not_found: gone'))}`],
    ]);
    expect(document.activeElement).toBe(clear);
  });

  it('a refusal is said on screen once, with how many and the first reason by its code', async () => {
    const first = new Error('typhon:download.not_found: gone');
    vi.mocked(removeDownload).mockImplementation(async (id) => {
      if (id === 'd1') throw first;
      if (id === 'd2') throw new Error('typhon:download.unavailable');
    });
    const onannounce = vi.fn();
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], installed('d1', 'd2'), { onannounce });

    await view.user.click(clearButton());

    await waitFor(() => expect(onannounce).toHaveBeenCalledWith('2 could not be removed'));
    expect(get(toasts).map((t) => [t.kind, t.message])).toEqual([
      ['danger', `2 downloads could not be removed from the list: ${installErrorText(first)}`],
    ]);
    expect(installErrorText(first)).not.toContain('typhon:');
  });

  it('a clear Go takes whole says nothing on screen beyond the rows going', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const onannounce = vi.fn();
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], installed('d1', 'd2'), { onannounce });

    await view.user.click(clearButton());

    await waitFor(() => expect(onannounce).toHaveBeenCalledWith('Removed 2 downloads from the list'));
    expect(get(toasts)).toEqual([]);
  });

  it('with nothing it may clear, it stays focusable and says why, and a press sends nothing', async () => {
    const view = show(
      [download(), download({ id: 'd2', name: 'Game Two', seeding: true }), download({ id: 'd3', name: 'Game Three' })],
      [installation({ id: 'i2', downloadId: 'd2', status: 'completed' }), installation({ id: 'i3', downloadId: 'd3', status: 'failed' })],
    );
    const clear = clearButton();

    expect(clear.getAttribute('aria-disabled')).toBe('true');
    expect(clear.hasAttribute('disabled')).toBe(false);
    const describedBy = clear.getAttribute('aria-describedby') ?? '';
    expect(describedBy.split(' ').map((id) => document.getElementById(id)?.textContent).join(' ')).toContain(
      'Nothing to clear: only installed downloads and update or repair files leave the list; downloads still to install, being installed or seeding stay',
    );
    clear.focus();
    await view.user.keyboard('{Enter}');
    expect(removeDownload).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(clear);
  });

  it('asks first when more than five would go, naming how many; five go at once', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const six = Array.from({ length: 6 }, (_, i) => download({ id: `x${i}`, name: `Game ${i}` }));
    const view = show([...six, download({ id: 'ready', name: 'Ready One' })], installed(...six.map((d) => d.id)));

    await view.user.click(clearButton());
    const dialog = screen.getByRole('dialog', { name: 'Clear the list of completed downloads?' });
    expect(dialog.textContent).toContain(
      '6 downloads that are installed or hold update or repair files leave the list; their files stay on disk. Downloads still to install stay.',
    );
    expect(removeDownload).not.toHaveBeenCalled();
    await view.user.click(within(dialog).getByRole('button', { name: 'Clear' }));
    await waitFor(() => expect(removeDownload).toHaveBeenCalledTimes(6));

    view.unmount();
    vi.mocked(removeDownload).mockClear();
    const again = show(six.slice(0, 5), installed(...six.map((d) => d.id)));
    await again.user.click(clearButton());
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(removeDownload).toHaveBeenCalledTimes(5));
  });

  it('when the whole section goes, focus goes to the heading the page gives', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], installed('d1', 'd2'));
    const clear = clearButton();
    clear.focus();

    await view.user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('region', { name: /^Completed/ })).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it('a second press while clearing sends nothing more', async () => {
    const gate = deferred();
    vi.mocked(removeDownload).mockImplementation(() => gate.promise);
    const view = show([download()], installed('d1'));
    const clear = clearButton();
    clear.focus();

    await view.user.keyboard('{Enter}');
    await view.user.keyboard('{Enter}');

    expect(removeDownload).toHaveBeenCalledTimes(1);
    expect(clear.getAttribute('aria-busy')).toBe('true');
    gate.resolve();
  });
});
