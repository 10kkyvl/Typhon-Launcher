import { Service as SearchService } from '../../../bindings/typhon/internal/search';
import { inWails } from './backend';

export interface GameHit {
  id: string;
  title: string;
  cover?: string;
  year?: number;
  installed: boolean;
  canonicalGameId?: string;
  version?: string;
  latestVersion?: string;
  releases: number;
  sources: number;
  score: number;
}

export interface ReleaseHit {
  id: string;
  sourceId: string;
  sourceName: string;
  title: string;
  version?: string;
  size: number;
}

export interface SearchResult {
  query: string;
  games: GameHit[];
  releases: ReleaseHit[];
  moreGames: number;
  moreReleases: number;
}

const empty = (query: string): SearchResult => ({
  query,
  games: [],
  releases: [],
  moreGames: 0,
  moreReleases: 0,
});

export async function searchAll(query: string): Promise<SearchResult> {
  if (!inWails) return empty(query);
  const result = (await SearchService.Search(query)) as unknown as SearchResult | null;
  if (!result) return empty(query);
  return {
    ...result,
    games: result.games ?? [],
    releases: result.releases ?? [],
  };
}
