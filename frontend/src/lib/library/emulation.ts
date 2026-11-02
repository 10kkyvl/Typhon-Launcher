import { errorCode } from '../i18n/errors';

export interface EmulatedGame {
  id?: string;
  emulation?: { platform: string; catalogGameId?: string; titleId?: string } | null;
  owned?: boolean;
  installType?: string;
}

export interface ConsoleInfo {
  id: string;
  name: string;
  extensions: string[] | null;
  imageNames?: string[] | null;
}

export const EMULATOR_PROMPT_CODES: readonly string[] = [
  'emulators.not_configured',
  'emulators.file_missing',
  'emulators.windows_program',
  'emulators.not_exe',
  'emulators.not_regular_file',
  'emulators.file_unreadable',
  'emulators.path_relative',
];

export function isEmulated(game: EmulatedGame | null | undefined): boolean {
  return Boolean(game?.emulation?.platform);
}

export function isInstalledImage(game: EmulatedGame | null | undefined): boolean {
  return isEmulated(game) && game?.owned === true && game.installType === 'console_image';
}

export interface CatalogPageGame extends EmulatedGame {
  uninstalled?: boolean;
}

export interface PageGame<T> {
  game: T | undefined;
  viaCatalog: boolean;
}

export function pageGame<T extends CatalogPageGame>(
  own: T | undefined,
  games: readonly T[],
  catalogIds: readonly string[],
): PageGame<T> {
  if (own) return { game: own, viaCatalog: false };
  const ids = catalogIds.filter(Boolean);
  const image = games.find(
    (game) => !game.uninstalled && Boolean(game.emulation?.catalogGameId) && ids.includes(game.emulation?.catalogGameId ?? ''),
  );
  return { game: image, viaCatalog: image !== undefined };
}

export function emulationPlatform(game: EmulatedGame | null | undefined): string {
  return game?.emulation?.platform ?? '';
}

export function copiesOnFirstLaunch(game: EmulatedGame | null | undefined): boolean {
  return game?.emulation?.platform === 'vita' && !game.emulation.titleId;
}

export function emulatorMissing(row: { status: string }): boolean {
  return row.status !== 'ready';
}

const BADGES: Record<string, string> = { vita: 'PS Vita' };

export function platformBadge(platform: string): string {
  return BADGES[platform] ?? platform.toUpperCase();
}

export function needsEmulator(err: unknown): boolean {
  return EMULATOR_PROMPT_CODES.includes(errorCode(err));
}

export function imageExtension(path: string): string {
  const base = path.split(/[\\/]/).pop() ?? '';
  const dot = base.lastIndexOf('.');
  return dot > 0 ? base.slice(dot).toLowerCase() : '';
}

export function imageBaseName(path: string): string {
  return (path.split(/[\\/]/).pop() ?? '').toLowerCase();
}

function claimsName(item: ConsoleInfo, base: string): boolean {
  return (item.imageNames ?? []).some((name) => name.toLowerCase() === base);
}

export function consolesForImage(consoles: readonly ConsoleInfo[], path: string): ConsoleInfo[] {
  const base = imageBaseName(path);
  if (!base) return [];
  const named = consoles.filter((item) => claimsName(item, base));
  if (named.length > 0) return named;
  const ext = imageExtension(path);
  if (!ext) return [];
  return consoles.filter((item) => (item.extensions ?? []).includes(ext));
}

export function consoleForImage(consoles: readonly ConsoleInfo[], path: string): string {
  const found = consolesForImage(consoles, path);
  return found.length === 1 ? found[0].id : '';
}

export function imageExtensions(consoles: readonly ConsoleInfo[]): string[] {
  return [...new Set(consoles.flatMap((item) => item.extensions ?? []))].sort();
}

export function imageNames(consoles: readonly ConsoleInfo[]): string[] {
  const seen = new Map<string, string>();
  for (const name of consoles.flatMap((item) => item.imageNames ?? [])) {
    if (!seen.has(name.toLowerCase())) seen.set(name.toLowerCase(), name);
  }
  return [...seen.values()].sort();
}
