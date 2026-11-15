import { updateSettingsResult } from '../stores/settings';
import type { Settings } from '../services/settings';

export const BIG_PICTURE_UI_SCALES = [0.9, 1, 1.1, 1.2] as const;
export const BIG_PICTURE_DOWNLOAD_LIMITS = [1, 2, 3, 5] as const;
export const BIG_PICTURE_LANGUAGES = ['system', 'ru', 'en'] as const;

/**
 * The concurrency row: the four, and a value another screen stored (Go accepts 1–10) in its place in
 * order, so the current value is always one of the choices and marked.
 */
export function bigPictureDownloadChoices(stored: number | undefined): number[] {
  const choices: number[] = [...BIG_PICTURE_DOWNLOAD_LIMITS];
  if (stored === undefined || !Number.isInteger(stored) || stored < 1 || choices.includes(stored)) return choices;
  return [...choices, stored].sort((a, b) => a - b);
}

export type PreferenceSaveState = 'saved' | 'failed';

export async function saveBigPicturePreference(patch: Partial<Settings>): Promise<PreferenceSaveState> {
  try {
    return (await updateSettingsResult(patch)) ? 'saved' : 'failed';
  } catch {
    return 'failed';
  }
}
