import { screen } from '@testing-library/svelte';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';

// Fixtures of the library page's dom tests. Each test file mocks the services itself (vi.mock is
// hoisted per file); this holds the games and the queries they share.

export function game(id: string, title: string, patch: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title,
    executable: 'game.exe',
    installDir: `C:\\Games\\${id}`,
    cover: '',
    version: '',
    sizeBytes: 0,
    lastPlayed: null,
    playtimeSeconds: 0,
    installedAt: '',
    uninstalled: false,
    tags: [],
    ...patch,
  };
}

/** A game last played on day `day` of November 2026: a larger day is played later. */
export function played(id: string, title: string, day: number, patch: Partial<LibraryGame> = {}): LibraryGame {
  return game(id, title, { lastPlayed: `2026-11-${String(day).padStart(2, '0')}T10:00:00Z`, playtimeSeconds: 3600, ...patch });
}

export const carousel = () => screen.queryByRole('region', { name: msg('library.carousel.label') });

/** The titles of the carousel's slides, in their order, the ones off screen included. */
export function slideTitles(): string[] {
  return [...(carousel()?.querySelectorAll('[data-picture] .title') ?? [])].map((el) => el.textContent?.trim() ?? '');
}

export const recentSection = () =>
  screen.queryAllByRole('heading', { level: 2, name: msg('games.recentLabel') })[0]?.closest('section') ?? null;

/** The titles of the "Recent" row's capsules, in their order. */
export function recentTitles(): string[] {
  return [...(recentSection()?.querySelectorAll('.recent-item .title') ?? [])].map((el) => el.textContent?.trim() ?? '');
}

/** The list or the grid of games under the toolbar. */
export const results = () => document.querySelector<HTMLElement>('[data-results]');

/** The titles the list or the grid shows, in their order. */
export function shownTitles(): string[] {
  const list = results();
  if (!list) return [];
  return [...list.children].map((item) => item.querySelector('[id$="-title"]')?.textContent?.trim() ?? '');
}
