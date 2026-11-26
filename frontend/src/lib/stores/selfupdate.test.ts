import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { msg } from '../i18n';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));

vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...await importOriginal<typeof import('../services/selfupdate')>(),
  getStatus: vi.fn(),
  getOutcome: vi.fn(),
  checkForUpdate: vi.fn(),
  downloadUpdate: vi.fn(),
  applyUpdate: vi.fn(),
  getRollback: vi.fn(),
  applyRollback: vi.fn(),
  installStable: vi.fn(),
  dismissUpdate: vi.fn(),
  cancelDownload: vi.fn(),
  getReleaseNotes: vi.fn(),
  acknowledgeReleaseNotes: vi.fn(),
  emptyReleaseNotes: () => ({ currentVersion: '', unseen: [], history: [] }),
}));

function makeNote(version: string) {
  return {
    version,
    publishedAt: '2026-08-28T12:00:00Z',
    summary: 'коротко о версии',
    changes: [{ kind: 'fixed', text: 'что-то починили' }],
  };
}

function makeStatus(overrides: Partial<Record<string, unknown>> = {}) {
  return { state: 'idle', currentVersion: '1.0.0', ...overrides };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const service = await import('../services/selfupdate');
  const store = await import('./selfupdate');
  const show = await import('./updateShow');
  vi.mocked(service.getReleaseNotes).mockResolvedValue({ currentVersion: '1.0.0', unseen: [], history: [] } as never);
  vi.mocked(service.getRollback).mockResolvedValue(service.noRollback() as never);
  return { service, store, show };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('initSelfUpdate', () => {
  it('reads the initial status from getStatus', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(
      makeStatus({ state: 'available', availableVersion: '1.1.0' }) as never,
    );

    await store.initSelfUpdate();

    expect(get(store.selfUpdateStatus)).toEqual(makeStatus({ state: 'available', availableVersion: '1.1.0' }));
  });

  it('updates the store when launcher:update_status fires', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_status']({ data: makeStatus({ state: 'ready', availableVersion: '1.2.0' }) });

    expect(get(store.selfUpdateStatus).state).toBe('ready');
  });

  it('recomputes progress from launcher:update_progress', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_progress']({ data: { version: '1.2.0', totalBytes: 200, downloadedBytes: 50 } });

    expect(get(store.selfUpdateProgress)).toEqual({ version: '1.2.0', totalBytes: 200, downloadedBytes: 50 });
  });

  it('keeps an errored status as failed instead of resetting to idle', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_status']({
      data: { state: 'idle', currentVersion: '1.0.0', error: 'network down', errorCode: 'manifest' },
    });

    const status = get(store.selfUpdateStatus);
    expect(status.error).toBe('network down');
    expect(get(store.selfUpdateView)).toBe('failed');
  });

  it('keeps a downloaded update installable when a later check fails', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_status']({
      data: { state: 'ready', currentVersion: '1.0.0', availableVersion: '1.1.0', error: 'network down', errorCode: 'manifest' },
    });

    expect(get(store.selfUpdateView)).toBe('ready');
  });

  it('keeps an available update downloadable when only the check failed', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_status']({
      data: { state: 'available', currentVersion: '1.0.0', availableVersion: '1.1.0', error: 'network down', errorCode: 'manifest' },
    });

    expect(get(store.selfUpdateView)).toBe('available');
  });

  it('shows a failed download as failed so it can be retried', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_status']({
      data: { state: 'available', currentVersion: '1.0.0', availableVersion: '1.1.0', error: 'dial tcp', errorCode: 'download' },
    });

    expect(get(store.selfUpdateView)).toBe('failed');
  });

  it('drops stale progress once the status leaves downloading', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:update_status']({ data: makeStatus({ state: 'downloading', availableVersion: '1.2.0' }) });
    handlers['launcher:update_progress']({ data: { version: '1.2.0', totalBytes: 200, downloadedBytes: 50 } });
    handlers['launcher:update_status']({ data: makeStatus({ state: 'available', availableVersion: '1.2.0', error: 'x', errorCode: 'download' }) });

    expect(get(store.selfUpdateProgress)).toBeNull();
  });

  it('a failed status never quietly claims the latest version', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    await store.initSelfUpdate();

    const toasts = await import('./toasts');
    toasts.toasts.set([]);

    handlers['launcher:update_status']({
      data: { state: 'idle', currentVersion: '1.0.0', error: 'manifest signature does not verify' },
    });

    expect(get(toasts.toasts)).toHaveLength(0);
  });

  it('toasts about a newly available version only once', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    await store.initSelfUpdate();

    const toasts = await import('./toasts');
    toasts.toasts.set([]);

    handlers['launcher:update_status']({ data: makeStatus({ state: 'available', availableVersion: '1.1.0' }) });
    handlers['launcher:update_status']({ data: makeStatus({ state: 'available', availableVersion: '1.1.0' }) });

    expect(get(toasts.toasts)).toHaveLength(1);
  });

  it('keeps a status that lands while the first getStatus is still in flight', async () => {
    const { service, store } = await load();
    let resolveStatus: (value: unknown) => void = () => {};
    vi.mocked(service.getStatus).mockReturnValue(
      new Promise((resolve) => {
        resolveStatus = resolve;
      }) as never,
    );

    const init = store.initSelfUpdate();
    // The backend's own startup check may answer before the first getStatus
    // does: the listener has to be in place before the request, or that
    // answer is lost until the next poll.
    expect(handlers['launcher:update_status']).toBeTypeOf('function');
    handlers['launcher:update_status']({ data: makeStatus({ state: 'available', availableVersion: '1.1.0' }) });
    resolveStatus(makeStatus({ state: 'idle' }));
    await init;

    expect(get(store.selfUpdateStatus)).toEqual(makeStatus({ state: 'available', availableVersion: '1.1.0' }));
  });

  it('returns a disposer that unsubscribes every listener', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    const runtime = await import('@wailsio/runtime');

    const dispose = await store.initSelfUpdate();
    const offFns = vi.mocked(runtime.Events.On).mock.results.map((r) => r.value as ReturnType<typeof vi.fn>);
    expect(offFns).toHaveLength(3);

    dispose();
    for (const off of offFns) expect(off).toHaveBeenCalledTimes(1);
  });
});

describe('release notes', () => {
  it('loads what the user has not seen yet', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getReleaseNotes).mockResolvedValue({
      currentVersion: '1.1.0',
      unseen: [makeNote('1.1.0')],
      history: [makeNote('1.1.0'), makeNote('1.0.0')],
    } as never);

    await store.initSelfUpdate();

    expect(get(store.unseenReleaseNotes).map((n) => n.version)).toEqual(['1.1.0']);
    expect(get(store.releaseNotesHistory)).toHaveLength(2);
  });

  it('announces a successful update with the outcome card instead of a toast', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getReleaseNotes).mockResolvedValue({
      currentVersion: '1.1.0',
      unseen: [makeNote('1.1.0')],
      history: [makeNote('1.1.0')],
    } as never);
    vi.mocked(service.getOutcome).mockResolvedValue({ version: '1.1.0', ok: true, finishedAt: '' } as never);

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.initSelfUpdate();

    expect(get(toasts.toasts)).toHaveLength(0);
    expect(get(store.selfUpdateOutcome)?.version).toBe('1.1.0');
  });

  // The start after an update plays the end of the update show first (stores/updateShow.ts); the card
  // says the result once the show is over, and hides on its own two seconds later.
  it('hides the success card on its own two seconds after the update show has ended', async () => {
    vi.useFakeTimers();
    try {
      const { service, store, show } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
      vi.mocked(service.getOutcome).mockResolvedValue({ version: '1.1.0', ok: true, finishedAt: '' } as never);

      const toasts = await import('./toasts');
      toasts.toasts.set([]);
      await store.initSelfUpdate();

      expect(get(toasts.toasts)).toHaveLength(0);
      expect(get(show.updateShow)?.phase).toBe('finish');
      vi.advanceTimersByTime(store.OUTCOME_SUCCESS_MS * 10);
      expect(get(store.selfUpdateOutcome)?.version).toBe('1.1.0');
      show.endUpdateShow();
      vi.advanceTimersByTime(store.OUTCOME_SUCCESS_MS - 1);
      expect(get(store.selfUpdateOutcome)?.version).toBe('1.1.0');
      vi.advanceTimersByTime(1);
      expect(get(store.selfUpdateOutcome)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('read just after the shell appeared (the sign-in settled first), still owes the finish', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date', 'performance'] });
    try {
      const { service, store, show } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
      vi.mocked(service.getOutcome).mockResolvedValue({ version: '1.1.0', ok: true, finishedAt: '' } as never);
      const gone = show.hostUpdateShow();
      vi.advanceTimersByTime(20);

      await store.initSelfUpdate();

      expect(get(show.updateShow)).toMatchObject({ phase: 'finish', owed: true });
      vi.advanceTimersByTime(store.OUTCOME_SUCCESS_MS * 10);
      expect(get(store.selfUpdateOutcome)?.version).toBe('1.1.0');
      show.endUpdateShow();
      gone();
    } finally {
      vi.useRealTimers();
    }
  });

  it('read once the shell has been up for seconds, plays no show: the card says the result and hides two seconds later', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date', 'performance'] });
    try {
      const { service, store, show } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
      vi.mocked(service.getOutcome).mockResolvedValue({ version: '1.1.0', ok: true, finishedAt: '' } as never);
      const gone = show.hostUpdateShow();
      vi.advanceTimersByTime(3000);

      await store.initSelfUpdate();

      expect(get(show.updateShow)).toBeNull();
      expect(get(store.selfUpdateOutcome)?.version).toBe('1.1.0');
      vi.advanceTimersByTime(store.OUTCOME_SUCCESS_MS);
      expect(get(store.selfUpdateOutcome)).toBeNull();
      gone();
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps a failed outcome until the player closes it', async () => {
    vi.useFakeTimers();
    try {
      const { service, store } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
      vi.mocked(service.getOutcome).mockResolvedValue({ version: '1.1.0', ok: false, finishedAt: '' } as never);

      await store.initSelfUpdate();
      vi.advanceTimersByTime(store.OUTCOME_SUCCESS_MS * 10);

      expect(get(store.selfUpdateOutcome)?.ok).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it('still toasts a failed update even when notes are waiting', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getReleaseNotes).mockResolvedValue({
      currentVersion: '1.1.0',
      unseen: [makeNote('1.1.0')],
      history: [makeNote('1.1.0')],
    } as never);
    vi.mocked(service.getOutcome).mockResolvedValue({ version: '1.1.0', ok: false, finishedAt: '' } as never);

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.initSelfUpdate();

    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
  });

  it('picks up notes that arrive with launcher:release_notes', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    handlers['launcher:release_notes']({
      data: { currentVersion: '1.1.0', unseen: [makeNote('1.1.0')], history: [makeNote('1.1.0')] },
    });

    expect(get(store.unseenReleaseNotes)).toHaveLength(1);
  });

  it('acknowledges the notes it just hid', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getReleaseNotes).mockResolvedValue({
      currentVersion: '1.1.0',
      unseen: [makeNote('1.1.0')],
      history: [makeNote('1.1.0')],
    } as never);
    vi.mocked(service.acknowledgeReleaseNotes).mockImplementation(async () => {
      handlers['launcher:release_notes']({
        data: { currentVersion: '1.1.0', unseen: null, history: [makeNote('1.1.0')] },
      });
    });

    await store.initSelfUpdate();
    await store.dismissReleaseNotes();

    expect(service.acknowledgeReleaseNotes).toHaveBeenCalledTimes(1);
    expect(get(store.unseenReleaseNotes)).toHaveLength(0);
    expect(get(store.releaseNotesHistory)).toHaveLength(1);
    // The modal reads .length again after the acknowledgement event arrives.
    expect(get(store.unseenReleaseNotes).length > 0).toBe(false);
    await store.dismissReleaseNotes();
    expect(service.acknowledgeReleaseNotes).toHaveBeenCalledTimes(1);
  });

  it('normalizes empty notes events from a background update check', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    await store.initSelfUpdate();
    handlers['launcher:release_notes']({
      data: { currentVersion: '1.1.0', unseen: null, history: null },
    });
    expect(get(store.unseenReleaseNotes)).toEqual([]);
    expect(get(store.releaseNotesHistory)).toEqual([]);
  });

  it('toasts when the acknowledgement cannot be saved', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getReleaseNotes).mockResolvedValue({
      currentVersion: '1.1.0',
      unseen: [makeNote('1.1.0')],
      history: [makeNote('1.1.0')],
    } as never);
    vi.mocked(service.acknowledgeReleaseNotes).mockRejectedValue(
      new Error('selfupdate: state failed to load, refusing to persist'),
    );

    await store.initSelfUpdate();
    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.dismissReleaseNotes();

    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
  });

  it('does not call the backend when there is nothing to acknowledge', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    await store.dismissReleaseNotes();

    expect(service.acknowledgeReleaseNotes).not.toHaveBeenCalled();
  });
});

const AVAILABLE_ROLLBACK = { available: true, version: '1.1.0', reason: '', reasonCode: '' };

describe('rollback', () => {
  it('loads what the launcher can go back to at start', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getRollback).mockResolvedValue(AVAILABLE_ROLLBACK as never);

    await store.initSelfUpdate();

    expect(get(store.rollbackInfo)).toEqual(AVAILABLE_ROLLBACK);
  });

  it('toasts when the rollback state cannot be read instead of showing nothing', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getRollback).mockRejectedValue(new Error('boom'));

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.initSelfUpdate();

    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
  });

  it('keeps the button busy after a started rollback until the process exits', async () => {
    const { service, store } = await load();
    vi.mocked(service.applyRollback).mockResolvedValue(undefined);

    await store.requestRollback();

    expect(service.applyRollback).toHaveBeenCalledTimes(1);
    expect(get(store.rollbackBusy)).toBe(true);
    expect(get(store.selfUpdateBusy)).toBe(true);
  });

  it('does not start a second rollback while one is running', async () => {
    const { service, store } = await load();
    vi.mocked(service.applyRollback).mockResolvedValue(undefined);

    await store.requestRollback();
    await store.requestRollback();

    expect(service.applyRollback).toHaveBeenCalledTimes(1);
  });

  it('frees the button and toasts the reason when the rollback is refused', async () => {
    const { service, store } = await load();
    vi.mocked(service.applyRollback).mockRejectedValue(
      new Error('typhon:selfupdate.rollback_broken: selfupdate: the saved previous launcher version is damaged'),
    );

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.requestRollback();

    expect(get(store.rollbackBusy)).toBe(false);
    expect(get(toasts.toasts).map((t) => t.message)).toContain('Сохранённая копия прежней версии повреждена.');
  });

  // Work that the launcher's shutdown would wait for: the player is asked to wait and press again, so the
  // reason stays in the dialog they pressed in, and nothing else says it.
  it('hands a rollback refused for running work back to the dialog without a toast', async () => {
    const { service, store } = await load();
    const refusal = new service.SelfUpdateError('typhon:selfupdate.work_running: selfupdate: an install is running');
    vi.mocked(service.applyRollback).mockRejectedValue(refusal);
    const toasts = await import('./toasts');
    toasts.toasts.set([]);

    await expect(store.requestRollback()).rejects.toBe(refusal);

    expect(get(store.rollbackBusy)).toBe(false);
    expect(get(toasts.toasts)).toEqual([]);
  });

  it('hands an install of the stable build refused for running work back to the dialog without a toast', async () => {
    const { service, store } = await load();
    const refusal = new service.SelfUpdateError('typhon:selfupdate.work_running: selfupdate: an install is running');
    vi.mocked(service.installStable).mockRejectedValue(refusal);
    const toasts = await import('./toasts');
    toasts.toasts.set([]);

    await expect(store.requestInstallStable()).rejects.toBe(refusal);

    expect(get(store.stableBusy)).toBe(false);
    expect(get(toasts.toasts)).toEqual([]);
  });

  it('still toasts an install of the stable build that failed for another reason', async () => {
    const { service, store } = await load();
    vi.mocked(service.installStable).mockRejectedValue(
      new service.SelfUpdateError('typhon:selfupdate.hash_mismatch: selfupdate: the download does not match'),
    );
    const toasts = await import('./toasts');
    toasts.toasts.set([]);

    await store.requestInstallStable();

    expect(get(store.stableBusy)).toBe(false);
    expect(get(toasts.toasts).map((t) => t.message)).toEqual([msg('state.selfupdateReasonCorruptDownload')]);
  });

  it('frees the button and reports it when the launcher did not exit in time', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus({ state: 'ready', availableVersion: '1.3.0' }) as never);
    vi.mocked(service.applyRollback).mockResolvedValue(undefined);
    await store.initSelfUpdate();
    await store.requestRollback();

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    handlers['launcher:update_status']({
      data: makeStatus({
        state: 'ready',
        availableVersion: '1.3.0',
        error: 'typhon:selfupdate.parent_still_running: launcher did not exit',
        errorCode: 'rollback',
      }),
    });

    expect(get(store.rollbackBusy)).toBe(false);
    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
    expect(get(store.selfUpdateView)).toBe('ready');
  });

  it('shows a rolled back start as a success card without an error toast', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getOutcome).mockResolvedValue({
      kind: 'rolled_back',
      version: '1.1.0',
      from: '1.2.0',
      to: '1.1.0',
      ok: true,
      finishedAt: '',
    } as never);

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.initSelfUpdate();

    expect(get(toasts.toasts)).toHaveLength(0);
    expect(get(store.selfUpdateOutcome)?.kind).toBe('rolled_back');
  });

  it('toasts a failed rollback with its own wording', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getOutcome).mockResolvedValue({
      kind: 'rolled_back',
      version: '1.1.0',
      from: '1.2.0',
      to: '1.1.0',
      ok: false,
      error: 'x',
      finishedAt: '',
    } as never);

    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    await store.initSelfUpdate();

    expect(get(toasts.toasts).map((t) => t.message)).toEqual(['Не удалось вернуться на версию 1.1.0']);
  });
});

describe('the update show', () => {
  it('starts when the status says the update is being applied, and not before', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    try {
      vi.setSystemTime(1_700_000_000_000);
      const { service, store, show } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus({ state: 'ready', availableVersion: '1.1.0' }) as never);
      vi.mocked(service.getOutcome).mockResolvedValue(null as never);
      await store.initSelfUpdate();
      expect(get(show.updateShow)).toBeNull();

      handlers['launcher:update_status']({ data: makeStatus({ state: 'applying', availableVersion: '1.1.0' }) });

      expect(get(show.updateShow)).toMatchObject({ startedAt: 1_700_000_000_000, phase: 'run', version: '1.1.0' });
      expect(show.updateShowHandoff()).toEqual({ startedAt: 1_700_000_000_000 });
    } finally {
      vi.useRealTimers();
    }
  });

  it('goes home when applying ends without a new version, while the failure is said as before', async () => {
    const { service, store, show } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus({ state: 'applying', availableVersion: '1.1.0' }) as never);
    vi.mocked(service.getOutcome).mockResolvedValue(null as never);
    await store.initSelfUpdate();
    expect(get(show.updateShow)?.phase).toBe('run');

    handlers['launcher:update_status']({ data: makeStatus({ state: 'ready', availableVersion: '1.1.0', error: 'still running', errorCode: 'apply' }) });

    expect(get(show.updateShow)?.phase).toBe('home');
    expect(get(store.selfUpdateView)).toBe('ready');
  });

  it('plays nothing at a start after a failed update or a rollback', async () => {
    for (const outcome of [
      { version: '1.1.0', ok: false, error: 'boom', finishedAt: '' },
      { kind: 'rolled_back', version: '1.0.0', ok: true, finishedAt: '' },
    ]) {
      const { service, store, show } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
      vi.mocked(service.getOutcome).mockResolvedValue(outcome as never);
      await store.initSelfUpdate();
      expect(get(show.updateShow), JSON.stringify(outcome)).toBeNull();
    }
  });

  it('hides a rolled back start\'s card two seconds after it was read, with no show before it', async () => {
    vi.useFakeTimers();
    try {
      const { service, store } = await load();
      vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
      vi.mocked(service.getOutcome).mockResolvedValue({ kind: 'rolled_back', version: '1.0.0', ok: true, finishedAt: '' } as never);
      await store.initSelfUpdate();
      vi.advanceTimersByTime(store.OUTCOME_SUCCESS_MS);
      expect(get(store.selfUpdateOutcome)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('initSelfUpdate outside Wails', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('does not subscribe to events and returns a no-op disposer', async () => {
    vi.doMock('../services/backend', () => ({ inWails: false }));
    for (const key of Object.keys(handlers)) delete handlers[key];
    const service = await import('../services/selfupdate');
    const store = await import('./selfupdate');
    const runtime = await import('@wailsio/runtime');
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);

    const dispose = await store.initSelfUpdate();

    expect(runtime.Events.On).not.toHaveBeenCalled();
    expect(() => dispose()).not.toThrow();
  });
});

describe('requestCheck', () => {
  it('marks checking while in flight and applies the result', async () => {
    const { service, store } = await load();
    let resolve!: (value: unknown) => void;
    vi.mocked(service.checkForUpdate).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );

    const pending = store.requestCheck();
    expect(get(store.selfUpdateChecking)).toBe(true);

    resolve(makeStatus({ state: 'available', availableVersion: '2.0.0' }));
    await pending;

    expect(get(store.selfUpdateChecking)).toBe(false);
    expect(get(store.selfUpdateStatus).availableVersion).toBe('2.0.0');
  });

  it('stays quiet when the check was cancelled by another action', async () => {
    const { service, store } = await load();
    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    vi.mocked(service.checkForUpdate).mockRejectedValue(
      Object.assign(new Error('fetch manifest: context canceled'), { code: 'canceled' }),
    );

    await store.requestCheck();

    expect(get(toasts.toasts)).toHaveLength(0);
    expect(get(store.selfUpdateChecking)).toBe(false);
  });

  it('toasts the failure and clears the checking flag instead of pretending success', async () => {
    const { service, store } = await load();
    vi.mocked(service.checkForUpdate).mockRejectedValue(new Error('selfupdate: manifest exceeds the size limit'));

    await store.requestCheck();

    const toasts = await import('./toasts');
    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
    expect(get(store.selfUpdateChecking)).toBe(false);
  });
});

describe('requestDownload', () => {
  it('marks downloading while in flight and applies the result', async () => {
    const { service, store } = await load();
    let resolve!: (value: unknown) => void;
    vi.mocked(service.downloadUpdate).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );

    const pending = store.requestDownload();
    expect(get(store.selfUpdateDownloading)).toBe(true);

    resolve(makeStatus({ state: 'ready' }));
    await pending;

    expect(get(store.selfUpdateDownloading)).toBe(false);
    expect(get(store.selfUpdateStatus).state).toBe('ready');
  });

  it('clears the progress of the previous attempt before a new download starts', async () => {
    const { service, store } = await load();
    store.selfUpdateProgress.set({ version: '1.2.0', totalBytes: 200, downloadedBytes: 150 });
    let resolve!: (value: unknown) => void;
    vi.mocked(service.downloadUpdate).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );

    const pending = store.requestDownload();
    expect(get(store.selfUpdateProgress)).toBeNull();
    resolve(makeStatus({ state: 'ready' }));
    await pending;
  });

  it('stays quiet when the user cancelled the download', async () => {
    const { service, store } = await load();
    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    vi.mocked(service.downloadUpdate).mockRejectedValue(
      Object.assign(new Error('download artifact: context canceled'), { code: 'canceled' }),
    );

    await store.requestDownload();

    expect(get(toasts.toasts)).toHaveLength(0);
    expect(get(store.selfUpdateDownloading)).toBe(false);
  });

  it('toasts the failure instead of silently staying idle', async () => {
    const { service, store } = await load();
    vi.mocked(service.downloadUpdate).mockRejectedValue(new Error('selfupdate: downloaded hash differs from the manifest'));

    await store.requestDownload();

    const toasts = await import('./toasts');
    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
    expect(get(store.selfUpdateDownloading)).toBe(false);
  });

  it('toasts the failure in russian instead of the go error text', async () => {
    const { service, store } = await load();
    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    vi.mocked(service.downloadUpdate).mockRejectedValue(
      new Error('download artifact: context deadline exceeded (Client.Timeout or context cancellation while reading body)'),
    );

    await store.requestDownload();

    expect(get(toasts.toasts)[0].message).toContain('не ответил вовремя');
  });
});

describe('requestCancelDownload', () => {
  it('asks the backend to stop the download', async () => {
    const { service, store } = await load();
    vi.mocked(service.cancelDownload).mockResolvedValue(undefined as never);

    await store.requestCancelDownload();

    expect(service.cancelDownload).toHaveBeenCalledTimes(1);
  });

  it('toasts when the backend refuses', async () => {
    const { service, store } = await load();
    const toasts = await import('./toasts');
    toasts.toasts.set([]);
    vi.mocked(service.cancelDownload).mockRejectedValue(new Error('boom'));

    await store.requestCancelDownload();

    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
  });
});

describe('requestApply / requestDismiss', () => {
  it('toasts when ApplyUpdate rejects', async () => {
    const { service, store } = await load();
    vi.mocked(service.applyUpdate).mockRejectedValue(new Error('selfupdate: no verified update is ready to apply'));

    await store.requestApply();

    const toasts = await import('./toasts');
    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
  });

  it('toasts when DismissUpdate rejects', async () => {
    const { service, store } = await load();
    vi.mocked(service.dismissUpdate).mockRejectedValue(new Error('boom'));

    await store.requestDismiss();

    const toasts = await import('./toasts');
    expect(get(toasts.toasts).some((t) => t.kind === 'danger')).toBe(true);
  });
});

describe('retryFailed', () => {
  it('retries the download when the last action was a download', async () => {
    const { service, store } = await load();
    vi.mocked(service.downloadUpdate).mockResolvedValue(makeStatus({ state: 'ready' }) as never);
    vi.mocked(service.checkForUpdate).mockResolvedValue(makeStatus() as never);

    await store.requestDownload();
    await store.retryFailed();

    expect(service.downloadUpdate).toHaveBeenCalledTimes(2);
    expect(service.checkForUpdate).not.toHaveBeenCalled();
  });

  it('retries the download when the status says the download failed, whatever came last', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.downloadUpdate).mockResolvedValue(makeStatus({ state: 'ready' }) as never);
    vi.mocked(service.checkForUpdate).mockResolvedValue(makeStatus() as never);

    await store.initSelfUpdate();
    await store.requestCheck();
    store.selfUpdateStatus.set(
      makeStatus({ state: 'available', availableVersion: '1.1.0', error: 'dial tcp', errorCode: 'download' }) as never,
    );
    await store.retryFailed();

    expect(service.downloadUpdate).toHaveBeenCalledTimes(1);
    expect(service.checkForUpdate).toHaveBeenCalledTimes(1);
  });

  it('retries the check when the last action was a check', async () => {
    const { service, store } = await load();
    vi.mocked(service.checkForUpdate).mockResolvedValue(makeStatus() as never);

    await store.requestCheck();
    await store.retryFailed();

    expect(service.checkForUpdate).toHaveBeenCalledTimes(2);
  });
});

describe('update outcome from the previous run', () => {
  it('surfaces a failed install left behind by the update worker', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    const outcome = {
      version: '1.2.0',
      ok: false,
      error: 'selfupdate: installer finished but left the launcher binary unchanged',
      finishedAt: '2026-08-25T12:00:00Z',
    };
    vi.mocked(service.getOutcome).mockResolvedValue(outcome as never);

    await store.initSelfUpdate();

    expect(get(store.selfUpdateOutcome)).toEqual(outcome);
  });

  it('surfaces a successful install', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getOutcome).mockResolvedValue({
      version: '1.2.0',
      ok: true,
      finishedAt: '2026-08-25T12:00:00Z',
    } as never);

    await store.initSelfUpdate();

    expect(get(store.selfUpdateOutcome)?.ok).toBe(true);
  });

  it('keeps the outcome empty when the worker left nothing', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getOutcome).mockResolvedValue(null as never);

    await store.initSelfUpdate();

    expect(get(store.selfUpdateOutcome)).toBeNull();
  });

  it('can be dismissed', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    vi.mocked(service.getOutcome).mockResolvedValue({
      version: '1.2.0',
      ok: true,
      finishedAt: '2026-08-25T12:00:00Z',
    } as never);

    await store.initSelfUpdate();
    store.dismissOutcome();

    expect(get(store.selfUpdateOutcome)).toBeNull();
  });

  it('still loads the status when the outcome lookup fails', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus({ state: 'available' }) as never);
    vi.mocked(service.getOutcome).mockRejectedValue(new Error('boom'));

    await store.initSelfUpdate();

    expect(get(store.selfUpdateStatus).state).toBe('available');
    expect(get(store.selfUpdateOutcome)).toBeNull();
  });
});
