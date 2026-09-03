import { Service as LibraryService } from '../../../bindings/typhon/internal/library';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';
import { markError } from '../game/markMessages';

export { markError };

export interface LibraryGame {
  id: string;
  title: string;
  executable: string;
  launchArgs?: string[];
  installDir: string;
  cover: string;
  version: string;
  sizeBytes: number;
  lastPlayed: string | null;
  playtimeSeconds: number;
  installedAt: string;
  releaseId?: string;
  sourceId?: string;
  canonicalGameId?: string;
  source?: string;
  uninstalled?: boolean;
  shortcutPath?: string;
  savesDir?: string;
  favorite?: boolean;
  favoriteAt?: string | null;
  status?: string;
  statusAt?: string | null;
}

export interface SavesResult {
  path: string;
  candidates: string[] | null;
  unreadable: number;
}

const unavailable = () => new Error('unavailable in browser');

export async function getGames(): Promise<LibraryGame[]> {
  if (!inWails) return [];
  return ((await LibraryService.GetGames()) ?? []) as unknown as LibraryGame[];
}

export async function getRunningGames(): Promise<string[]> {
  if (!inWails) return [];
  return (await LibraryService.GetRunningGames()) ?? [];
}

export async function addGame(executable: string, title: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.AddGame(executable, title)) as unknown as LibraryGame;
}

export async function addCatalogGame(canonicalGameId: string, title: string, cover: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.AddCatalogGame(canonicalGameId, title, cover)) as unknown as LibraryGame;
}

export async function setExecutable(id: string, executable: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetExecutable(id, executable)) as unknown as LibraryGame;
}

export async function playGame(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.PlayGame(id);
}

export async function stopGame(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.StopGame(id);
}

export async function createShortcut(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.CreateShortcut(id);
}

export async function removeShortcut(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.RemoveShortcut(id);
}

export async function setFavorite(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetFavorite(id, on)) as unknown as LibraryGame;
}

export async function setStatus(id: string, status: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetStatus(id, status)) as unknown as LibraryGame;
}

export async function locateSaves(id: string): Promise<SavesResult> {
  if (!inWails) throw unavailable();
  return (await LibraryService.LocateSaves(id)) as unknown as SavesResult;
}

export async function setSavesDir(id: string, dir: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetSavesDir(id, dir)) as unknown as LibraryGame;
}

export async function selectExecutable(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectExecutable(title);
}
