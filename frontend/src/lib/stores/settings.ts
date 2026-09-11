import { applyPersonalAccent } from '../theme/apply';
import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getSettings, saveSettings, setupLibrary, type Settings } from '../services/settings';
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
      settings.set(event.data as Settings);
    });
  }
}

// Saves run one at a time. saveSettings writes the whole object, so two of
// them in flight together race over every field, not just the one the user
// clicked, and Go stores whichever request happens to arrive last.
let saving: Promise<void> = Promise.resolve();
let revision = 0;
let queued = 0;
let confirmed: Settings | null = null;
const fieldRevision = new Map<string, number>();

export async function updateSettings(patch: Partial<Settings>) {
  const before = get(settings);
  if (!before) {
    toast(msg('state.settingsNotLoaded'), 'danger');
    return;
  }
  if (queued++ === 0) confirmed = { ...before };
  const mine = ++revision;
  for (const key of Object.keys(patch)) fieldRevision.set(key, mine);
  settings.set({ ...before, ...patch });
  saving = saving.then(async () => {
    const next = get(settings);
    if (!next) return;
    try {
      await saveSettings(next);
      confirmed = { ...next };
    } catch (err) {
      console.error('save settings', err);
      toast(msg('state.settingsSaveFailed') + ': ' + String(err instanceof Error ? err.message : err), 'danger');
      // Undo this call's own keys against the latest state instead of
      // restoring the whole snapshot: another call may have saved a
      // different field successfully while this one was in flight, and
      // that value must survive the rollback.
      const latest = get(settings);
      if (!latest) return;
      const reverted: Settings = { ...latest };
      const target = reverted as unknown as Record<string, unknown>;
      const source = confirmed as unknown as Record<string, unknown>;
      for (const key of Object.keys(patch)) {
        if (fieldRevision.get(key) === mine) target[key] = source[key];
      }
      settings.set(reverted);
    }
  }).catch((err) => {
    // Nothing above is expected to throw -- saveSettings is already caught --
    // but a rejected link would poison every later save in the chain.
    console.error('settings save chain', err);
  }).finally(() => { queued--; });
  return saving;
}

export async function createLibrary(parent: string): Promise<Settings> {
  const next = await setupLibrary(parent);
  settings.set(next);
  return next;
}
