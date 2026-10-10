import { applyPersonalAccent } from '../theme/apply';
import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getSettings, saveSettingsPatch, setupLibrary, type Settings } from '../services/settings';
import { toast } from './toasts';
import { applyLanguage, msg } from '../i18n';

export const settings = writable<Settings | null>(null);

settings.subscribe((value) => {
  if (!value) return;
  applyPersonalAccent(value.accentColor ?? '');
  applyLanguage(value.language);
  document.documentElement.style.setProperty('--ui-scale', String(value.uiScale));
  document.documentElement.classList.toggle('no-anim', !value.animationsEnabled);
});

export async function initSettings() {
  settings.set(await getSettings());
  if (inWails) {
    Events.On('settings:updated', (event) => {
      applyExternalSettings(event.data as Settings);
    });
  }
}

// Saves run one at a time so that clicks on the same field reach Go in the
// order they were made, and so that a failed save is rolled back against the
// settings the previous save confirmed.
let saving: Promise<void> = Promise.resolve();
let revision = 0;
let queued = 0;
let confirmed: Settings | null = null;
const fieldRevision = new Map<string, number>();
const unsettled: Partial<Settings>[] = [];

// Go changed settings on its own (account sync). Saves still waiting would
// roll back to a baseline that no longer matches the store, and their
// optimistic values must not flip back before the save lands.
function applyExternalSettings(next: Settings) {
  if (queued === 0) {
    settings.set(next);
    return;
  }
  confirmed = { ...next };
  const merged: Settings = { ...next };
  for (const patch of unsettled) Object.assign(merged, patch);
  settings.set(merged);
}

function dropUnsettled(patch: Partial<Settings>) {
  const at = unsettled.indexOf(patch);
  if (at >= 0) unsettled.splice(at, 1);
}

function enqueueSettingsUpdate(patch: Partial<Settings>, onError?: (err: unknown) => void): Promise<boolean> {
  const before = get(settings);
  if (!before) {
    toast(msg('state.settingsNotLoaded'), 'danger');
    return Promise.resolve(false);
  }
  if (queued++ === 0) confirmed = { ...before };
  const mine = ++revision;
  unsettled.push(patch);
  for (const key of Object.keys(patch)) fieldRevision.set(key, mine);
  settings.set({ ...before, ...patch });

  const operation = saving.then(async () => {
    try {
      confirmed = { ...(await saveSettingsPatch(patch)) };
      dropUnsettled(patch);
      return true;
    } catch (err) {
      console.error('save settings', err);
      // Undo this call's own keys against the latest state instead of
      // restoring the whole snapshot: another call may have saved a
      // different field successfully while this one was in flight, and
      // that value must survive the rollback.
      dropUnsettled(patch);
      const latest = get(settings);
      if (latest) {
        const reverted: Settings = { ...latest };
        const target = reverted as unknown as Record<string, unknown>;
        const source = confirmed as unknown as Record<string, unknown>;
        for (const key of Object.keys(patch)) {
          if (fieldRevision.get(key) === mine) target[key] = source[key];
        }
        settings.set(reverted);
      }
      try {
        if (onError) onError(err);
        else toast(msg('state.settingsSaveFailed'), 'danger');
      } catch (reportErr) {
        console.error('settings save error handler', reportErr);
      }
      return false;
    }
  }).catch((err) => {
    // Nothing above is expected to throw -- saveSettingsPatch is already caught --
    // but a rejected link would poison every later save in the chain.
    console.error('settings save chain', err);
    return false;
  }).finally(() => { queued--; });
  saving = operation.then(() => undefined);
  return operation;
}

/** Persist a settings patch and report whether the write succeeded. */
export function updateSettingsResult(patch: Partial<Settings>): Promise<boolean> {
  return enqueueSettingsUpdate(patch);
}

export function updateSettingsReporting(patch: Partial<Settings>, onError: (err: unknown) => void): Promise<boolean> {
  return enqueueSettingsUpdate(patch, onError);
}

/** Existing callers rely on the toast-based, fire-and-forget result. */
export async function updateSettings(patch: Partial<Settings>): Promise<void> {
  await enqueueSettingsUpdate(patch);
}

export async function createLibrary(parent: string): Promise<Settings> {
  const next = await setupLibrary(parent);
  settings.set(next);
  return next;
}
