import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  GetStatus: vi.fn(),
  CheckForUpdate: vi.fn(),
  DownloadUpdate: vi.fn(),
  ApplyUpdate: vi.fn(),
  DismissUpdate: vi.fn(),
  GetOutcome: vi.fn(),
  GetReleaseNotes: vi.fn(),
  AcknowledgeReleaseNotes: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/selfupdate', () => ({ Service: bindings }));
vi.mock('./backend', () => ({ inWails: true }));

const status = {
  state: 'available',
  currentVersion: '1.0.0',
  availableVersion: '1.1.0',
};

describe('selfupdate service calls', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns the status from GetStatus unchanged', async () => {
    const { getStatus } = await import('./selfupdate');
    bindings.GetStatus.mockResolvedValueOnce(status);

    await expect(getStatus()).resolves.toEqual(status);
  });

  it('returns the status from CheckForUpdate unchanged', async () => {
    const { checkForUpdate } = await import('./selfupdate');
    bindings.CheckForUpdate.mockResolvedValueOnce(status);

    await expect(checkForUpdate()).resolves.toEqual(status);
    expect(bindings.CheckForUpdate).toHaveBeenCalledTimes(1);
  });

  it('returns the status from DownloadUpdate unchanged', async () => {
    const { downloadUpdate } = await import('./selfupdate');
    const ready = { ...status, state: 'ready' };
    bindings.DownloadUpdate.mockResolvedValueOnce(ready);

    await expect(downloadUpdate()).resolves.toEqual(ready);
  });

  it('calls ApplyUpdate through the backend', async () => {
    const { applyUpdate } = await import('./selfupdate');
    bindings.ApplyUpdate.mockResolvedValueOnce(undefined);

    await expect(applyUpdate()).resolves.toBeUndefined();
    expect(bindings.ApplyUpdate).toHaveBeenCalledTimes(1);
  });

  it('calls DismissUpdate through the backend', async () => {
    const { dismissUpdate } = await import('./selfupdate');
    bindings.DismissUpdate.mockResolvedValueOnce(undefined);

    await expect(dismissUpdate()).resolves.toBeUndefined();
    expect(bindings.DismissUpdate).toHaveBeenCalledTimes(1);
  });
});

describe('selfupdate service errors', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it.each([
    ['CheckForUpdate', 'checkForUpdate'],
    ['DownloadUpdate', 'downloadUpdate'],
  ] as const)('normalizes a %s failure into a SelfUpdateError instead of losing it', async (bindingName, fnName) => {
    const module = await import('./selfupdate');
    const { SelfUpdateError } = module;
    bindings[bindingName].mockRejectedValueOnce(new Error('selfupdate: manifest exceeds the size limit'));

    let err: InstanceType<typeof SelfUpdateError> | undefined;
    try {
      await (module[fnName] as () => Promise<unknown>)();
    } catch (e) {
      err = e as InstanceType<typeof SelfUpdateError>;
    }
    expect(err).toBeInstanceOf(SelfUpdateError);
    expect(err?.message).toBe('selfupdate: manifest exceeds the size limit');
    expect(err?.code).toBe('unknown');
  });

  it('maps the busy sentinel message to code busy', async () => {
    const { SelfUpdateError, checkForUpdate } = await import('./selfupdate');
    bindings.CheckForUpdate.mockRejectedValueOnce(new Error('selfupdate: another update operation is in progress'));

    const err = await checkForUpdate().catch((e) => e);
    expect(err).toBeInstanceOf(SelfUpdateError);
    expect(err.code).toBe('busy');
  });

  it('maps the not-ready sentinel message to code not_ready', async () => {
    const { SelfUpdateError, applyUpdate } = await import('./selfupdate');
    bindings.ApplyUpdate.mockRejectedValueOnce(new Error('selfupdate: no verified update is ready to apply'));

    const err = await applyUpdate().catch((e) => e);
    expect(err).toBeInstanceOf(SelfUpdateError);
    expect(err.code).toBe('not_ready');
  });

  it('does not swallow a DismissUpdate failure', async () => {
    const { dismissUpdate } = await import('./selfupdate');
    bindings.DismissUpdate.mockRejectedValueOnce(new Error('boom'));

    await expect(dismissUpdate()).rejects.toMatchObject({ code: 'unknown', message: 'boom' });
  });
});

describe('getOutcome', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns the outcome recorded by the update worker', async () => {
    const { getOutcome } = await import('./selfupdate');
    const outcome = { version: '1.2.0', ok: false, error: 'boom', finishedAt: '2026-08-25T12:00:00Z' };
    bindings.GetOutcome.mockResolvedValueOnce(outcome);

    await expect(getOutcome()).resolves.toEqual(outcome);
  });

  it('returns null when no update has run', async () => {
    const { getOutcome } = await import('./selfupdate');
    bindings.GetOutcome.mockResolvedValueOnce({ version: '', ok: false, finishedAt: '0001-01-01T00:00:00Z' });

    await expect(getOutcome()).resolves.toBeNull();
  });
});

describe('release notes calls', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns the notes from GetReleaseNotes', async () => {
    const { getReleaseNotes } = await import('./selfupdate');
    const notes = {
      currentVersion: '1.1.0',
      unseen: [{ version: '1.1.0', publishedAt: '2026-08-28T12:00:00Z', changes: [{ kind: 'fixed', text: 'x' }] }],
      history: [{ version: '1.1.0', publishedAt: '2026-08-28T12:00:00Z', changes: [{ kind: 'fixed', text: 'x' }] }],
    };
    bindings.GetReleaseNotes.mockResolvedValueOnce(notes);

    await expect(getReleaseNotes()).resolves.toEqual(notes);
  });

  it('turns the nil slices Go sends into empty lists', async () => {
    const { getReleaseNotes } = await import('./selfupdate');
    bindings.GetReleaseNotes.mockResolvedValueOnce({ currentVersion: '1.0.0', unseen: null, history: null });

    await expect(getReleaseNotes()).resolves.toEqual({ currentVersion: '1.0.0', unseen: [], history: [] });
  });

  it('reports a failed GetReleaseNotes instead of returning empty notes', async () => {
    const { getReleaseNotes } = await import('./selfupdate');
    bindings.GetReleaseNotes.mockRejectedValueOnce(new Error('selfupdate: state failed to load, refusing to persist'));

    await expect(getReleaseNotes()).rejects.toThrow('refusing to persist');
  });

  it('calls AcknowledgeReleaseNotes through the backend', async () => {
    const { acknowledgeReleaseNotes } = await import('./selfupdate');
    bindings.AcknowledgeReleaseNotes.mockResolvedValueOnce(undefined);

    await expect(acknowledgeReleaseNotes()).resolves.toBeUndefined();
    expect(bindings.AcknowledgeReleaseNotes).toHaveBeenCalledTimes(1);
  });

  it('reports a failed acknowledgement', async () => {
    const { acknowledgeReleaseNotes } = await import('./selfupdate');
    bindings.AcknowledgeReleaseNotes.mockRejectedValueOnce(new Error('selfupdate: state failed to load, refusing to persist'));

    await expect(acknowledgeReleaseNotes()).rejects.toThrow('refusing to persist');
  });
});

describe('selfupdate service outside Wails', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
  });

  it('getStatus returns an idle default instead of throwing', async () => {
    vi.doMock('./backend', () => ({ inWails: false }));
    const { getStatus } = await import('./selfupdate');

    await expect(getStatus()).resolves.toEqual({ state: 'idle', currentVersion: '' });
  });

  it('getReleaseNotes returns empty notes instead of throwing', async () => {
    vi.doMock('./backend', () => ({ inWails: false }));
    const { getReleaseNotes } = await import('./selfupdate');

    await expect(getReleaseNotes()).resolves.toEqual({ currentVersion: '', unseen: [], history: [] });
  });

  it.each(['checkForUpdate', 'downloadUpdate', 'applyUpdate', 'dismissUpdate', 'acknowledgeReleaseNotes'] as const)(
    '%s throws instead of pretending to succeed',
    async (fnName) => {
      vi.doMock('./backend', () => ({ inWails: false }));
      const module = await import('./selfupdate');

      await expect((module[fnName] as () => Promise<unknown>)()).rejects.toMatchObject({ code: 'unavailable' });
    },
  );
});
