import { get, writable } from 'svelte/store';
import type { Locale } from './types';

export const LANGUAGE_SYSTEM = 'system';

export function resolveLocale(setting: string, systemLanguage: string | undefined): Locale {
  if (setting === 'ru' || setting === 'en') return setting;
  return systemLanguage?.toLowerCase().startsWith('ru') ? 'ru' : 'en';
}

function systemLanguage() {
  if (typeof navigator === 'undefined') return undefined;
  return navigator.language;
}

export const locale = writable<Locale>(resolveLocale(LANGUAGE_SYSTEM, systemLanguage()));

locale.subscribe((value) => {
  if (typeof document !== 'undefined') document.documentElement.lang = value;
});

export function applyLanguage(setting: string) {
  const next = resolveLocale(setting, systemLanguage());
  if (get(locale) !== next) locale.set(next);
}
