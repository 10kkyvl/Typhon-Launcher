import { Service as SourcesService } from '../../../bindings/typhon/internal/sources';
import { Service as CatalogService } from '../../../bindings/typhon/internal/catalog';
import { inWails } from './backend';

export type SourceType = 'url' | 'file';
export type SourceStatus = 'active' | 'disabled' | 'error' | 'updating';
export type SourceHealth = 'healthy' | 'warning' | 'error';
export type MatchStatus = 'matched' | 'review' | 'unmatched';
export type Availability = 'available' | 'removed';

export interface Source {
  id: string;
  name: string;
  type: SourceType;
  url: string;
  path?: string;
  enabled: boolean;
  status: SourceStatus;
  health: SourceHealth;
  lastUpdatedAt: string | null;
  lastError: string;
  fingerprint: string;
  feedVersion: number;
  entries: number;
  invalid: number;
  matched: number;
  review: number;
  unmatched: number;
  warnings?: string[];
  createdAt: string;
}

export interface Release {
  id: string;
  sourceId: string;
  kind?: 'release' | 'patch';
  rawTitle: string;
  title: string;
  normalizedTitle: string;
  canonicalGameId: string | null;
  version: string;
  rawVersion?: string;
  fromVersion?: string;
  toVersion?: string;
  sequence?: number;
  edition?: string;
  languages?: string[];
  year?: number;
  tags?: string[];
  repacker?: string;
  dlcCount?: number;
  size: number;
  sizeUnknown?: boolean;
  uploadedAt: string | null;
  uris: string[];
  infoHash?: string;
  matchStatus: MatchStatus;
  matchConfidence: number;
  matchMethod: string;
  availability: Availability;
  locked?: boolean;
  ignored?: boolean;
  new?: boolean;
  firstSeenAt: string;
  lastSeenAt: string;
  createdAt: string;
}

export interface SourceRef {
  releaseId: string;
  sourceId: string;
  sourceName: string;
}

export interface ReleaseView {
  release: Release;
  sourceName: string;
  gameTitle: string;
}

export interface ReleaseGroup {
  release: Release;
  sourceName: string;
  duplicates?: SourceRef[];
}

export interface ReleaseQuery {
  sourceId?: string;
  search?: string;
  status?: string;
  sort?: string;
  page?: number;
  pageSize?: number;
}

export interface ReleasePage {
  items: ReleaseView[];
  total: number;
  page: number;
  pageSize: number;
}

export interface SourcePreview {
  name: string;
  type: SourceType;
  url: string;
  path?: string;
  feedVersion: number;
  entries: number;
  invalid: number;
  warnings?: string[];
  fingerprint: string;
  duplicate: boolean;
}

export interface RefreshSummary {
  sourceId: string;
  name: string;
  entries: number;
  invalid: number;
  added: number;
  updated: number;
  removed: number;
  restored: number;
  matched: number;
  review: number;
  unmatched: number;
  new: number;
  notModified: boolean;
  durationMs: number;
}

export interface SourceDetails {
  source: Source;
  total: number;
  available: number;
  removed: number;
  new: number;
  ignored: number;
}

export interface MatchCandidate {
  gameId: string;
  title: string;
  year?: number;
  score: number;
  method: string;
}

export interface CatalogGame {
  id: string;
  title: string;
  sortTitle: string;
  releaseYear?: number;
  developer?: string;
  publisher?: string;
  aliases?: string[];
  provisional?: boolean;
  createdAt: string;
  releaseDate?: string;
  summary?: string;
  genres?: string[];
  themes?: string[];
  platforms?: string[];
  coverAssetId?: string;
  heroAssetId?: string;
  metadataUpdatedAt?: string;
}

export interface CatalogQuery {
  search?: string;
  genre?: string;
  sort?: string;
  page?: number;
  pageSize?: number;
}

export interface CatalogPage {
  items: CatalogGame[];
  total: number;
  page: number;
  pageSize: number;
}

export interface GenreFacet {
  label: string;
  count: number;
}

export interface ReleaseDownloadRequest {
  uri: string;
  name: string;
  releaseId: string;
  sourceId: string;
  gameId: string;
  version?: string;
}

const unavailable = () => new Error('unavailable in browser');

const emptyPage = (query: ReleaseQuery): ReleasePage => ({
  items: [],
  total: 0,
  page: query.page ?? 1,
  pageSize: query.pageSize ?? 50,
});

export async function listSources(): Promise<Source[]> {
  if (!inWails) return [];
  return ((await SourcesService.ListSources()) ?? []) as unknown as Source[];
}

export async function getSourceDetails(id: string): Promise<SourceDetails> {
  if (!inWails) throw unavailable();
  return (await SourcesService.GetSourceDetails(id)) as unknown as SourceDetails;
}

export async function testSource(url: string): Promise<SourcePreview> {
  if (!inWails) throw unavailable();
  return (await SourcesService.TestSource(url)) as unknown as SourcePreview;
}

export async function addSource(url: string): Promise<Source> {
  if (!inWails) throw unavailable();
  return (await SourcesService.AddSource(url)) as unknown as Source;
}

export async function testSourceFile(path: string): Promise<SourcePreview> {
  if (!inWails) throw unavailable();
  return (await SourcesService.TestSourceFile(path)) as unknown as SourcePreview;
}

export async function addSourceFile(path: string): Promise<Source> {
  if (!inWails) throw unavailable();
  return (await SourcesService.AddSourceFile(path)) as unknown as Source;
}

export async function selectFeedFile(): Promise<string> {
  if (!inWails) throw unavailable();
  return (await SourcesService.SelectFeedFile()) ?? '';
}

export function sourceLocation(source: Pick<Source, 'type' | 'url' | 'path'>): string {
  return source.type === 'file' ? (source.path ?? '') : source.url;
}

export async function removeSource(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await SourcesService.RemoveSource(id);
}

export async function setSourceEnabled(id: string, enabled: boolean): Promise<void> {
  if (!inWails) throw unavailable();
  await SourcesService.SetSourceEnabled(id, enabled);
}

export async function refreshSource(id: string): Promise<RefreshSummary> {
  if (!inWails) throw unavailable();
  return (await SourcesService.RefreshSource(id)) as unknown as RefreshSummary;
}

export async function refreshAllSources(): Promise<RefreshSummary[]> {
  if (!inWails) throw unavailable();
  return ((await SourcesService.RefreshAll()) ?? []) as unknown as RefreshSummary[];
}

export async function queryReleases(query: ReleaseQuery): Promise<ReleasePage> {
  if (!inWails) return emptyPage(query);
  const payload = {
    sourceId: query.sourceId ?? '',
    search: query.search ?? '',
    status: query.status ?? 'all',
    sort: query.sort ?? '',
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 50,
  };
  return (await SourcesService.QueryReleases(payload as never)) as unknown as ReleasePage;
}

export async function getReleasesForGame(gameId: string): Promise<ReleaseGroup[]> {
  if (!inWails) return [];
  return ((await SourcesService.GetReleasesForGame(gameId)) ?? []) as unknown as ReleaseGroup[];
}

export async function getReleasesForTitle(title: string): Promise<ReleaseGroup[]> {
  if (!inWails) return [];
  return ((await SourcesService.GetReleasesForTitle(title)) ?? []) as unknown as ReleaseGroup[];
}

export async function getMatchCandidates(releaseId: string): Promise<MatchCandidate[]> {
  if (!inWails) return [];
  return ((await SourcesService.GetCandidates(releaseId)) ?? []) as unknown as MatchCandidate[];
}

export async function confirmMatch(releaseId: string, gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await SourcesService.ConfirmMatch(releaseId, gameId);
}

export async function ignoreRelease(releaseId: string, ignored: boolean): Promise<void> {
  if (!inWails) throw unavailable();
  await SourcesService.IgnoreRelease(releaseId, ignored);
}

export async function acknowledgeNewReleases(sourceId: string): Promise<void> {
  if (!inWails) return;
  await SourcesService.AcknowledgeNew(sourceId);
}

export async function prepareReleaseDownload(releaseId: string): Promise<ReleaseDownloadRequest> {
  if (!inWails) throw unavailable();
  return (await SourcesService.PrepareDownload(releaseId)) as unknown as ReleaseDownloadRequest;
}

export async function getRelease(releaseId: string): Promise<ReleaseView | null> {
  if (!inWails) return null;
  try {
    return (await SourcesService.GetRelease(releaseId)) as unknown as ReleaseView;
  } catch {
    return null;
  }
}

export async function searchGames(query: string, limit = 20): Promise<CatalogGame[]> {
  if (!inWails) return [];
  return ((await CatalogService.SearchGames(query, limit)) ?? []) as unknown as CatalogGame[];
}

export async function getCatalogGame(id: string): Promise<CatalogGame | null> {
  if (!inWails) return null;
  try {
    return (await CatalogService.GetGame(id)) as unknown as CatalogGame;
  } catch {
    return null;
  }
}

export async function queryCatalogGames(query: CatalogQuery): Promise<CatalogPage> {
  const page = query.page ?? 1;
  const pageSize = query.pageSize ?? 60;
  if (!inWails) return { items: [], total: 0, page, pageSize };
  const payload = { search: query.search ?? '', genre: query.genre ?? '', sort: query.sort ?? '', page, pageSize };
  const result = (await CatalogService.QueryGames(payload as never)) as unknown as CatalogPage;
  return { ...result, items: result.items ?? [] };
}

export async function getGenreFacets(): Promise<GenreFacet[]> {
  if (!inWails) return [];
  return ((await CatalogService.GenreFacets()) ?? []) as unknown as GenreFacet[];
}

export async function openByIGDB(igdbId: number, title: string): Promise<CatalogGame> {
  if (!inWails) throw unavailable();
  return (await CatalogService.OpenByIGDB(String(igdbId), title)) as unknown as CatalogGame;
}

export async function getCatalogGames(ids: string[]): Promise<CatalogGame[]> {
  if (!inWails || ids.length === 0) return [];
  return ((await CatalogService.GetGames(ids)) ?? []) as unknown as CatalogGame[];
}
