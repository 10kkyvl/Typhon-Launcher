import { updateSettingsResult } from '../stores/settings';
import type { Settings } from '../services/settings';

export const BIG_PICTURE_UI_SCALES = [0.9, 1, 1.1, 1.2] as const;
export const BIG_PICTURE_DOWNLOAD_LIMITS = [1, 2, 3, 5] as const;
export const BIG_PICTURE_LANGUAGES = ['system', 'ru', 'en'] as const;

export type PreferenceSaveState = 'saved' | 'failed';

export async function saveBigPicturePreference(patch: Partial<Settings>): Promise<PreferenceSaveState> {
  try {
    return (await updateSettingsResult(patch)) ? 'saved' : 'failed';
  } catch {
    return 'failed';
  }
}
