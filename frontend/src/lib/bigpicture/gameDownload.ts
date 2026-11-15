import { releaseOrigin } from '../game/releases';
import { errorCode } from '../i18n/errors';
import { cancelFetchMetadata, discardMetadata, fetchMetadata, startDownloadFrom, type DownloadOrigin, type TorrentInfo } from '../services/downloads';
import { prepareReleaseDownload, type ReleaseDownloadRequest } from '../services/sources';
import { LatestRequestGate } from './latestRequest';

export type GameDownloadPhase = 'idle' | 'preparing' | 'fetching' | 'ready' | 'prepare-error' | 'fetch-error' | 'starting' | 'started';

export interface GameDownloadServices {
  prepareRelease: (releaseId: string) => Promise<ReleaseDownloadRequest>;
  fetchMetadata: (source: string) => Promise<TorrentInfo>;
  cancelFetchMetadata: (source: string) => Promise<void>;
  discardMetadata: (infoHash: string) => Promise<void>;
  startDownload: (infoHash: string, destination: string, selected: number[], origin: DownloadOrigin) => Promise<unknown>;
}

/**
 * `kept` is set on a fetch that followed a Start Go answered with nothing waiting: the files chosen
 * before, by path (a file new to the list is chosen), and Start has to be pressed again.
 */
export type GameDownloadResult =
  | { kind: 'ready'; source: string; origin: DownloadOrigin; torrent: TorrentInfo; kept?: boolean[] }
  | { kind: 'error'; phase: 'prepare-error' | 'fetch-error'; error: unknown }
  | { kind: 'stale' };

/** A Start either started the download or, with nothing waiting in Go, fetched the source again. */
export type GameDownloadStartResult = { kind: 'started' } | GameDownloadResult;

/**
 * Go holds nothing for the torrent any more (its client was stopped or replaced, or the fetched torrent
 * could not be kept): one fetch of the same source brings it back. Any other failure leaves it waiting.
 */
export function nothingWaiting(error: unknown): boolean {
  return errorCode(error) === 'download.metadata_required';
}

/** The choices, by path, that a fetch after such a Start keeps. */
export function choicesByPath(files: readonly { path: string }[], selected: (index: number) => boolean): Map<string, boolean> {
  return new Map(files.map((file, index) => [file.path, selected(index)]));
}

export function keptChoices(choices: ReadonlyMap<string, boolean>, files: readonly { path: string }[]): boolean[] {
  return files.map((file) => choices.get(file.path) ?? true);
}

const backend: GameDownloadServices = {
  prepareRelease: prepareReleaseDownload,
  fetchMetadata,
  cancelFetchMetadata,
  discardMetadata,
  startDownload: startDownloadFrom,
};

/** Owns the temporary metadata reservation between release selection and download start/cancel. */
export class GameDownloadFlow {
  private readonly gate = new LatestRequestGate();
  private readonly services: GameDownloadServices;
  private phaseValue: GameDownloadPhase = 'idle';
  private releaseId = '';
  private source = '';
  private origin: DownloadOrigin = {};
  private infoHash = '';
  private torrent: TorrentInfo | null = null;
  // Set by a Start Go answered with nothing waiting, until a fetch brings the files back.
  private kept: Map<string, boolean> | null = null;

  constructor(services: GameDownloadServices = backend) {
    this.services = services;
  }

  get phase(): GameDownloadPhase {
    return this.phaseValue;
  }

  async prepare(releaseId: string, onPhase?: (phase: 'preparing' | 'fetching') => void): Promise<GameDownloadResult> {
    const pendingCleanup = this.cancel();
    const ticket = this.gate.begin();
    this.releaseId = releaseId;
    this.phaseValue = 'preparing';
    onPhase?.('preparing');
    await pendingCleanup;
    if (!this.gate.isCurrent(ticket)) return { kind: 'stale' };
    try {
      const request = await this.services.prepareRelease(releaseId);
      if (!this.gate.isCurrent(ticket)) return { kind: 'stale' };
      this.source = request.uri;
      this.origin = releaseOrigin(request);
      return await this.fetchPrepared(ticket, onPhase);
    } catch (error) {
      if (!this.gate.isCurrent(ticket)) return { kind: 'stale' };
      this.phaseValue = 'prepare-error';
      return { kind: 'error', phase: 'prepare-error', error };
    }
  }

  async retry(onPhase?: (phase: 'preparing' | 'fetching') => void): Promise<GameDownloadResult> {
    if (!this.source) return this.releaseId ? this.prepare(this.releaseId, onPhase) : { kind: 'stale' };
    const ticket = this.gate.begin();
    return this.fetchPrepared(ticket, onPhase);
  }

  private async fetchPrepared(ticket: number, onPhase?: (phase: 'preparing' | 'fetching') => void): Promise<GameDownloadResult> {
    this.phaseValue = 'fetching';
    onPhase?.('fetching');
    try {
      const torrent = await this.services.fetchMetadata(this.source);
      if (!this.gate.isCurrent(ticket)) {
        await this.services.discardMetadata(torrent.infoHash).catch(() => undefined);
        return { kind: 'stale' };
      }
      this.torrent = torrent;
      this.infoHash = torrent.infoHash;
      this.phaseValue = 'ready';
      const kept = this.kept;
      this.kept = null;
      if (kept) return { kind: 'ready', source: this.source, origin: this.origin, torrent, kept: keptChoices(kept, torrent.files) };
      return { kind: 'ready', source: this.source, origin: this.origin, torrent };
    } catch (error) {
      if (!this.gate.isCurrent(ticket)) return { kind: 'stale' };
      this.phaseValue = 'fetch-error';
      return { kind: 'error', phase: 'fetch-error', error };
    }
  }

  /**
   * Starts the download. A Start Go answers with nothing waiting fetches the same source once and
   * answers with that fetch; any other failure is thrown and leaves the torrent ready for the next Start.
   */
  async start(
    destination: string,
    selected: number[],
    options: DownloadOrigin,
    onPhase?: (phase: 'preparing' | 'fetching') => void,
  ): Promise<GameDownloadStartResult> {
    if (this.phaseValue !== 'ready' || !this.torrent || !this.infoHash) {
      throw new Error('game download is not ready');
    }
    const ticket = this.gate.begin();
    const torrent = this.torrent;
    this.phaseValue = 'starting';
    try {
      await this.services.startDownload(this.infoHash, destination, selected, { ...this.origin, ...options });
    } catch (error) {
      if (!this.gate.isCurrent(ticket)) throw error;
      if (!nothingWaiting(error)) {
        this.phaseValue = 'ready';
        throw error;
      }
      const chosen = new Set(selected);
      this.kept = choicesByPath(torrent.files, (index) => chosen.has(index));
      this.infoHash = '';
      this.torrent = null;
      return this.fetchPrepared(ticket, onPhase);
    }
    if (this.gate.isCurrent(ticket)) {
      this.infoHash = '';
      this.torrent = null;
      this.phaseValue = 'started';
    }
    return { kind: 'started' };
  }

  async cancel(): Promise<void> {
    const wasFetching = this.phaseValue === 'fetching';
    const source = this.source;
    const infoHash = this.infoHash;
    this.gate.invalidate();
    this.phaseValue = 'idle';
    this.releaseId = '';
    this.source = '';
    this.origin = {};
    this.infoHash = '';
    this.torrent = null;
    this.kept = null;
    const cleanup: Promise<unknown>[] = [];
    if (wasFetching && source) cleanup.push(this.services.cancelFetchMetadata(source).catch(() => undefined));
    if (infoHash) cleanup.push(this.services.discardMetadata(infoHash).catch(() => undefined));
    await Promise.all(cleanup);
  }
}
