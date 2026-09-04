export type Locale = 'ru' | 'en';

export type PluralForms = Partial<Record<Intl.LDMLPluralRule, string>>;

export type Message = string | PluralForms;

export type Params = Record<string, string | number>;

export type Catalog<K extends string> = Record<K, Message>;
