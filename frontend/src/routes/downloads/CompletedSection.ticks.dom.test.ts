import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The row's own work, counted: every call is one row redoing its finishing time.
vi.mock('./completedRow', async (actual) => {
  const real = await actual<typeof import('./completedRow')>();
  return { ...real, completedWhen: vi.fn(real.completedWhen) };
});

import type { Download } from '../../lib/services/downloads';
import type { Installation } from '../../lib/services/install';
import { downloads } from '../../lib/stores/downloads';
import { installations } from '../../lib/stores/install';
import { renderDom } from '../../lib/testing/dom';
import CompletedSection from './CompletedSection.svelte';
import { completedWhen } from './completedRow';

function download(id: string, patch: Partial<Download> = {}): Download {
  return {
    id,
    name: `Game ${id}`,
    type: 'torrent',
    source: '',
    infoHash: id,
    destination: `D:/Games/${id}`,
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
    addedAt: '2026-10-28T10:00:00Z',
    completedAt: '2026-10-28T15:00:00Z',
    error: '',
    origin: { gameId: `g-${id}` },
    ...patch,
  };
}

const progressTick = (id: string) =>
  downloads.update((list) => list.map((d) => (d.id === id ? { ...d, downloaded: d.downloaded + 1024, downloadSpeed: 2048 } : d)));

afterEach(() => {
  downloads.set([]);
  installations.set([]);
  document.body.replaceChildren();
});

describe('completed rows and the ticks of other downloads', () => {
  it('ticks of a download under way recompute nothing in the completed rows', async () => {
    installations.set([{ id: 'ib', downloadId: 'b', gameId: 'lib-b', status: 'completed', progress: 1 } as Installation]);
    downloads.set([download('a'), download('b'), download('c'), download('x', { status: 'downloading', progress: 0.1, downloaded: 100, completedAt: null })]);
    renderDom(CompletedSection, { part: 'completed', onopen: vi.fn(), oninstall: vi.fn() });
    await tick();
    const before = vi.mocked(completedWhen).mock.calls.length;
    expect(before).toBeGreaterThan(0);

    for (let i = 0; i < 4; i++) {
      progressTick('x');
      await tick();
    }

    expect(vi.mocked(completedWhen).mock.calls.length).toBe(before);
  });

  it('a change of a completed download itself is shown', async () => {
    downloads.set([download('a'), download('x', { status: 'downloading', completedAt: null })]);
    const view = renderDom(CompletedSection, { part: 'completed', onopen: vi.fn(), oninstall: vi.fn() });
    await tick();

    downloads.update((list) => list.map((d) => (d.id === 'a' ? { ...d, seeding: true } : d)));
    await tick();

    expect(view.container.ownerDocument.querySelector('[data-download-row="a"]')?.textContent).toMatch(/Раздаётся|Seeding/);
  });
});
