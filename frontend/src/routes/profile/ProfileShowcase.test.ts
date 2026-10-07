import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../lib/services/backend', () => ({ inWails: false }));

import ProfileShowcase from './ProfileShowcase.svelte';
import { locale } from '../../lib/i18n';
import type { GameRef, ShowcaseBlock } from '../../lib/services/profile';

function game(id: string, patch: Partial<GameRef> = {}): GameRef {
  return { id, title: `Game ${id}`, cover: '', playtimeSeconds: 0, status: '', ...patch };
}

function html(blocks: ShowcaseBlock[], showEmpty = false): string {
  return render(ProfileShowcase, { props: { blocks, onmanage: () => {}, showEmpty } }).body;
}

beforeEach(() => {
  locale.set('en');
});

afterEach(() => {
  locale.set('ru');
});

describe('profile showcase', () => {
  it('draws nothing for a block with no games on someone else’s profile', () => {
    const out = html([{ kind: 'favorites', games: [] }, { kind: 'most_played', games: [] }]);

    expect(out).not.toContain('class="grid');
    expect(out).not.toContain('Manage');
  });

  it('draws an empty block with a hint when the owner asks to see empty blocks', () => {
    const out = html([{ kind: 'favorites', games: [] }], true);

    expect(out).toContain('class="grid');
    expect(out).toContain('class="empty');
  });

  it('shows one tile per game with its title', () => {
    const out = html([{ kind: 'most_played', games: [game('a'), game('b'), game('c')] }]);

    expect(out.match(/class="tile/g)).toHaveLength(3);
    expect(out).toContain('Game a');
    expect(out).toContain('Game c');
  });

  it('offers to manage favorites and nothing else', () => {
    const favorites = html([{ kind: 'favorites', games: [game('a')] }]);
    const other = html([{ kind: 'most_played', games: [game('a')] }]);

    expect(favorites).toContain('class="manage');
    expect(other).not.toContain('class="manage');
  });

  it('dates a recently completed game and leaves other blocks without a date', () => {
    const done = html([{ kind: 'recently_completed', games: [game('a', { statusAt: '2026-09-01T10:00:00Z' })] }]);
    const undated = html([{ kind: 'recently_completed', games: [game('a', { statusAt: null })] }]);
    const elsewhere = html([{ kind: 'most_played', games: [game('a', { statusAt: '2026-09-01T10:00:00Z' })] }]);

    expect(done).toContain('class="completed');
    expect(undated).not.toContain('class="completed');
    expect(elsewhere).not.toContain('class="completed');
  });

  it('keeps the order of the blocks it was given', () => {
    const out = html([
      { kind: 'most_played', games: [game('a')] },
      { kind: 'favorites', games: [game('b')] },
    ]);

    expect(out.indexOf('Game a')).toBeLessThan(out.indexOf('Game b'));
  });
});
