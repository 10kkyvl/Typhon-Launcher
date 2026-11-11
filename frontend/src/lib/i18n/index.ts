import { derived, get } from 'svelte/store';
import { locale } from './locale';
import { translate } from './translate';
import { ru, type MessageKey } from './catalog/ru';
import { en } from './catalog/en';
import type { Locale, Params } from './types';

const catalogs = { ru, en };

export type { Locale, MessageKey, Params };
export { locale, applyLanguage, resolveLocale, LANGUAGE_SYSTEM } from './locale';
import { errorCode, hasMessage } from './errors';
export { errorCode, hasMessage };

export function msg(key: MessageKey, params?: Params) {
  return translate(catalogs, get(locale), key, params);
}

/** The text of a backend error's code (`typhon:<code>`) when the catalog has one, else `fallback`; never the raw code. */
export function codeText(err: unknown, fallback: string = msg('common.error')): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

export function translator($locale: Locale) {
  return (key: MessageKey, params?: Params) => translate(catalogs, $locale, key, params);
}

export type Translate = ReturnType<typeof translator>;

export const t = derived(locale, translator);
