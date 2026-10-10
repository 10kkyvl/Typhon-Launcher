import { describe, expect, it, vi } from 'vitest';
import ts from 'typescript';
import source from './GamePage.svelte?raw';
import { LatestRequestGate } from '../latestRequest';

interface Deps {
  getCatalogGame: () => Promise<unknown>;
  getMetadataView: () => Promise<unknown>;
  ensureMetadataFresh: () => Promise<boolean>;
}

function harness(deps: Deps) {
  const start = source.indexOf('  async function loadDetails(');
  const end = source.indexOf('\n  async function loadReleases', start);
  const js = ts.transpile(source.slice(start, end), { target: ts.ScriptTarget.ES2022 });
  const ensureMetadataFresh = vi.fn(deps.ensureMetadataFresh);
  const run = new Function('deps', 'gameRequests', 'ensureMetadataFresh', `
    const { getCatalogGame, getMetadataView } = deps;
    const metadataErrorText = (err, fallback) => fallback;
    const bp = (key) => key;
    let detailLoadId = '', detailsLoading = false, detailsFailed = false, catalogGame = null, metadata = null, activeTab = '', metadataError = '';
    ${js}
    return {
      loadDetails,
      state: () => ({ detailsLoading, detailsFailed, catalogGame, metadata, metadataError }),
    };
  `)(deps, new LatestRequestGate(), ensureMetadataFresh) as {
    loadDetails: (id: string, game: unknown) => Promise<void>;
    state: () => { detailsLoading: boolean; detailsFailed: boolean; catalogGame: unknown; metadata: unknown; metadataError: string };
  };
  return { ...run, ensureMetadataFresh };
}

const local = { canonicalGameId: 'c1' };

describe('big picture game page metadata', () => {
  it('says so, and keeps the local page, when the catalog lookup failed', async () => {
    const h = harness({
      getCatalogGame: () => Promise.reject(new Error('catalog offline')),
      getMetadataView: () => Promise.resolve({ resolved: true }),
      ensureMetadataFresh: () => Promise.resolve(false),
    });

    await h.loadDetails('c1', local);

    expect(h.state()).toMatchObject({ detailsFailed: true, catalogGame: null, metadataError: 'bp.game.loadFailed' });
  });

  it('clears the previous error when the details are loaded again', async () => {
    let fail = true;
    const h = harness({
      getCatalogGame: () => Promise.resolve({ id: 'c1' }),
      getMetadataView: () => (fail ? Promise.reject(new Error('disk offline')) : Promise.resolve({ resolved: true })),
      ensureMetadataFresh: () => Promise.resolve(false),
    });

    await h.loadDetails('c1', local);
    expect(h.state().metadataError).toBe('bp.game.loadFailed');
    fail = false;
    await h.loadDetails('c2', local);

    expect(h.state().metadataError).toBe('');
  });

  it('keeps the catalog entry and says so when the metadata view could not be read', async () => {
    const h = harness({
      getCatalogGame: () => Promise.resolve({ id: 'c1' }),
      getMetadataView: () => Promise.reject(new Error('disk offline')),
      ensureMetadataFresh: () => Promise.resolve(false),
    });

    await h.loadDetails('c1', local);

    expect(h.state()).toEqual({
      detailsLoading: false,
      detailsFailed: false,
      catalogGame: { id: 'c1' },
      metadata: null,
      metadataError: 'bp.game.loadFailed',
    });
    expect(h.ensureMetadataFresh).not.toHaveBeenCalled();
  });

  it('says so when the freshness check could not start', async () => {
    const h = harness({
      getCatalogGame: () => Promise.resolve({ id: 'c1' }),
      getMetadataView: () => Promise.resolve({ resolved: true }),
      ensureMetadataFresh: () => Promise.reject(new Error('disk offline')),
    });

    await h.loadDetails('c1', local);
    await vi.waitFor(() => expect(h.state().metadataError).toBe('bp.game.loadFailed'));

    expect(h.state()).toMatchObject({ metadata: { resolved: true }, detailsFailed: false, detailsLoading: false });
  });

  it('shows no error when everything loaded', async () => {
    const h = harness({
      getCatalogGame: () => Promise.resolve({ id: 'c1' }),
      getMetadataView: () => Promise.resolve({ resolved: true }),
      ensureMetadataFresh: () => Promise.resolve(true),
    });

    await h.loadDetails('c1', local);

    expect(h.state()).toMatchObject({ metadataError: '', detailsFailed: false, metadata: { resolved: true } });
    expect(h.ensureMetadataFresh).toHaveBeenCalledWith('c1');
  });

  it('still reports a game found neither in the catalog nor in the library', async () => {
    const h = harness({
      getCatalogGame: () => Promise.resolve(null),
      getMetadataView: () => Promise.resolve({ resolved: false }),
      ensureMetadataFresh: () => Promise.resolve(false),
    });

    await h.loadDetails('x1', undefined);

    expect(h.state().detailsFailed).toBe(true);
  });
});

describe('big picture game page error state', () => {
  const start = source.indexOf('  async function loadDetails(');
  const end = source.indexOf('\n  async function loadReleases', start);
  const outside = (source.slice(0, start) + source.slice(end)).replace("let metadataError = $state('')", '');

  it('lets only the details loader write the metadata error, so no action clears it', () => {
    expect(/\bmetadataError\s*=(?!=)/.test(outside)).toBe(false);
  });

  it('shows the metadata error as an alert of its own', () => {
    expect(/\{#if metadataError\}<div class="page-error" role="alert">\{metadataError\}<\/div>\{\/if\}/.test(source)).toBe(true);
  });
});
