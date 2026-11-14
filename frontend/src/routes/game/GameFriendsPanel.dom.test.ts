import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { emptyFriendsPage, gameFriends, type FriendView, type GameFriends } from '../../lib/services/social';
import { resetHistory, route } from '../../lib/stores/router';
import { friendsPage } from '../../lib/stores/social';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import GameFriendsPanel from './GameFriendsPanel.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  gameFriends: vi.fn(),
}));

const ann = { id: 'u1', username: 'ann', displayName: 'Ann', avatarUrl: '' };

afterEach(() => {
  authState.set('bootstrapping');
  friendsPage.set(emptyFriendsPage());
  vi.mocked(gameFriends).mockReset();
  resetHistory();
});

describe('GameFriendsPanel', () => {
  it('leads to all friends through a text link the keyboard presses', async () => {
    authState.set('authenticated');
    friendsPage.set({ ...emptyFriendsPage(), friends: [ann as unknown as FriendView] });
    vi.mocked(gameFriends).mockResolvedValue({ played: [{ ...ann, playtimeSeconds: 0, status: '' }], playingNow: [] } as unknown as GameFriends);
    const { user } = renderDom(GameFriendsPanel, { canonicalGameId: 'c1' });

    const all = await screen.findByRole('button', { name: msg('games.friendsPanelSeeAll') });
    expect(all.classList.contains('text-link')).toBe(true);
    all.focus();
    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('friends');
  });
});

describe('GameFriendsPanel: the game changes under the panel', () => {
  it("does not show the previous game's friends while the next game's load", async () => {
    authState.set('authenticated');
    friendsPage.set({ ...emptyFriendsPage(), friends: [ann as unknown as FriendView] });
    let answer!: (value: GameFriends) => void;
    vi.mocked(gameFriends)
      .mockResolvedValueOnce({ played: [{ ...ann, playtimeSeconds: 0, status: '' }], playingNow: [] } as unknown as GameFriends)
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { rerender } = renderDom(GameFriendsPanel, { canonicalGameId: 'c1' });
    await screen.findByText('Ann');

    await rerender({ canonicalGameId: 'c2' });

    expect(screen.queryByText('Ann')).toBeNull();
    answer({ played: [{ ...ann, id: 'u2', displayName: 'Bo', playtimeSeconds: 0, status: '' }], playingNow: [] } as unknown as GameFriends);
    await screen.findByText('Bo');
  });
});
