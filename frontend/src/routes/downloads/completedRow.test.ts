import { describe, expect, it, vi } from 'vitest';

vi.mock('../../lib/services/backend', () => ({ inWails: false }));

import { translator } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import type { InstallStatus } from '../../lib/services/install';
import type { LibraryGame } from '../../lib/services/library';
import { makeFormat } from '../../lib/utils/format';
import { canClear, canRemove, completedKind, completedPart, completedWhen, installedGame, removeAsks, type CompletedKind } from './completedRow';

const release: Pick<Download, 'origin'> = { origin: { gameId: 'g1' } };

describe('what became of a completed download', () => {
  it.each<[string, Pick<Download, 'origin'>, InstallStatus | undefined, CompletedKind]>([
    ['a download no installation took yet', release, undefined, 'ready'],
    ['a download of no known game', { origin: {} }, undefined, 'ready'],
    ['an update, which the updates apply, not an install', { origin: { purpose: 'update' } }, undefined, 'none'],
    ['a repair, which the updates apply', { origin: { purpose: 'repair' } }, undefined, 'none'],
    ['waiting for its turn', release, 'pending', 'installing'],
    ['preparing', release, 'preparing', 'installing'],
    ['installing', release, 'installing', 'installing'],
    ['extracting', release, 'extracting', 'installing'],
    ['verifying', release, 'verifying', 'installing'],
    ['the installer waiting for the player', release, 'waiting_for_user', 'waiting'],
    ['a failed installation', release, 'failed', 'failed'],
    ['a cancelled installation', release, 'cancelled', 'stopped'],
    ['an interrupted installation', release, 'interrupted', 'stopped'],
    ['an installed game', release, 'completed', 'installed'],
    ['an update with an installation of its own', { origin: { purpose: 'update' } }, 'completed', 'installed'],
  ])('%s', (_, download, status, kind) => {
    expect(completedKind(download, status ? { status } : undefined)).toBe(kind);
  });

  it('puts what is installing or waits for the player under "Installing", the rest under "Completed"', () => {
    const parts = (['installing', 'waiting', 'failed', 'stopped', 'installed', 'ready', 'none'] as const).map(completedPart);
    expect(parts).toEqual(['installing', 'installing', 'completed', 'completed', 'completed', 'completed', 'completed']);
  });

  it.each<[CompletedKind, boolean, boolean]>([
    ['installed', false, true],
    ['none', false, true],
    ['installed', true, false],
    ['none', true, false],
    ['ready', false, false],
    ['failed', false, false],
    ['stopped', false, false],
    ['installing', false, false],
    ['waiting', false, false],
  ])('Clear the list takes %s (seeding: %s): %s', (kind, seeding, clears) => {
    expect(canClear({ seeding }, kind)).toBe(clears);
  });

  it.each<[CompletedKind, boolean, boolean]>([
    ['installing', false, false],
    ['waiting', false, false],
    ['ready', true, true],
    ['failed', true, true],
    ['stopped', true, true],
    ['installed', true, false],
    ['none', true, false],
  ])('Remove from list on %s: offered %s, asks first %s', (kind, offered, asks) => {
    expect(canRemove(kind)).toBe(offered);
    expect(removeAsks(kind)).toBe(asks);
  });
});

describe('when a download finished, in words', () => {
  const en = makeFormat('en', translator('en'));
  const t = translator('en');
  // Half past midnight on 28 October 2026, local time: "yesterday" ended thirty minutes ago.
  const now = new Date(2026, 9, 28, 0, 30);
  const at = (...parts: [number, number, number, number, number]) => new Date(...parts).toISOString();

  it('today, just after midnight', () => {
    const when = completedWhen(at(2026, 9, 28, 0, 10), now, en, t);
    expect(when?.words).toBe(`today, ${en.clockTime(new Date(2026, 9, 28, 0, 10))}`);
  });

  it('yesterday, just before midnight', () => {
    const when = completedWhen(at(2026, 9, 27, 23, 50), now, en, t);
    expect(when?.words).toBe(`yesterday, ${en.clockTime(new Date(2026, 9, 27, 23, 50))}`);
  });

  it('the day before yesterday is a date, without the year in the same year', () => {
    const date = new Date(2026, 9, 26, 23, 59);
    expect(completedWhen(date.toISOString(), now, en, t)?.words).toBe(`${en.shortDate(date)}, ${en.clockTime(date)}`);
    expect(en.shortDate(date)).not.toContain('2026');
  });

  it('another year says the year', () => {
    const date = new Date(2025, 11, 31, 18, 6);
    const words = completedWhen(date.toISOString(), now, en, t)?.words;
    expect(words).toBe(`${en.mediumDate(date)}, ${en.clockTime(date)}`);
    expect(words).toContain('2025');
  });

  it('the exact time names the full date and the clock', () => {
    const date = new Date(2026, 9, 27, 23, 50);
    expect(completedWhen(date.toISOString(), now, en, t)?.exact).toBe(
      `Downloaded on ${en.longDate(date)} at ${en.clockTime(date)}`,
    );
  });

  it('says nothing for a time it does not have', () => {
    expect(completedWhen(null, now, en, t)).toBeNull();
    expect(completedWhen('not a date', now, en, t)).toBeNull();
  });
});

describe('the library game an installation made', () => {
  const game = (patch: Partial<LibraryGame>) => ({ id: 'g1', title: 'Game One', executable: 'game.exe', uninstalled: false, ...patch }) as LibraryGame;

  it('is the game of the installation’s id, none without one', () => {
    const games = new Map([game({ id: 'g0' }), game({ id: 'g1' })].map((item) => [item.id, item]));
    expect(installedGame({ gameId: 'g1' }, games)?.id).toBe('g1');
    expect(installedGame({ gameId: '' }, games)).toBeUndefined();
    expect(installedGame(undefined, games)).toBeUndefined();
    expect(installedGame({ gameId: 'gone' }, games)).toBeUndefined();
  });
});
