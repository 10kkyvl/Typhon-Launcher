import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

import { locale } from '../i18n';
import type { Download } from '../services/downloads';
import { statusLabels } from '../stores/downloads';
import { renderDom } from '../testing/dom';
import DownloadItem from './DownloadItem.svelte';

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
    downloaded: 420,
    total: 1000,
    downloadSpeed: 2048,
    uploadSpeed: 512,
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

beforeEach(() => {
  locale.set('en');
});

// The row is a button, and a button's content is its name: the row's text says the download and its
// percent, and the bar in it only draws them (a button exposes no progress bar inside it).
describe('a download row', () => {
  function rowName(view: ReturnType<typeof renderDom>, row: Element | null): string {
    let name = '';
    view.getByRole('button', {
      name: (computed, element) => {
        if (element === row) name = computed;
        return element === row;
      },
    });
    return name;
  }

  it('is named after the download and says its percent once, the bar hidden', () => {
    const view = renderDom(DownloadItem, { download: download() });
    const row = view.container.querySelector('.item');

    expect(rowName(view, row)).toContain('Game One 42% ');
    expect(rowName(view, row)).not.toMatch(/42% 42\b/);
    expect(view.queryAllByRole('progressbar')).toEqual([]);
    expect(row?.querySelector<HTMLElement>('.track')?.getAttribute('aria-hidden')).toBe('true');
    expect(row?.querySelector<HTMLElement>('.track')?.dataset.tone).toBe('accent');
  });

  it('shows a paused download in the muted tone', () => {
    const { container } = renderDom(DownloadItem, { download: download({ status: 'paused' }) });
    expect(container.querySelector<HTMLElement>('.track')?.dataset.tone).toBe('muted');
  });

  it('says the metadata stage in words and draws the unknown amount while the torrent fetches it', () => {
    const view = renderDom(DownloadItem, { download: download({ status: 'metadata', progress: 0 }) });
    const row = view.container.querySelector('.item');

    expect(rowName(view, row)).toContain(statusLabels('metadata'));
    expect(rowName(view, row)).not.toContain('%');
    expect(row?.querySelector('.fill')?.classList.contains('indeterminate')).toBe(true);
  });
});
