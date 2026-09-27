import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/downloads', () => ({
  cancelFetchMetadata: vi.fn(),
  discardMetadata: vi.fn(),
  fetchMetadata: vi.fn(),
  startDownloadFrom: vi.fn(),
}));
vi.mock('../services/sources', () => ({ prepareReleaseDownload: vi.fn() }));

import { GameDownloadFlow, type GameDownloadServices } from './gameDownload';
import type { ReleaseDownloadRequest } from '../services/sources';
import type { TorrentInfo } from '../services/downloads';

const request: ReleaseDownloadRequest = {
  uri: 'magnet:?xt=urn:btih:feed123',
  name: 'Game build',
  releaseId: 'release-1',
  sourceId: 'source-1',
  gameId: 'canonical-1',
  version: '1.0',
};
const metadata: TorrentInfo = {
  infoHash: 'feed123',
  name: 'Game build',
  totalBytes: 100,
  files: [{ path: 'game.bin', size: 100, selected: true, bytesDone: 0 }],
};

function services(patch: Partial<GameDownloadServices> = {}): GameDownloadServices {
  return {
    prepareRelease: vi.fn().mockResolvedValue(request),
    fetchMetadata: vi.fn().mockResolvedValue(metadata),
    cancelFetchMetadata: vi.fn().mockResolvedValue(undefined),
    discardMetadata: vi.fn().mockResolvedValue(undefined),
    startDownload: vi.fn().mockResolvedValue(undefined),
    ...patch,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

describe('GameDownloadFlow', () => {
  it('prepares a real release, fetches torrent metadata, and keeps provenance through start', async () => {
    const api = services();
    const flow = new GameDownloadFlow(api);
    const result = await flow.prepare('release-1');
    expect(result).toEqual({
      kind: 'ready',
      source: request.uri,
      origin: { releaseId: 'release-1', sourceId: 'source-1', gameId: 'canonical-1', version: '1.0' },
      torrent: metadata,
    });

    await flow.start('/downloads', [0], { autoInstall: true, elevateAhead: false });
    expect(api.startDownload).toHaveBeenCalledWith('feed123', '/downloads', [0], {
      releaseId: 'release-1', sourceId: 'source-1', gameId: 'canonical-1', version: '1.0',
      autoInstall: true, elevateAhead: false,
    });
    expect(flow.phase).toBe('started');
  });

  it('keeps preparation and metadata failures retryable', async () => {
    const preparationFailure = new Error('feed offline');
    const metadataFailure = new Error('no peers');
    const api = services({
      prepareRelease: vi.fn().mockRejectedValueOnce(preparationFailure).mockResolvedValue(request),
      fetchMetadata: vi.fn().mockRejectedValueOnce(metadataFailure).mockResolvedValue(metadata),
    });
    const flow = new GameDownloadFlow(api);

    await expect(flow.prepare('release-1')).resolves.toMatchObject({ kind: 'error', phase: 'prepare-error', error: preparationFailure });
    await expect(flow.retry()).resolves.toMatchObject({ kind: 'error', phase: 'fetch-error', error: metadataFailure });
    await expect(flow.retry()).resolves.toMatchObject({ kind: 'ready', torrent: metadata });
    expect(api.prepareRelease).toHaveBeenCalledTimes(2);
    expect(api.fetchMetadata).toHaveBeenCalledTimes(2);
  });

  it('cancels a pending metadata fetch and discards its reservation if completion races cancellation', async () => {
    const pending = deferred<TorrentInfo>();
    const fetchStarted = deferred<void>();
    const api = services({
      fetchMetadata: vi.fn(() => {
        fetchStarted.resolve();
        return pending.promise;
      }),
    });
    const flow = new GameDownloadFlow(api);
    const preparing = flow.prepare('release-1');
    await fetchStarted.promise;
    await flow.cancel();
    expect(api.cancelFetchMetadata).toHaveBeenCalledWith(request.uri);
    pending.resolve(metadata);

    await expect(preparing).resolves.toEqual({ kind: 'stale' });
    expect(api.discardMetadata).toHaveBeenCalledWith(metadata.infoHash);
    expect(flow.phase).toBe('idle');
  });

  it('does not prepare a new release if the user cancels while old reservation cleanup is pending', async () => {
    const cleanup = deferred<void>();
    const api = services({
      discardMetadata: vi.fn().mockReturnValueOnce(cleanup.promise).mockResolvedValue(undefined),
    });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');

    const preparing = flow.prepare('release-2');
    expect(api.discardMetadata).toHaveBeenCalledWith(metadata.infoHash);
    await flow.cancel();
    cleanup.resolve();

    await expect(preparing).resolves.toEqual({ kind: 'stale' });
    expect(api.prepareRelease).toHaveBeenCalledTimes(1);
    expect(flow.phase).toBe('idle');
  });

  it('retains fetched metadata after a failed start so the user can retry without fetching again', async () => {
    const failure = new Error('destination is unavailable');
    const api = services({ startDownload: vi.fn().mockRejectedValueOnce(failure).mockResolvedValue(undefined) });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');

    await expect(flow.start('/downloads', [0], {})).rejects.toBe(failure);
    expect(flow.phase).toBe('ready');
    await flow.start('/downloads/retry', [0], {});
    expect(api.fetchMetadata).toHaveBeenCalledTimes(1);
    expect(api.startDownload).toHaveBeenCalledTimes(2);
  });
});
