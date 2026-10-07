import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  SyncNow: vi.fn(),
  ForgetRemote: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/accountsync', () => ({ Service: bindings }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./accountSync');
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe.each([
  ['syncNow', 'SyncNow'],
  ['forgetRemote', 'ForgetRemote'],
] as const)('%s', (fn, method) => {
  it('refuses in a browser preview with its own error type', async () => {
    const sync = await load(false);

    const error = await sync[fn]().catch((err: unknown) => err);

    expect(error).toBeInstanceOf(sync.AccountSyncError);
    expect((error as Error).message).toBe('unavailable in browser');
    expect(bindings[method]).not.toHaveBeenCalled();
  });

  it('calls the backend once', async () => {
    bindings[method].mockResolvedValue(undefined);
    const sync = await load(true);

    await sync[fn]();

    expect(bindings[method]).toHaveBeenCalledTimes(1);
  });

  it('wraps a failure so the caller can tell a sync failure from any other error', async () => {
    bindings[method].mockRejectedValue(new Error('server said no'));
    const sync = await load(true);

    const error = await sync[fn]().catch((err: unknown) => err);

    expect(error).toBeInstanceOf(sync.AccountSyncError);
    expect((error as Error).message).toBe('server said no');
    expect((error as Error).name).toBe('AccountSyncError');
  });

  it('keeps the text of a failure that was not an Error', async () => {
    bindings[method].mockRejectedValue('plain text failure');
    const sync = await load(true);

    const error = await sync[fn]().catch((err: unknown) => err);

    expect(error).toBeInstanceOf(sync.AccountSyncError);
    expect((error as Error).message).toBe('plain text failure');
  });
});
