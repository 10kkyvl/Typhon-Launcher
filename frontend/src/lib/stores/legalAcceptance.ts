import { derived, get, writable, type Readable } from 'svelte/store';
import { inWails } from '../services/backend';
import { legalVersion } from '../services/legal';
import { saveLegalAcceptance } from '../services/settings';
import { settings } from './settings';

export const currentLegalVersion = writable<string | null>(null);
export const legalVersionFailed = writable(false);

export const showLegalAcceptance: Readable<boolean> = derived(
  [settings, currentLegalVersion, legalVersionFailed],
  ([$settings, $version, $failed]) => {
    if ($settings === null) return false;
    if ($version === null) return $failed;
    return $settings.legalAcceptedVersion !== $version;
  },
);

export async function loadLegalVersion(): Promise<void> {
  legalVersionFailed.set(false);
  try {
    currentLegalVersion.set(await legalVersion());
  } catch {
    currentLegalVersion.set(null);
    legalVersionFailed.set(true);
  }
}

export function initLegalAcceptance(): void {
  if (!inWails) return;
  void loadLegalVersion();
}

export async function respondLegalAcceptance(): Promise<void> {
  const version = get(currentLegalVersion);
  if (version === null) {
    throw new Error('legal version not loaded');
  }
  if (!get(settings)) {
    throw new Error('settings not loaded');
  }
  const next = await saveLegalAcceptance(version);
  settings.set(next);
}
