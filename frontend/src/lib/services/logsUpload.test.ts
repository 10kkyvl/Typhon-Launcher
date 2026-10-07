import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  SendLogs: vi.fn(),
};

const events = {
  On: vi.fn(),
};

vi.mock('@wailsio/runtime', () => ({ Events: events }));
vi.mock('../../../bindings/typhon/internal/diagnostics', () => ({ Service: bindings }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./logsUpload');
}

function listen(name = 'diagnostics:logs_status') {
  let push: (event: { data: unknown }) => void = () => {};
  events.On.mockImplementation((event: string, cb: (event: { data: unknown }) => void) => {
    expect(event).toBe(name);
    push = cb;
    return vi.fn();
  });
  return (data: unknown) => push({ data });
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('log upload status', () => {
  it('passes on each stage of the upload', async () => {
    const emit = listen();
    const upload = await load(true);
    const seen: unknown[] = [];

    upload.onLogUploadStatus((status) => seen.push(status));
    for (const state of ['preparing', 'sending', 'waiting', 'success', 'error']) emit({ state });

    expect(seen.map((status) => (status as { state: string }).state)).toEqual([
      'preparing',
      'sending',
      'waiting',
      'success',
      'error',
    ]);
  });

  it('carries the byte counts only when they are numbers', async () => {
    const emit = listen();
    const upload = await load(true);
    const seen: unknown[] = [];

    upload.onLogUploadStatus((status) => seen.push(status));
    emit({ state: 'sending', sentBytes: 10, totalBytes: 100 });
    emit({ state: 'sending', sentBytes: '10', totalBytes: null });

    expect(seen).toEqual([
      { state: 'sending', sentBytes: 10, totalBytes: 100 },
      { state: 'sending', sentBytes: undefined, totalBytes: undefined },
    ]);
  });

  it('ignores a stage it does not know', async () => {
    const emit = listen();
    const upload = await load(true);
    const handler = vi.fn();

    upload.onLogUploadStatus(handler);
    emit({ state: 'compressing' });
    emit({});

    expect(handler).not.toHaveBeenCalled();
  });

  it('listens to nothing in a browser preview', async () => {
    const upload = await load(false);

    const stop = upload.onLogUploadStatus(() => {});
    stop();

    expect(events.On).not.toHaveBeenCalled();
  });
});

describe('sending logs', () => {
  it('returns the upload id and no dropped files when Go reports none', async () => {
    bindings.SendLogs.mockResolvedValue({ id: 'u1', dropped: null });
    const upload = await load(true);

    await expect(upload.sendLogs()).resolves.toEqual({ id: 'u1', dropped: [] });
  });

  it('names the files that were left out', async () => {
    bindings.SendLogs.mockResolvedValue({ id: 'u2', dropped: ['old.log'] });
    const upload = await load(true);

    await expect(upload.sendLogs()).resolves.toEqual({ id: 'u2', dropped: ['old.log'] });
  });

  it('lets a failure through and refuses in a browser preview', async () => {
    bindings.SendLogs.mockRejectedValue(new Error('typhon:diagnostics.log_upload_busy: busy'));
    const upload = await load(true);
    await expect(upload.sendLogs()).rejects.toThrow('log_upload_busy');

    const preview = await load(false);
    await expect(preview.sendLogs()).rejects.toThrow('unavailable in browser');
  });
});
