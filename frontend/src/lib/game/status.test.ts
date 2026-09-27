import { describe, expect, it } from 'vitest';
import { GAME_STATUSES, STATUS_LABELS, friendStatusLabel, statusBadgeKind, statusLabel } from './status';

describe('statusLabel', () => {
  it('labels every known status', () => {
    for (const status of GAME_STATUSES) {
      expect(statusLabel(status)).toBe(STATUS_LABELS[status]);
    }
  });

  it('falls back to "Без статуса" for empty or missing status', () => {
    expect(statusLabel('')).toBe('Без статуса');
    expect(statusLabel(undefined)).toBe('Без статуса');
  });
});

describe('friendStatusLabel', () => {
  it('reads the playing status in the third person', () => {
    expect(friendStatusLabel('playing')).toBe('Проходит');
    expect(friendStatusLabel('playing')).not.toBe(statusLabel('playing'));
  });

  it('keeps the other labels', () => {
    for (const status of GAME_STATUSES) {
      if (status === 'playing') continue;
      expect(friendStatusLabel(status)).toBe(STATUS_LABELS[status]);
    }
  });

  it('stays empty without a status', () => {
    expect(friendStatusLabel('')).toBe('');
    expect(friendStatusLabel(undefined)).toBe('');
    expect(friendStatusLabel('nonsense')).toBe('');
  });
});

describe('statusBadgeKind', () => {
  it('maps completed to success', () => {
    expect(statusBadgeKind('completed')).toBe('success');
  });

  it('maps playing to accent', () => {
    expect(statusBadgeKind('playing')).toBe('accent');
  });

  it('maps paused and backlog to neutral', () => {
    expect(statusBadgeKind('paused')).toBe('neutral');
    expect(statusBadgeKind('backlog')).toBe('neutral');
  });

  it('maps dropped to warning', () => {
    expect(statusBadgeKind('dropped')).toBe('warning');
  });
});
