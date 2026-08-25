import { Service as LibraryService } from '../../../bindings/typhon/internal/library';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';

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

export async function selectExecutable(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectExecutable(title);
}
