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

    await expect(flow.start('/downloads', [0], { autoInstall: true, elevateAhead: false })).resolves.toEqual({ kind: 'started' });
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

describe('GameDownloadFlow: a start Go answers with nothing waiting', () => {
  const twoFiles: TorrentInfo = {
    infoHash: 'feed123',
    name: 'Game build',
    totalBytes: 150,
    files: [
      { path: 'game.bin', size: 100, selected: true, bytesDone: 0 },
      { path: 'extras.bin', size: 50, selected: true, bytesDone: 0 },
    ],
  };
  // The same source fetched again: Go may list the files in another order, with one more.
  const fetchedAgain: TorrentInfo = {
    infoHash: 'feed123',
    name: 'Game build',
    totalBytes: 160,
    files: [
      { path: 'extras.bin', size: 50, selected: true, bytesDone: 0 },
      { path: 'readme.txt', size: 10, selected: true, bytesDone: 0 },
      { path: 'game.bin', size: 100, selected: true, bytesDone: 0 },
    ],
  };
  const nothingWaiting = () => new Error('typhon:download.metadata_required: fetch the torrent metadata first');

  it('fetches the same source once and is ready to start again with the files chosen before', async () => {
    const api = services({
      fetchMetadata: vi.fn().mockResolvedValueOnce(twoFiles).mockResolvedValueOnce(fetchedAgain),
      startDownload: vi.fn().mockRejectedValueOnce(nothingWaiting()).mockResolvedValue(undefined),
    });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');
    const phases: string[] = [];

    const result = await flow.start('/downloads', [0], {}, (phase) => phases.push(phase));

    expect(result).toMatchObject({ kind: 'ready', torrent: fetchedAgain, kept: [false, true, true] });
    expect(phases).toEqual(['fetching']);
    expect(flow.phase).toBe('ready');
    expect(api.fetchMetadata).toHaveBeenCalledTimes(2);
    expect(vi.mocked(api.fetchMetadata).mock.calls[1]).toEqual([request.uri]);
    expect(api.discardMetadata).not.toHaveBeenCalled();

    await expect(flow.start('/downloads', [2], {})).resolves.toEqual({ kind: 'started' });
    expect(api.startDownload).toHaveBeenCalledTimes(2);
    expect(flow.phase).toBe('started');
  });

  it('does not fetch again after any other failure', async () => {
    const failure = new Error('typhon:download.destination_unavailable: no disk');
    const api = services({ startDownload: vi.fn().mockRejectedValueOnce(failure) });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');

    await expect(flow.start('/downloads', [0], {})).rejects.toBe(failure);
    expect(flow.phase).toBe('ready');
    expect(api.fetchMetadata).toHaveBeenCalledTimes(1);
  });

  it('says a failed fetch after the start as a fetch error, fetches no more by itself, and its retry keeps the choices', async () => {
    const offline = new Error('no peers');
    const api = services({
      fetchMetadata: vi.fn().mockResolvedValueOnce(twoFiles).mockRejectedValueOnce(offline).mockResolvedValueOnce(fetchedAgain),
      startDownload: vi.fn().mockRejectedValueOnce(nothingWaiting()),
    });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');

    await expect(flow.start('/downloads', [1], {})).resolves.toEqual({ kind: 'error', phase: 'fetch-error', error: offline });
    expect(flow.phase).toBe('fetch-error');
    await Promise.resolve();
    expect(api.fetchMetadata).toHaveBeenCalledTimes(2);
    expect(api.startDownload).toHaveBeenCalledTimes(1);

    await expect(flow.retry()).resolves.toMatchObject({ kind: 'ready', torrent: fetchedAgain, kept: [true, true, false] });
    expect(api.fetchMetadata).toHaveBeenCalledTimes(3);
  });

  it('a fresh preparation after it chooses every file again', async () => {
    const api = services({
      fetchMetadata: vi.fn().mockResolvedValueOnce(twoFiles).mockRejectedValueOnce(new Error('no peers')).mockResolvedValue(twoFiles),
      startDownload: vi.fn().mockRejectedValueOnce(nothingWaiting()),
    });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');
    await flow.start('/downloads', [1], {});

    const fresh = await flow.prepare('release-1');
    expect(fresh).toMatchObject({ kind: 'ready', torrent: twoFiles });
    expect(fresh).not.toHaveProperty('kept');
  });

  it('cancels the fetch after the start when the player leaves', async () => {
    const pending = deferred<TorrentInfo>();
    const api = services({
      fetchMetadata: vi.fn().mockResolvedValueOnce(twoFiles).mockReturnValueOnce(pending.promise),
      startDownload: vi.fn().mockRejectedValueOnce(nothingWaiting()),
    });
    const flow = new GameDownloadFlow(api);
    await flow.prepare('release-1');

    const starting = flow.start('/downloads', [0], {});
    await vi.waitFor(() => expect(api.fetchMetadata).toHaveBeenCalledTimes(2));
    await flow.cancel();
    expect(api.cancelFetchMetadata).toHaveBeenCalledWith(request.uri);
    pending.resolve(fetchedAgain);

    await expect(starting).resolves.toEqual({ kind: 'stale' });
    expect(api.discardMetadata).toHaveBeenCalledWith(fetchedAgain.infoHash);
    expect(flow.phase).toBe('idle');
  });
});
