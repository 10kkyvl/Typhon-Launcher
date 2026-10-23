import { describe, expect, it } from 'vitest';
import { completionDateIssue, dateFromInput, dateInputValue, dayValue } from './completion';

const now = new Date(2026, 9, 23, 9, 30);

describe('dateInputValue', () => {
  it('uses the saved completion day', () => {
    expect(dateInputValue(new Date(2026, 9, 12, 12).toISOString(), now)).toBe('2026-10-12');
  });

  it('falls back to today without a date or with a broken one', () => {
    expect(dateInputValue(null, now)).toBe('2026-10-23');
    expect(dateInputValue(undefined, now)).toBe('2026-10-23');
    expect(dateInputValue('nonsense', now)).toBe('2026-10-23');
  });
});

describe('dateFromInput', () => {
  it('turns a day into local noon of that day', () => {
    const date = dateFromInput('2026-10-12');
    expect(date).not.toBeNull();
    expect(dayValue(date as Date)).toBe('2026-10-12');
    expect((date as Date).getHours()).toBe(12);
  });

  it.each(['', '2026-13-01', '2026-02-30', '12.10.2026', '2026-1-1'])('rejects %j', (value) => {
    expect(dateFromInput(value)).toBeNull();
  });
});

describe('completionDateIssue', () => {
  it('accepts today and earlier days from 1980', () => {
    expect(completionDateIssue('2026-10-23', now)).toBe(false);
    expect(completionDateIssue('1980-01-01', now)).toBe(false);
  });

  it('rejects the future, days before 1980 and empty input', () => {
    expect(completionDateIssue('2026-10-24', now)).toBe(true);
    expect(completionDateIssue('1979-12-31', now)).toBe(true);
    expect(completionDateIssue('', now)).toBe(true);
  });
});
