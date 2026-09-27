import type { Catalog, Locale, Message, Params, PluralForms } from './types';

const FALLBACK: Locale = 'ru';

const pluralRules = new Map<Locale, Intl.PluralRules>();
const numberFormats = new Map<Locale, Intl.NumberFormat>();

function rules(locale: Locale) {
  let cached = pluralRules.get(locale);
  if (!cached) {
    cached = new Intl.PluralRules(locale);
    pluralRules.set(locale, cached);
  }
  return cached;
}

function numbers(locale: Locale) {
  let cached = numberFormats.get(locale);
  if (!cached) {
    cached = new Intl.NumberFormat(locale);
    numberFormats.set(locale, cached);
  }
  return cached;
}

const PLURAL_ORDER: Intl.LDMLPluralRule[] = ['other', 'many', 'few', 'one', 'two', 'zero'];

function selectForm(forms: PluralForms, locale: Locale, count: number): string | undefined {
  const exact = forms[rules(locale).select(count)];
  if (exact !== undefined) return exact;
  for (const form of PLURAL_ORDER) {
    const value = forms[form];
    if (value !== undefined) return value;
  }
  return undefined;
}

function resolve(message: Message | undefined, locale: Locale, params?: Params): string | undefined {
  if (message === undefined) return undefined;
  if (typeof message === 'string') return message;
  const count = params?.count;
  if (typeof count !== 'number') return selectForm(message, locale, 0);
  return selectForm(message, locale, count);
}

function interpolate(text: string, locale: Locale, params?: Params) {
  if (!params) return text;
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => {
    const value = params[name];
    if (value === undefined) return whole;
    return typeof value === 'number' ? numbers(locale).format(value) : value;
  });
}

export function translate<K extends string>(
  catalogs: Record<Locale, Partial<Catalog<K>>>,
  locale: Locale,
  key: K,
  params?: Params,
): string {
  const text =
    resolve(catalogs[locale]?.[key], locale, params) ??
    resolve(catalogs[FALLBACK]?.[key], FALLBACK, params);
  if (text === undefined) return key;
  return interpolate(text, locale, params);
}
