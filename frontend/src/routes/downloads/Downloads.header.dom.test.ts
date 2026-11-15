import { within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import { idlePlayThrottle } from '../../lib/services/playThrottle';
import { saveSettingsPatch, type Settings } from '../../lib/services/settings';
import { downloads } from '../../lib/stores/downloads';
import { playThrottle } from '../../lib/stores/playThrottle';
import { resetHistory, route } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { renderDom } from '../../lib/testing/dom';
import { speedBytes } from '../../lib/utils/format';
import Downloads from './Downloads.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

const MB = 1024 ** 2;

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'queued',
    progress: 0,
    downloaded: 0,
    total: 1000,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
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

const queued = (n: number) => Array.from({ length: n }, (_, i) => download({ id: `q${i}`, name: `Queued ${i}` }));

function stored(patch: Partial<Settings> = {}): Settings {
  return { language: 'system', uiScale: 1, animationsEnabled: true, maxActiveDownloads: 2, downloadRateLimit: 0, downloadSchedule: [], ...patch } as Settings;
}

const header = () => document.querySelector('header.page-header') as HTMLElement;
const summary = () => header().querySelector('.meta .summary');

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  settings.set(stored());
});

afterEach(() => {
  downloads.set([]);
  playThrottle.set(idlePlayThrottle);
  settings.set(null);
  resetHistory();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe('Downloads header: the summary under the title', () => {
  it('says nothing when nothing is listed', () => {
    renderDom(Downloads);
    expect(summary()).toBeNull();
  });

  it('counts what downloads, what is paused and what waits, and adds the speeds while data moves', () => {
    downloads.set([
      download({ id: 'a', status: 'downloading', downloadSpeed: 2 * MB, uploadSpeed: MB / 2 }),
      download({ id: 'b', status: 'metadata' }),
      download({ id: 'p', status: 'paused', downloadSpeed: 9 * MB }),
      ...queued(4),
    ]);
    renderDom(Downloads);

    const words = summary()?.textContent ?? '';
    expect(words).toContain(msg('downloads.page.running', { count: 2 }));
    expect(words).toContain(msg('downloads.page.paused', { count: 1 }));
    expect(words).toContain(msg('downloads.page.queued', { count: 4 }));
    // The paused download's last speed is not counted: only a moving download adds to the total.
    expect(words).toContain(msg('downloads.page.speeds', { down: speedBytes(2 * MB), up: speedBytes(MB / 2) }));
  });

  it('says only the queue when nothing else is there, with no speeds', () => {
    downloads.set(queued(4));
    renderDom(Downloads);

    expect(summary()?.textContent?.trim()).toBe(msg('downloads.page.queued', { count: 4 }));
  });

  it('puts the history link beside it, under the title, and it goes to the history', async () => {
    const { user } = renderDom(Downloads);
    const link = within(header()).getByRole('button', { name: msg('downloads.page.history') });
    expect(link.closest('.meta')).not.toBeNull();

    link.focus();
    await user.keyboard('{Enter}');
    expect(get(route).name).toBe('history');
  });
});

describe('Downloads header: the schedule and the banner say the schedule once', () => {
  const line = () => header().querySelector('.schedule');
  const banner = () => document.querySelector('.play-banner');

  it('shows no schedule line without a schedule', () => {
    renderDom(Downloads);
    expect(line()).toBeNull();
  });

  it('names the one window by its start, or the count of windows, and leads to the schedule in Settings', async () => {
    settings.set(stored({ downloadSchedule: [{ days: [1, 2], from: '01:00', to: '07:00' }] }));
    const { user, unmount } = renderDom(Downloads);
    expect(line()?.textContent).toContain(msg('downloads.page.scheduleFrom', { from: '01:00' }));
    unmount();

    settings.set(stored({ downloadSchedule: [{ days: [1], from: '01:00', to: '07:00' }, { days: [6], from: '10:00', to: '12:00' }] }));
    renderDom(Downloads);
    expect(line()?.textContent).toContain(msg('downloads.page.scheduleWindows', { count: 2 }));

    const link = within(line() as HTMLElement).getByRole('button');
    link.focus();
    await user.keyboard('{Enter}');
    expect(get(route)).toMatchObject({ name: 'settings', params: { tab: 'downloads' } });
  });

  it('while the schedule holds downloads, the line says it and no banner repeats it', () => {
    playThrottle.set({ ...idlePlayThrottle, schedule: { blocked: true, nextOpenAt: '' } });
    renderDom(Downloads);

    expect(line()?.textContent).toContain(msg('downloads.page.scheduleHeldNoNext'));
    expect(banner()).toBeNull();
  });

  it('with a game running as well, the banner keeps the game and leaves the schedule to the line', () => {
    playThrottle.set({ active: true, mode: 'pause', downloadLimit: 0, uploadLimit: 0, schedule: { blocked: true, nextOpenAt: '' } });
    renderDom(Downloads);

    const said = banner()?.textContent ?? '';
    expect(said).toContain(msg('transfers.playPausedBanner'));
    expect(said).not.toContain(msg('downloads.page.scheduleHeldNoNext'));
    expect(line()?.textContent).toContain(msg('downloads.page.scheduleHeldNoNext'));
  });
});
