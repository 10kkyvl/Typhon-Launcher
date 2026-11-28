import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../i18n';
import type { BlockBody, GridBlock } from '../../profile/layoutView';
import { calls, resetBindings } from '../../testing/fakeBinding';
import AboutBlock from './AboutBlock.svelte';
import BlockGrid from './BlockGrid.svelte';
import TextBlock from './TextBlock.svelte';

vi.mock('../../services/backend', () => ({ inWails: false }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

/**
 * What each block's body draws with no data, and with data the owner keeps from others: the page
 * draws the blocks in three places (the own profile, another player's page, the editor).
 */

function cell(id: string, type: string, body: BlockBody, patch: Partial<GridBlock> = {}): GridBlock {
  return { id, index: 0, type, width: 'full', hidden: false, issue: false, body, ...patch };
}

const flag = createRawSnippet(() => ({ render: () => '<b class="flag-mark">hidden</b>' }));
const grid = (blocks: GridBlock[], props: Record<string, unknown> = {}) => render(BlockGrid, { props: { blocks, flag, ...props } }).body;

beforeEach(() => {
  locale.set('en');
  resetBindings();
});

afterEach(() => {
  locale.set('ru');
  expect(calls).toEqual([]);
});

describe('a block with no data', () => {
  it('a collection with no games says so in the editor', () => {
    const out = grid([cell('c', 'collection', { kind: 'collection', title: 'Mine', games: [], dated: false, hearts: false })], { editing: true });

    expect(out).toContain('Mine');
    expect(out).toContain(msg('profile.showcaseEmpty'));
  });

  it('a pinned block with no game, an empty text and a block with no data each say what to do in the editor', () => {
    const out = grid(
      [
        cell('p', 'pinned', { kind: 'pinned', game: null, caption: '' }),
        cell('t', 'text', { kind: 'text', title: 'Title only', body: '' }, { index: 1 }),
        cell('s', 'stats', { kind: 'external', empty: true }, { index: 2 }),
      ],
      { editing: true },
    );

    expect(out).toContain(msg('profile.blockChooseGame'));
    expect(out).toContain(msg('profile.blockEmptyText'));
    expect(out).toContain(msg('profile.blockNoData'));
  });

  it('the fingerprint shows a dash for a count it does not have, when the page gives no hidden mark', () => {
    const out = grid([cell('f', 'fingerprint', { kind: 'fingerprint', breakdown: { genres: [], other: 0, unknown: 0 }, hours: null, games: 3, completed: null })]);

    expect(out.match(/—/g)).toHaveLength(2);
    expect(out).toContain('<svg');
  });

  it('a text block with no title draws no heading', () => {
    expect(render(TextBlock, { props: { title: '', body: 'words' } }).body).not.toContain('<h3');
    expect(render(TextBlock, { props: { title: 'Notes', body: 'words' } }).body).toContain('Notes');
  });

  it('the about block with no text says so under its heading, instead of an empty frame', () => {
    for (const bio of ['', '  \n ']) {
      const out = render(AboutBlock, { props: { bio } }).body;

      expect(out).toContain(msg('profile.blockAbout'));
      expect(out).toContain(msg('profile.aboutEmpty'));
      expect(out).not.toContain('class="bio');
    }
  });

  it('the about block draws the text under its own heading', () => {
    const out = render(AboutBlock, { props: { bio: 'line one\nline two' } }).body;

    expect(out).toContain(msg('profile.blockAbout'));
    expect(out).toContain('line one\nline two');
  });
});

describe('a block the owner keeps from others', () => {
  it('in the editor carries the page flag in its bar, an external block too', () => {
    const out = grid(
      [
        cell('g', 'genres', { kind: 'genres', breakdown: { genres: [{ name: 'A', share: 1 }], other: 0, unknown: 0 } }, { hidden: true }),
        cell('s', 'stats', { kind: 'external', empty: false }, { index: 1, hidden: true }),
      ],
      { editing: true },
    );

    expect(out.match(/flag-mark/g)).toHaveLength(2);
  });

  it('in view is drawn with the flag over it, the others without', () => {
    const out = grid([
      cell('g', 'genres', { kind: 'genres', breakdown: { genres: [{ name: 'A', share: 1 }], other: 0, unknown: 0 } }, { hidden: true }),
      cell('t', 'text', { kind: 'text', title: '', body: 'open' }, { index: 1 }),
    ]);

    expect(out.match(/flag-mark/g)).toHaveLength(1);
    expect(out).toContain('open');
  });
});
