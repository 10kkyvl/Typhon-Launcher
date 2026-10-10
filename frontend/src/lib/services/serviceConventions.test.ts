import { beforeEach, describe, expect, it, vi } from 'vitest';
import { answer, calls, resetBindings } from '../testing/fakeBinding';

const FAKE = '../testing/fakeBinding';

vi.mock('../../../bindings/typhon/internal/history', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('history'),
}));
vi.mock('../../../bindings/typhon/internal/lan', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('lan'),
}));
vi.mock('../../../bindings/typhon/internal/install', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('install'),
}));
vi.mock('../../../bindings/typhon/internal/library', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('library'),
}));
vi.mock('../../../bindings/typhon/internal/compat/service', async () => (await import(FAKE)).fakeBinding('compat'));
vi.mock('../../../bindings/typhon/internal/metadata', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('metadata'),
}));
vi.mock('../../../bindings/typhon/internal/relocate', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('relocate'),
}));
vi.mock('../../../bindings/typhon/internal/savebackup', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('savebackup'),
}));
vi.mock('../../../bindings/typhon/internal/sources', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('sources'),
}));
vi.mock('../../../bindings/typhon/internal/updates', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('updates'),
}));
vi.mock('../../../bindings/typhon/internal/theme', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('theme'),
}));
vi.mock('../../../bindings/typhon/internal/legal', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('legal'),
}));
vi.mock('../../../bindings/typhon/internal/search', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('search'),
}));
vi.mock('../../../bindings/typhon/internal/discovery', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('discovery'),
}));
vi.mock('../../../bindings/typhon/internal/catalog', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('catalog'),
}));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('diagnostics'),
}));

const loaders = import.meta.glob([
  './history.ts',
  './lan.ts',
  './install.ts',
  './library.ts',
  './compat.ts',
  './metadata.ts',
  './relocate.ts',
  './savebackup.ts',
  './sources.ts',
  './updates.ts',
  './theme.ts',
  './legal.ts',
  './search.ts',
  './discovery.ts',
  './recommendations.ts',
  './logsUpload.ts',
]);

type Fn = (...args: unknown[]) => Promise<unknown>;

async function service(name: string, inWails: boolean): Promise<Record<string, Fn>> {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  const load = loaders[`./${name}.ts`];
  expect(load, name).toBeTypeOf('function');
  return (await load()) as Record<string, Fn>;
}

beforeEach(() => {
  resetBindings();
});

interface ListCase {
  module: string;
  fn: string;
  args: unknown[];
  binding: string;
  empty: unknown;
  converted?: boolean;
}

const LISTS: ListCase[] = [
  { module: 'history', fn: 'listHistory', args: [], binding: 'history.List', empty: [] },
  { module: 'history', fn: 'recentHistory', args: [5], binding: 'history.Recent', empty: [] },
  { module: 'lan', fn: 'getPeers', args: [], binding: 'lan.Peers', empty: [] },
  { module: 'lan', fn: 'getOffers', args: [], binding: 'lan.Available', empty: [] },
  { module: 'lan', fn: 'getShares', args: [], binding: 'lan.Shares', empty: [] },
  { module: 'install', fn: 'listInstallations', args: [], binding: 'install.List', empty: [] },
  { module: 'library', fn: 'getGames', args: [], binding: 'library.GetGames', empty: [] },
  { module: 'library', fn: 'getRunningGames', args: [], binding: 'library.GetRunningGames', empty: [] },
  { module: 'relocate', fn: 'listMoves', args: [], binding: 'relocate.List', empty: [] },
  { module: 'sources', fn: 'listSources', args: [], binding: 'sources.ListSources', empty: [] },
  { module: 'sources', fn: 'getReleasesForGame', args: ['g'], binding: 'sources.GetReleasesForGame', empty: [] },
  { module: 'sources', fn: 'getReleasesForTitle', args: ['t'], binding: 'sources.GetReleasesForTitle', empty: [] },
  { module: 'sources', fn: 'getMatchCandidates', args: ['r'], binding: 'sources.GetCandidates', empty: [] },
  { module: 'updates', fn: 'listUpdates', args: [], binding: 'updates.GetUpdates', empty: [] },
  { module: 'updates', fn: 'checkUpdates', args: [], binding: 'updates.CheckUpdates', empty: [] },
  { module: 'updates', fn: 'getUpdateHistory', args: ['g'], binding: 'updates.GetHistory', empty: [] },
  { module: 'theme', fn: 'listThemes', args: [], binding: 'theme.List', empty: [], converted: true },
  { module: 'metadata', fn: 'findMetadataCandidates', args: ['g'], binding: 'metadata.FindCandidates', empty: [] },
  { module: 'metadata', fn: 'searchMetadataCandidates', args: ['q'], binding: 'metadata.SearchCandidates', empty: [] },
  { module: 'metadata', fn: 'getGameArt', args: [['g']], binding: 'metadata.GetArt', empty: {} },
  { module: 'metadata', fn: 'ensureArt', args: [['g']], binding: 'metadata.EnsureArt', empty: [] },
];

describe('lists that Go may send as null', () => {
  it.each(LISTS)('$module / $fn gives an empty collection for null', async ({ module, fn, args, binding, empty }) => {
    answer(binding, null);
    const api = await service(module, true);

    await expect(api[fn](...args)).resolves.toEqual(empty);
    expect(calls.map((call) => call.key)).toContain(binding);
  });

  it.each(LISTS)('$module / $fn returns what Go sent when there is something', async ({ module, fn, args, binding, empty, converted }) => {
    const sent = Array.isArray(empty) ? [{ id: 'x' }] : { g: { cover: 'c' } };
    answer(binding, sent);
    const api = await service(module, true);

    const result = await api[fn](...args);

    if (converted) expect(result).toHaveLength(1);
    else expect(result).toEqual(sent);
  });

  it.each(LISTS)('$module / $fn does not touch the backend in a browser preview', async ({ module, fn, args, empty }) => {
    const api = await service(module, false);

    const result = await api[fn](...args);

    if (module === 'metadata' && fn === 'ensureArt') expect(result).toEqual(args[0]);
    else expect(result).toEqual(empty);
    expect(calls.filter((call) => !call.key.endsWith('.SetLanguage'))).toEqual([]);
  });

  it('the compat journal is an empty map for null', async () => {
    answer('compat.All', null);
    const api = await service('compat', true);

    const map = (await api.compatStatuses()) as unknown as Map<string, unknown>;
    expect(map).toBeInstanceOf(Map);
    expect(map.size).toBe(0);
  });

  it('the compat journal is keyed by game', async () => {
    answer('compat.All', [{ gameId: 'a', state: 'works' }, { gameId: 'b', state: 'broken' }]);
    const api = await service('compat', true);

    const map = (await api.compatStatuses()) as unknown as Map<string, { state: string }>;
    expect(map.get('b')?.state).toBe('broken');
    expect([...map.keys()]).toEqual(['a', 'b']);
  });

  it('a search that found nothing has empty lists, not nulls', async () => {
    const api = await service('search', true);

    answer('search.Search', null);
    await expect(api.searchAll('zelda')).resolves.toEqual({ query: 'zelda', games: [], releases: [], moreGames: 0, moreReleases: 0 });

    answer('search.Search', { query: 'zelda', games: null, releases: null, moreGames: 0, moreReleases: 3 });
    await expect(api.searchAll('zelda')).resolves.toEqual({ query: 'zelda', games: [], releases: [], moreGames: 0, moreReleases: 3 });
  });

  it('discovery items are an empty list when Go sent null', async () => {
    const api = await service('recommendations', true);

    answer('catalog.GetDiscovery', { items: null, fallback: false, profile: {} });
    const result = (await api.getDiscovery({})) as unknown as { items: unknown[] };

    expect(result.items).toEqual([]);
  });

  it('the not-interested list is empty when Go sent null', async () => {
    const api = await service('recommendations', true);

    answer('catalog.GetRecommendationPreferences', { notInterested: null, hideLibrary: true });
    const prefs = (await api.getRecommendationPreferences()) as unknown as { notInterested: string[]; hideLibrary: boolean };

    expect(prefs.notInterested).toEqual([]);
    expect(prefs.hideLibrary).toBe(true);
  });

  it('keeps the defaults for preference fields Go did not send', async () => {
    const api = await service('recommendations', true);

    answer('catalog.GetRecommendationPreferences', { genre: 'rpg' });
    const prefs = (await api.getRecommendationPreferences()) as unknown as Record<string, unknown>;

    expect(prefs).toMatchObject({ genre: 'rpg', hideNotInterested: true, hideLibrary: false, notInterested: [] });
  });

  it('a log upload reports no dropped files as an empty list', async () => {
    const api = await service('logsUpload', true);

    answer('diagnostics.SendLogs', { id: 'u1', dropped: null });
    await expect(api.sendLogs()).resolves.toEqual({ id: 'u1', dropped: [] });
  });
});

interface ActionCase {
  module: string;
  fn: string;
  args: unknown[];
  binding: string;
  sent: unknown[];
}

const ACTIONS: ActionCase[] = [
  { module: 'updates', fn: 'checkGameUpdate', args: ['g'], binding: 'updates.CheckGame', sent: ['g'] },
  { module: 'updates', fn: 'prepareUpdatePlan', args: ['g'], binding: 'updates.PreparePlan', sent: ['g'] },
  { module: 'updates', fn: 'startUpdate', args: ['g'], binding: 'updates.StartUpdate', sent: ['g'] },
  { module: 'updates', fn: 'prefetchUpdate', args: ['g'], binding: 'updates.PrefetchUpdate', sent: ['g'] },
  { module: 'updates', fn: 'cancelUpdate', args: ['g'], binding: 'updates.CancelUpdate', sent: ['g'] },
  { module: 'updates', fn: 'rollbackUpdate', args: ['g'], binding: 'updates.Rollback', sent: ['g'] },
  { module: 'updates', fn: 'verifyGame', args: ['g'], binding: 'updates.VerifyGame', sent: ['g'] },
  { module: 'updates', fn: 'repairGame', args: ['g'], binding: 'updates.RepairGame', sent: ['g'] },
  { module: 'updates', fn: 'buildManifest', args: ['g'], binding: 'updates.BuildManifest', sent: ['g'] },
  { module: 'lan', fn: 'share', args: ['g'], binding: 'lan.Share', sent: ['g'] },
  { module: 'lan', fn: 'unshare', args: ['g'], binding: 'lan.Unshare', sent: ['g'] },
  { module: 'lan', fn: 'receive', args: ['hash', 'peer'], binding: 'lan.Receive', sent: ['hash', 'peer'] },
  { module: 'lan', fn: 'cancel', args: ['t1'], binding: 'lan.Cancel', sent: ['t1'] },
  { module: 'relocate', fn: 'moveGame', args: ['g', 'D:\\Games'], binding: 'relocate.MoveGame', sent: ['g', 'D:\\Games'] },
  { module: 'relocate', fn: 'moveLibrary', args: ['D:\\'], binding: 'relocate.MoveLibrary', sent: ['D:\\'] },
  { module: 'relocate', fn: 'cancelMove', args: ['m1'], binding: 'relocate.Cancel', sent: ['m1'] },
  { module: 'savebackup', fn: 'createBackup', args: ['g'], binding: 'savebackup.Create', sent: ['g'] },
  { module: 'savebackup', fn: 'restoreBackup', args: ['g', 's1'], binding: 'savebackup.Restore', sent: ['g', 's1'] },
  { module: 'savebackup', fn: 'deleteBackup', args: ['g', 's1'], binding: 'savebackup.Delete', sent: ['g', 's1'] },
  { module: 'history', fn: 'clearHistory', args: [], binding: 'history.Clear', sent: [] },
  { module: 'discovery', fn: 'scanInstalledGames', args: [], binding: 'discovery.Scan', sent: [] },
  { module: 'discovery', fn: 'cancelScan', args: [], binding: 'discovery.CancelScan', sent: [] },
  { module: 'sources', fn: 'removeSource', args: ['s1'], binding: 'sources.RemoveSource', sent: ['s1'] },
  { module: 'sources', fn: 'setSourceEnabled', args: ['s1', false], binding: 'sources.SetSourceEnabled', sent: ['s1', false] },
  { module: 'sources', fn: 'confirmMatch', args: ['r1', 'g1'], binding: 'sources.ConfirmMatch', sent: ['r1', 'g1'] },
];

describe('actions that need the desktop app', () => {
  it.each(ACTIONS)('$module / $fn calls $binding with its arguments', async ({ module, fn, args, binding, sent }) => {
    answer(binding, {});
    const api = await service(module, true);

    await api[fn](...args);

    expect(calls.filter((call) => call.key === binding)).toEqual([{ key: binding, args: sent }]);
  });

  it.each(ACTIONS)('$module / $fn refuses in a browser preview instead of pretending it worked', async ({ module, fn, args }) => {
    const api = await service(module, false);

    await expect(api[fn](...args)).rejects.toThrow('unavailable in browser');
    expect(calls).toEqual([]);
  });

  it.each(ACTIONS)('$module / $fn lets a backend refusal through', async ({ module, fn, args, binding }) => {
    answer(binding, () => {
      throw new Error('typhon:test.refused: no');
    });
    const api = await service(module, true);

    await expect(api[fn](...args)).rejects.toThrow('test.refused');
  });
});
