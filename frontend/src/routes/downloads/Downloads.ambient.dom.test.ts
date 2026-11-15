import { waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Download } from '../../lib/services/downloads';
import { ambientState } from '../../lib/stores/ambient';
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

describe('Downloads: the backdrop', () => {
  it('claims the game of the first download under way, follows it, and lets go when the page goes', async () => {
    downloads.set([
      download({ id: 'c', status: 'completed', origin: { gameId: 'done-game' } }),
      download({ id: 'p', status: 'paused', progress: 0.2, completedAt: null, origin: { gameId: 'paused-game' } }),
      download({ id: 'a', status: 'downloading', progress: 0.5, completedAt: null, origin: { gameId: 'first-game' } }),
      download({ id: 'b', status: 'downloading', progress: 0.1, completedAt: null, origin: { gameId: 'second-game' } }),
    ]);
    const { unmount } = renderDom(Downloads);
    expect(get(ambientState).claim).toBe('first-game');

    downloads.update((list) => list.filter((d) => d.id !== 'a'));
    await waitFor(() => expect(get(ambientState).claim).toBe('second-game'));

    downloads.update((list) => list.filter((d) => d.id !== 'b'));
    await waitFor(() => expect(get(ambientState).claim).toBe('paused-game'));

    unmount();
    expect(get(ambientState).claim).toBeNull();
  });
});
