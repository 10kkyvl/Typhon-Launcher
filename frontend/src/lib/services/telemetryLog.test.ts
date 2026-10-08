import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  List: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/telemetrylog', () => ({ Service: bindings }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./telemetryLog');
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('sent data log', () => {
  it('renames the Go fields to the ones the screen reads', async () => {
    bindings.List.mockResolvedValue([
      { Kind: 'diagnostics', Endpoint: '/v1/errors', SentAt: '2026-09-01T10:00:00Z', Payload: '{\n  "a": 1\n}', Formatted: true },
      { Kind: 'compat', Endpoint: '/v1/compat/reports', SentAt: '2026-09-02T10:00:00Z', Payload: 'raw', Formatted: false },
    ]);
    const log = await load(true);

    await expect(log.listSentData()).resolves.toEqual([
      { kind: 'diagnostics', endpoint: '/v1/errors', sentAt: '2026-09-01T10:00:00Z', payload: '{\n  "a": 1\n}', formatted: true },
      { kind: 'compat', endpoint: '/v1/compat/reports', sentAt: '2026-09-02T10:00:00Z', payload: 'raw', formatted: false },
    ]);
  });

  it('shows an empty log when nothing was sent', async () => {
    bindings.List.mockResolvedValue(null);
    const log = await load(true);

    await expect(log.listSentData()).resolves.toEqual([]);
  });

  it('lets a failure reach the dialog instead of showing an empty log', async () => {
    bindings.List.mockRejectedValue(new Error('log unreadable'));
    const log = await load(true);

    await expect(log.listSentData()).rejects.toThrow('log unreadable');
  });

  it('refuses in a browser preview', async () => {
    const log = await load(false);

    await expect(log.listSentData()).rejects.toThrow('unavailable in browser');
    expect(bindings.List).not.toHaveBeenCalled();
  });
});
