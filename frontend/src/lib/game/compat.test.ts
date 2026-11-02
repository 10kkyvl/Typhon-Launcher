import { describe, expect, it } from 'vitest';
import type { CompatStatus } from '../services/compat';
import { brokenStatus, compatBadge } from './compat';

describe('brokenStatus', () => {
  const status = (state: CompatStatus['state']) => ({ gameId: 'a', state }) as CompatStatus;
  const emulated = { emulation: { platform: 'ps2' } };

  it.each([
    ['a broken native game', {}, status('broken'), true],
    ['a working native game', {}, status('works'), false],
    ['a native game without a record', {}, undefined, false],
    ['an emulated game with an old broken record', emulated, status('broken'), false],
  ])('%s', (_, game, record, shown) => {
    expect(brokenStatus(game, record)).toBe(shown ? record : null);
  });
});

describe('compatBadge', () => {
  it('молчит, когда цифры нет', () => {
    expect(compatBadge(undefined)).toBeNull();
  });

  // Строка без наблюдений — не «ноль процентов», а отсутствие ответа.
  it('молчит, когда наблюдений ноль', () => {
    expect(compatBadge({ works: 0, total: 0 })).toBeNull();
  });

  it('молчит на невозможных числах', () => {
    expect(compatBadge({ works: 11, total: 10 })).toBeNull();
    expect(compatBadge({ works: -1, total: 10 })).toBeNull();
  });

  it('считает игру работающей, когда она идёт у большинства', () => {
    expect(compatBadge({ works: 9, total: 10 })).toEqual({ works: true, works_count: 9, total: 10 });
  });

  it('считает игру неработающей, когда она не идёт у большинства', () => {
    expect(compatBadge({ works: 2, total: 10 })).toEqual({ works: false, works_count: 2, total: 10 });
  });

  // Ровно половина — ещё не приговор: обещать «не поедет» на таких данных
  // так же неверно, как обещать обратное.
  it('на ровной половине не клеймит игру', () => {
    expect(compatBadge({ works: 5, total: 10 })?.works).toBe(true);
  });
});
