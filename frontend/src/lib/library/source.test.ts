import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { gameSource } from './source';

type Input = Pick<LibraryGame, 'source' | 'external'>;

const external = (provider: string) => ({ provider }) as unknown as LibraryGame['external'];

describe('gameSource', () => {
  const cases: [string, Input, ReturnType<typeof gameSource>][] = [
    ['managed', { source: 'managed' }, 'typhon'],
    ['empty source', { source: '' }, 'typhon'],
    ['missing source', {}, 'typhon'],
    ['discovered', { source: 'discovered' }, 'disk'],
    ['steam', { source: 'external', external: external('steam') }, 'steam'],
    ['epic', { source: 'external', external: external('epic') }, 'epic'],
    ['gog', { source: 'external', external: external('gog') }, 'gog'],
    ['unknown provider', { source: 'external', external: external('uplay') }, null],
    ['unknown source', { source: 'imported' }, null],
  ];

  it.each(cases)('%s', (_, game, want) => {
    expect(gameSource(game)).toBe(want);
  });
});
