import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getSettings, saveSettings, setupLibrary, type Settings } from '../services/settings';
import { toast } from './toasts';
import { applyLanguage } from '../i18n';

export const settings = writable<Settings | null>(null);

settings.subscribe((value) => {
  if (!value) return;
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

export async function updateSettings(patch: Partial<Settings>) {
  const current = get(settings);
  if (!current) {
    toast('Настройки ещё не загружены', 'danger');
    return;
  }
  const next = { ...current, ...patch };
  settings.set(next);
  try {
    await saveSettings(next);
  } catch (err) {
    console.error('save settings', err);
    toast('Не удалось сохранить настройки', 'danger');
    settings.set(current);
  }
}

export async function createLibrary(parent: string): Promise<Settings> {
  const next = await setupLibrary(parent);
  settings.set(next);
  return next;
}
