import { render, screen, within } from '@testing-library/svelte';
import { flushSync, tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { installErrorText } from '../install/installErrors';
import { cancelInstall } from '../services/install';
import { pauseDownload, resumeDownload, type Download } from '../services/downloads';
import type { Installation } from '../services/install';
import type { LibraryGame } from '../services/library';
import type { MoveJob } from '../services/relocate';
import type { VerifyState } from '../services/updates';
import { downloads } from '../stores/downloads';
import { installations } from '../stores/install';
import { activeTheme } from '../stores/theme';
import { libraryGames, runningGames, sessionStarts } from '../stores/library';
import { moves } from '../stores/relocate';
import { navigate, resetHistory, route } from '../stores/router';
import { toasts } from '../stores/toasts';
import { verifications } from '../stores/updates';
import { user } from '../testing/dom';
import { stageLabel } from '../relocate/moveText';
import ActivityWidget from './ActivityWidget.svelte';
import Sidebar from './Sidebar.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', async (actual) => ({
  ...(await actual<typeof import('../services/downloads')>()),
  pauseDownload: vi.fn(),
  resumeDownload: vi.fn(),
}));
vi.mock('../services/install', async (actual) => ({
  ...(await actual<typeof import('../services/install')>()),
  cancelInstall: vi.fn(),
}));

function download(over: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Hades',
    type: 'torrent',
    source: '',
    infoHash: '',
    destination: 'D:/games',
    status: 'downloading',
    progress: 0.42,
    downloaded: 42,
    total: 100,
    downloadSpeed: 1024 * 1024,
    uploadSpeed: 0,
    etaSeconds: 60,
    seeders: 1,
    peers: 1,
    files: [],
    seeding: false,
    addedAt: '2026-01-01T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...over,
  } as Download;
}

function install(over: Partial<Installation> = {}): Installation {
  return {
    id: 'i1',
    downloadId: 'dx',
    gameId: '',
    name: 'Celeste',
    type: 'exe_installer',
    status: 'waiting_for_user',
    mode: 'copy',
    sourcePath: '',
    contentRoot: '',
    destination: '',
    installerPath: '',
    workingDir: '',
    archivePath: '',
    progress: 0.1,
    currentFile: '',
    bytesDone: 0,
    bytesTotal: 0,
    executable: '',
    candidates: null,
    detectedVersion: '',
    versionSource: '',
    startedAt: '2026-01-01T00:00:00Z',
    completedAt: null,
    error: '',
    engine: '',
    silent: false,
    ...over,
  } as Installation;
}

const move: MoveJob = {
  id: 'm1',
  scope: 'game',
  stage: 'copy',
  gameId: 'g9',
  title: 'Hollow Knight',
  source: '/a',
  target: '/b',
  staging: '/b/.staging',
  renamed: false,
  totalBytes: 100,
  copiedBytes: 30,
  phase: '',
  currentFile: '',
  startedAt: '',
  updatedAt: '',
};

const verify = { gameId: 'g2', method: 'manifest', running: true, repairing: false, progress: 0.5, processedBytes: 1, currentFile: '', ratio: 0, totalBytes: 2, okBytes: 0, missingFiles: 0, corruptedPieces: 0, unreadableFiles: 0, repairable: false, checkedAt: null } as VerifyState;
const games = [{ id: 'g1', title: 'Elden Ring', cover: '' }, { id: 'g2', title: 'Celeste' }] as LibraryGame[];

let app: HTMLDivElement;

function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function mount(host: 'widget' | 'sidebar' = 'widget') {
  const view = host === 'sidebar' ? render(Sidebar, { target: app }) : render(ActivityWidget, { target: app });
  const keys = user();
  const head = () => screen.queryByRole('button', { name: (_name, el) => el.classList.contains('head') });
  const panel = () => screen.queryByRole('dialog', { name: msg('ui.activityNow') });
  const live = () => app.querySelector('[role="status"]')?.textContent ?? '';
  const rows = () => [...(panel()?.querySelectorAll<HTMLElement>('li') ?? [])];
  return { keys, head, panel, live, rows, unmount: view.unmount };
}

function set(fn: () => void) {
  fn();
  flushSync();
}

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set(games);
});

afterEach(() => {
  document.body.replaceChildren();
  downloads.set([]);
  installations.set([]);
  moves.set([]);
  verifications.set({});
  runningGames.set(new Set());
  sessionStarts.set(new Map());
  libraryGames.set([]);
  toasts.set([]);
  activeTheme.set(null);
  resetHistory();
  vi.mocked(pauseDownload).mockReset();
  vi.mocked(resumeDownload).mockReset();
  vi.mocked(cancelInstall).mockReset();
});

describe('ActivityWidget: nothing going on', () => {
  it('shows no control and puts nothing in the Tab order', () => {
    const { head } = mount();
    expect(head()).toBeNull();
    expect(app.querySelectorAll('button, a, [tabindex]')).toHaveLength(0);
  });
});

describe('ActivityWidget: one download', () => {
  it('names it with its percent on the line, and a bar named by it with its value', () => {
    downloads.set([download()]);
    const { head } = mount();
    expect(head()?.textContent?.replace(/\s+/g, ' ').trim()).toBe('Hades 42%');
    const bar = screen.getByRole('progressbar', { name: 'Hades' });
    expect(bar.getAttribute('aria-valuenow')).toBe('42');
    expect(bar.hasAttribute('aria-valuetext')).toBe(false);
  });

  it('says a paused download is paused, on the line and in the bar', () => {
    downloads.set([download({ status: 'paused' })]);
    const { head } = mount();
    const paused = msg('state.downloadsStatusPaused');
    expect(head()?.textContent?.replace(/\s+/g, ' ').trim()).toBe(`Hades ${paused}`);
    expect(screen.getByRole('progressbar', { name: 'Hades' }).getAttribute('aria-valuetext')).toBe(
      msg('ui.activityPausedValue', { status: paused, percent: 42 }),
    );
  });
});

describe('ActivityWidget: several items and the panel', () => {
  it('leads with the item that needs the user, says how many more, and opens a row for each on a press', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Stardew' })]);
    installations.set([install()]);
    const { keys, head, panel, rows } = mount();
    const line = head() as HTMLElement;
    expect(line.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      `Celeste ${msg('state.installStatusWaitingForUser')} ${msg('ui.activityMore', { count: 2 })} ${msg('ui.activityMoreLabel', { count: 2 })}`,
    );
    expect(line.getAttribute('aria-expanded')).toBe('false');

    await keys.click(line);

    expect(panel()).not.toBeNull();
    expect(line.getAttribute('aria-expanded')).toBe('true');
    expect(rows().map((row) => row.querySelector('.name')?.textContent)).toEqual(['Celeste', 'Hades', 'Stardew']);
    expect(app.contains(panel())).toBe(false);
  });

  it('leads with a download that moves rather than a paused one before it', () => {
    downloads.set([download({ status: 'paused' }), download({ id: 'd2', name: 'Stardew', progress: 0.1 })]);
    const { head } = mount();
    expect(head()?.textContent?.replace(/\s+/g, ' ').trim()).toMatch(/^Stardew 10%/);
  });

  it('does not open on hover', async () => {
    downloads.set([download()]);
    const { keys, head, panel } = mount();
    expect(head()).not.toBeNull();
    await keys.hover(head() as HTMLElement);
    expect(panel()).toBeNull();
  });

  it.each([
    ['Enter', '{Enter}'],
    ['Space', ' '],
  ])('opens on %s; Escape closes it and gives focus back to the head', async (_key, press) => {
    downloads.set([download()]);
    const { keys, head, panel } = mount();
    (head() as HTMLElement).focus();

    await keys.keyboard(press);
    expect(panel()).not.toBeNull();
    expect(panel()?.contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(head());
  });

  it('closes on a press outside', async () => {
    downloads.set([download()]);
    const outside = document.createElement('button');
    outside.textContent = 'outside';
    app.append(outside);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    expect(panel()).not.toBeNull();

    await keys.click(outside);

    expect(panel()).toBeNull();
  });

  it('Tab past its last control closes it and moves on; Shift+Tab before the first closes it onto the head', async () => {
    downloads.set([download()]);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    within(panel() as HTMLElement).getByRole('button', { name: msg('ui.allDownloads') }).focus();

    await keys.tab();
    expect(panel()).toBeNull();

    await keys.click(head() as HTMLElement);
    expect(document.activeElement?.classList.contains('row')).toBe(true);
    await keys.tab({ shift: true });
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(head());
  });

  it('opens what a row shows: a download its page, a verification and a game the game, a move the settings', async () => {
    downloads.set([download()]);
    verifications.set({ g2: verify });
    moves.set([move]);
    runningGames.set(new Set(['g1']));
    const { keys, head, panel } = mount();
    const press = async (name: string) => {
      await keys.click(head() as HTMLElement);
      await keys.click(within(panel() as HTMLElement).getByRole('button', { name: new RegExp(`^${name}`) }));
      expect(panel()).toBeNull();
      return get(route);
    };
    expect((await press('Hades')).name).toBe('downloads');
    navigate('library');
    expect(await press('Celeste')).toMatchObject({ name: 'game', params: { id: 'g2' } });
    navigate('library');
    expect((await press('Hollow Knight')).name).toBe('settings');
    expect(await press('Elden Ring')).toMatchObject({ name: 'game', params: { id: 'g1' } });
  });
});

describe('ActivityWidget: actions from the keyboard', () => {
  it('Pause is called, stays focused and busy while it runs, and turns into Resume in place', async () => {
    const call = deferred();
    vi.mocked(pauseDownload).mockReturnValue(call.promise);
    downloads.set([download()]);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    const pause = within(panel() as HTMLElement).getByRole('button', { name: msg('ui.pause') });
    pause.focus();

    await keys.keyboard('{Enter}');

    expect(pauseDownload).toHaveBeenCalledWith('d1');
    expect(pause.getAttribute('aria-busy')).toBe('true');
    expect(pause.hasAttribute('disabled')).toBe(false);
    expect(document.activeElement).toBe(pause);

    call.resolve();
    set(() => downloads.set([download({ status: 'paused' })]));
    await tick();
    expect(pause.isConnected).toBe(true);
    expect(pause.textContent).toContain(msg('common.continue'));
    await vi.waitFor(() => expect(pause.getAttribute('aria-busy')).not.toBe('true'));
    expect(document.activeElement).toBe(pause);
  });

  // Pause and Continue differ in length: a width of its own keeps the swap from moving the row's name.
  it('keeps one width for the action whichever label it shows', async () => {
    downloads.set([download()]);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    const action = within(panel() as HTMLElement).getByRole('button', { name: msg('ui.pause') });
    expect(action.style.minWidth).toBe('var(--activity-action-w)');

    set(() => downloads.set([download({ status: 'paused' })]));
    expect(action.textContent).toContain(msg('common.continue'));
    expect(action.style.minWidth).toBe('var(--activity-action-w)');

    type Fs = { readFileSync(path: string, encoding: 'utf8'): string };
    const node = (globalThis as unknown as { process: { cwd(): string; getBuiltinModule(id: 'node:fs'): Fs } }).process;
    const shell = node.getBuiltinModule('node:fs').readFileSync(`${node.cwd()}/src/styles/shell.css`, 'utf8');
    expect(/--activity-action-w:\s*[\d.]+rem;/.test(shell)).toBe(true);
  });

  it('Resume is called on a paused download', async () => {
    vi.mocked(resumeDownload).mockResolvedValue();
    downloads.set([download({ status: 'paused' })]);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    within(panel() as HTMLElement).getByRole('button', { name: msg('common.continue') }).focus();

    await keys.keyboard(' ');

    expect(resumeDownload).toHaveBeenCalledWith('d1');
  });

  it('Cancel install is called for the waiting installation; a failed cancel says its error and the button stays', async () => {
    const call = deferred();
    vi.mocked(cancelInstall).mockReturnValue(call.promise);
    installations.set([install()]);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    const cancel = within(panel() as HTMLElement).getByRole('button', { name: msg('modals.installCancelInstall') });
    cancel.focus();

    await keys.keyboard('{Enter}');
    expect(cancelInstall).toHaveBeenCalledWith('i1');
    expect(cancel.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(cancel);

    const err = new Error('typhon:install.cancel_failed');
    call.reject(err);
    await vi.waitFor(() => expect(cancel.getAttribute('aria-busy')).not.toBe('true'));
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[installErrorText(err), 'danger']]);
    expect(document.activeElement).toBe(cancel);
  });
});

describe('ActivityWidget: a row that goes', () => {
  it('hands focus from a finished row to the next row, and from the last row to the one before', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Stardew' }), download({ id: 'd3', name: 'Tunic' })]);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    within(panel() as HTMLElement).getAllByRole('button', { name: msg('ui.pause') })[0].focus();

    set(() => downloads.set([download({ status: 'completed' }), download({ id: 'd2', name: 'Stardew' }), download({ id: 'd3', name: 'Tunic' })]));
    await vi.waitFor(() => expect(document.activeElement?.closest('li')?.querySelector('.name')?.textContent).toBe('Stardew'));
    expect(document.activeElement?.classList.contains('row')).toBe(true);

    within(panel() as HTMLElement).getAllByRole('button', { name: msg('ui.pause') })[1].focus();
    set(() => downloads.set([download({ id: 'd2', name: 'Stardew' })]));
    await vi.waitFor(() => expect(document.activeElement?.closest('li')?.querySelector('.name')?.textContent).toBe('Stardew'));
  });

  it('moves nothing when the press held no focus (<body>): a cancelled install goes and focus is not put anywhere by the widget', async () => {
    vi.mocked(cancelInstall).mockResolvedValue();
    installations.set([install()]);
    downloads.set([download()]);
    const { head, panel } = mount();
    (head() as HTMLElement).click();
    flushSync();
    const cancel = within(panel() as HTMLElement).getByRole('button', { name: msg('modals.installCancelInstall') });
    (document.activeElement as HTMLElement).blur();
    expect(document.activeElement).toBe(document.body);

    cancel.click();
    await tick();
    set(() => installations.set([install({ status: 'cancelled' })]));
    await tick();
    await tick();

    const rowButtons = [...(panel()?.querySelectorAll('.row') ?? [])];
    expect([head(), ...rowButtons]).not.toContain(document.activeElement);
  });
});

describe('ActivityWidget: the last row goes', () => {
  // The head and the panel go with the last row: focus would fall to <body>. Settings, right above the
  // zone, stays.
  it('hands focus from the last row to the sidebar\'s Settings, which stays', async () => {
    downloads.set([download()]);
    const { keys, head, panel } = mount('sidebar');
    await keys.click(head() as HTMLElement);
    within(panel() as HTMLElement).getByRole('button', { name: msg('ui.pause') }).focus();

    set(() => downloads.set([download({ status: 'completed' })]));

    expect(head()).toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('ui.settingsNav') })));
  });

  it('moves nothing when the last row goes with focus on <body>', async () => {
    downloads.set([download()]);
    const { keys, head, panel } = mount('sidebar');
    await keys.click(head() as HTMLElement);
    expect(panel()).not.toBeNull();
    (document.activeElement as HTMLElement).blur();

    set(() => downloads.set([download({ status: 'completed' })]));
    await tick();
    await tick();

    expect(document.activeElement).toBe(document.body);
  });
});

describe('ActivityWidget: moves and running games', () => {
  it('shows a move as an item with its stage', async () => {
    moves.set([move]);
    const { keys, head, panel } = mount();
    expect(head()?.textContent?.replace(/\s+/g, ' ').trim()).toBe('Hollow Knight 30%');
    await keys.click(head() as HTMLElement);
    expect(within(panel() as HTMLElement).getByRole('button', { name: /^Hollow Knight/ }).textContent).toContain(stageLabel('copy'));
  });

  it('counts a game this launcher saw start, and shows no time for one already running', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] });
    vi.setSystemTime(new Date('2026-11-13T10:00:00Z'));
    runningGames.set(new Set(['g1', 'g2']));
    sessionStarts.set(new Map([['g1', Date.now()]]));
    const { head, panel } = mount();
    // The cover beside the name is hidden from assistive tech: the name is the text.
    expect(screen.getByRole('button', { name: `Elden Ring 0:00 ${msg('ui.activityMoreLabel', { count: 1 })}` })).toBe(head());

    vi.advanceTimersByTime(65_000);
    flushSync();
    expect(screen.getByRole('button', { name: `Elden Ring 1:05 ${msg('ui.activityMoreLabel', { count: 1 })}` })).toBe(head());

    (head() as HTMLElement).click();
    flushSync();
    const details = [...(panel()?.querySelectorAll('.detail') ?? [])].map((el) => el.textContent);
    expect(details).toEqual([msg('ui.activityPlayingFor', { time: '1:05' }), msg('ui.activityPlaying')]);

    // Alone, the game already running says its name and no time at all, not 0:00.
    set(() => runningGames.set(new Set(['g2'])));
    expect(screen.getByRole('button', { name: 'Celeste' })).toBe(head());
  });
});

describe('ActivityWidget: the session clock', () => {
  it('reads the wall clock on each tick, not a count of ticks', () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] });
    vi.setSystemTime(new Date('2026-11-13T10:00:00Z'));
    runningGames.set(new Set(['g1']));
    sessionStarts.set(new Map([['g1', Date.now()]]));
    const { head } = mount();

    // The window slept: the wall clock moved ten minutes, no tick ran meanwhile.
    vi.setSystemTime(new Date('2026-11-13T10:10:00Z'));
    vi.advanceTimersByTime(1000);
    flushSync();

    expect(screen.getByRole('button', { name: 'Elden Ring 10:01' })).toBe(head());
  });

  it('leaves no timer running once it is gone', () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] });
    runningGames.set(new Set(['g1']));
    sessionStarts.set(new Map([['g1', Date.now()]]));
    const { unmount } = mount();
    expect(vi.getTimerCount()).toBe(1);

    unmount();

    expect(vi.getTimerCount()).toBe(0);
  });
});

describe('ActivityWidget: rows the page already shows', () => {
  it('hides downloads and installs on the Downloads page and is empty when nothing else goes on', () => {
    downloads.set([download()]);
    installations.set([install()]);
    navigate('downloads');
    const { head } = mount();
    expect(head()).toBeNull();

    set(() => verifications.set({ g2: verify }));
    expect(head()?.textContent).toContain('Celeste');
    expect(head()?.textContent).not.toContain(msg('ui.activityMore', { count: 1 }));
  });

  it("hides a game's own row and its verification on that game's page", () => {
    runningGames.set(new Set(['g1']));
    verifications.set({ g2: verify });
    navigate('game', { id: 'g1' });
    const { head } = mount();
    expect(head()?.textContent).toContain('Celeste');
    expect(head()?.textContent).not.toContain('Elden Ring');

    set(() => navigate('game', { id: 'g2' }));
    expect(head()?.textContent).toContain('Elden Ring');
    expect(head()?.textContent).not.toContain(msg('ui.activityMore', { count: 1 }));
  });
});

describe('ActivityWidget: the live region', () => {
  it('says what appears, finishes, fails or needs the user, once each, and never progress', () => {
    downloads.set([download()]);
    const { live } = mount();
    expect(live()).toBe('');

    set(() => downloads.set([download({ progress: 0.5 })]));
    expect(live()).toBe('');

    set(() => downloads.set([download({ progress: 0.5 }), download({ id: 'd2', name: 'Stardew' })]));
    expect(live()).toBe(msg('ui.activityLiveDownload', { name: 'Stardew' }));
    set(() => downloads.set([download({ progress: 0.6 }), download({ id: 'd2', name: 'Stardew', progress: 0.9 })]));
    expect(live()).toBe(msg('ui.activityLiveDownload', { name: 'Stardew' }));

    set(() => downloads.set([download({ status: 'completed' }), download({ id: 'd2', name: 'Stardew', status: 'failed' })]));
    expect(live()).toBe(`${msg('ui.activityLiveDone', { name: 'Hades' })}. ${msg('ui.activityLiveFailed', { name: 'Stardew' })}`);

    set(() => installations.set([install({ status: 'installing' })]));
    expect(live()).toBe(msg('ui.activityLiveInstall', { name: 'Celeste' }));
    set(() => installations.set([install()]));
    expect(live()).toBe(msg('ui.activityLiveAttention', { name: 'Celeste' }));
  });

  it('a page that hides rows makes nothing appear or end', () => {
    downloads.set([download()]);
    const { live } = mount();
    set(() => navigate('downloads'));
    set(() => navigate('library'));
    expect(live()).toBe('');
    set(() => downloads.set([download(), download({ id: 'd2', name: 'Stardew' })]));
    expect(live()).toBe(msg('ui.activityLiveDownload', { name: 'Stardew' }));
  });
});

describe('ActivityWidget: the rail', () => {
  it('is one control named by the lead and its percent, which opens the panel', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Stardew' })]);
    const { keys, panel } = mount();
    const controls = app.querySelectorAll('button');
    expect(controls).toHaveLength(1);
    const control = screen.getByRole('button', { name: `Hades 42% ${msg('ui.activityMoreLabel', { count: 1 })}` });
    await keys.click(control);
    expect(panel()).not.toBeNull();
  });

  it('keeps the name and the percent as the name of the head when the rail hides them from sight', async () => {
    const source = (await import('./ActivityWidget.svelte?raw')).default;
    const rail = source.slice(source.indexOf('@media (max-width: 1140px)'));
    const hidden = /\.head \.name,\s*\.head \.reading \{([^}]*)\}/.exec(rail)?.[1] ?? '';
    expect(hidden).toMatch(/clip-path: inset\(50%\)/);
    expect(hidden).not.toMatch(/display:\s*none/);
    expect(rail).not.toMatch(/\.bar[^{]*\{[^}]*display:\s*none/);
  });
});

describe('ActivityWidget: the side the panel opens on', () => {
  it('opens towards the content: right of a left sidebar, left of a right one', async () => {
    downloads.set([download()]);
    activeTheme.set({ id: 'x', layout: { sidebar: { side: 'right' } } } as never);
    const { keys, head, panel } = mount();
    await keys.click(head() as HTMLElement);
    expect(panel()?.dataset.placement).toMatch(/-end$/);
  });
});
