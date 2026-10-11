import { beforeEach, describe, expect, it, vi } from 'vitest';

const CancelFetchMetadata = vi.fn();
const List = vi.fn();
const Get = vi.fn();
const FetchMetadata = vi.fn();
const StartDownload = vi.fn();
const StartDownloadFrom = vi.fn();

vi.mock('../../../bindings/typhon/internal/download', () => ({
  Manager: { CancelFetchMetadata, List, Get, FetchMetadata, StartDownload, StartDownloadFrom },
}));

beforeEach(() => {
  vi.clearAllMocks();
});

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

describe('file lists that Go may send as null', () => {
  async function load() {
    vi.resetModules();
    vi.doMock('./backend', () => ({ inWails: true }));
    return await import('./downloads');
  }

  const file = { path: 'a.bin', size: 1, selected: true, bytesDone: 0 };

  it('listDownloads gives an empty file list for a download without files', async () => {
    List.mockResolvedValueOnce([{ id: 'd1', files: null }, { id: 'd2', files: [file] }]);
    const { listDownloads } = await load();

    const list = await listDownloads();

    expect(list.map((d) => d.files)).toEqual([[], [file]]);
  });

  it('listDownloads gives an empty list when Go sent no downloads at all', async () => {
    List.mockResolvedValueOnce(null);
    const { listDownloads } = await load();

    await expect(listDownloads()).resolves.toEqual([]);
  });

  it.each([
    ['getDownload', Get, ['d1']],
    ['startDownload', StartDownload, ['hash', 'D:\Games', [0]]],
    ['startDownloadFrom', StartDownloadFrom, ['hash', 'D:\Games', [0], {}]],
  ])('%s gives an empty file list for a download without files', async (name, binding, args) => {
    binding.mockResolvedValueOnce({ id: 'd1', files: null });
    const api = (await load()) as unknown as Record<string, (...a: unknown[]) => Promise<{ files: unknown[] }>>;

    const download = await api[name](...args);

    expect(download.files).toEqual([]);
  });

  it('fetchMetadata gives an empty file list for a torrent without files', async () => {
    FetchMetadata.mockResolvedValueOnce({ infoHash: 'h', name: 'n', totalBytes: 0, files: null });
    const { fetchMetadata } = await load();

    await expect(fetchMetadata('magnet:?xt=urn:btih:aaaa')).resolves.toMatchObject({ files: [] });
  });

  it('keeps the files Go did send', async () => {
    Get.mockResolvedValueOnce({ id: 'd1', files: [file] });
    const { getDownload } = await load();

    await expect(getDownload('d1')).resolves.toMatchObject({ files: [file] });
  });
});
