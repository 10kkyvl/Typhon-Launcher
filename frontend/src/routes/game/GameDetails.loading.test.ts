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
