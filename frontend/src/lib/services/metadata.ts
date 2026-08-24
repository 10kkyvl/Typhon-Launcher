import { Service as MetadataService } from '../../../bindings/typhon/internal/metadata';
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

export interface MetadataView {
  game: CatalogGame;
  cover: string;
  hero: string;
  screenshots: MediaAsset[];
  resolved: boolean;
  stale: boolean;
  provider: string;
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
});

export async function isMetadataAvailable(): Promise<boolean> {
  if (!inWails) return false;
  try {
    return await MetadataService.Available();
  } catch {
    return false;
  }
}

export async function getMetadataView(gameId: string): Promise<MetadataView> {
  if (!inWails) return emptyView(gameId);
  try {
    return (await MetadataService.GetView(gameId)) as unknown as MetadataView;
  } catch {
    return emptyView(gameId);
  }
}

export async function getGameArt(gameIds: string[]): Promise<Record<string, GameArt>> {
  if (!inWails || gameIds.length === 0) return {};
  try {
    return ((await MetadataService.GetArt(gameIds)) ?? {}) as unknown as Record<string, GameArt>;
  } catch {
    return {};
  }
}

export async function ensureArt(gameIds: string[]): Promise<string[]> {
  if (!inWails || gameIds.length === 0) return gameIds;
  return (await MetadataService.EnsureArt(gameIds)) ?? [];
}

export async function findMetadataCandidates(gameId: string): Promise<MetadataCandidate[]> {
  if (!inWails) return [];
  return ((await MetadataService.FindCandidates(gameId)) ?? []) as unknown as MetadataCandidate[];
}

export async function searchMetadataCandidates(query: string): Promise<MetadataCandidate[]> {
  if (!inWails) return [];
  return ((await MetadataService.SearchCandidates(query)) ?? []) as unknown as MetadataCandidate[];
}

export async function applyMetadataMatch(gameId: string, providerId: string): Promise<MetadataView> {
  if (!inWails) throw unavailable();
  return (await MetadataService.ApplyMatch(gameId, providerId)) as unknown as MetadataView;
}

export async function refreshMetadata(gameId: string): Promise<MetadataView> {
  if (!inWails) throw unavailable();
  return (await MetadataService.Refresh(gameId)) as unknown as MetadataView;
}

export async function ensureMetadataFresh(gameId: string): Promise<boolean> {
  if (!inWails) return false;
  try {
    return await MetadataService.EnsureFresh(gameId);
  } catch {
    return false;
  }
}
