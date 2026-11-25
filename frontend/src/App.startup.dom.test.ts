import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getHandoffInfo } from './lib/services/selfupdate';
import { get } from 'svelte/store';
import { loadHandoff } from './lib/stores/handoff';
import { endUpdateShow, resumeFinishedShow, updateShow } from './lib/stores/updateShow';
import { authState, bootSplash, retryBootstrap } from './lib/stores/user';
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

const handlers = vi.hoisted(() => new Map<string, Array<(event: { data: unknown }) => void>>());

// A module may call Go as it loads (the metadata service sets its language); that answer never comes.
vi.mock('@wailsio/runtime', async () => {
  const runtime = (await import('./lib/testing/dom')).wailsRuntime();
  runtime.Events.On = vi.fn((name: string | undefined, cb: (event: unknown) => void) => {
    if (!name) return () => {};
    const list = handlers.get(name) ?? [];
    list.push(cb as (event: { data: unknown }) => void);
    handlers.set(name, list);
    return () => handlers.set(name, (handlers.get(name) ?? []).filter((other) => other !== cb));
  });
  return { ...runtime, Call: { ByID: vi.fn(() => new Promise(() => {})) } };
});
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

// The stores whose init reaches a Go service that a successor holds back (handoffgate.Startup) until
// the old launcher is gone; in a successor they start when Go says so, in this order.
const HELD = [
  'initGameModeNotices',
  'initDownloads',
  'initPlayThrottle',
  'initNetwork',
  'initInstalls',
  'initLibrary',
  'initMoves',
  'initSources',
  'initUpdates',
  'initMetadata',
  'initLan',
  'initSocial',
  'initPresence',
  'initSaveBackups',
  'initCollections',
  'initCloudSaves',
  'initMissingImages',
];
const EARLY = ORDINARY.filter((name) => !HELD.includes(name));

const ACTIVE = { active: true, phaseStart: 1_700_000_000_000, version: '1.5.0', background: '#101418', waiting: false };
const DONE = 'launcher:handoff_done';

function handOver() {
  for (const handler of [...(handlers.get(DONE) ?? [])]) handler({ data: { ...ACTIVE, active: false } });
}

beforeEach(() => {
  calls.length = 0;
  handlers.clear();
});

describe('an ordinary start', () => {
  it('starts every store as App mounts, in the same order, before the first frame', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue({ active: false, phaseStart: 0, version: '', background: '', waiting: false });
    await loadHandoff();

    const view = render(App);

    expect(calls).toEqual(ORDINARY);
    view.unmount();
  });
});

describe('a successor of a seamless restart', () => {
  it('starts only what Go does not hold back, and the rest once the old launcher is gone, in its order', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue(ACTIVE);
    await loadHandoff();

    const view = render(App);

    expect(calls).toEqual(EARLY);
    handOver();
    expect(calls).toEqual([...EARLY, ...HELD]);
    handOver();
    expect(calls).toEqual([...EARLY, ...HELD]);
    view.unmount();
  });

  it('whose old launcher never goes keeps the show held and starts nothing held, on no clock of its own', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'] });
    vi.mocked(getHandoffInfo).mockResolvedValue(ACTIVE);
    await loadHandoff();

    const view = render(App);
    // Past the 90 s after which Go logs that the old launcher is still going and waits on.
    vi.advanceTimersByTime(120_000);

    expect(calls).toEqual(EARLY);
    expect(get(updateShow)).toMatchObject({ held: true, phase: 'run' });
    view.unmount();
  });

  it('starts everything at once when the old launcher was gone before the app mounted', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue(ACTIVE);
    await loadHandoff();
    handOver();

    const view = render(App);

    expect(calls).toEqual(ORDINARY);
    view.unmount();
  });

  it('with Go not saying whether it is a successor, is an ordinary start', async () => {
    vi.mocked(getHandoffInfo).mockRejectedValue(new Error('boom'));
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
    await loadHandoff();
    quiet.mockRestore();

    const view = render(App);

    expect(calls).toEqual(ORDINARY);
    view.unmount();
  });
});

// Last in the file: the retried sign-in below is left waiting for an answer that never comes.
describe('the update show over the sign-in that could not reach the server', () => {
  it('stays up while a retried sign-in passes through "bootstrapping" with the screen kept', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] });
    const rect = (left: number, top: number, width: number, height: number) =>
      ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top, toJSON: () => ({}) }) as DOMRect;
    const boxes = vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      if (this.matches('[data-logo-origin]')) return rect(20, 30, 28, 28);
      if (this.matches('.funnel-box')) return rect(500, 300, 360, 277);
      return rect(0, 0, 0, 0);
    });
    Object.defineProperty(Element.prototype, 'animate', {
      value: () => ({
        cancel: () => {},
        finished: Promise.resolve(),
        set onfinish(done: () => void) {
          done();
        },
      }),
      configurable: true,
      writable: true,
    });
    vi.mocked(getHandoffInfo).mockResolvedValue({ active: false, phaseStart: 0, version: '', background: '', waiting: false });
    await loadHandoff();
    // The successors above leave their held shows behind.
    endUpdateShow();
    const view = render(App);
    try {
      authState.set('unavailable');
      await tick();
      // The start after an ordinary update: a finish with no words, owed and not held.
      expect(resumeFinishedShow()).toBe(true);
      await tick();
      const shown = () => document.querySelector<HTMLElement>('[data-testid="update-show"]');
      expect(shown()?.dataset.stage).toBe('finish');

      void retryBootstrap();
      await tick();
      expect(get(authState)).toBe('bootstrapping');
      expect(get(bootSplash)).toBe(false);
      expect(shown()?.dataset.stage).toBe('finish');
      expect(get(updateShow)).not.toBeNull();
    } finally {
      view.unmount();
      endUpdateShow();
      boxes.mockRestore();
      Reflect.deleteProperty(Element.prototype, 'animate');
      vi.useRealTimers();
    }
  });
});
