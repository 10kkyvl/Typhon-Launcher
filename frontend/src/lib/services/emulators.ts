import { Browser } from '@wailsio/runtime';
import { Service as EmulatorsService } from '../../../bindings/typhon/internal/emulators';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { get } from 'svelte/store';
import { msg } from '../i18n';
import { locale } from '../i18n/locale';
import { Service as LibraryService } from '../../../bindings/typhon/internal/library';
import { inWails } from './backend';

export type EmulatorStatus = 'not_set' | 'ready' | 'missing' | 'unavailable';

export interface PlatformStatus {
  id: string;
  name: string;
  emulator: string;
  homepage: string;
  extensions: string[];
  imageNames: string[];
  path: string;
  status: EmulatorStatus | string;
  reason: string;
  warnings: string[];
}

export interface FoundEmulator {
  platform: string;
  path: string;
}

export interface DetectResult {
  found: FoundEmulator[];
  unreadable: number;
}

const unavailable = () => new Error('unavailable in browser');

function row(value: PlatformStatus): PlatformStatus {
  return {
    ...value,
    extensions: value.extensions ?? [],
    imageNames: value.imageNames ?? [],
    warnings: value.warnings ?? [],
  };
}

export async function listPlatforms(): Promise<PlatformStatus[]> {
  if (!inWails) return [];
  return ((await EmulatorsService.List()) ?? []).map((item) => row(item as PlatformStatus));
}

// imageConsoles asks which consoles the files around the image are laid out for.
export async function imageConsoles(path: string): Promise<string[]> {
  if (!inWails) return [];
  return (await EmulatorsService.ImageConsoles(path)) ?? [];
}

// imageTitle suggests the name the game carries in its param.sfo; empty when it
// has none.
export async function imageTitle(path: string, platform: string): Promise<string> {
  if (!inWails) return '';
  return (await EmulatorsService.ImageTitle(path, platform)) ?? '';
}

export async function biosFolder(platform: string): Promise<string> {
  if (!inWails) throw unavailable();
  return await EmulatorsService.BIOSFolder(platform);
}

export async function openBiosFolder(platform: string): Promise<void> {
  if (!inWails) throw unavailable();
  await EmulatorsService.OpenBIOSFolder(platform);
}

export async function closeEmulator(platform: string): Promise<void> {
  if (!inWails) throw unavailable();
  await EmulatorsService.CloseEmulator(platform);
}

export async function setEmulatorPath(platform: string, path: string): Promise<PlatformStatus> {
  if (!inWails) throw unavailable();
  return row((await EmulatorsService.SetPath(platform, path)) as PlatformStatus);
}

export async function detectEmulators(): Promise<DetectResult> {
  if (!inWails) throw unavailable();
  const result = await EmulatorsService.Detect();
  return { found: result.found ?? [], unreadable: result.unreadable };
}

export async function openOfficialPage(platform: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Browser.OpenURL(await EmulatorsService.OfficialPage(platform));
}

export async function selectEmulatorFile(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectEmulator(title, get(locale));
}

export async function selectImageFile(title: string, platformIDs: string[] | null = null): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectImageFile(title, msg('emul.imagesFilter'), get(locale), platformIDs);
}

export async function gameInEmulator(id: string): Promise<boolean> {
  if (!inWails) return false;
  return await LibraryService.GameInEmulator(id);
}

export async function missingImages(): Promise<string[]> {
  if (!inWails) return [];
  return (await LibraryService.MissingImages()) ?? [];
}
