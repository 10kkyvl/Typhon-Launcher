import { describe, expect, it } from 'vitest';
import { en } from '../i18n/catalog/en';
import { ru } from '../i18n/catalog/ru';
import type { DownloadUsage, GameUsage } from '../services/diskusage';
import {
  ageOf,
  ageText,
  canManageGame,
  countUnknown,
  downloadBlocked,
  gameBlockedKey,
  isStale,
  leftoverKindKey,
  REASON_CODES,
  reasonLabel,
  sizeSorted,
  sizeText,
  sortGames,
  totalText,
} from './usage';

const NOW = Date.parse('2026-10-20T12:00:00Z');
const DAY = 86400000;

function ago(days: number) {
  return new Date(NOW - days * DAY).toISOString();
}

function game(patch: Partial<GameUsage>): GameUsage {
  return {
    id: 'g',
    title: 'Game',
    installDir: 'C:\Games\Game',
    sizeBytes: 100,
    sizeUnknown: false,
    missing: false,
    external: false,
    linked: false,
    lastPlayed: null,
    installedAt: '',
    running: false,
    busy: false,
    ...patch,
  };
}

function download(patch: Partial<DownloadUsage>): DownloadUsage {
  return {
    id: 'd',
    name: 'Download',
    root: 'C:\Downloads\Download',
    sizeBytes: 100,
    sizeUnknown: false,
    missing: false,
    seeding: false,
    installed: false,
    gameId: '',
    referenced: false,
    reason: '',
    reasonCode: '',
    ...patch,
  };
}

describe('downloadBlocked', () => {
  it('does not block a download only because it is seeding', () => {
    expect(downloadBlocked(download({ seeding: true }))).toBe(false);
  });

  it('blocks a referenced download, seeding or not', () => {
    expect(downloadBlocked(download({ referenced: true }))).toBe(true);
    expect(downloadBlocked(download({ referenced: true, seeding: true }))).toBe(true);
  });

  it('leaves a plain download deletable', () => {
    expect(downloadBlocked(download({}))).toBe(false);
  });
});

describe('ageOf', () => {
  it('treats a missing or broken date as never played', () => {
    expect(ageOf(null, NOW)).toEqual({ kind: 'never' });
    expect(ageOf('not a date', NOW)).toEqual({ kind: 'never' });
  });

  it('buckets days, months and years', () => {
    expect(ageOf(ago(0), NOW)).toEqual({ kind: 'days', count: 0 });
    expect(ageOf(ago(29), NOW)).toEqual({ kind: 'days', count: 29 });
    expect(ageOf(ago(95), NOW)).toEqual({ kind: 'months', count: 3 });
    expect(ageOf(ago(400), NOW)).toEqual({ kind: 'years', count: 1 });
  });

  it('never goes negative for a date in the future', () => {
    expect(ageOf(ago(-3), NOW)).toEqual({ kind: 'days', count: 0 });
  });
});

describe('ageText', () => {
  it('says never played in words', () => {
    expect(ageText({ kind: 'never' })).toBe('Не запускалась');
  });

  it('spells out months', () => {
    expect(ageText({ kind: 'months', count: 3 })).toBe('3 месяца назад');
  });
});

describe('isStale', () => {
  it('flags a game that was never played', () => {
    expect(isStale(null, NOW)).toBe(true);
  });

  it('flags a game after more than 90 days only', () => {
    expect(isStale(ago(90), NOW)).toBe(false);
    expect(isStale(ago(91), NOW)).toBe(true);
    expect(isStale(ago(3), NOW)).toBe(false);
  });
});

describe('sortGames', () => {
  const small = game({ id: 'small', title: 'B', sizeBytes: 10, lastPlayed: ago(1) });
  const big = game({ id: 'big', title: 'A', sizeBytes: 500, lastPlayed: ago(200) });
  const unknown = game({ id: 'unknown', title: 'C', sizeBytes: 0, sizeUnknown: true, lastPlayed: null });

  it('puts the biggest first and unknown sizes last', () => {
    expect(sortGames([unknown, small, big], 'size').map((g) => g.id)).toEqual(['big', 'small', 'unknown']);
  });

  it('puts never played first, then the longest unplayed', () => {
    expect(sortGames([small, big, unknown], 'played').map((g) => g.id)).toEqual(['unknown', 'big', 'small']);
  });

  it('does not change the input order', () => {
    const input = [small, big];
    sortGames(input, 'size');
    expect(input.map((g) => g.id)).toEqual(['small', 'big']);
  });
});

describe('game actions', () => {
  it('blocks a running or busy game with a reason', () => {
    expect(gameBlockedKey(game({ running: true }))).toBe('diskusage.blockedRunning');
    expect(gameBlockedKey(game({ busy: true }))).toBe('diskusage.blockedBusy');
    expect(gameBlockedKey(game({}))).toBeNull();
  });

  it('offers no actions for external games', () => {
    expect(canManageGame(game({ external: true }))).toBe(false);
    expect(canManageGame(game({ busy: true }))).toBe(false);
    expect(canManageGame(game({}))).toBe(true);
  });
});

describe('leftoverKindKey', () => {
  it('names every kind the backend sends', () => {
    const kinds = [
      'install_partial',
      'download_part',
      'relocate_staging',
      'update_staging',
      'update_previous',
      'update_replaced',
      'download_orphan',
    ];
    const keys = kinds.map(leftoverKindKey);
    expect(new Set(keys).size).toBe(kinds.length);
    expect(keys).not.toContain('diskusage.kindUnknown');
  });

  it('falls back for a kind it does not know', () => {
    expect(leftoverKindKey('something_new')).toBe('diskusage.kindUnknown');
  });
});

describe('size text', () => {
  const fmt = (n: number) => `${n} B`;

  it('never shows zero for an unknown size', () => {
    expect(sizeText(0, true, fmt, '?')).toBe('?');
  });

  it('marks a partial measurement as a lower bound', () => {
    expect(sizeText(50, true, fmt, '?')).toBe('≥ 50 B');
  });

  it('shows a known size as is, including zero', () => {
    expect(sizeText(0, false, fmt, '?')).toBe('0 B');
    expect(sizeText(7, false, fmt, '?')).toBe('7 B');
  });

  it('wraps a total with unknown parts as at least', () => {
    const atLeast = (size: string) => `>= ${size}`;
    expect(totalText(10, true, fmt, atLeast)).toBe('>= 10 B');
    expect(totalText(10, false, fmt, atLeast)).toBe('10 B');
  });
});

describe('sizeSorted', () => {
  it('sorts by size with unknown last', () => {
    const items = [
      { id: 'a', sizeBytes: 1, sizeUnknown: false },
      { id: 'u', sizeBytes: 0, sizeUnknown: true },
      { id: 'b', sizeBytes: 9, sizeUnknown: false },
    ];
    expect(sizeSorted(items).map((i) => i.id)).toEqual(['b', 'a', 'u']);
  });
});

describe('countUnknown', () => {
  it('counts every item whose size is unknown', () => {
    const one = { sizeBytes: 0, sizeUnknown: true };
    const known = { sizeBytes: 1, sizeUnknown: false };
    const report = {
      games: [game({ sizeUnknown: true }), game({})],
      downloads: [],
      leftovers: [],
      screenshots: [{ gameId: 'a', title: 'a', inLibrary: true, count: 1, ...one }],
      snapshots: [{ gameId: 'b', title: 'b', inLibrary: true, count: 1, ...known }],
    };
    expect(countUnknown(report)).toBe(2);
  });
});

describe('reason codes', () => {
  it('have ru and en text for every code in the closed set', () => {
    for (const code of REASON_CODES) {
      const key = `diskusage.reason.${code}`;
      expect(Object.hasOwn(ru, key), key).toBe(true);
      expect(Object.hasOwn(en, key), key).toBe(true);
    }
  });

  it('translates by code and falls back to the backend text for an unknown one', () => {
    expect(reasonLabel('diskusage.reason.seeding', 'x')).toBe('Раздаётся, при удалении остановится');
    expect(reasonLabel('diskusage.reason.new_one', 'text from go')).toBe('text from go');
    expect(reasonLabel('', 'text from go')).toBe('text from go');
  });
});
