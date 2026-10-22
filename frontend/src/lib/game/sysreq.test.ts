import { afterEach, describe, expect, it } from 'vitest';
import { locale } from '../i18n';
import { cacheKey, haveText, sortChecks, statusTone, verdictLabelKey, verdictTone } from './sysreq';
import type { SysreqCheck, SysreqHave } from '../services/sysreq';

function check(kind: SysreqCheck['kind'], status: SysreqCheck['status']): SysreqCheck {
  return { kind, required: '', recommended: '', have: { text: '', cores: 0, bits: 0, bytes: 0, free: null, directX: 0, installed: false }, status };
}

describe('verdictLabelKey', () => {
  it('maps each verdict to its label', () => {
    expect(verdictLabelKey({ verdict: 'good', reason: '' })).toBe('sysreq.verdictGood');
    expect(verdictLabelKey({ verdict: 'minimum', reason: '' })).toBe('sysreq.verdictMinimum');
    expect(verdictLabelKey({ verdict: 'not_enough', reason: '' })).toBe('sysreq.verdictNotEnough');
  });

  it('tells missing requirements from a failed comparison', () => {
    expect(verdictLabelKey({ verdict: 'unknown', reason: 'no_requirements' })).toBe('sysreq.verdictNoData');
    expect(verdictLabelKey({ verdict: 'unknown', reason: 'not_comparable' })).toBe('sysreq.verdictUnknown');
    expect(verdictLabelKey({ verdict: 'unknown', reason: '' })).toBe('sysreq.verdictUnknown');
    expect(verdictLabelKey({ verdict: 'unknown', reason: 'partial' })).toBe('sysreq.verdictPartial');
  });
});

describe('tones', () => {
  it('colors verdicts and statuses', () => {
    expect(verdictTone('good')).toBe('success');
    expect(verdictTone('minimum')).toBe('warning');
    expect(verdictTone('not_enough')).toBe('danger');
    expect(verdictTone('unknown')).toBe('neutral');
    expect(statusTone('below_minimum')).toBe('danger');
    expect(statusTone('below_recommended')).toBe('warning');
    expect(statusTone('ok')).toBe('success');
    expect(statusTone('unknown')).toBe('neutral');
  });
});

describe('sortChecks', () => {
  it('puts the worst first, then keeps the hardware order', () => {
    const sorted = sortChecks([
      check('storage', 'ok'),
      check('ram', 'below_minimum'),
      check('cpu', 'ok'),
      check('gpu', 'unknown'),
      check('os', 'below_minimum'),
    ]);
    expect(sorted.map((c) => c.kind)).toEqual(['os', 'ram', 'gpu', 'cpu', 'storage']);
  });

  it('does not mutate the input', () => {
    const input = [check('ram', 'ok'), check('os', 'below_minimum')];
    sortChecks(input);
    expect(input[0].kind).toBe('ram');
  });
});

describe('cacheKey', () => {
  it('separates games and paths', () => {
    expect(cacheKey('a', 'D:\\G')).not.toBe(cacheKey('a', 'E:\\G'));
    expect(cacheKey('a', 'b')).not.toBe(cacheKey('ab', ''));
    expect(cacheKey('a', 'b', '1')).not.toBe(cacheKey('a', 'b', '2'));
  });
});

function withHave(kind: SysreqCheck['kind'], have: Partial<SysreqHave>): SysreqCheck {
  return { ...check(kind, 'ok'), have: { ...check(kind, 'ok').have, ...have } };
}

describe('haveText', () => {
  afterEach(() => locale.set('ru'));

  it('words the machine side in the interface language', () => {
    locale.set('ru');
    expect(haveText(withHave('cpu', { text: 'Ryzen 5 3600', cores: 6 }))).toBe('Ryzen 5 3600, 6 ядер');
    expect(haveText(withHave('cpu', { text: 'Celeron', cores: 2 }))).toBe('Celeron, 2 ядра');
    expect(haveText(withHave('os', { text: 'Windows 10 Pro', bits: 64 }))).toBe('Windows 10 Pro, 64-бит');
    expect(haveText(withHave('storage', { free: 7.6 * 1024 ** 3 }))).toMatch(/^свободно 7[.,]6 ГБ$/);
    expect(haveText(withHave('ram', { bytes: 24 * 1024 ** 3 }))).toMatch(/ГБ$/);
    locale.set('en');
    expect(haveText(withHave('cpu', { text: 'Ryzen 5 3600', cores: 6 }))).toBe('Ryzen 5 3600, 6 cores');
    expect(haveText(withHave('storage', { free: 7.6 * 1024 ** 3 }))).toMatch(/^7[.,]6 GB free$/);
  });

  it('leaves unknown values empty, but keeps a full disk', () => {
    expect(haveText(withHave('ram', {}))).toBe('');
    expect(haveText(withHave('storage', {}))).toBe('');
    expect(haveText(withHave('directx', {}))).toBe('');
    expect(haveText(withHave('cpu', { text: 'X' }))).toBe('X');
    expect(haveText(withHave('storage', { free: 0 }))).not.toBe('');
  });

  it('says an installed game already sits on the disk instead of quoting free space', () => {
    locale.set('ru');
    expect(haveText(withHave('storage', { installed: true, free: 0 }))).toBe('игра уже установлена');
    locale.set('en');
    expect(haveText(withHave('storage', { installed: true, free: null }))).toBe('the game is already installed');
  });
});
