import { get } from 'svelte/store';
import { locale } from '../i18n/locale';
import { Service as LibraryService } from '../../../bindings/typhon/internal/library';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';
import { errorCode } from '../i18n/errors';
import { markError } from '../game/markMessages';
import { emulationPlatform, emulatorAlreadyOpen, installingPackage, needsBios, needsEmulator } from '../library/emulation';
import { libraryErrorText } from '../library/errors';
import { toast } from '../stores/toasts';
import { askForBios } from '../stores/biosPrompt';
import { askToCloseEmulator } from '../stores/emulatorBusyPrompt';
import { askForEmulator } from '../stores/emulatorPrompt';

export { markError };

export interface LibraryExternal {
  provider: 'steam' | 'gog' | 'epic' | string;
  appId: string;
  launchUri?: string;
}

export interface LaunchProfile {
  id: string;
  name: string;
  executable: string;
  args?: string[] | null;
  runAsAdmin?: boolean;
  shortcutPath?: string;
}

export interface ProfileInput {
  name: string;
  executable: string;
  argsLine: string;
  runAsAdmin: boolean;
}

export interface ProfileView {
  id: string;
  name: string;
  executable: string;
  argsLine: string;
  runAsAdmin: boolean;
  isDefault: boolean;
  isLast: boolean;
  shortcutPath?: string;
}

export interface GameModeOverride {
  highPerformance?: boolean | null;
  hideLauncher?: boolean | null;
  raisePriority?: boolean | null;
}

export interface LibraryEmulation {
  platform: string;
  catalogGameId?: string;
  titleId?: string;
  files?: string[] | null;
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
  emulation?: LibraryEmulation | null;
  owned?: boolean;
  installType?: string;
  uninstalled?: boolean;
  shortcutPath?: string;
  savesDir?: string;
  runAsAdmin?: boolean;
  hidePresence?: boolean;
  favorite?: boolean;
  favoriteAt?: string | null;
  status?: string;
  statusAt?: string | null;
  completedAt?: string | null;
  tags?: string[];
  hidden?: boolean;
  notes?: string;
  profiles?: LaunchProfile[] | null;
  lastProfile?: string;
  gameMode?: GameModeOverride | null;
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
  hintsError?: string;
  hint?: { source: string; license: string; via: string; pageUrl: string } | null;
}

export interface ProfileFallback {
  gameId: string;
  profileName: string;
  code: string;
}

export interface SessionEvent {
  gameId: string;
  sessionSeconds: number;
  playtimeSeconds: number;
  exitCode: number;
  exitKnown: boolean;
  crashed: boolean;
  reasonCode: string;
  emulatorQuickExit: boolean;
  emulator: string;
}

export type CrashHint = 'vcredist' | 'directx' | 'compat' | 'antivirus' | 'runtimes' | 'unknown';

export interface CrashEvent {
  gameId: string;
  exitCode: number;
  exitKnown: boolean;
  reasonCode: string;
  faultModule: string;
  exceptionCode: string;
  hint: string;
  missingFile: string;
  sessionSeconds: number;
  dumpFound: boolean;
  journalUnknown: boolean;
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

export async function addEmulatedGame(romPath: string, platformID: string, title: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.AddEmulatedGame(romPath, platformID, title)) as unknown as LibraryGame;
}

export async function findEmulatedCover(id: string): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.FindEmulatedCover(id)) as unknown as LibraryGame;
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
    return await recoverLaunch(id, err);
  }
  return true;
}

// recoverLaunch turns a launch the player can fix into a question; any other
// error is shown as it is.
async function recoverLaunch(id: string, err: unknown): Promise<boolean> {
  if (errorCode(err) === 'library.launch_cancelled') return false;
  if (installingPackage(err)) {
    // Not a failure: the emulator's installer is open and waits for the player.
    toast(libraryErrorText(err, ''), 'info');
    return false;
  }
  if (needsBios(err)) return await launchAfterBios(id, err);
  if (emulatorAlreadyOpen(err)) return await launchAfterClosing(id, err);
  if (!needsEmulator(err)) throw err;
  return await launchAfterEmulator(id, err);
}

// The emulator is closed only with the player's yes; if another copy opens in
// the meantime, the next error is shown instead of asking in a loop.
async function launchAfterClosing(id: string, first: unknown): Promise<boolean> {
  const platform = emulationPlatform((await getGames()).find((game) => game.id === id));
  if (!platform) throw first;
  if (!(await askToCloseEmulator(platform))) return false;
  try { await LibraryService.PlayGame(id); } catch (err) {
    if (needsBios(err)) return await launchAfterBios(id, err);
    if (errorCode(err) !== 'library.launch_cancelled') throw err;
    return false;
  }
  return true;
}

// The game is asked for again only once: if the emulator the user just
// pointed at is rejected, that error is shown as it is instead of looping.
async function launchAfterEmulator(id: string, first: unknown): Promise<boolean> {
  const platform = emulationPlatform((await getGames()).find((game) => game.id === id));
  if (!platform) throw first;
  if (!(await askForEmulator(platform, errorCode(first)))) return false;
  try { await LibraryService.PlayGame(id); } catch (err) {
    if (needsBios(err)) return await launchAfterBios(id, err);
    if (errorCode(err) !== 'library.launch_cancelled') throw err;
    return false;
  }
  return true;
}

// The player may check again as often as they like: each try only reads the
// folder, and closing the prompt gives the launch up.
async function launchAfterBios(id: string, first: unknown): Promise<boolean> {
  const platform = emulationPlatform((await getGames()).find((game) => game.id === id));
  if (!platform) throw first;
  for (let stillMissing = false; ; stillMissing = true) {
    if (!(await askForBios(platform, stillMissing))) return false;
    try { await LibraryService.PlayGame(id); } catch (err) {
      if (needsBios(err)) continue;
      if (errorCode(err) !== 'library.launch_cancelled') throw err;
      return false;
    }
    return true;
  }
}

export async function playGameProfile(id: string, profileID: string): Promise<boolean> {
  if (!inWails) throw unavailable();
  try { await LibraryService.PlayGameProfile(id, profileID); } catch (err) {
    if (errorCode(err) !== 'library.launch_cancelled') throw err;
    return false;
  }
  return true;
}

export async function getProfiles(id: string): Promise<ProfileView[]> {
  if (!inWails) return [];
  return ((await LibraryService.Profiles(id)) ?? []) as ProfileView[];
}

export async function addProfile(id: string, input: ProfileInput): Promise<ProfileView> {
  if (!inWails) throw unavailable();
  return (await LibraryService.AddProfile(id, input)) as ProfileView;
}

export async function updateProfile(id: string, profileID: string, input: ProfileInput): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.UpdateProfile(id, profileID, input);
}

export async function removeProfile(id: string, profileID: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.RemoveProfile(id, profileID);
}

export async function createProfileShortcut(id: string, profileID: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.CreateProfileShortcut(id, profileID);
}

export async function profileSteamStatus(id: string, profileID: string): Promise<SteamStatus> {
  if (!inWails) return { supported: false, installed: false, added: false };
  return await LibraryService.SteamProfileStatus(id, profileID);
}

export async function addProfileToSteam(id: string, profileID: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.AddProfileToSteam(id, profileID);
}

export async function removeProfileFromSteam(id: string, profileID: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.RemoveProfileFromSteam(id, profileID);
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

export async function setCompletedAt(id: string, date: Date): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.SetCompletedAt(id, date.toISOString());
}

export async function setTags(id: string, tags: string[]): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetTags(id, tags)) as unknown as LibraryGame;
}

export async function setHidden(id: string, on: boolean): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetHidden(id, on)) as unknown as LibraryGame;
}

export async function setNotes(id: string, text: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LibraryService.SetNotes(id, text);
}

export async function notesLimit(): Promise<number> {
  if (!inWails) throw unavailable();
  return await LibraryService.NotesLimit();
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

export async function setGameModeOverride(id: string, o: GameModeOverride): Promise<LibraryGame> {
  if (!inWails) throw unavailable();
  return (await LibraryService.SetGameModeOverride(id, o)) as unknown as LibraryGame;
}

export async function selectExecutable(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectExecutable(title, get(locale));
}

export async function selectGameExecutable(title: string, installDir: string, current: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectGameExecutable(title, installDir, current, get(locale));
}
