import { derived, get, type Readable } from 'svelte/store';
import { saveConsent } from '../services/settings';
import { settings } from './settings';

// The version this build's prompt text answers. It must match
// settings.CurrentTelemetryConsent: an answer given to older wording covers
// only what that wording described, and version 2 added the compatibility
// report, which carries the chip and the macOS version.
export const currentTelemetryConsent = 2;

// Anything below the current version counts as unanswered, including a settings
// object saved before this field existed. Erring towards asking costs one
// prompt; erring the other way silently applies a default nobody chose.
export const showTelemetryConsent: Readable<boolean> = derived(
  settings,
  ($settings) => $settings !== null && !($settings.telemetryConsentVersion >= currentTelemetryConsent),
);

export async function respondTelemetryConsent(usageStats: boolean, diagnostics: boolean): Promise<void> {
  if (!get(settings)) {
    throw new Error('settings not loaded');
  }
  const next = await saveConsent(usageStats, diagnostics);
  settings.set(next);
}
