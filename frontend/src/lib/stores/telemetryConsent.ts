import { derived, get, type Readable } from 'svelte/store';
import { saveConsent, type Settings } from '../services/settings';
import { settings } from './settings';

// The version this build's prompt text answers. It must match
// settings.CurrentTelemetryConsent: an answer given to older wording covers
// only what that wording described, and version 2 added the compatibility
// report, which carries the chip and the macOS version; version 3 moved the
// heartbeat's installation ID and catalog game ID under the usage statistics
// switch.
export const currentTelemetryConsent = 3;

export interface ConsentChoice {
  usageStats: boolean;
  diagnostics: boolean;
}

// Before the first answer the stored switches are only defaults, so nothing is
// preselected. A re-prompt repeats the previous answer, otherwise accepting it
// would silently switch off what the user had turned on.
export function consentPreselection(current: Settings | null): ConsentChoice {
  if (!current || !(current.telemetryConsentVersion > 0)) {
    return { usageStats: false, diagnostics: false };
  }
  return {
    usageStats: Boolean(current.anonymousUsageStats),
    diagnostics: Boolean(current.anonymousDiagnostics),
  };
}

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
