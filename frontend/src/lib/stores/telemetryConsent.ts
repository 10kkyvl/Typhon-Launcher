import { derived, get, type Readable } from 'svelte/store';
import { saveConsent } from '../services/settings';
import { settings } from './settings';

// Anything that is not a recorded answer counts as unanswered, including a
// settings object saved before this field existed. Erring towards asking costs
// one prompt; erring the other way silently applies a default nobody chose.
export const showTelemetryConsent: Readable<boolean> = derived(
  settings,
  ($settings) => $settings !== null && !($settings.telemetryConsentVersion > 0),
);

export async function respondTelemetryConsent(usageStats: boolean, diagnostics: boolean): Promise<void> {
  if (!get(settings)) {
    throw new Error('settings not loaded');
  }
  const next = await saveConsent(usageStats, diagnostics);
  settings.set(next);
}
