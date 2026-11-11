import { get } from 'svelte/store';
import { settings, updateSettings, updateSettingsReporting } from './settings';

export function needsSourcesNotice(): boolean {
  return get(settings)?.sourcesNoticeAccepted !== true;
}

/** Saves the acceptance. A failed save goes to `onError` when given (the notice shows it), else to a toast. */
export async function acceptSourcesNotice(onError?: (err: unknown) => void): Promise<boolean> {
  if (onError) await updateSettingsReporting({ sourcesNoticeAccepted: true }, onError);
  else await updateSettings({ sourcesNoticeAccepted: true });
  return get(settings)?.sourcesNoticeAccepted === true;
}
