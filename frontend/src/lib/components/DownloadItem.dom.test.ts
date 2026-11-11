import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

import { locale } from '../i18n';
import type { Download } from '../services/downloads';
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

describe('a download row', () => {
  it('has a progress bar named after the download that reads its percent', () => {
    const { getByRole } = renderDom(DownloadItem, { download: download() });
    const bar = getByRole('progressbar', { name: 'Game One' });

    expect(bar.getAttribute('aria-valuenow')).toBe('42');
    expect(bar.getAttribute('aria-valuemax')).toBe('100');
    expect(bar.dataset.tone).toBe('accent');
  });

  it('shows a paused download in the muted tone', () => {
    const { getByRole } = renderDom(DownloadItem, { download: download({ status: 'paused' }) });
    expect(getByRole('progressbar', { name: 'Game One' }).dataset.tone).toBe('muted');
  });

  it('claims no value while the torrent is still fetching its metadata', () => {
    const { getByRole } = renderDom(DownloadItem, { download: download({ status: 'metadata', progress: 0 }) });
    const bar = getByRole('progressbar', { name: 'Game One' });

    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(bar.getAttribute('aria-busy')).toBe('true');
  });
});
