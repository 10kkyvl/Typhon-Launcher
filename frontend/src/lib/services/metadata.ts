import { Service as MetadataService } from '../../../bindings/typhon/internal/metadata';
import { get } from 'svelte/store';
import { locale } from '../i18n/locale';
import { errorCode } from '../i18n/errors';
import { inWails } from './backend';
import type { CatalogGame } from './sources';

export interface MediaAsset {
  id: string;
  gameId: string;
  type: 'cover' | 'screenshot';
  sourceUrl: string;
  path: string;
  url?: string;
  width: number;
  height: number;
  createdAt: string;
}

export interface MetadataCandidate {
  providerId: string;
  title: string;
  releaseYear?: number;
  developer?: string;
  thumb?: string;
  confidence: number;
}

export interface GameArt {
  cover: string;
  hero: string;
}

export type MetadataMatch = 'idle' | 'searching' | 'unmatched' | 'failed' | 'skipped';

export interface MetadataView {
  game: CatalogGame;
  cover: string;
  hero: string;
  screenshots: MediaAsset[];
  resolved: boolean;
  stale: boolean;
  provider: string;
  match: MetadataMatch;
}

let languageQueue: Promise<void> = Promise.resolve();
locale.subscribe((language) => {
  if (inWails) languageQueue = languageQueue.catch(() => {}).then(() => MetadataService.SetLanguage(language));
});
async function syncLanguage() {
  await languageQueue;
  await MetadataService.SetLanguage(get(locale));
}

const unavailable = () => new Error('unavailable in browser');

const emptyView = (gameId: string): MetadataView => ({
  game: { id: gameId, title: '', sortTitle: '', createdAt: '' },
  cover: '',
  hero: '',
  screenshots: [],
  resolved: false,
  stale: false,
  provider: '',
  match: 'idle',
});

const unknownGame = (err: unknown) => errorCode(err) === 'catalog.game_not_found';

export async function isMetadataAvailable(): Promise<boolean> {
  if (!inWails) return false;
  await syncLanguage();
  return await MetadataService.Available();
}

export async function getMetadataView(gameId: string): Promise<MetadataView> {
  if (!inWails) return emptyView(gameId);
  await syncLanguage();
  try {
    return (await MetadataService.GetView(gameId)) as unknown as MetadataView;
  } catch (err) {
    if (unknownGame(err)) return emptyView(gameId);
    throw err;
  }
}

export async function getGameArt(gameIds: string[]): Promise<Record<string, GameArt>> {
  if (!inWails || gameIds.length === 0) return {};
  await syncLanguage();
  return ((await MetadataService.GetArt(gameIds)) ?? {}) as unknown as Record<string, GameArt>;
}

export async function ensureArt(gameIds: string[]): Promise<string[]> {
  if (!inWails || gameIds.length === 0) return gameIds;
  await syncLanguage();
  return (await MetadataService.EnsureArt(gameIds)) ?? [];
}

export async function findMetadataCandidates(gameId: string): Promise<MetadataCandidate[]> {
  if (!inWails) return [];
  await syncLanguage();
  return ((await MetadataService.FindCandidates(gameId)) ?? []) as unknown as MetadataCandidate[];
}

export async function searchMetadataCandidates(query: string): Promise<MetadataCandidate[]> {
  if (!inWails) return [];
  await syncLanguage();
  return ((await MetadataService.SearchCandidates(query)) ?? []) as unknown as MetadataCandidate[];
}

export async function applyMetadataMatch(gameId: string, providerId: string): Promise<MetadataView> {
  if (!inWails) throw unavailable();
  await syncLanguage();
  return (await MetadataService.ApplyMatch(gameId, providerId)) as unknown as MetadataView;
}

export async function dismissMetadataMatch(gameId: string): Promise<MetadataView> {
  if (!inWails) throw unavailable();
  await syncLanguage();
  return (await MetadataService.DismissMatch(gameId)) as unknown as MetadataView;
}

export async function refreshMetadata(gameId: string): Promise<MetadataView> {
  if (!inWails) throw unavailable();
  await syncLanguage();
  return (await MetadataService.Refresh(gameId)) as unknown as MetadataView;
}

export async function ensureMetadataFresh(gameId: string): Promise<boolean> {
  if (!inWails) return false;
  await syncLanguage();
  try {
    return await MetadataService.EnsureFresh(gameId);
  } catch (err) {
    if (unknownGame(err)) return false;
    throw err;
  }
}
