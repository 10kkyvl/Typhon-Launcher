import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import type { Installation } from '../../lib/services/install';
import { downloads, type DownloadProgress } from '../../lib/stores/downloads';
import { installations } from '../../lib/stores/install';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';
import { downloadsSectionIds } from './sections';

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

function installing(downloadId: string): Installation {
  return { id: `i-${downloadId}`, downloadId, gameId: '', name: '', status: 'extracting', progress: 0.5, bytesDone: 1, bytesTotal: 2 } as Installation;
}

/** One download in each of the five sections. */
function everySection() {
  downloads.set([
    download({ id: 'c', name: 'Done One' }),
    download({ id: 'i', name: 'Installing One' }),
    download({ id: 'q', name: 'Queued One', status: 'queued', progress: 0 }),
    download({ id: 'f', name: 'Failed One', status: 'failed', error: 'x' }),
    download({ id: 'a', name: 'Active One', status: 'downloading', progress: 0.5, completedAt: null }),
  ]);
  installations.set([installing('i')]);
}

afterEach(() => {
  downloads.set([]);
  installations.set([]);
  document.body.replaceChildren();
});

describe('Downloads: the sections', () => {
  it('stand in order: active, installing, failed, queue, completed', () => {
    everySection();
    renderDom(Downloads);

    const titles = screen.getAllByRole('heading', { level: 2 }).map((h) => h.firstChild?.textContent?.trim());
    expect(titles).toEqual([
      msg('transfers.downloadsActiveHeading'),
      msg('downloads.done.installingTitle'),
      msg('transfers.downloadsFailedHeading'),
      msg('transfers.downloadsQueueHeading'),
      msg('downloads.done.title'),
    ]);
  });

  it('take their heading ids from sections.ts, one each, and are named by them', () => {
    everySection();
    renderDom(Downloads);

    const named = [...document.querySelectorAll<HTMLElement>('section[aria-labelledby]')]
      .map((section) => section.getAttribute('aria-labelledby') ?? '')
      .filter((id) => document.getElementById(id)?.tagName === 'H2');
    expect([...named].sort()).toEqual(Object.values(downloadsSectionIds).sort());
    for (const id of named) expect(document.getElementById(id)?.getAttribute('tabindex')).toBe('-1');
  });

  it('name the active heading by its id, a focus target for script', () => {
    downloads.set([download({ id: 'a', status: 'downloading', progress: 0.5, completedAt: null })]);
    renderDom(Downloads);

    const heading = document.getElementById(downloadsSectionIds.active) as HTMLElement;
    expect(heading.textContent).toContain(msg('transfers.downloadsActiveHeading'));
    heading.focus();
    expect(document.activeElement).toBe(heading);
    expect(heading.closest('section')?.getAttribute('aria-labelledby')).toBe(downloadsSectionIds.active);
  });
});


describe('Downloads: the screen’s live region', () => {
  const regions = () => [...document.querySelectorAll('[aria-live="polite"]')].filter((el) => !el.closest('.notice, .toast'));

  it('is one polite region that says nothing by itself, a progress tick included', async () => {
    downloads.set([download({ id: 'a', status: 'downloading', progress: 0.2, downloaded: 200, downloadSpeed: 1000, completedAt: null })]);
    renderDom(Downloads);
    expect(regions()).toHaveLength(1);

    const tick: DownloadProgress = { id: 'a', status: 'downloading', progress: 0.6, downloaded: 600, downloadSpeed: 4000, uploadSpeed: 0, etaSeconds: 3, seeders: 1, peers: 2, stalled: false };
    downloads.update((list) => list.map((d) => (d.id === tick.id ? { ...d, ...tick } : d)));
    await Promise.resolve();

    expect(regions()[0].textContent?.trim()).toBe('');
  });
});
