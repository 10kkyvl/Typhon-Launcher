import { render } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getHandoffInfo } from './lib/services/selfupdate';
import { loadHandoff } from './lib/stores/handoff';
import App from './App.svelte';

// The stores App.svelte starts as it mounts, by name and in order. Each init is replaced by a recorder
// that answers a disposer, so what is pinned is App's own wiring, not what a store does.

const calls = vi.hoisted(() => [] as string[]);
const record = vi.hoisted(
  () =>
    (name: string) =>
    (..._args: unknown[]) => {
      calls.push(name);
      return () => {};
    },
);

// A module may call Go as it loads (the metadata service sets its language); that answer never comes.
vi.mock('@wailsio/runtime', async () => ({
  ...(await import('./lib/testing/dom')).wailsRuntime(),
  Call: { ByID: vi.fn(() => new Promise(() => {})) },
}));
vi.mock('./lib/services/backend', () => ({ inWails: true }));
vi.mock('./lib/services/selfupdate', async (original) => ({
  ...(await original<typeof import('./lib/services/selfupdate')>()),
  getHandoffInfo: vi.fn(),
  handoffRendered: vi.fn(async () => {}),
}));
vi.mock('./lib/services/system', async (original) => ({
  ...(await original<typeof import('./lib/services/system')>()),
  initNativeLanguage: record('initNativeLanguage'),
}));
vi.mock('./lib/stores/window', async (original) => ({ ...(await original<object>()), initWindowState: record('initWindowState') }));
vi.mock('./lib/stores/degraded', async (original) => ({ ...(await original<object>()), initDegradedNotices: record('initDegradedNotices') }));
vi.mock('./lib/stores/gamemode', async (original) => ({ ...(await original<object>()), initGameModeNotices: record('initGameModeNotices') }));
vi.mock('./lib/stores/downloads', async (original) => ({ ...(await original<object>()), initDownloads: record('initDownloads') }));
vi.mock('./lib/stores/playThrottle', async (original) => ({ ...(await original<object>()), initPlayThrottle: record('initPlayThrottle') }));
vi.mock('./lib/stores/achievements', async (original) => ({ ...(await original<object>()), initAchievements: record('initAchievements') }));
vi.mock('./lib/stores/completionHints', async (original) => ({ ...(await original<object>()), initCompletionHints: record('initCompletionHints') }));
vi.mock('./lib/stores/network', async (original) => ({ ...(await original<object>()), initNetwork: record('initNetwork') }));
vi.mock('./lib/stores/install', async (original) => ({ ...(await original<object>()), initInstalls: record('initInstalls') }));
vi.mock('./lib/stores/storage', async (original) => ({ ...(await original<object>()), refreshStorage: record('refreshStorage') }));
vi.mock('./lib/stores/library', async (original) => ({ ...(await original<object>()), initLibrary: record('initLibrary') }));
vi.mock('./lib/stores/relocate', async (original) => ({ ...(await original<object>()), initMoves: record('initMoves') }));
vi.mock('./lib/stores/sources', async (original) => ({ ...(await original<object>()), initSources: record('initSources') }));
vi.mock('./lib/stores/sourceProgress', async (original) => ({ ...(await original<object>()), initSourceProgress: record('initSourceProgress') }));
vi.mock('./lib/stores/updates', async (original) => ({ ...(await original<object>()), initUpdates: record('initUpdates') }));
vi.mock('./lib/stores/selfupdate', async (original) => ({ ...(await original<object>()), initSelfUpdate: record('initSelfUpdate') }));
vi.mock('./lib/stores/metadata', async (original) => ({ ...(await original<object>()), initMetadata: record('initMetadata') }));
vi.mock('./lib/stores/discovery', async (original) => ({ ...(await original<object>()), initDiscovery: record('initDiscovery') }));
vi.mock('./lib/stores/user', async (original) => ({ ...(await original<object>()), initAuth: record('initAuth') }));
vi.mock('./lib/stores/messaging', async (original) => ({ ...(await original<object>()), initMessaging: record('initMessaging') }));
vi.mock('./lib/stores/history', async (original) => ({ ...(await original<object>()), initHistory: record('initHistory') }));
vi.mock('./lib/stores/watchlist', async (original) => ({ ...(await original<object>()), initWatchlist: record('initWatchlist') }));
vi.mock('./lib/stores/screenshots', async (original) => ({ ...(await original<object>()), initScreenshots: record('initScreenshots') }));
vi.mock('./lib/stores/launchNavigation', async (original) => ({ ...(await original<object>()), initLaunchNavigation: record('initLaunchNavigation') }));
vi.mock('./lib/stores/lan', async (original) => ({ ...(await original<object>()), initLan: record('initLan') }));
vi.mock('./lib/stores/social', async (original) => ({ ...(await original<object>()), initSocial: record('initSocial') }));
vi.mock('./lib/stores/support', async (original) => ({ ...(await original<object>()), initSupport: record('initSupport') }));
vi.mock('./lib/stores/presence', async (original) => ({ ...(await original<object>()), initPresence: record('initPresence') }));
vi.mock('./lib/stores/savebackup', async (original) => ({ ...(await original<object>()), initSaveBackups: record('initSaveBackups') }));
vi.mock('./lib/stores/collections', async (original) => ({ ...(await original<object>()), initCollections: record('initCollections') }));
vi.mock('./lib/stores/cloudsaves', async (original) => ({ ...(await original<object>()), initCloudSaves: record('initCloudSaves') }));
vi.mock('./lib/stores/missingImages', async (original) => ({ ...(await original<object>()), initMissingImages: record('initMissingImages') }));
vi.mock('./lib/stores/pendingLaunch', async (original) => ({ ...(await original<object>()), initPendingLaunch: record('initPendingLaunch') }));
vi.mock('./lib/stores/profileAccent', async (original) => ({ ...(await original<object>()), initProfileAccentSync: record('initProfileAccentSync') }));
vi.mock('./lib/stores/legalAcceptance', async (original) => ({ ...(await original<object>()), initLegalAcceptance: record('initLegalAcceptance') }));

const ORDINARY = [
  'initNativeLanguage',
  'initWindowState',
  'initDegradedNotices',
  'initGameModeNotices',
  'initDownloads',
  'initPlayThrottle',
  'initAchievements',
  'initCompletionHints',
  'initNetwork',
  'initInstalls',
  'refreshStorage',
  'initLibrary',
  'initMoves',
  'initSources',
  'initSourceProgress',
  'initUpdates',
  'initSelfUpdate',
  'initMetadata',
  'initDiscovery',
  'initAuth',
  'initMessaging',
  'initHistory',
  'initWatchlist',
  'initScreenshots',
  'initLaunchNavigation',
  'initLan',
  'initSocial',
  'initSupport',
  'initPresence',
  'initSaveBackups',
  'initCollections',
  'initCloudSaves',
  'initMissingImages',
  'initPendingLaunch',
  'initProfileAccentSync',
  'initLegalAcceptance',
];

beforeEach(() => {
  calls.length = 0;
});

describe('an ordinary start', () => {
  it('starts every store as App mounts, in the same order, before the first frame', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue({ active: false, phaseStart: 0, version: '', background: '' });
    await loadHandoff();

    const view = render(App);

    expect(calls).toEqual(ORDINARY);
    view.unmount();
  });
});
