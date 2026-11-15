import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import DownloadItem from './DownloadItem.svelte';
import { locale } from '../i18n';
import type { Download } from '../services/downloads';
import { downloads } from '../stores/downloads';
import { speedBytes } from '../utils/format';

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

// The panel reads what Go offers for its download from the store, as the page has it.
function html(item: Download): string {
  downloads.set([item]);
  return render(DownloadItem, { props: { download: item } }).body;
}

beforeEach(() => {
  locale.set('en');
});

afterEach(() => {
  locale.set('ru');
  downloads.set([]);
});

describe('download card', () => {
  it('shows progress, speed and time left for a running download', () => {
    const out = html(download());

    expect(out).toContain('Game One');
    expect(out).toContain('42%');
    expect(out).toContain('data-testid="download-speed"');
    // A reading sets its number and unit in spans of their own: compare the words.
    const words = out.replace(/<[^>]*>/g, '');
    expect(words).toContain(speedBytes(2048));
    expect(words).toContain(`upload ${speedBytes(512)}`);
    expect(out).toContain('Pause');
    expect(out).not.toContain('Resume');
  });

  it('says the download is waiting for sources instead of a frozen bar when it stalled', () => {
    const out = html(download({ stalled: true, stalledSince: '2026-09-08T00:05:00Z', seeders: 0, peers: 0 }));

    expect(out).toContain('class="stalled');
    expect(out).toContain('Pause');
  });

  it('does not call a healthy download stalled', () => {
    expect(html(download({ stalled: false }))).not.toContain('class="stalled');
  });

  it('does not call a paused download stalled even if the flag is left over', () => {
    const out = html(download({ status: 'paused', stalled: true }));

    expect(out).not.toContain('class="stalled');
    expect(out).toContain('Resume');
    expect(out).not.toContain('Pause “');
  });

  it('shows no speed and offers no pause while the files are checked', () => {
    const out = html(download({ status: 'verifying', downloadSpeed: 0, uploadSpeed: 0, etaSeconds: -1 }));

    expect(out).not.toContain('data-testid="download-speed"');
    expect(out).not.toContain('Pause');
    expect(out).not.toContain('Resume');
    expect(out).toContain('More actions for “Game One”');
  });

  it('leaves out the time left when the estimate is unknown', () => {
    const known = html(download({ etaSeconds: 90 }));
    const unknown = html(download({ etaSeconds: -1 }));

    expect(known).not.toBe(unknown);
    expect(unknown.length).toBeLessThan(known.length);
  });

  it('marks a game download with its kind, and an update or repair with its own', () => {
    const game = html(download({ origin: { gameId: 'g1' } }));
    const update = html(download({ origin: { gameId: 'g1', purpose: 'update' } }));
    const repair = html(download({ origin: { gameId: 'g1', purpose: 'repair' } }));
    const plain = html(download({ origin: {} }));

    expect(game).toContain('Game</span>');
    expect(update).toContain('Update');
    expect(repair).toContain('Repair');
    expect(plain).not.toMatch(/>(Game|Update|Repair)</);
  });
});
