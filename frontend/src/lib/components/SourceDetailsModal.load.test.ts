import { describe, expect, it, vi } from 'vitest';
import ts from 'typescript';
import raw from './SourceDetailsModal.svelte?raw';
import { LatestRequestGate } from '../bigpicture/latestRequest';

const source = raw.replace(/\r\n/g, '\n');

interface Deps {
  getRelease: (id: string) => Promise<unknown>;
  getSourceDetails: (id: string) => Promise<unknown>;
  queryReleases: (query: { sourceId: string }) => Promise<{ items: unknown[]; total: number }>;
}

interface State {
  details: unknown;
  releases: unknown[];
  total: number;
  loadingReleases: boolean;
  detailsError: string;
  releasesError: string;
  search: string;
}

function harness(deps: Deps) {
  const start = source.indexOf('  $effect(() => {');
  const end = source.indexOf('\n  function reloadAll()', start);
  const js = ts.transpile(source.slice(start, end), { target: ts.ScriptTarget.ES2022 });
  const toast = vi.fn();
  let effect!: () => void;
  const run = new Function('deps', 'gate', 'toast', '$effect', `
    const { getRelease, getSourceDetails, queryReleases } = deps;
    const LatestRequestGate = gate;
    const detailsRequests = new gate();
    const releaseRequests = new gate();
    const untrack = (fn) => fn();
    const sourceErrorText = (err, fallback) => fallback;
    const msg = (key) => key;
    let open = true, sourceId = null, focusReleaseId = null;
    let details = null, releases = [], total = 0, loadingReleases = false, detailsError = '', releasesError = '';
    let filterStatus = 'all', search = '', page = 1, searchTimer;
    const pageSize = 50;
    ${js}
    return {
      show: (id, releaseId = null) => { sourceId = id; focusReleaseId = releaseId; },
      loadDetails,
      loadReleases,
      state: () => ({ details, releases, total, loadingReleases, detailsError, releasesError, search }),
    };
  `)(deps, LatestRequestGate, toast, (fn: () => void) => { effect = fn; }) as {
    show: (id: string | null, releaseId?: string | null) => void;
    loadDetails: (id: string) => Promise<void>;
    loadReleases: (id: string) => Promise<void>;
    state: () => State;
  };
  return { ...run, toast, rerun: () => effect() };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

const ok = (items: unknown[] = [], total = items.length) => Promise.resolve({ items, total });
const base: Deps = {
  getRelease: () => Promise.resolve(null),
  getSourceDetails: () => Promise.resolve({ total: 0 }),
  queryReleases: () => ok(),
};

describe('source details loading', () => {
  it('says so, and shows no releases, when the release list could not be loaded', async () => {
    const h = harness({ ...base, queryReleases: () => Promise.reject(new Error('ipc down')) });

    await h.loadReleases('s1');

    expect(h.state()).toMatchObject({ releasesError: 'modals.sourceDetailsReleasesFailed', releases: [], total: 0, loadingReleases: false });
  });

  it('says so when the source details could not be loaded', async () => {
    const h = harness({ ...base, getSourceDetails: () => Promise.reject(new Error('ipc down')) });

    await h.loadDetails('s1');

    expect(h.state()).toMatchObject({ detailsError: 'modals.sourceDetailsLoadFailed', details: null });
  });

  it('clears the errors when a retry succeeds', async () => {
    let fail = true;
    const h = harness({
      ...base,
      getSourceDetails: () => (fail ? Promise.reject(new Error('x')) : Promise.resolve({ total: 3 })),
      queryReleases: () => (fail ? Promise.reject(new Error('x')) : ok(['r1'], 1)),
    });
    await Promise.all([h.loadDetails('s1'), h.loadReleases('s1')]);
    expect(h.state().detailsError).not.toBe('');
    expect(h.state().releasesError).not.toBe('');

    fail = false;
    await Promise.all([h.loadDetails('s1'), h.loadReleases('s1')]);

    expect(h.state()).toMatchObject({ detailsError: '', releasesError: '', details: { total: 3 }, releases: ['r1'], total: 1 });
  });

  it('keeps a genuinely empty source apart from a failure', async () => {
    const h = harness(base);

    await h.loadReleases('s1');

    expect(h.state()).toMatchObject({ releasesError: '', releases: [], total: 0 });
  });

  it('ignores a slow answer for the previous source after switching', async () => {
    const slow = deferred<{ items: unknown[]; total: number }>();
    const slowDetails = deferred<unknown>();
    const h = harness({
      ...base,
      getSourceDetails: (id) => (id === 'a' ? slowDetails.promise : Promise.resolve({ id })),
      queryReleases: (query) => (query.sourceId === 'a' ? slow.promise : ok(['from-b'], 1)),
    });
    h.show('a');
    h.rerun();
    h.show('b');
    h.rerun();
    await vi.waitFor(() => expect(h.state().releases).toEqual(['from-b']));

    slow.resolve({ items: ['from-a'], total: 9 });
    slowDetails.resolve({ id: 'a' });
    await Promise.resolve();
    await Promise.resolve();

    expect(h.state()).toMatchObject({ releases: ['from-b'], total: 1, details: { id: 'b' }, loadingReleases: false });
  });

  it('does not let a late failure for the previous source show an error on the new one', async () => {
    const slow = deferred<{ items: unknown[]; total: number }>();
    const h = harness({ ...base, queryReleases: (query) => (query.sourceId === 'a' ? slow.promise : ok(['b'], 1)) });
    h.show('a');
    h.rerun();
    h.show('b');
    h.rerun();
    await vi.waitFor(() => expect(h.state().releases).toEqual(['b']));

    slow.reject(new Error('late'));
    await Promise.resolve();
    await Promise.resolve();

    expect(h.state().releasesError).toBe('');
  });

  it('forgets the previous source while the next one loads', async () => {
    const h = harness({
      ...base,
      getSourceDetails: (id) => (id === 'a' ? Promise.resolve({ id }) : new Promise(() => {})),
      queryReleases: (query) => (query.sourceId === 'a' ? ok(['from-a'], 1) : new Promise(() => {})),
    });
    await Promise.all([h.loadDetails('a'), h.loadReleases('a')]);
    expect(h.state()).toMatchObject({ details: { id: 'a' }, releases: ['from-a'] });

    h.show('b');
    h.rerun();

    expect(h.state()).toMatchObject({ details: null, releases: [], total: 0, loadingReleases: true });
  });
});

describe('focusing a release from a notification', () => {
  it('toasts when the release lookup fails and still loads the list', async () => {
    const queryReleases = vi.fn(() => ok());
    const h = harness({ ...base, getRelease: () => Promise.reject(new Error('ipc down')), queryReleases });
    h.show('a', 'r1');

    h.rerun();
    await vi.waitFor(() => expect(queryReleases).toHaveBeenCalledTimes(1));

    expect(h.toast).toHaveBeenCalledWith('modals.sourceDetailsReleaseLoadFailed', 'danger');
  });

  it('puts the release title in the search box', async () => {
    const h = harness({ ...base, getRelease: () => Promise.resolve({ release: { rawTitle: 'Game v1' } }) });
    h.show('a', 'r1');

    h.rerun();
    await vi.waitFor(() => expect(h.state().search).toBe('Game v1'));
  });

  it('stays silent, and loads nothing for the old source, when the user switched away first', async () => {
    const lookup = deferred<unknown>();
    const queryReleases = vi.fn((query: { sourceId: string }) => ok([query.sourceId], 1));
    const h = harness({ ...base, getRelease: () => lookup.promise, queryReleases });
    h.show('a', 'r1');
    h.rerun();
    h.show('b');
    h.rerun();
    await vi.waitFor(() => expect(h.state().releases).toEqual(['b']));

    lookup.reject(new Error('late'));
    await Promise.resolve();
    await Promise.resolve();

    expect(h.toast).not.toHaveBeenCalled();
    expect(queryReleases.mock.calls.map(([query]) => query.sourceId)).toEqual(['b']);
  });
});

describe('source details template', () => {
  it('shows each load failure as an alert with a retry, ahead of the empty state', () => {
    expect(/\{#if detailsError\}\s*<section class="load-error" role="alert">[\s\S]*?loadDetails\(sourceId\)/.test(source)).toBe(true);
    expect(/\{:else if releasesError\}\s*<div class="empty load-error" role="alert">[\s\S]*?loadReleases\(sourceId\)[\s\S]*?\{:else if releases\.length === 0\}/.test(source)).toBe(true);
  });

  it('hides the range footer while the release list is in error', () => {
    expect(/\{#if !releasesError\}\s*<div class="tfoot">/.test(source)).toBe(true);
  });
});
