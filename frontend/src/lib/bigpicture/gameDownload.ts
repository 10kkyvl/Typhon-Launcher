import { releaseOrigin } from '../game/releases';
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

export type GameDownloadResult =
  | { kind: 'ready'; source: string; origin: DownloadOrigin; torrent: TorrentInfo }
  | { kind: 'error'; phase: 'prepare-error' | 'fetch-error'; error: unknown }
  | { kind: 'stale' };

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
      return { kind: 'ready', source: this.source, origin: this.origin, torrent };
    } catch (error) {
      if (!this.gate.isCurrent(ticket)) return { kind: 'stale' };
      this.phaseValue = 'fetch-error';
      return { kind: 'error', phase: 'fetch-error', error };
    }
  }

  async start(destination: string, selected: number[], options: DownloadOrigin): Promise<void> {
    if (this.phaseValue !== 'ready' || !this.torrent || !this.infoHash) {
      throw new Error('game download is not ready');
    }
    const ticket = this.gate.begin();
    this.phaseValue = 'starting';
    try {
      await this.services.startDownload(this.infoHash, destination, selected, { ...this.origin, ...options });
      if (!this.gate.isCurrent(ticket)) return;
      this.infoHash = '';
      this.torrent = null;
      this.phaseValue = 'started';
    } catch (error) {
      if (this.gate.isCurrent(ticket)) this.phaseValue = 'ready';
      throw error;
    }
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
    const cleanup: Promise<unknown>[] = [];
    if (wasFetching && source) cleanup.push(this.services.cancelFetchMetadata(source).catch(() => undefined));
    if (infoHash) cleanup.push(this.services.discardMetadata(infoHash).catch(() => undefined));
    await Promise.all(cleanup);
  }
}
