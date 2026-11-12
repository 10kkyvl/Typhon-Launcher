import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import type { Offer } from '../../lib/services/lan';
import { hashing, offers } from '../../lib/stores/lan';
import { libraryGames } from '../../lib/stores/library';
import { styleOf, topLevel } from '../../lib/testing/css';
import { renderDom } from '../../lib/testing/dom';
import Lan from './Lan.svelte';
import page from './Lan.svelte?raw';

afterEach(() => {
  hashing.set(new Map());
  offers.set([]);
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

describe('Lan: empty sections', () => {
  it('with nothing shared and nothing coming in, each section says so with the base empty state', () => {
    renderDom(Lan);

    for (const key of ['transfers.lanNoShares', 'transfers.lanNoTransfers'] as const) {
      expect(screen.getByText(msg(key)).closest('.empty.inline'), key).not.toBeNull();
    }
  });
});

describe('Lan: a game offered nearby', () => {
  // A row takes no focus: a hint on it reaches only a mouse. What it said is the row's own text.
  it('says its version, size and host in the row, with no hover-only hint', () => {
    offers.set([
      { peerId: 'p1', host: 'PC-1', addr: '', port: 0, gameId: 'g1', title: 'Portal 2', version: '1.0', exe: '', sizeBytes: 1024, infoHash: 'h1', lastSeen: '' } as Offer,
    ]);
    renderDom(Lan);

    const row = screen.getByText('Portal 2').closest('.row') as HTMLElement;
    expect(row.hasAttribute('title')).toBe(false);
    expect(row.textContent).toContain('1.0');
    expect(row.textContent).toContain(msg('transfers.lanFromHost', { host: 'PC-1' }));
  });

  // The version tells two offers of the same game apart. jsdom ignores media queries: the test reads
  // every rule the page hides something with inside one and checks none of them reaches the version.
  it('keeps its version visible at every window width', () => {
    offers.set([
      { peerId: 'p1', host: 'PC-1', addr: '', port: 0, gameId: 'g1', title: 'Portal 2', version: '1.0', exe: '', sizeBytes: 1024, infoHash: 'h1', lastSeen: '' } as Offer,
    ]);
    renderDom(Lan);
    const version = screen.getByText('1.0');
    const css = styleOf(page, 'Lan');
    const hiding = [...css.matchAll(/@media[^{]*\{((?:[^{}]*\{[^{}]*\})*)\s*\}/g)].flatMap((block) =>
      [...block[1].matchAll(/([^{}]+)\{([^{}]*)\}/g)].filter((rule) => /display:\s*none/.test(rule[2])).flatMap((rule) => topLevel(rule[1].trim())),
    );

    expect(hiding.length).toBeGreaterThan(0);
    expect(hiding.filter((selector) => version.closest(selector))).toEqual([]);
  });
});
