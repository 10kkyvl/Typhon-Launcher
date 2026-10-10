import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../services/backend', () => ({ inWails: false }));

import BlockGrid from './BlockGrid.svelte';
import { locale } from '../../i18n';
import type { BlockBody, BlockGame, GridBlock } from '../../profile/layoutView';

function game(key: string, patch: Partial<BlockGame> = {}): BlockGame {
  return { key, title: `Game ${key}`, cover: '', hero: '', open: () => {}, ...patch };
}

function cell(id: string, body: BlockBody, patch: Partial<GridBlock> = {}): GridBlock {
  const types: Record<BlockBody['kind'], string> = {
    pinned: 'pinned',
    collection: 'collection',
    genres: 'genres',
    fingerprint: 'fingerprint',
    text: 'text',
    external: 'stats',
    error: 'pinned',
    unknown: 'future',
  };
  return { id, index: 0, type: types[body.kind], width: 'full', hidden: false, issue: false, body, ...patch };
}

const external = createRawSnippet((block: () => GridBlock) => ({
  render: () => `<i class="external-${block().type}">external</i>`,
}));

const flag = createRawSnippet(() => ({ render: () => '<b class="flag-mark">hidden</b>' }));

function html(blocks: GridBlock[], props: Record<string, unknown> = {}): string {
  return render(BlockGrid, { props: { blocks, external, flag, ...props } }).body;
}

beforeEach(() => {
  locale.set('en');
});

afterEach(() => {
  locale.set('ru');
});

describe('BlockGrid cells', () => {
  it('gives a cell the width class of its block', () => {
    const out = html([
      cell('a', { kind: 'text', title: '', body: 'x' }, { width: 'half', index: 0 }),
      cell('b', { kind: 'text', title: '', body: 'y' }, { width: 'third', index: 1 }),
      cell('c', { kind: 'text', title: '', body: 'z' }, { width: 'full', index: 2 }),
    ]);

    expect(out).toContain('w-half');
    expect(out).toContain('w-third');
    expect(out).toContain('w-full');
  });

  it('keeps the order it was given', () => {
    const out = html([
      cell('a', { kind: 'text', title: '', body: 'first-text' }),
      cell('b', { kind: 'text', title: '', body: 'second-text' }, { index: 1 }),
    ]);

    expect(out.indexOf('first-text')).toBeLessThan(out.indexOf('second-text'));
  });

  it('renders nothing for an unknown type', () => {
    const out = html([cell('x', { kind: 'unknown' })]);

    expect(out).not.toContain('class="cell');
  });
});

describe('BlockGrid block types', () => {
  it('draws a pinned block as a hero with title, hours, completed date and caption', () => {
    const out = html([
      cell('p', {
        kind: 'pinned',
        caption: 'my favourite',
        game: game('1', { title: 'Elden Ring', seconds: 7200, completedAt: '2026-09-01T10:00:00Z', hero: 'hero.jpg' }),
      }),
    ]);

    expect(out).toContain('Elden Ring');
    expect(out).toContain('my favourite');
    expect(out).toContain('2 h');
    expect(out).toContain('Completed');
    expect(out).toContain('hero.jpg');
  });

  it('falls back from the hero art to the cover', () => {
    const out = html([cell('p', { kind: 'pinned', caption: '', game: game('1', { cover: 'cover.jpg', hero: '' }) })]);

    expect(out).toContain('cover.jpg');
  });

  it('shows the status of an unfinished pinned game and no caption line when it is empty', () => {
    const out = html([cell('p', { kind: 'pinned', caption: '', game: game('1', { status: 'playing', seconds: 0 }) })]);

    expect(out).toContain('Playing');
    expect(out).not.toContain('class="caption');
  });

  it('draws a collection with its title and one tile per game', () => {
    const out = html([
      cell('c', { kind: 'collection', title: 'Best of 2025', games: [game('a'), game('b'), game('c')], dated: false, hearts: false }),
    ]);

    expect(out).toContain('Best of 2025');
    expect(out.match(/class="tile/g)).toHaveLength(3);
  });

  it('dates the completed games of a dated collection only', () => {
    const body = (dated: boolean): BlockBody => ({
      kind: 'collection',
      title: 'T',
      games: [game('a', { completedAt: '2026-09-01T10:00:00Z' })],
      dated,
      hearts: false,
    });

    expect(html([cell('c', body(true))])).toContain('class="completed');
    expect(html([cell('c', body(false))])).not.toContain('class="completed');
  });

  it('puts a heart on the favourites', () => {
    const body = (hearts: boolean): BlockBody => ({ kind: 'collection', title: 'T', games: [game('a')], dated: false, hearts });

    expect(html([cell('c', body(true))])).toContain('class="heart');
    expect(html([cell('c', body(false))])).not.toContain('class="heart');
  });

  it('draws genres as a bar and a legend with percentages and a muted unknown segment', () => {
    const out = html([
      cell('g', {
        kind: 'genres',
        breakdown: { genres: [{ name: 'Role-playing (RPG)', share: 0.72 }, { name: 'Adventure', share: 0.2 }], other: 0.05, unknown: 0.03 },
      }),
    ]);

    expect(out).toContain('Role-playing (RPG)');
    expect(out).toContain('72%');
    expect(out).toContain('20%');
    expect(out).toContain('5%');
    expect(out).toContain('3%');
    expect(out).toContain('tone-unknown');
    expect(out).toContain('tone-other');
    expect(out).toContain('No genre');
  });

  it('shows a tiny share as less than one percent and says so when there are no genres at all', () => {
    const tiny = html([cell('g', { kind: 'genres', breakdown: { genres: [{ name: 'Rare', share: 0.004 }], other: 0, unknown: 0 } })]);
    const none = html([cell('g', { kind: 'genres', breakdown: { genres: [], other: 0, unknown: 0 } })]);

    expect(tiny).toContain('&lt;1%');
    expect(none).toContain('No genre data yet');
    expect(none).not.toContain('class="bar');
  });

  it('draws the fingerprint emblem with the three counters', () => {
    const out = html([
      cell('f', { kind: 'fingerprint', breakdown: { genres: [{ name: 'RPG', share: 0.6 }], other: 0, unknown: 0 }, hours: 412, games: 38, completed: 12 }),
    ]);

    expect(out).toContain('<svg');
    expect(out).toContain('class="arc ');
    expect(out).toContain('412');
    expect(out).toContain('38');
    expect(out).toContain('12');
  });

  it('shows the hidden mark instead of hours the owner keeps private, and never a zero', () => {
    const masked = createRawSnippet(() => ({ render: () => '<b class="masked-mark">hidden</b>' }));
    const body: BlockBody = {
      kind: 'fingerprint',
      breakdown: { genres: [{ name: 'RPG', share: 0.6 }], other: 0, unknown: 0 },
      hours: null,
      games: 38,
      completed: 12,
    };

    const out = html([cell('f', body)], { masked });

    expect(out.match(/masked-mark/g)).toHaveLength(1);
    expect(out).toContain('38');
    expect(out).not.toMatch(/class="num[^"]*">\s*(<!---->)?0\b/);
  });

  it('keeps the newlines of a text block and shows markup and links as plain text', () => {
    const out = html([cell('t', { kind: 'text', title: 'Notes', body: 'line one\nline two <b>bold</b> https://example.com' })]);

    expect(out).toContain('Notes');
    expect(out).toContain('line one\nline two');
    expect(out).toContain('&lt;b>bold&lt;/b>');
    expect(out).not.toContain('<b>bold</b>');
    expect(out).not.toContain('<a ');
  });

  it('hands an external block to the page', () => {
    const out = html([cell('s', { kind: 'external', empty: false })]);

    expect(out).toContain('external-stats');
  });
});

describe('BlockGrid error state', () => {
  it('shows an inline failure in the cell and keeps the other blocks', () => {
    const out = html([
      cell('p', { kind: 'error' }, { index: 0 }),
      cell('t', { kind: 'text', title: '', body: 'still here' }, { index: 1 }),
    ]);

    expect(out).toContain('Could not load');
    expect(out).toContain('Pinned game');
    expect(out).toContain('still here');
  });
});

describe('BlockGrid in view mode', () => {
  it('leaves out blocks with nothing to show', () => {
    const out = html([
      cell('p', { kind: 'pinned', caption: '', game: null }, { index: 0 }),
      cell('c', { kind: 'collection', title: 'Empty', games: [], dated: false, hearts: false }, { index: 1 }),
      cell('t', { kind: 'text', title: '', body: '  \n' }, { index: 2 }),
      cell('s', { kind: 'external', empty: true }, { index: 3 }),
    ]);

    expect(out).not.toContain('class="cell');
  });

  it('marks a block hidden from others with the page flag, but not an external block', () => {
    const native = html([cell('g', { kind: 'genres', breakdown: { genres: [{ name: 'A', share: 1 }], other: 0, unknown: 0 } }, { hidden: true })]);
    const outer = html([cell('s', { kind: 'external', empty: false }, { hidden: true })]);

    expect(native).toContain('flag-mark');
    expect(outer).not.toContain('flag-mark');
  });

  it('has no editing controls', () => {
    const out = html([cell('t', { kind: 'text', title: '', body: 'x' })]);

    expect(out).not.toContain('class="handle');
    expect(out).not.toContain('class="bar"');
  });
});

describe('BlockGrid in edit mode', () => {
  const blocks = [
    cell('a', { kind: 'text', title: '', body: 'alpha' }, { index: 0 }),
    cell('b', { kind: 'pinned', caption: '', game: null }, { index: 1, issue: true }),
    cell('c', { kind: 'external', empty: true }, { index: 2 }),
    cell('x', { kind: 'unknown' }, { index: 3 }),
  ];

  it('gives every known block a handle, a width switch and the move, settings and remove buttons', () => {
    const out = html(blocks, { editing: true });

    expect(out.match(/class="handle/g)).toHaveLength(3);
    expect(out).toContain('Drag block: Text');
    expect(out).toContain('Full width');
    expect(out).toContain('Half width');
    expect(out).toContain('One third width');
    expect(out).toContain('Block settings: Text');
    expect(out).toContain('Remove block: Text');
    expect(out).toContain('Move up: Text');
    expect(out).toContain('Move down: Text');
  });

  it('keeps empty blocks visible with a hint and marks unfinished ones', () => {
    const out = html(blocks, { editing: true });

    expect(out).toContain('Pick a game in the block settings');
    expect(out).toContain('Nothing to show yet');
    expect(out).toContain('Incomplete');
  });

  it('still draws nothing for an unknown type', () => {
    const out = html(blocks, { editing: true });

    expect(out.match(/data-index=/g)).toHaveLength(3);
  });

  it('marks the selected block', () => {
    const out = html(blocks, { editing: true, selectedId: 'a' });

    expect(out.match(/class="frame [^"]*selected/g)).toHaveLength(1);
  });
});
