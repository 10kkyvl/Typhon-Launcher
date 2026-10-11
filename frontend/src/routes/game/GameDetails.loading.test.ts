import { describe, expect, it, vi } from 'vitest';
import ts from 'typescript';
import source from './GameDetails.svelte?raw';

// Execute the component's real asynchronous loader with deferred service calls.
// This catches stale data while a second request is pending as well as on failure.
function releasesHarness(load: (id: string) => Promise<unknown[]>) {
  const start = source.indexOf('  async function loadReleases(');
  const end = source.indexOf('\n  $effect', start);
  const js = ts.transpile(source.slice(start, end), { target: ts.ScriptTarget.ES2022 });
  return new Function('getReleasesForGame', 'getReleasesForTitle', `
    let releaseToken = 0, releasesLoading = false, releasesFailed = false, releaseGroups = [];
    ${js}
    return { loadReleases, state: () => ({ releasesLoading, releasesFailed, releaseGroups }) };
  `)(load, load) as { loadReleases: (id: string) => Promise<void>; state: () => { releasesLoading: boolean; releasesFailed: boolean; releaseGroups: unknown[] } };
}

describe('game releases navigation', () => {
  it('removes A downloads immediately and keeps them absent when B fails, then permits retry', async () => {
    let rejectB!: (err: Error) => void;
    const load = vi.fn().mockResolvedValueOnce(['A']).mockImplementationOnce(() => new Promise((_, reject) => { rejectB = reject; })).mockResolvedValueOnce(['B']);
    const h = releasesHarness(load);
    await h.loadReleases('A');
    expect(h.state().releaseGroups).toEqual(['A']);
    const pending = h.loadReleases('B');
    expect(h.state()).toEqual({ releaseGroups: [], releasesLoading: true, releasesFailed: false });
    rejectB(new Error('offline'));
    await pending;
    expect(h.state()).toEqual({ releaseGroups: [], releasesLoading: false, releasesFailed: true });
    await h.loadReleases('B');
    expect(h.state()).toEqual({ releaseGroups: ['B'], releasesLoading: false, releasesFailed: false });
  });

  it('ignores an A response arriving after B', async () => {
    let resolveA!: (value: unknown[]) => void;
    const h = releasesHarness(vi.fn().mockImplementationOnce(() => new Promise(resolve => { resolveA = resolve; })).mockResolvedValueOnce(['B']));
    const a = h.loadReleases('A');
    await h.loadReleases('B');
    resolveA(['A']);
    await a;
    expect(h.state().releaseGroups).toEqual(['B']);
  });
});

function metaHarness(deps: { getMetadataView: () => Promise<unknown>; ensureMetadataFresh: () => Promise<boolean> }) {
  const start = source.indexOf('  async function loadMetaView(');
  const end = source.indexOf('\n  $effect', start);
  const js = ts.transpile(source.slice(start, end), { target: ts.ScriptTarget.ES2022 });
  const toast = vi.fn();
  const run = new Function('deps', 'toast', `
    const { getMetadataView, ensureMetadataFresh } = deps;
    const metadataErrorText = (err, fallback) => fallback;
    const msg = (key) => key;
    const preferView = (current, next) => next;
    let metaToken = 0, metaEventVersion = 0, metaReading = false, metaView = null, metaSearching = false;
    ${js}
    return { loadMetaView, state: () => ({ metaReading, metaView, metaSearching }) };
  `)(deps, toast) as { loadMetaView: (id: string) => Promise<void>; state: () => { metaReading: boolean; metaView: unknown; metaSearching: boolean } };
  return { ...run, toast };
}

describe('game metadata loading', () => {
  it('tells the user when the metadata view could not be read, and stops the spinner', async () => {
    const h = metaHarness({
      getMetadataView: () => Promise.reject(new Error('disk offline')),
      ensureMetadataFresh: () => Promise.resolve(false),
    });

    await expect(h.loadMetaView('g1')).resolves.toBeUndefined();

    expect(h.toast).toHaveBeenCalledWith('games.detailMetaLoadError', 'danger');
    expect(h.state()).toMatchObject({ metaReading: false, metaView: null });
  });

  it('tells the user when the freshness check could not start', async () => {
    const h = metaHarness({
      getMetadataView: () => Promise.resolve({ match: 'idle' }),
      ensureMetadataFresh: () => Promise.reject(new Error('disk offline')),
    });

    await h.loadMetaView('g1');

    expect(h.toast).toHaveBeenCalledWith('games.detailMetaLoadError', 'danger');
    expect(h.state().metaReading).toBe(false);
  });

  it('shows nothing when both calls succeed', async () => {
    const h = metaHarness({
      getMetadataView: () => Promise.resolve({ match: 'idle' }),
      ensureMetadataFresh: () => Promise.resolve(true),
    });

    await h.loadMetaView('g1');

    expect(h.toast).not.toHaveBeenCalled();
    expect(h.state()).toMatchObject({ metaReading: false, metaSearching: true });
  });
});
