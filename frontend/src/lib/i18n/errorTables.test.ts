import { afterEach, describe, expect, it } from 'vitest';
import { locale, translator, type Locale, type MessageKey } from './index';
import { ru } from './catalog/ru';
import { en } from './catalog/en';
import { installErrorText, REASONS as installReasons } from '../install/installErrors';
import { metadataErrorText, REASONS as metadataReasons } from '../metadata/metadataErrors';
import { sourceErrorText, REASONS as sourceReasons } from '../sources/sourceErrors';
import { themeErrorText, REASONS as themeReasons } from '../theme/themeErrors';
import { moveErrorText, REASONS as moveReasons } from '../relocate/moveMessages';
import { logsUploadErrorText, REASONS as logsReasons } from '../services/logsUploadErrors';
import { updateErrorText } from '../updates/updateErrors';
import { consentErrorText } from '../services/consentErrors';
import { updateReason } from '../services/selfupdateMessages';
import { reviewToastMessage } from '../game/reviewMessages';
import { rejectedSummary } from '../lan/lanText';
import { updateReasonKey } from '../components/updateReason';
import { frontendSources } from '../testing/sources';
import type { Stats } from '../services/lan';
import type { Message } from './types';

const LANGUAGES: Locale[] = ['ru', 'en'];
const CODE = /^[a-z0-9_]+(\.[a-z0-9_]+)+$/;
const FALLBACK = 'fallback-sentinel';

afterEach(() => locale.set('ru'));

function texts(message: Message): string[] {
  return typeof message === 'string' ? [message] : (Object.values(message) as string[]);
}

function inCatalog(catalog: Record<string, Message>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(catalog, key);
}

function pairs(path: string): Array<[string, MessageKey]> {
  const source = frontendSources[path];
  expect(source, `source ${path}`).toBeTypeOf('string');
  const found: Array<[string, MessageKey]> = [];
  const pattern = /(?:'([^'\n]+)'|"([^"\n]+)"|\b(\w+))\s*:\s*'([a-z][A-Za-z0-9]*(?:\.[A-Za-z0-9_]+)+)'/g;
  for (const match of source.matchAll(pattern)) {
    found.push([match[1] ?? match[2] ?? match[3], match[4] as MessageKey]);
  }
  return found;
}

function coded(code: string): Error {
  return new Error(`typhon:${code}: backend detail`);
}

const tables = {
  install: { reasons: installReasons, text: installErrorText },
  metadata: { reasons: metadataReasons, text: metadataErrorText },
  sources: { reasons: sourceReasons, text: sourceErrorText },
  theme: { reasons: themeReasons, text: themeErrorText },
  move: { reasons: moveReasons, text: moveErrorText },
  logs: { reasons: logsReasons, text: logsUploadErrorText },
} as const;

describe.each(Object.entries(tables))('%s error codes', (_name, { reasons, text }) => {
  const entries = Object.entries(reasons);

  it('is not empty', () => {
    expect(entries.length).toBeGreaterThan(0);
  });

  it('uses only codes the error parser can read back', () => {
    expect(entries.map(([code]) => code).filter((code) => !CODE.test(code))).toEqual([]);
  });

  it('points every code at a message that both languages define', () => {
    const missing = entries.filter(
      ([, key]) => !inCatalog(ru, key) || !inCatalog(en, key),
    );
    expect(missing).toEqual([]);
  });

  it('points every code at a message with text in both languages', () => {
    const blank = entries.filter(([, key]) =>
      [ru, en].some((catalog) => texts(catalog[key]).some((value) => value.trim() === '')),
    );
    expect(blank).toEqual([]);
  });

  it.each(LANGUAGES)('answers every code with its own message in %s', (language) => {
    locale.set(language);
    const t = translator(language);
    const wrong = entries.filter(([code, key]) => text(coded(code), FALLBACK) !== t(key));
    expect(wrong.map(([code]) => code)).toEqual([]);
  });

  it('falls back for a code it has no message for', () => {
    expect(text(coded('nothing.registered_here'), FALLBACK)).toBe(FALLBACK);
    expect(text(new Error('no code at all'), FALLBACK)).toBe(FALLBACK);
    expect(text(undefined, FALLBACK)).toBe(FALLBACK);
  });
});

describe('codes shared between tables', () => {
  it('does not give one code two meanings across the tables', () => {
    const owner = new Map<string, string>();
    const clashes: string[] = [];
    for (const [name, { reasons }] of Object.entries(tables)) {
      for (const code of Object.keys(reasons)) {
        const previous = owner.get(code);
        if (previous && previous !== name) clashes.push(`${code}: ${previous} and ${name}`);
        else owner.set(code, name);
      }
    }
    expect(clashes).toEqual([]);
  });
});

describe('private code tables', () => {
  const files = [
    'lib/updates/updateErrors.ts',
    'lib/services/consentErrors.ts',
    'lib/services/selfupdateMessages.ts',
    'lib/game/reviewMessages.ts',
    'lib/lan/lanText.ts',
    'lib/components/updateReason.ts',
    'lib/stores/history.ts',
    'lib/game/reviewEligibility.ts',
  ];

  it.each(files)('%s keeps its table readable by this guard', (path) => {
    expect(pairs(path).length).toBeGreaterThan(0);
  });

  it.each(files)('%s points only at messages that both languages define', (path) => {
    const missing = pairs(path).filter(([, key]) => !inCatalog(ru, key) || !inCatalog(en, key));
    expect(missing).toEqual([]);
  });

  it.each(LANGUAGES)('resolves every update error code to its message in %s', (language) => {
    locale.set(language);
    const t = translator(language);
    const table = pairs('lib/updates/updateErrors.ts').filter(([code]) => CODE.test(code));
    expect(table.length).toBeGreaterThan(10);
    const wrong = table.filter(([code, key]) => updateErrorText(coded(code), FALLBACK) !== t(key));
    expect(wrong.map(([code]) => code)).toEqual([]);
  });

  it.each(LANGUAGES)('resolves the telemetry consent code to its message in %s', (language) => {
    locale.set(language);
    const t = translator(language);
    const table = pairs('lib/services/consentErrors.ts');
    expect(table.length).toBeGreaterThan(0);
    for (const [code, key] of table) {
      expect(consentErrorText(coded(code))).toBe(t(key));
    }
    expect(consentErrorText(coded('nothing.registered_here'))).toBe(t('modals.telemetryConsentSaveFallback'));
  });

  it.each(LANGUAGES)('resolves every launcher update code to its reason in %s', (language) => {
    locale.set(language);
    const t = translator(language);
    const table = pairs('lib/services/selfupdateMessages.ts').filter(([code]) => CODE.test(code));
    expect(table.length).toBeGreaterThan(30);
    const wrong = table.filter(([code, key]) => updateReason(coded(code)) !== t(key));
    expect(wrong.map(([code]) => code)).toEqual([]);
  });

  it.each(LANGUAGES)('resolves every review toast code to its message in %s', (language) => {
    locale.set(language);
    const t = translator(language);
    const table = pairs('lib/game/reviewMessages.ts').filter(([code]) => /^[a-z_]+$/.test(code));
    expect(table.length).toBeGreaterThan(5);
    const wrong = table.filter(([code, key]) => reviewToastMessage(code, FALLBACK) !== t(key));
    expect(wrong.map(([code]) => code)).toEqual([]);
    expect(reviewToastMessage('nothing_registered_here', FALLBACK)).toBe(FALLBACK);
  });

  it.each(LANGUAGES)('names every rejected LAN offer reason in %s', (language) => {
    locale.set(language);
    const t = translator(language);
    const table = pairs('lib/lan/lanText.ts').filter(([, key]) => key.startsWith('transfers.lanReject'));
    expect(table.length).toBeGreaterThan(10);
    for (const [code, key] of table) {
      const stats: Stats = {
        announcesSent: 0,
        announcesReceived: 0,
        rejected: { [code]: 2 },
        peersKnown: 0,
        offersKnown: 0,
        sharesActive: 0,
      };
      expect(rejectedSummary(stats)).toBe(`${t(key)}: 2`);
    }
  });

  it('maps every incompatibility reason to the message key written beside it', () => {
    const table = pairs('lib/components/updateReason.ts');
    expect(table.length).toBeGreaterThan(5);
    for (const [reason, key] of table) {
      expect(updateReasonKey(reason), reason).toBe(key);
    }
  });
});
