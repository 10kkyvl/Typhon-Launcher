import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { profile as fetchProfile, userCommon, type GameCard, type PlayedGame, type UserCard } from '../../lib/services/social';
import { resetHistory, route } from '../../lib/stores/router';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import GameRow from './GameRow.svelte';
import UserCommonModal from './UserCommonModal.svelte';
import UserMutual from './UserMutual.svelte';
import UserProfile from './UserProfile.svelte';
import UserRecent from './UserRecent.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  userCommon: vi.fn(),
  profile: vi.fn(),
}));

// The accessible name of the one button whose name holds `title`.
function nameOf(title: string): string {
  let found = '';
  screen.getByRole('button', {
    name: (name) => {
      if (!name.includes(title)) return false;
      found = name;
      return true;
    },
  });
  return found;
}

const times = (text: string, word: string) => text.split(word).length - 1;

afterEach(() => {
  document.body.replaceChildren();
});

describe("a user's page: rows that open a game", () => {
  it('a game row is named by its title once: the cover says nothing of its own', () => {
    renderDom(GameRow, { game: { igdbId: 1, title: 'Hades', coverUrl: '' } as GameCard, meta: 'Both own it' });

    const name = nameOf('Hades');
    expect(name.startsWith('Hades')).toBe(true);
    expect(times(name, 'Hades')).toBe(1);
  });

  it('a recently played game is named by its title once', () => {
    const games = [{ igdbId: 1, title: 'Hades', coverUrl: '', heroUrl: '', lastPlayedAt: '2026-11-01T10:00:00Z', playtimeSeconds: 0 }] as unknown as PlayedGame[];
    renderDom(UserRecent, { games });

    const name = nameOf('Hades');
    expect(name.startsWith('Hades')).toBe(true);
    expect(times(name, 'Hades')).toBe(1);
  });
});

describe("a user's page: mutual friends", () => {
  afterEach(() => resetHistory());

  it('"All" is a text link the keyboard presses, and it opens the friends', async () => {
    const friends = [{ id: 'u1', username: 'ann', displayName: 'Ann', avatarUrl: '' }] as unknown as UserCard[];
    const { user } = renderDom(UserMutual, { friends, count: 3 });
    const all = screen.getByRole('button', { name: msg('social.viewAllButton') });
    expect(all.classList.contains('text-link')).toBe(true);
    all.focus();

    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('friends');
  });
});

describe("a user's page: loading", () => {
  afterEach(() => {
    vi.mocked(fetchProfile).mockReset();
    authState.set('bootstrapping');
  });

  it('shows the base spinner named by what is loading', async () => {
    authState.set('authenticated');
    vi.mocked(fetchProfile).mockReturnValue(new Promise(() => {}));
    renderDom(UserProfile, { username: 'ann' });

    expect((await screen.findByText(msg('social.loadingEllipsis'))).closest('.spinner[role="status"]')).not.toBeNull();
  });
});

describe("a user's page: all the games in common", () => {
  afterEach(() => vi.mocked(userCommon).mockReset());

  it('with none, the dialog says so with the base empty state', async () => {
    vi.mocked(userCommon).mockResolvedValue({ games: [], next: '' });
    renderDom(UserCommonModal, { open: true, username: 'ann', name: 'Ann' });

    expect((await screen.findByText(msg('social.userCommonEmpty'))).closest('.empty.inline')).not.toBeNull();
  });
});
