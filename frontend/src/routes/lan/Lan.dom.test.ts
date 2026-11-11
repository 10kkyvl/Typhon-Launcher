import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { hashing } from '../../lib/stores/lan';
import { libraryGames } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import Lan from './Lan.svelte';

afterEach(() => {
  hashing.set(new Map());
  libraryGames.set([]);
});

const game = (id: string, title: string) => ({ id, title }) as LibraryGame;
const progress = (gameId: string, processedBytes: number) => ({ gameId, processedBytes, totalBytes: 200, currentFile: '', done: false });

describe('Lan: games being prepared for sharing', () => {
  it('each row and its bar are named after the game, the amount is the bar’s value', () => {
    libraryGames.set([game('g1', 'Hades'), game('g2', 'Celeste')]);
    hashing.set(new Map([['g1', progress('g1', 50)], ['g2', progress('g2', 150)]]));
    const { getByRole, getByText } = renderDom(Lan);

    expect(getByRole('progressbar', { name: 'Hades' }).getAttribute('aria-valuenow')).toBe('25');
    expect(getByRole('progressbar', { name: 'Celeste' }).getAttribute('aria-valuenow')).toBe('75');
    expect(getByText('Hades').closest('.row')?.textContent).toContain(msg('transfers.lanPreparingPercent', { percent: 25 }));
  });

  it('a game the library no longer lists is still a named row', () => {
    hashing.set(new Map([['gone', progress('gone', 100)]]));
    const { getByRole } = renderDom(Lan);

    expect(getByRole('progressbar', { name: msg('ui.game') }).getAttribute('aria-valuenow')).toBe('50');
  });
});
