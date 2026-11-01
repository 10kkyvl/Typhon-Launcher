import { Browser } from '@wailsio/runtime';
import { Service as EmulatorsService } from '../../../bindings/typhon/internal/emulators';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { get } from 'svelte/store';
import { locale } from '../i18n/locale';
import { inWails } from './backend';

export type EmulatorStatus = 'not_set' | 'ready' | 'missing' | 'unavailable';

export interface PlatformStatus {
  id: string;
  name: string;
  emulator: string;
  homepage: string;
  extensions: string[];
  path: string;
  status: EmulatorStatus | string;
  reason: string;
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
  return { ...value, extensions: value.extensions ?? [] };
}

export async function listPlatforms(): Promise<PlatformStatus[]> {
  if (!inWails) return [];
  return ((await EmulatorsService.List()) ?? []).map((item) => row(item as PlatformStatus));
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
  return await AppService.SelectExecutable(title, get(locale));
}

export async function selectImageFile(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectExecutable(title, get(locale));
}
