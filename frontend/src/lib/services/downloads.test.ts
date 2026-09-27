import { describe, expect, it, vi } from 'vitest';

const CancelFetchMetadata = vi.fn();

vi.mock('../../../bindings/typhon/internal/download', () => ({
  Manager: { CancelFetchMetadata },
}));

describe('cancelFetchMetadata', () => {
  it('reaches the backend when running inside Wails', async () => {
    vi.doMock('./backend', () => ({ inWails: true }));
    const { cancelFetchMetadata } = await import('./downloads');

    await cancelFetchMetadata('magnet:?xt=urn:btih:aaaa');

    expect(CancelFetchMetadata).toHaveBeenCalledWith('magnet:?xt=urn:btih:aaaa');
  });

  it('does nothing outside Wails', async () => {
    vi.resetModules();
    CancelFetchMetadata.mockClear();
    vi.doMock('./backend', () => ({ inWails: false }));
    const { cancelFetchMetadata } = await import('./downloads');

    await cancelFetchMetadata('magnet:?xt=urn:btih:aaaa');

    expect(CancelFetchMetadata).not.toHaveBeenCalled();
  });
});
