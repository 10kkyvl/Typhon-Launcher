import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { UserCard } from '../../lib/services/social';
import { openGameByIGDB } from '../../lib/social/openGame';
import { renderDom } from '../../lib/testing/dom';
import overlayFriends from '../../overlay/OverlayFriends.svelte?raw';
import FriendRow from './FriendRow.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/social/openGame', () => ({ openGameByIGDB: vi.fn() }));

const ann = { id: 'u1', username: 'ann', displayName: 'Ann', avatarUrl: '' } as unknown as UserCard;
const TITLE = 'The Legend of Heroes: Trails in the Sky the 3rd';

afterEach(() => {
  vi.mocked(openGameByIGDB).mockReset();
});

describe('FriendRow: the game a friend is playing', () => {
  it.each(['list', 'grid'] as const)('opens it through a one-line text link the keyboard presses (%s)', async (variant) => {
    const { user } = renderDom(FriendRow, { user: ann, variant, status: 'online', game: { igdbId: 7, title: TITLE } });
    const link = screen.getByRole('button', { name: TITLE });
    expect(link.classList.contains('text-link')).toBe(true);
    expect(link.classList.contains('truncate')).toBe(true);
    link.focus();

    await user.keyboard('{Enter}');

    expect(openGameByIGDB).toHaveBeenCalledExactlyOnceWith(7, TITLE);
  });

  it('draws no link for a friend who is not in a game', () => {
    const { container } = renderDom(FriendRow, { user: ann, compact: true, status: 'online', meta: 'Online' });
    expect(container.querySelector('.text-link')).toBeNull();
    expect(screen.getByText('Online')).toBeTruthy();
  });

  // The in-game overlay draws its friends with this row and its look is not to change with the main
  // window's: it never passes a game, so the link is never drawn there.
  it('is never given a game by the overlay', () => {
    const rows = overlayFriends.match(/<FriendRow\b[^]*?>/g) ?? [];
    expect(rows).toHaveLength(1);
    expect(rows[0]).not.toMatch(/\bgame\b/);
  });
});
