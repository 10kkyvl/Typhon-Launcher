import { Service as SettingsService } from '../../../bindings/typhon/internal/settings';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';

export interface Settings {
  theme: string;
  language: string;
  uiScale: number;
  downloadsPath: string;
  gamesPath: string;
  screenshotsPath: string;
  launchOnStartup: boolean;
  minimizeToTray: boolean;
  hardwareAcceleration: boolean;
  animationsEnabled: boolean;
  maxActiveDownloads: number;
  downloadRateLimit: number;
  uploadRateLimit: number;
  seedAfterDownload: boolean;
  installCleanupPolicy: string;
  autoInstall: boolean;
  verifyAfterInstall: boolean;
}

const FALLBACK_KEY = 'typhon.settings';

const fallbackDefaults: Settings = {
  theme: 'dark',
  language: 'ru',
  uiScale: 1,
  downloadsPath: 'D:\\Typhon\\Downloads',
  gamesPath: 'D:\\Typhon\\Games',
  screenshotsPath: 'D:\\Typhon\\Screenshots',
  launchOnStartup: false,
  minimizeToTray: true,
  hardwareAcceleration: true,
  animationsEnabled: true,
  maxActiveDownloads: 2,
  downloadRateLimit: 0,
  uploadRateLimit: 0,
  seedAfterDownload: false,
  installCleanupPolicy: 'keep',
  autoInstall: false,
  verifyAfterInstall: true,
};

export async function getSettings(): Promise<Settings> {
  if (inWails) {
    return (await SettingsService.GetSettings()) as Settings;
  }
  try {
    const raw = localStorage.getItem(FALLBACK_KEY);
    return raw ? { ...fallbackDefaults, ...JSON.parse(raw) } : { ...fallbackDefaults };
  } catch {
    return { ...fallbackDefaults };
  }
}

export async function saveSettings(next: Settings): Promise<void> {
  if (inWails) {
    await SettingsService.SaveSettings(next);
    return;
  }
  localStorage.setItem(FALLBACK_KEY, JSON.stringify(next));
}

export async function selectFolder(title: string): Promise<string> {
  if (!inWails) return '';
  return await AppService.SelectFolder(title);
}

export async function openFolder(path: string): Promise<void> {
  if (!inWails) throw new Error('unavailable in browser');
  await AppService.OpenFolder(path);
}
