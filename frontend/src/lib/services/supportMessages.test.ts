import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: false }));

import { locale, translator, type Locale } from '../i18n';
import { ru } from '../i18n/catalog/ru';
import { en } from '../i18n/catalog/en';
import { goSources } from '../testing/sources';
import { AccountError, toAccountError } from './account';
import { SUPPORT_CODES, SUPPORT_CODE_FIELDS } from './supportCodes';
import { REASONS, supportErrorText } from './supportMessages';

afterEach(() => locale.set('ru'));

function goCodes(): string[] {
  const source = goSources['internal/support/model.go'];
  expect(source, 'internal/support/model.go').toBeTypeOf('string');
  return [...source.matchAll(/\bCode\w+\s*=\s*"([a-z_]+)"/g)].map((match) => match[1]);
}

describe('support error codes', () => {
  it('lists exactly the codes the Go client names', () => {
    expect([...SUPPORT_CODES].sort()).toEqual(goCodes().sort());
  });

  it('has a message for every code in both languages', () => {
    for (const code of SUPPORT_CODES) {
      const key = REASONS[code];
      expect(key, code).toBeTruthy();
      expect(String(ru[key]).trim(), `${code} ru`).not.toBe('');
      expect(String(en[key]).trim(), `${code} en`).not.toBe('');
    }
  });

  it('lets every code through the account error contract with its field', () => {
    for (const code of SUPPORT_CODES) {
      const err = toAccountError(new Error(code));
      expect(err.code).toBe(code);
      expect(err.field).toBe(SUPPORT_CODE_FIELDS[code] ?? '');
    }
  });
});

describe('supportErrorText', () => {
  it.each<Locale>(['ru', 'en'])('says something specific for each ticket code in %s', (language) => {
    locale.set(language);
    const generic = translator(language)('errSupport.generic');
    for (const code of SUPPORT_CODES) {
      expect(supportErrorText(new AccountError(code)), code).not.toBe(generic);
      expect(supportErrorText(new Error(code)), code).not.toBe(generic);
    }
  });

  it('reads the log upload codes the logs flow already knows', () => {
    expect(supportErrorText(new Error('typhon:diagnostics.log_upload_rate_limited: 429'))).toBe(
      ru['errLogs.uploadRateLimited'],
    );
  });

  it('explains a launcher without diagnostics', () => {
    expect(supportErrorText(new Error('typhon:support.logs_unavailable: not available'))).toBe(
      ru['errSupport.logsUnavailable'],
    );
  });

  it('falls back to the account messages for the shared codes', () => {
    expect(supportErrorText(new AccountError('rate_limited'))).toBe(ru['state.accountErrorRateLimited']);
    expect(supportErrorText(new AccountError('unauthenticated'))).toBe(ru['state.accountErrorUnauthenticated']);
  });

  it('uses the generic message for what it does not know', () => {
    expect(supportErrorText(new Error('boom'))).toBe(ru['errSupport.generic']);
    expect(supportErrorText(undefined)).toBe(ru['errSupport.generic']);
    expect(supportErrorText(new Error('boom'), 'custom')).toBe('custom');
  });
});
