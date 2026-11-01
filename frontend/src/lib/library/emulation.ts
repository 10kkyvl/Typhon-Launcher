import { errorCode } from '../i18n/errors';

export interface EmulatedGame {
  emulation?: { platform: string } | null;
}

export interface ConsoleInfo {
  id: string;
  name: string;
  extensions: string[] | null;
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

export function emulationPlatform(game: EmulatedGame | null | undefined): string {
  return game?.emulation?.platform ?? '';
}

export function platformBadge(platform: string): string {
  return platform.toUpperCase();
}

export function needsEmulator(err: unknown): boolean {
  return EMULATOR_PROMPT_CODES.includes(errorCode(err));
}

export function imageExtension(path: string): string {
  const base = path.split(/[\\/]/).pop() ?? '';
  const dot = base.lastIndexOf('.');
  return dot > 0 ? base.slice(dot).toLowerCase() : '';
}

export function consolesForImage(consoles: readonly ConsoleInfo[], path: string): ConsoleInfo[] {
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

export function titleFromImage(path: string): string {
  const base = (path.split(/[\\/]/).pop() ?? '').trim();
  const dot = base.lastIndexOf('.');
  const stem = dot > 0 && /^\.[a-z0-9]{1,5}$/i.test(base.slice(dot)) ? base.slice(0, dot) : base;
  const plain = stem.replace(/_/g, ' ').replace(/\s+/g, ' ').trim();
  const cleaned = plain
    .replace(/\s*\([^()]*\)/g, '')
    .replace(/\s*\[[^[\]]*\]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
  return cleaned || plain;
}
