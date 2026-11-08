import { describe, expect, it } from 'vitest';
import { formatCountdown, formatLinkCode, secondsLeft } from './format';

describe('discord format', () => {
  it('splits an 8-character code into XXXX-XXXX', () => {
    expect(formatLinkCode('abcd2345')).toBe('ABCD-2345');
    expect(formatLinkCode('ABCD-2345')).toBe('ABCD-2345');
  });

  it('leaves unexpected lengths unsplit', () => {
    expect(formatLinkCode('ABC')).toBe('ABC');
  });

  it('counts down in whole seconds and never goes negative', () => {
    expect(secondsLeft(10_500, 10_000)).toBe(1);
    expect(secondsLeft(10_000, 20_000)).toBe(0);
    expect(formatCountdown(599)).toBe('09:59');
    expect(formatCountdown(-3)).toBe('00:00');
  });
});
