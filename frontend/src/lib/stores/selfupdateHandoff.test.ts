import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SelfUpdateStatus } from '../services/selfupdate';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) } }));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  applyUpdate: vi.fn(),
  beginHandoff: vi.fn(),
}));

// The old launcher's side of the seamless restart: the press starts the show and hands Go the moment
// it began; until Go answers, the restart counts as applying on every surface; a refused hand-over
// restarts through the worker without a word; a rolled-back one sends the show home and is said.

const START = 1_700_000_000_000;
const READY: SelfUpdateStatus = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' };
const APPLYING: SelfUpdateStatus = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' };
const ROLLED_BACK: SelfUpdateStatus = { ...READY, error: 'selfupdate: the new launcher did not show its window in time', errorCode: 'handoff_failed' };
const UNSUPPORTED = 'selfupdate: updating without a restart is not available here';

async function load() {
  vi.resetModules();
  const service = await import('../services/selfupdate');
  const store = await import('./selfupdate');
  const show = await import('./updateShow');
  const toasts = await import('./toasts');
  store.selfUpdateStatus.set(READY);
  return { service, store, show, toasts };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(START);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('confirming the restart of a downloaded update', () => {
  it('starts the show at the press and hands Go that very moment for a seamless update', async () => {
    const { service, store, show } = await load();
    vi.mocked(service.beginHandoff).mockResolvedValue(undefined);

    await store.requestHandoff();

    expect(get(show.updateShow)).toMatchObject({ startedAt: START, phase: 'run', version: '1.5.0', handoff: true });
    expect(service.beginHandoff).toHaveBeenCalledExactlyOnceWith(START);
    expect(service.applyUpdate).not.toHaveBeenCalled();
  });

  it('counts as applying on every surface from the press until Go answers, whatever status lands meanwhile', async () => {
    const { service, store, show } = await load();
    let answer = () => {};
    vi.mocked(service.beginHandoff).mockReturnValue(new Promise<void>((done) => (answer = done)));

    const request = store.requestHandoff();
    expect(get(store.selfUpdateApplying)).toBe(true);
    expect(get(store.selfUpdateView)).toBe('applying');
    expect(get(store.selfUpdateBusy)).toBe(true);
    // A background check Go finished while the hand-over waited for it: still the old "ready".
    store.selfUpdateStatus.set({ ...READY });
    expect(get(show.updateShow)?.phase).toBe('run');
    expect(get(store.selfUpdateView)).toBe('applying');

    answer();
    await request;
    store.selfUpdateStatus.set(APPLYING);
    expect(get(show.updateShow)).toMatchObject({ startedAt: START, phase: 'run', handoff: true });
    expect(get(store.selfUpdateApplying)).toBe(true);
  });

  it('where the hand-over is refused, restarts through the worker at once, says no error and keeps the show', async () => {
    const { service, store, show, toasts } = await load();
    vi.mocked(service.beginHandoff).mockRejectedValue(new service.SelfUpdateError(UNSUPPORTED));
    vi.mocked(service.applyUpdate).mockResolvedValue(undefined);

    await store.requestHandoff();

    expect(service.applyUpdate).toHaveBeenCalledOnce();
    expect(get(toasts.toasts)).toEqual([]);
    expect(get(show.updateShow)).toMatchObject({ startedAt: START, phase: 'run' });
    expect(get(show.updateShow)?.handoff).toBeFalsy();
    expect(get(store.selfUpdateApplying)).toBe(true);
  });

  it('refused on both paths, says the worker\'s reason and sends the show home', async () => {
    const { service, store, show, toasts } = await load();
    vi.mocked(service.beginHandoff).mockRejectedValue(new service.SelfUpdateError(UNSUPPORTED));
    vi.mocked(service.applyUpdate).mockRejectedValue(new Error('typhon:selfupdate.not_ready'));

    await store.requestHandoff();

    expect(get(toasts.toasts).map((item) => item.kind)).toEqual(['danger']);
    expect(get(show.updateShow)?.phase).toBe('home');
    expect(get(store.selfUpdateApplying)).toBe(false);
    expect(get(store.selfUpdateView)).toBe('ready');
  });

  it('rolled back by Go, sends the show home and leaves the update ready with its failure', async () => {
    const { service, store, show } = await load();
    vi.mocked(service.beginHandoff).mockResolvedValue(undefined);
    await store.requestHandoff();
    store.selfUpdateStatus.set(APPLYING);

    store.selfUpdateStatus.set(ROLLED_BACK);

    expect(get(show.updateShow)?.phase).toBe('home');
    expect(get(store.selfUpdateApplying)).toBe(false);
    expect(get(store.selfUpdateView)).toBe('ready');
    expect(get(store.handoffFailed)).toBe(true);
  });

  it('rolled back before the applying event reached the window, still ends: Go\'s newer status is the truth', async () => {
    const { service, store, show } = await load();
    vi.mocked(service.beginHandoff).mockResolvedValue(undefined);
    await store.requestHandoff();

    store.selfUpdateStatus.set(ROLLED_BACK);

    expect(get(show.updateShow)?.phase).toBe('home');
    expect(get(store.selfUpdateApplying)).toBe(false);
  });

  it('"update with a restart" goes straight to the worker, with the show and no hand-over', async () => {
    const { service, store, show } = await load();
    store.selfUpdateStatus.set(ROLLED_BACK);
    vi.mocked(service.applyUpdate).mockResolvedValue(undefined);

    await store.requestApply();

    expect(service.beginHandoff).not.toHaveBeenCalled();
    expect(service.applyUpdate).toHaveBeenCalledOnce();
    expect(get(show.updateShow)).toMatchObject({ startedAt: START, phase: 'run' });
    expect(get(show.updateShow)?.handoff).toBeFalsy();
    expect(get(store.handoffFailed)).toBe(false);
  });

  it('once Go has spoken, the press is over: the status it answered over does not count as applying again', async () => {
    const { service, store } = await load();
    vi.mocked(service.applyUpdate).mockResolvedValue(undefined);
    await store.requestApply();
    store.selfUpdateStatus.set(APPLYING);
    store.selfUpdateStatus.set(ROLLED_BACK);

    store.selfUpdateStatus.set(READY);

    expect(get(store.selfUpdateView)).toBe('ready');
    expect(get(store.selfUpdateApplying)).toBe(false);
  });

  it('pressed twice, asks Go once', async () => {
    const { service, store } = await load();
    vi.mocked(service.beginHandoff).mockReturnValue(new Promise<void>(() => {}));

    void store.requestHandoff();
    void store.requestHandoff();

    expect(service.beginHandoff).toHaveBeenCalledOnce();
  });
});
