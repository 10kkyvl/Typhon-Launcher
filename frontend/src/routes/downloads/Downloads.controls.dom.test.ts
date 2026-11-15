import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import { idlePlayThrottle } from '../../lib/services/playThrottle';
import { downloads } from '../../lib/stores/downloads';
import { playThrottle } from '../../lib/stores/playThrottle';
import { resetHistory, route } from '../../lib/stores/router';
import { ruleBody, styleOf } from '../../lib/testing/css';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';
import completedList from './CompletedList.svelte?raw';
import failedList from './FailedList.svelte?raw';
import queueRow from './QueueRow.svelte?raw';

// The parts of the screen that draw a row with a name, each with its own copy of the row's rules.
const ROW_PARTS: Record<string, string> = { FailedList: failedList, QueueRow: queueRow, CompletedList: completedList };

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

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
    downloaded: 1000,
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
    completedAt: '2026-09-08T01:00:00Z',
    error: '',
    origin: {},
    ...patch,
  };
}

afterEach(() => {
  downloads.set([]);
  resetHistory();
  document.body.replaceChildren();
});

describe('Downloads: the cover of a row', () => {
  it('says nothing of its own: the row already names the download', () => {
    downloads.set([
      download({ id: 'f', name: 'Failed One', status: 'failed', error: 'x' }),
      download({ id: 'q', name: 'Queued One', status: 'queued', progress: 0 }),
      download({ id: 'c', name: 'Done One' }),
    ]);
    renderDom(Downloads);

    for (const name of ['Failed One', 'Queued One', 'Done One']) {
      expect(screen.queryByLabelText(name), name).toBeNull();
      expect(screen.queryByRole('img', { name }), name).toBeNull();
    }
  });
});

describe('Downloads: downloads held while a game runs', () => {
  afterEach(() => playThrottle.set(idlePlayThrottle));

  it('is said on the base notice, announced as a status, with its settings as the action', async () => {
    playThrottle.set({ ...idlePlayThrottle, active: true, mode: 'pause' });
    const { user } = renderDom(Downloads);

    const status = await screen.findByRole('status');
    expect(status.textContent).toContain(msg('transfers.playPausedBanner'));
    const notice = status.closest('.notice') as HTMLElement;
    expect(notice).not.toBeNull();
    const configure = within(notice).getByRole('button', { name: msg('transfers.playBannerSettings') });
    configure.focus();
    await user.keyboard('{Enter}');

    expect(get(route)).toMatchObject({ name: 'settings', params: { tab: 'downloads' } });
  });
});

describe('Downloads: no active download', () => {
  it('says so with the base empty state, as a line under its heading', () => {
    downloads.set([download({ id: 'c', name: 'Done One' })]);
    renderDom(Downloads);

    expect(screen.getByText(msg('transfers.downloadsEmptyActiveTitle')).closest('.empty.inline')).not.toBeNull();
  });
});

describe('Downloads: links', () => {
  it('the name of a finished download is a text link that opens its details', async () => {
    downloads.set([download({ id: 'c', name: 'Done One' })]);
    const { user } = renderDom(Downloads);

    const link = screen.getByRole('button', { name: 'Done One' });
    expect(link.classList.contains('text-link')).toBe(true);
    link.focus();
    await user.keyboard('{Enter}');

    expect(await screen.findByRole('dialog')).toBeTruthy();
  });

  // Decision В13: a failed download's name leads to its game's page; its details have a control named
  // with the download.
  it('the name of a failed download is a text link to its game’s page; Details opens its details', async () => {
    downloads.set([download({ id: 'f', name: 'Failed One', status: 'failed', error: 'x', origin: { gameId: 'g-f' } })]);
    const { user } = renderDom(Downloads);

    const link = screen.getByRole('button', { name: 'Failed One' });
    expect(link.classList.contains('text-link')).toBe(true);
    screen.getByRole('button', { name: msg('downloads.failed.detailsFor', { title: 'Failed One' }) }).focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('dialog')).toBeTruthy();

    link.focus();
    await user.keyboard('{Enter}');
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-f' } });
  });

  // A text link given an action is a button, an atomic box: an ellipsis on the line around it would
  // hide the whole link, so a long name wraps inside the row instead, shown in full.
  it('a long download name wraps inside its row, shown in full, with no hint repeating it', () => {
    downloads.set([
      download({ id: 'f', name: 'Failed One', status: 'failed', error: 'x', origin: { gameId: 'g-f' } }),
      download({ id: 'c', name: 'Done One' }),
    ]);
    renderDom(Downloads);

    for (const name of ['Failed One', 'Done One']) {
      const link = screen.getByRole('button', { name });
      expect(link.closest('.title')?.classList.contains('wraps'), name).toBe(true);
      expect(link.closest('.tooltip-host'), name).toBeNull();
    }
    for (const [part, source] of Object.entries(ROW_PARTS)) {
      const wraps = ruleBody(styleOf(source, part), '.title.wraps');
      expect(wraps['white-space'], part).toBe('normal');
      expect(wraps['overflow-wrap'], part).toBe('anywhere');
      expect(wraps.overflow, part).toBe('visible');
    }
  });

  // The row takes no focus: a hint on its name reaches a mouse only. The name is shown whole instead.
  it('a queued download’s name wraps inside its row too, with no hover-only hint', () => {
    downloads.set([download({ id: 'q', name: 'Queued One', status: 'queued', progress: 0 })]);
    renderDom(Downloads);

    const name = screen.getByText('Queued One');
    expect(name.hasAttribute('title')).toBe(false);
    expect(name.closest('.title')?.classList.contains('wraps')).toBe(true);
  });

  it('"history" is a text link under the page title that goes to the history', async () => {
    const { user } = renderDom(Downloads);
    const link = screen.getByRole('button', { name: msg('downloads.page.history') });
    expect(link.classList.contains('text-link')).toBe(true);
    expect(link.closest('header.page-header .meta')).not.toBeNull();
    link.focus();

    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('history');
  });
});
