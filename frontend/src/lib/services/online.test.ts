import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  Status: vi.fn(),
  SetStatus: vi.fn(),
  Kick: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/online', () => ({ Service: bindings }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./online');
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('toPresenceStatus', () => {
  it('keeps every status the backend defines', async () => {
    const online = await load(true);

    for (const status of online.PRESENCE_STATUSES) {
      expect(online.toPresenceStatus(status)).toBe(status);
    }
  });

  it('falls back to online for anything it does not know', async () => {
    const online = await load(true);

    expect(online.toPresenceStatus('')).toBe('online');
    expect(online.toPresenceStatus('dnd')).toBe('online');
    expect(online.toPresenceStatus('ONLINE')).toBe('online');
  });
});

describe('presence calls', () => {
  it('reports online in a browser preview and sends nothing', async () => {
    const online = await load(false);

    await expect(online.status()).resolves.toBe('online');
    await online.setStatus('busy');
    await online.kick();

    expect(bindings.Status).not.toHaveBeenCalled();
    expect(bindings.SetStatus).not.toHaveBeenCalled();
    expect(bindings.Kick).not.toHaveBeenCalled();
  });

  it('normalises the status the backend reports', async () => {
    bindings.Status.mockResolvedValueOnce('away').mockResolvedValueOnce('something-new');
    const online = await load(true);

    await expect(online.status()).resolves.toBe('away');
    await expect(online.status()).resolves.toBe('online');
  });

  it('sends the chosen status to the backend and lets a refusal through', async () => {
    bindings.SetStatus.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('typhon:account.sync_disabled: off'));
    const online = await load(true);

    await online.setStatus('invisible');
    expect(bindings.SetStatus).toHaveBeenCalledWith('invisible');

    await expect(online.setStatus('busy')).rejects.toThrow('sync_disabled');
  });

  it('nudges the backend to re-announce', async () => {
    bindings.Kick.mockResolvedValue(undefined);
    const online = await load(true);

    await online.kick();

    expect(bindings.Kick).toHaveBeenCalledTimes(1);
  });
});
