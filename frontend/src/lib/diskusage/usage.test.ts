import { describe, expect, it } from 'vitest';
import { msg } from '../i18n';
import { en } from '../i18n/catalog/en';
import { ru } from '../i18n/catalog/ru';
import { goSources } from '../testing/sources';
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
  summarySegments,
  totalText,
  warningText,
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
  it('leaves a never played game to the played column', () => {
    expect(isStale(null, NOW)).toBe(false);
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

// The Go constants of a file: name → value, for `Name = "value"` lines.
function goConstants(path: string): Map<string, string> {
  const source = goSources[path];
  expect(source, path).toBeTypeOf('string');
  return new Map([...source.matchAll(/^\t(\w+)\s*=\s*"([^"]*)"/gm)].map((m) => [m[1], m[2]]));
}

describe('leftoverKindKey', () => {
  it('names every kind the backend sends, each in its own words', () => {
    const kinds = [...goConstants('internal/diskusage/model.go')].filter(([name]) => name.startsWith('Kind')).map(([, value]) => value);
    expect(kinds).toContain('removal');
    expect(kinds).toContain('relocate_copy_source');
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
      screenshots: [{ gameId: 'a', title: 'a', titleUnknown: false, inLibrary: true, count: 1, ...one }],
      snapshots: [{ gameId: 'b', title: 'b', titleUnknown: false, inLibrary: true, count: 1, ...known }],
    };
    expect(countUnknown(report)).toBe(2);
  });
});

describe('reason codes', () => {
  it('are the set Go gives, read from internal/diskusage/codes.go and the diskreason package', () => {
    const named = new Map([...goConstants('internal/diskreason/reason.go'), ...goConstants('internal/diskusage/codes.go')]);
    const body = /var reasonCodes = \[\]string\{([\s\S]*?)\n\}/.exec(goSources['internal/diskusage/codes.go'])?.[1] ?? '';
    const fromGo = [...body.matchAll(/(?:diskreason\.)?(\w+),/g)].map((m) => named.get(m[1]));
    expect(fromGo.length).toBeGreaterThan(20);
    expect(fromGo.every((code) => typeof code === 'string')).toBe(true);
    expect([...REASON_CODES].sort()).toEqual([...(fromGo as string[])].sort());
  });

  it('have ru and en text for every code in the closed set', () => {
    for (const code of REASON_CODES) {
      expect(Object.hasOwn(ru, code), code).toBe(true);
      expect(Object.hasOwn(en, code), code).toBe(true);
    }
  });

  it('translates by code and says a code it does not know in its own plain words, never in Go’s', () => {
    expect(reasonLabel('diskusage.reason.seeding')).toBe('Раздаётся, при удалении остановится');
    expect(reasonLabel('diskusage.reason.folder_held')).toBe(ru['diskusage.reason.folder_held']);
    expect(reasonLabel('diskusage.reason.new_one')).toBe(msg('diskusage.cannotDelete'));
    expect(reasonLabel('')).toBe(msg('diskusage.cannotDelete'));
    expect(reasonLabel('diskusage.title')).toBe(msg('diskusage.cannotDelete'));
  });
});

describe('summarySegments', () => {
  const totals = { gamesBytes: 100, downloadsBytes: 0, leftoversBytes: 30, screenshotsBytes: 0, snapshotsBytes: 0, totalBytes: 130, reclaimableBytes: 20, unknownSizes: false };
  const left = (bytes: number, deletable: boolean, sizeUnknown = false) => ({ path: `p${bytes}${deletable}`, kind: 'install_partial', sizeBytes: bytes, sizeUnknown, deletable, reason: '', reasonCode: '' });
  const base = { games: [game({ sizeBytes: 100 })], downloads: [], screenshots: [], snapshots: [], warnings: [], scannedAt: '' };

  it('splits Go’s leftovers figure into what can be deleted and the rest, and leaves out empty kinds', () => {
    const segments = summarySegments({ ...base, leftovers: [left(20, true), left(25, false)], totals });
    expect(segments.map((s) => [s.key, s.bytes, s.tone])).toEqual([
      ['games', 100, 'game'],
      ['leftovers-reclaim', 20, 'reclaim'],
      ['leftovers-kept', 10, 'more'],
    ]);
  });

  it('never draws more deletable leftovers than Go counted, and says an unmeasured row as at least', () => {
    const segments = summarySegments({ ...base, leftovers: [left(40, true, true)], totals });
    expect(segments.find((s) => s.key === 'leftovers-reclaim')).toMatchObject({ bytes: 30, sizeUnknown: true });
    expect(segments.find((s) => s.key === 'leftovers-kept')).toBeUndefined();
  });
});

describe('warning codes', () => {
  const codes = [...goConstants('internal/diskusage/codes.go')].filter(([name]) => name.startsWith('Warn')).map(([, value]) => value);

  it('word every code Go gives, in both languages', () => {
    expect(codes.length).toBe(8);
    for (const code of codes) {
      expect(Object.hasOwn(ru, code), code).toBe(true);
      expect(Object.hasOwn(en, code), code).toBe(true);
      expect(warningText({ area: 'x', code, message: 'go text' })).toBe(ru[code as keyof typeof ru]);
    }
  });

  it('says a code it does not know in plain words, never Go’s message or area', () => {
    for (const code of ['diskusage.warning.new_one', '', 'diskusage.title']) {
      expect(warningText({ area: 'downloads', code, message: 'permission denied' }), code).toBe(msg('diskusage.warningUnknown'));
    }
  });
});
