import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';

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
