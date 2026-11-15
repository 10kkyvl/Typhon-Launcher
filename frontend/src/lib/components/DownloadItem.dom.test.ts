import { screen, waitFor, within } from '@testing-library/svelte';
import { get, type Readable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', async (actual) => ({
  ...(await actual<typeof import('../services/downloads')>()),
  pauseDownload: vi.fn(),
  resumeDownload: vi.fn(),
  cancelDownload: vi.fn(),
  removeDownload: vi.fn(),
  forceStartDownload: vi.fn(),
  downloadNow: vi.fn(),
}));
vi.mock('../services/settings', async (actual) => ({
  ...(await actual<typeof import('../services/settings')>()),
  openFolder: vi.fn(),
}));

// Every reader of a panel's speed series, counted through the store's own `speedHistory`.
const readers = vi.hoisted(() => ({ count: 0, ids: [] as string[] }));
vi.mock('../stores/downloads', async (actual) => {
  const real = await actual<typeof import('../stores/downloads')>();
  return {
    ...real,
    speedHistory: vi.fn((id: string) => {
      const inner = real.speedHistory(id);
      readers.ids.push(id);
      return {
        subscribe(run: Parameters<typeof inner.subscribe>[0]) {
          readers.count++;
          const stop = inner.subscribe(run);
          return () => {
            readers.count--;
            stop();
          };
        },
      } satisfies Readable<unknown>;
    }),
  };
});

import { locale } from '../i18n';
import { installErrorText } from '../install/installErrors';
import {
  cancelDownload,
  downloadNow,
  forceStartDownload,
  pauseDownload,
  resumeDownload,
  type Download,
} from '../services/downloads';
import { idlePlayThrottle } from '../services/playThrottle';
import { openFolder } from '../services/settings';
import { downloads, statusLabels } from '../stores/downloads';
import { playThrottle } from '../stores/playThrottle';
import { resetHistory, route } from '../stores/router';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import DownloadPanelsHarness from '../testing/DownloadPanelsHarness.svelte';
import { renderDom } from '../testing/dom';
import { bytesSize, etaLabel, speedBytes } from '../utils/format';

const MB = 1024 * 1024;

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'downloading',
    progress: 0.42,
    downloaded: 420 * MB,
    total: 1000 * MB,
    downloadSpeed: 2 * MB,
    uploadSpeed: 512 * 1024,
    etaSeconds: 90,
    seeders: 3,
    peers: 8,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  };
}

/** What Go sends after a call: the record as it now is. */
function goSets(id: string, patch: Partial<Download>) {
  downloads.update((list) => list.map((d) => (d.id === id ? { ...d, ...patch } : d)));
}

function goRemoves(id: string) {
  downloads.update((list) => list.filter((d) => d.id !== id));
}

function show(list: Download[], props: { onopen?: (d: Download) => void; onannounce?: (text: string) => void } = {}) {
  downloads.set(list);
  return renderDom(DownloadPanelsHarness, props);
}

const panel = (name: string) => screen.getByRole('region', { name });

function deferred() {
  let resolve: () => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<void>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  locale.set('en');
  playThrottle.set(idlePlayThrottle);
  settings.set(null);
  toasts.set([]);
  resetHistory();
  readers.count = 0;
  readers.ids = [];
  for (const fn of [pauseDownload, resumeDownload, cancelDownload, forceStartDownload, downloadNow, openFolder]) {
    vi.mocked(fn).mockReset();
    vi.mocked(fn).mockResolvedValue(undefined);
  }
});

afterEach(() => {
  downloads.set([]);
});

describe('the panel of a download under way: what it says in each state', () => {
  it('a running download: one panel named after it, its progress said once by the bar, the speed, time left and peers', () => {
    show([download()]);
    const own = panel('Game One');

    const bar = within(own).getByRole('progressbar', { name: 'Progress of “Game One”' });
    const amount = `${bytesSize(420 * MB)} of ${bytesSize(1000 * MB)} · 42%`;
    expect(bar.getAttribute('aria-valuetext')).toBe(amount);
    expect(bar.dataset.tone).toBe('game');
    // The amount is drawn as text too, hidden from assistive tech: the bar says it once.
    const drawn = within(own).getByTestId('download-amount');
    expect(drawn.textContent).toBe(amount);
    expect(drawn.closest('[aria-hidden="true"]')).not.toBeNull();

    expect(within(own).getByTestId('download-speed').textContent).toBe(`Speed${speedBytes(2 * MB)}`);
    expect(own.textContent).toContain(`${etaLabel(90)} left`);
    expect(own.textContent).toContain('peers 8, seeds 3');
    expect(own.textContent).toContain(`upload ${speedBytes(512 * 1024)}`);
    expect(within(own).getByRole('figure')).toBeTruthy();
    // Nothing in the panel is a live region: a tick announces nothing.
    expect(own.querySelector('[aria-live], [role="status"], [role="alert"]')).toBeNull();
  });

  it('a stalled download says it waits for sources instead of a time left', () => {
    show([download({ stalled: true, downloadSpeed: 0, etaSeconds: -1 })]);
    const own = panel('Game One');

    expect(own.textContent).toContain('Waiting for sources');
    expect(own.textContent).not.toContain(' left');
    expect(within(own).getByRole('button', { name: 'Pause “Game One”' })).toBeTruthy();
  });

  it('fetching the torrent: the unknown amount drawn, the stage in words, no number it does not know and nothing to press but the menu', () => {
    show([download({ status: 'metadata', progress: 0, downloaded: 0, total: 0, downloadSpeed: 0, etaSeconds: -1 })]);
    const own = panel('Game One');

    const bar = within(own).getByRole('progressbar');
    expect(bar.getAttribute('aria-busy')).toBe('true');
    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(own.textContent).toContain(statusLabels('metadata'));
    expect(own.textContent).not.toContain('%');
    expect(own.textContent).not.toContain(' left');
    expect(within(own).queryByTestId('download-speed')).toBeNull();
    expect(within(own).getAllByRole('button').map((b) => b.getAttribute('aria-label') ?? b.textContent)).toEqual([
      'More actions for “Game One”',
    ]);
  });

  it('checking files: no speed and no time left, the check said with the amount', () => {
    show([download({ status: 'verifying', downloadSpeed: 0, etaSeconds: -1, progress: 1, downloaded: 1000 * MB })]);
    const own = panel('Game One');

    expect(within(own).getByRole('progressbar').getAttribute('aria-valuetext')).toBe(
      `${bytesSize(1000 * MB)} of ${bytesSize(1000 * MB)} · 100%, checking files`,
    );
    expect(own.textContent).toContain(statusLabels('verifying'));
    expect(within(own).queryByTestId('download-speed')).toBeNull();
    expect(own.textContent).not.toContain(' left');
    expect(within(own).queryByRole('button', { name: /Pause|Resume/ })).toBeNull();
  });

  it('a paused download is the compact form: no graph and no speed, the pause said in words and drawn on the bar', () => {
    show([download({ status: 'paused', downloadSpeed: 0, etaSeconds: -1 })]);
    const own = panel('Game One');

    const bar = within(own).getByRole('progressbar');
    expect(bar.getAttribute('aria-valuetext')).toContain(', paused');
    expect(bar.dataset.tone).toBe('game');
    expect(bar.querySelector('.fill')?.classList.contains('paused')).toBe(true);
    expect(within(own).queryByRole('figure')).toBeNull();
    expect(within(own).queryByTestId('download-speed')).toBeNull();
    expect(own.dataset.form).toBe('compact');
    expect(within(own).getByRole('button', { name: 'Resume “Game One”' })).toBeTruthy();
  });

  it('held by the schedule: the note, and "Download now" leads', () => {
    playThrottle.set({ ...idlePlayThrottle, active: true, schedule: { blocked: true, nextOpenAt: '' } });
    show([download({ downloadSpeed: 0 })]);
    const own = panel('Game One');

    expect(own.textContent).toContain('Outside the schedule window');
    const buttons = within(own).getAllByRole('button');
    expect(buttons[buttons.length - 2].getAttribute('aria-label')).toBe('Download now “Game One”');
  });

  it('a download that ignores the schedule says so, and is not held', () => {
    playThrottle.set({ ...idlePlayThrottle, active: true, schedule: { blocked: true, nextOpenAt: '' } });
    show([download({ ignoreSchedule: true })]);
    const own = panel('Game One');

    expect(own.textContent).toContain('Outside schedule');
    expect(own.textContent).not.toContain('Outside the schedule window');
    expect(within(own).getByRole('button', { name: 'Pause “Game One”' })).toBeTruthy();
  });

  it('with a speed limit set, says it beside the speed while downloading and draws it on the graph; not on a paused one', () => {
    settings.update(() => ({ downloadRateLimit: 5 * MB }) as never);
    show([download(), download({ id: 'd2', name: 'Game Two', status: 'paused' })]);

    expect(panel('Game One').textContent).toContain(`limit 5 ${'MB/s'}`);
    expect(panel('Game One').querySelector('line.limit')).not.toBeNull();
    expect(panel('Game Two').textContent).not.toContain('limit');
  });

  it('while a game runs with the limit mode, the limit said is the one in force', () => {
    settings.update(() => ({ downloadRateLimit: 0 }) as never);
    playThrottle.set({ ...idlePlayThrottle, active: true, mode: 'limit', downloadLimit: 3 * MB });
    show([download()]);

    expect(panel('Game One').textContent).toContain('limit 3 MB/s');
  });

  it('the title and cover of a game download lead to its page; a download of no game is named in plain text', async () => {
    const view = show([download({ origin: { gameId: 'g1' } }), download({ id: 'd2', name: 'Loose files', origin: {} })]);

    const link = within(panel('Game One')).getByRole('button', { name: 'Game One' });
    await view.user.click(link);
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
    expect(within(panel('Loose files')).queryByRole('button', { name: 'Loose files' })).toBeNull();
    expect(within(panel('Loose files')).getByRole('heading', { name: 'Loose files' })).toBeTruthy();
  });
});

describe('the panel of a download under way: keys reach the control they are pressed on (D-1)', () => {
  it('Enter and Space on Pause pause the download and open nothing', async () => {
    const onopen = vi.fn();
    const view = show([download()], { onopen });
    // Found by its name alone, so that the same test runs against the old row, a button holding buttons.
    const pauseButton = screen.getByRole('button', { name: /^Pause/ });

    pauseButton.focus();
    await view.user.keyboard('{Enter}');
    await waitFor(() => expect(pauseDownload).toHaveBeenCalledTimes(1));
    goSets('d1', { status: 'downloading' });
    await view.user.keyboard(' ');
    await waitFor(() => expect(pauseDownload).toHaveBeenCalledTimes(2));
    expect(onopen).not.toHaveBeenCalled();
  });

  it('Enter on the menu button opens the menu; Enter on Details opens the details once', async () => {
    const onopen = vi.fn();
    const view = show([download()], { onopen });
    const more = within(panel('Game One')).getByRole('button', { name: 'More actions for “Game One”' });

    more.focus();
    await view.user.keyboard('{Enter}');
    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(onopen).not.toHaveBeenCalled();

    expect(document.activeElement?.textContent).toContain('Details');
    await view.user.keyboard('{Enter}');
    expect(onopen).toHaveBeenCalledTimes(1);
    expect(onopen.mock.calls[0][0]).toMatchObject({ id: 'd1' });
  });

  it('Open folder opens the download folder, and says so when it cannot', async () => {
    const view = show([download()]);
    const more = within(panel('Game One')).getByRole('button', { name: 'More actions for “Game One”' });

    await view.user.click(more);
    await view.user.click(screen.getByRole('menuitem', { name: 'Open folder' }));
    expect(openFolder).toHaveBeenCalledWith('C:\\Games\\d1');

    vi.mocked(openFolder).mockRejectedValueOnce(new Error('gone'));
    await view.user.click(more);
    await view.user.click(screen.getByRole('menuitem', { name: 'Open folder' }));
    await waitFor(() => expect(get(toasts).map((t) => t.message)).toContain('Folder unavailable'));
  });
});

describe('the panel of a download under way: focus stays where the player is (D-7)', () => {
  it('Pause turns into Resume on the same focused button, and Resume back into Pause', async () => {
    vi.mocked(pauseDownload).mockImplementation(async (id) => goSets(id, { status: 'paused', downloadSpeed: 0 }));
    vi.mocked(resumeDownload).mockImplementation(async (id) => goSets(id, { status: 'downloading' }));
    const view = show([download()]);
    const toggle = within(panel('Game One')).getByRole('button', { name: 'Pause “Game One”' });

    toggle.focus();
    await view.user.keyboard('{Enter}');
    await waitFor(() => expect(toggle.getAttribute('aria-label')).toBe('Resume “Game One”'));
    expect(toggle.isConnected).toBe(true);
    expect(document.activeElement).toBe(toggle);
    expect(panel('Game One').dataset.form).toBe('compact');

    await view.user.keyboard('{Enter}');
    await waitFor(() => expect(toggle.getAttribute('aria-label')).toBe('Pause “Game One”'));
    expect(document.activeElement).toBe(toggle);
    expect(panel('Game One').dataset.form).toBe('full');
  });

  it('a pause Go refuses leaves the button as it was, focused, and says why by its code', async () => {
    vi.mocked(pauseDownload).mockRejectedValue(new Error('typhon:download.unavailable'));
    const view = show([download()]);
    const toggle = within(panel('Game One')).getByRole('button', { name: 'Pause “Game One”' });

    toggle.focus();
    await view.user.keyboard('{Enter}');
    await waitFor(() => expect(get(toasts).map((t) => t.message)).toContain(installErrorText(new Error('typhon:download.unavailable'))));
    expect(toggle.getAttribute('aria-label')).toBe('Pause “Game One”');
    expect(toggle.hasAttribute('aria-busy')).toBe(false);
    expect(document.activeElement).toBe(toggle);
  });

  it('a double press sends one request, the button busy and focused while it runs', async () => {
    const call = deferred();
    vi.mocked(pauseDownload).mockReturnValue(call.promise);
    const view = show([download()]);
    const toggle = within(panel('Game One')).getByRole('button', { name: 'Pause “Game One”' });

    await view.user.click(toggle);
    await view.user.click(toggle);
    expect(pauseDownload).toHaveBeenCalledTimes(1);
    expect(toggle.getAttribute('aria-busy')).toBe('true');
    expect(toggle.hasAttribute('disabled')).toBe(false);

    call.resolve();
    await waitFor(() => expect(toggle.hasAttribute('aria-busy')).toBe(false));
  });

  it('a resumed download that goes to the queue hands focus to the panel now in its place', async () => {
    vi.mocked(resumeDownload).mockImplementation(async (id) => goSets(id, { status: 'queued' }));
    const view = show([download({ status: 'paused' }), download({ id: 'd2', name: 'Game Two' })]);
    const toggle = within(panel('Game One')).getByRole('button', { name: 'Resume “Game One”' });

    toggle.focus();
    await view.user.keyboard('{Enter}');
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Game One' })).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(within(panel('Game Two')).getByRole('button', { name: 'Pause “Game Two”' })));
  });
});

describe('the panel of a download under way: Cancel means Go’s Cancel and asks first (D-2, D-7)', () => {
  async function openCancel(view: ReturnType<typeof show>, name: string) {
    const more = within(panel(name)).getByRole('button', { name: `More actions for “${name}”` });
    more.focus();
    await view.user.keyboard('{Enter}');
    const item = screen.getByRole('menuitem', { name: 'Cancel download' });
    item.focus();
    await view.user.keyboard('{Enter}');
    return screen.getByRole('dialog', { name: `Cancel downloading “${name}”?` });
  }

  it('names the download and the data it deletes; confirmed, the panel goes and focus goes to the one now in its place', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    const onannounce = vi.fn();
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })], { onannounce });

    const dialog = await openCancel(view, 'Game One');
    expect(dialog.textContent).toContain(`the ${bytesSize(420 * MB)} downloaded so far is deleted from disk`);
    await view.user.click(within(dialog).getByRole('button', { name: 'Cancel download' }));

    await waitFor(() => expect(screen.queryByRole('region', { name: 'Game One' })).toBeNull());
    expect(cancelDownload).toHaveBeenCalledWith('d1');
    await waitFor(() =>
      expect(document.activeElement).toBe(within(panel('Game Two')).getByRole('button', { name: 'More actions for “Game Two”' })),
    );
    expect(onannounce).toHaveBeenCalledWith('Download “Game One” cancelled');
  });

  it('the last panel to go hands focus to the one above it, and the only one to the heading the page gives', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })]);

    let dialog = await openCancel(view, 'Game Two');
    await view.user.click(within(dialog).getByRole('button', { name: 'Cancel download' }));
    await waitFor(() =>
      expect(document.activeElement).toBe(within(panel('Game One')).getByRole('button', { name: 'More actions for “Game One”' })),
    );

    dialog = await openCancel(view, 'Game One');
    await view.user.click(within(dialog).getByRole('button', { name: 'Cancel download' }));
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Active' })));
  });

  it('hands focus on also when Go answers before the panel leaves', async () => {
    let removeLater = () => {};
    vi.mocked(cancelDownload).mockImplementation(async (id) => {
      removeLater = () => goRemoves(id);
    });
    const view = show([download(), download({ id: 'd2', name: 'Game Two' })]);

    const dialog = await openCancel(view, 'Game One');
    await view.user.click(within(dialog).getByRole('button', { name: 'Cancel download' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    removeLater();

    await waitFor(() => expect(screen.queryByRole('region', { name: 'Game One' })).toBeNull());
    expect(document.activeElement).toBe(within(panel('Game Two')).getByRole('button', { name: 'More actions for “Game Two”' }));
  });

  it('a cancel Go refuses keeps the dialog open with the error by its code, and the panel as it was', async () => {
    vi.mocked(cancelDownload).mockRejectedValue(new Error('typhon:download.not_found'));
    const view = show([download()]);

    const dialog = await openCancel(view, 'Game One');
    await view.user.click(within(dialog).getByRole('button', { name: 'Cancel download' }));

    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toBe(installErrorText(new Error('typhon:download.not_found'))));
    expect(screen.getByRole('dialog')).toBe(dialog);
    expect(panel('Game One')).toBeTruthy();
    expect(get(toasts)).toEqual([]);
  });

  it('does not promise a deletion Go skips: an in-place or flat download keeps its files', async () => {
    const view = show([download({ inPlace: true, origin: { gameId: 'g1', purpose: 'update' } } as Partial<Download>)]);

    const dialog = await openCancel(view, 'Game One');
    expect(dialog.textContent).toMatch(/downloaded so far stays on disk, in “[^”]+”/);
    expect(dialog.textContent).not.toContain('deleted');
  });

  it('a download with nothing downloaded yet is cancelled at once, with nothing to ask', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    const view = show([download({ status: 'metadata', downloaded: 0, progress: 0, total: 0 })]);
    const more = within(panel('Game One')).getByRole('button', { name: 'More actions for “Game One”' });

    more.focus();
    await view.user.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: 'Cancel download' }).focus();
    await view.user.keyboard('{Enter}');

    await waitFor(() => expect(cancelDownload).toHaveBeenCalledWith('d1'));
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Active' })));
  });
});

describe('the panel of a download under way: what it offers (D-9)', () => {
  async function menuItems(view: ReturnType<typeof show>, name: string): Promise<string[]> {
    await view.user.click(within(panel(name)).getByRole('button', { name: `More actions for “${name}”` }));
    const items = screen.getAllByRole('menuitem').map((item) => item.textContent?.trim() ?? '');
    await view.user.keyboard('{Escape}');
    return items;
  }

  it('never offers "Download now" on a paused download, schedule or not', async () => {
    playThrottle.set({ ...idlePlayThrottle, active: true, schedule: { blocked: true, nextOpenAt: '' } });
    const view = show([download({ status: 'paused' })]);

    expect(within(panel('Game One')).queryByRole('button', { name: /Download now/ })).toBeNull();
    expect(await menuItems(view, 'Game One')).not.toContain('Download now');
  });

  it('"Download now" leads while the schedule holds the download, and turns into Pause on the same focused button', async () => {
    playThrottle.set({ ...idlePlayThrottle, active: true, schedule: { blocked: true, nextOpenAt: '' } });
    vi.mocked(downloadNow).mockImplementation(async (id) => goSets(id, { ignoreSchedule: true }));
    const view = show([download({ downloadSpeed: 0 })]);
    const lead = within(panel('Game One')).getByRole('button', { name: 'Download now “Game One”' });

    lead.focus();
    await view.user.keyboard('{Enter}');
    expect(downloadNow).toHaveBeenCalledWith('d1');
    await waitFor(() => expect(lead.getAttribute('aria-label')).toBe('Pause “Game One”'));
    expect(document.activeElement).toBe(lead);
  });

  it('offers "Start now" on a paused download only while nothing holds downloads back', async () => {
    const view = show([download({ status: 'paused' })]);
    expect(await menuItems(view, 'Game One')).toEqual(['Details', 'Start now', 'Open folder', 'Cancel download']);

    playThrottle.set({ ...idlePlayThrottle, active: true, schedule: { blocked: true, nextOpenAt: '' } });
    expect(await menuItems(view, 'Game One')).not.toContain('Start now');

    playThrottle.set({ ...idlePlayThrottle, active: true, mode: 'pause' });
    expect(await menuItems(view, 'Game One')).not.toContain('Start now');
  });

  it('Start now asks Go to force the start', async () => {
    const view = show([download({ status: 'paused' })]);
    await view.user.click(within(panel('Game One')).getByRole('button', { name: 'More actions for “Game One”' }));
    await view.user.click(screen.getByRole('menuitem', { name: 'Start now' }));
    expect(forceStartDownload).toHaveBeenCalledWith('d1');
  });

  it('a running download offers its details, its folder and Cancel, and no Start now', async () => {
    const view = show([download()]);
    expect(await menuItems(view, 'Game One')).toEqual(['Details', 'Open folder', 'Cancel download']);
  });
});

describe('the panel of a download under way: the speed graph', () => {
  it('reads the panel’s series while drawn, stops in the compact form and after unmount', async () => {
    const view = show([download()]);
    expect(readers.count).toBe(1);
    expect(readers.ids).toContain('d1');

    goSets('d1', { status: 'paused' });
    await waitFor(() => expect(within(panel('Game One')).queryByRole('figure')).toBeNull());
    expect(readers.count).toBe(0);

    goSets('d1', { status: 'downloading' });
    await waitFor(() => expect(readers.count).toBe(1));

    view.unmount();
    expect(readers.count).toBe(0);
  });

  it('captions the last 40 seconds', () => {
    show([download()]);
    expect(within(panel('Game One')).getByRole('figure').textContent).toContain('Speed, last 40 s');
  });
});
