import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

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

vi.mock('../services/selfupdate', () => ({
  getStatus: vi.fn(),
  getOutcome: vi.fn(),
  checkForUpdate: vi.fn(),
  downloadUpdate: vi.fn(),
  applyUpdate: vi.fn(),
  dismissUpdate: vi.fn(),
}));

function makeStatus(overrides: Partial<Record<string, unknown>> = {}) {
  return { state: 'idle', currentVersion: '1.0.0', ...overrides };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const service = await import('../services/selfupdate');
  const store = await import('./selfupdate');
  return { service, store };
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
      data: {
        state: 'available',
        currentVersion: '1.0.0',
        availableVersion: '1.1.0',
        error: 'network down',
        errorCode: 'manifest',
      },
    });

    const status = get(store.selfUpdateStatus);
    expect(status.error).toBe('network down');
    expect(get(store.selfUpdateView)).toBe('failed');
    expect(get(store.selfUpdateView)).not.toBe('idle');
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

  it('returns a disposer that unsubscribes both listeners', async () => {
    const { service, store } = await load();
    vi.mocked(service.getStatus).mockResolvedValue(makeStatus() as never);
    const runtime = await import('@wailsio/runtime');

    const dispose = await store.initSelfUpdate();
    const offFns = vi.mocked(runtime.Events.On).mock.results.map((r) => r.value as ReturnType<typeof vi.fn>);
    expect(offFns).toHaveLength(2);

    dispose();
    for (const off of offFns) expect(off).toHaveBeenCalledTimes(1);
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
