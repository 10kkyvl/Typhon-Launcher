import { get } from 'svelte/store';
import { locale } from '../i18n/locale';
import { Service as LibraryService } from '../../../bindings/typhon/internal/library';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';
import { errorCode } from '../i18n/errors';
import { markError } from '../game/markMessages';

export { markError };

export interface LibraryExternal {
  provider: 'steam' | 'gog' | 'epic' | string;
  appId: string;
  launchUri?: string;
}

export interface LibraryGame {
  id: string;
  title: string;
  executable: string;
  launchArgs?: string[];
  requiresSteam?: boolean | null;
  installDir: string;
  cover: string;
  version: string;
  sizeBytes: number;
  sizeUnknown?: boolean;
  lastPlayed: string | null;
  playtimeSeconds: number;
  installedAt: string;
  releaseId?: string;
  sourceId?: string;
  distributionId?: string;
  releaseUploadedAt?: string;
  canonicalGameId?: string;
  source?: 'external' | string;
  external?: LibraryExternal | null;
  uninstalled?: boolean;
  shortcutPath?: string;
  savesDir?: string;
  runAsAdmin?: boolean;
  hidePresence?: boolean;
  favorite?: boolean;
  favoriteAt?: string | null;
  status?: string;
  statusAt?: string | null;
  tags?: string[];
  hidden?: boolean;
}

export interface LibraryCollection {
  id: string;
  name: string;
  tags?: string[];
  genres?: string[];
  status?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SavesResult {
  path: string;
  candidates: string[] | null;
  unreadable: number;
}

export interface SessionEvent {
  gameId: string;
  sessionSeconds: number;
  playtimeSeconds: number;
  exitCode: number;
  exitKnown: boolean;
  crashed: boolean;
  reasonCode: string;
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
  await launchGame(id);
}

// Keep cancellation distinguishable for surfaces that own the return from a game.
// Existing callers of playGame still treat a dismissed native prompt as a no-op.
export async function launchGame(id: string): Promise<boolean> {
  if (!inWails) throw unavailable();
  try { await LibraryService.PlayGame(id); } catch (err) {
    if (errorCode(err) !== 'library.launch_cancelled') throw err;
    return false;
  }
  return true;
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

export interface SteamStatus {
  supported: boolean;
  installed: boolean;
  added: boolean;
}

export async function steamStatus(id: string): Promise<SteamStatus> {
  if (!inWails) return { supported: false, installed: false, added: false };
  return await LibraryService.SteamStatus(id);
}

export async function addToSteam(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.AddToSteam(id);
}

export async function removeFromSteam(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.RemoveFromSteam(id);
}

export async function setFavorite(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetFavorite(id, on)) as unknown as LibraryGame;
}

export async function setRequiresSteam(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetRequiresSteam(id, on)) as unknown as LibraryGame;
}

export async function setStatus(id: string, status: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetStatus(id, status)) as unknown as LibraryGame;
}

export async function setTags(id: string, tags: string[]): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetTags(id, tags)) as unknown as LibraryGame;
}

export async function setHidden(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetHidden(id, on)) as unknown as LibraryGame;
}

export async function allTags(): Promise<string[]> {
  if (!inWails) return [];
  return (await LibraryService.AllTags()) ?? [];
}

export async function getCollections(): Promise<LibraryCollection[]> {
  if (!inWails) return [];
  return ((await LibraryService.GetCollections()) ?? []) as unknown as LibraryCollection[];
}

export async function saveCollection(collection: LibraryCollection): Promise<LibraryCollection> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SaveCollection(collection as never)) as unknown as LibraryCollection;
}

export async function deleteCollection(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.DeleteCollection(id);
}

export async function locateSaves(id: string): Promise<SavesResult> {
  if (!inWails) throw unavailable();
  return (await LibraryService.LocateSaves(id)) as unknown as SavesResult;
}

export async function setSavesDir(id: string, dir: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetSavesDir(id, dir)) as unknown as LibraryGame;
}

export async function clearSavesDir(id: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.ClearSavesDir(id)) as unknown as LibraryGame;
}

export async function getLaunchArgsLine(id: string): Promise<string> {
  if (!inWails) throw unavailable();
  return await LibraryService.GetLaunchArgsLine(id);
}

export async function setLaunchArgsLine(id: string, line: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetLaunchArgsLine(id, line)) as unknown as LibraryGame;
}

export async function setRunAsAdmin(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetRunAsAdmin(id, on)) as unknown as LibraryGame;
}

export async function setHidePresence(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetHidePresence(id, on)) as unknown as LibraryGame;
}

export async function selectExecutable(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectExecutable(title, get(locale));
}

export async function selectGameExecutable(title: string, installDir: string, current: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectGameExecutable(title, installDir, current, get(locale));
}
