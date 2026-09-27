import { Service as CatalogService } from '../../../bindings/typhon/internal/catalog';
import { inWails } from './backend';
import type { CatalogQuery } from './sources';
import type { Recommendation } from '../recommendations/display';

export type CatalogSort = 'auto' | 'popular' | 'for-you' | 'rating' | 'year' | 'title';
export interface RecommendationPreferences {
  defaultSort: string;
  genre: string;
  platform: string;
  kind: string;
  compatOnly: boolean;
  hideLibrary: boolean;
  hideNotInterested: boolean;
  notInterested: string[];
}
export interface RecommendationProfile {
  defaultSort: string;
  confidence: number;
  evidenceGames: number;
  evidencePlaytimeSeconds: number;
}
export interface DiscoveryResult {
  items: Recommendation[];
  fallback: boolean;
  profile: RecommendationProfile;
}
export const defaultPreferences = (): RecommendationPreferences => ({
  defaultSort: '', genre: '', platform: '', kind: '', compatOnly: false,
  hideLibrary: false, hideNotInterested: true, notInterested: [],
});
export const emptyProfile = (): RecommendationProfile => ({
  defaultSort: 'popular', confidence: 0, evidenceGames: 0, evidencePlaytimeSeconds: 0,
});

export async function getRecommendationPreferences(): Promise<RecommendationPreferences> {
  if (!inWails) return defaultPreferences();
  const result = await CatalogService.GetRecommendationPreferences();
  return { ...defaultPreferences(), ...result, notInterested: result.notInterested ?? [] } as RecommendationPreferences;
}
export async function saveRecommendationPreferences(prefs: RecommendationPreferences): Promise<void> {
  if (!inWails) throw new Error('unavailable in browser');
  await CatalogService.SaveRecommendationPreferences(prefs as never);
}
export async function getRecommendationProfile(): Promise<RecommendationProfile> {
  if (!inWails) return emptyProfile();
  return await CatalogService.GetRecommendationProfile() as unknown as RecommendationProfile;
}
export async function setNotInterested(gameId: string, on: boolean): Promise<void> {
  if (!inWails) throw new Error('unavailable in browser');
  await CatalogService.SetNotInterested(gameId, on);
}
export async function getDiscovery(query: CatalogQuery, refreshExcludeIds: string[] = []): Promise<DiscoveryResult> {
  if (!inWails) return { items: [], fallback: false, profile: emptyProfile() };
  const result = await CatalogService.GetDiscovery({ ...query, refreshExcludeIds, limit: 5 } as never) as unknown as DiscoveryResult;
  return { ...result, items: result.items ?? [] };
}
