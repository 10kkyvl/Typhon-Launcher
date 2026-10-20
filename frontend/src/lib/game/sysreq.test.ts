import { describe, expect, it } from 'vitest';
import { cacheKey, sortChecks, statusTone, verdictLabelKey, verdictTone } from './sysreq';
import type { SysreqCheck } from '../services/sysreq';

function check(kind: SysreqCheck['kind'], status: SysreqCheck['status']): SysreqCheck {
  return { kind, required: '', recommended: '', have: '', status };
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
