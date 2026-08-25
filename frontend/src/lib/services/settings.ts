import { Service as SettingsService } from '../../../bindings/typhon/internal/settings';
import { Service as AppService } from '../../../bindings/typhon/internal/app';
import { inWails } from './backend';

export interface Settings {
  theme: string;
  language: string;
  uiScale: number;
  libraryPath: string;
  downloadsPath: string;
  gamesPath: string;
  screenshotsPath: string;
  launchOnStartup: boolean;
  minimizeToTray: boolean;
  discordRichPresence: boolean;
  hardwareAcceleration: boolean;
  animationsEnabled: boolean;
  maxActiveDownloads: number;
  downloadRateLimit: number;
  uploadRateLimit: number;
  uploadWhileDownloading: boolean;
  seedAfterDownload: boolean;
  installCleanupPolicy: string;
  autoInstall: boolean;
  sourceRefreshInterval: string;
  verifyAfterInstall: boolean;
  installSkipShortcuts: boolean;
  installSkipExtras: boolean;
  updateCheckAutomatically: boolean;
  updateAutoDownload: boolean;
  updateAutoInstall: boolean;
  updateSaveBackup: boolean;
  keepPreviousVersion: string;
  allowTorrentReuse: boolean;
  sourcesNoticeAccepted: boolean;
}

const FALLBACK_KEY = 'typhon.settings';

const fallbackDefaults: Settings = {
  theme: 'dark',
  language: 'ru',
  uiScale: 1,
  libraryPath: '',
  downloadsPath: '',
  gamesPath: '',
  screenshotsPath: '',
  launchOnStartup: false,
  minimizeToTray: true,
  discordRichPresence: false,
  hardwareAcceleration: true,
  animationsEnabled: true,
  maxActiveDownloads: 2,
  downloadRateLimit: 0,
  uploadRateLimit: 0,
  uploadWhileDownloading: false,
  seedAfterDownload: false,
  installCleanupPolicy: 'delete',
  autoInstall: false,
  sourceRefreshInterval: '6h',
  verifyAfterInstall: true,
  installSkipShortcuts: true,
  installSkipExtras: true,
  updateCheckAutomatically: true,
  updateAutoDownload: false,
  updateAutoInstall: false,
  updateSaveBackup: true,
  keepPreviousVersion: 'first_launch',
  allowTorrentReuse: true,
  sourcesNoticeAccepted: false,
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

export async function proposeLibraryPath(parent: string): Promise<string> {
  if (!inWails) throw new Error('unavailable in browser');
  return await SettingsService.ProposeLibraryPath(parent);
}

export async function setupLibrary(parent: string): Promise<Settings> {
  if (!inWails) throw new Error('unavailable in browser');
  return (await SettingsService.SetupLibrary(parent)) as Settings;
}
