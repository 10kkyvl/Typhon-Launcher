import { get } from 'svelte/store';
import { settings, updateSettings } from './settings';

export function needsSourcesNotice(): boolean {
  return get(settings)?.sourcesNoticeAccepted !== true;
}

export async function acceptSourcesNotice(): Promise<boolean> {
  await updateSettings({ sourcesNoticeAccepted: true });
  return get(settings)?.sourcesNoticeAccepted === true;
}
