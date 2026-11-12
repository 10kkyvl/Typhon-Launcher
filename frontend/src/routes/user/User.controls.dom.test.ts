import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import {
  profile as fetchProfile,
  userCommon,
  type CommonGame,
  type GameCard,
  type PlayedGame,
  type PublicProfile,
  type UserCard,
} from '../../lib/services/social';
import { AccountError } from '../../lib/services/account';
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

const closedProfile = {
  id: 'u1',
  username: 'ann',
  displayName: 'Ann',
  avatarUrl: '',
  bio: '',
  relation: 'none',
  visibility: 'private',
  stats: null,
  favorites: [],
  showcase: [],
  recentlyPlayed: [],
  recentActivity: [],
  common: null,
  mutualFriends: [],
  mutualCount: 0,
  createdAt: '2026-01-01T00:00:00Z',
} as unknown as PublicProfile;

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

describe("a user's page: retrying a failed load", () => {
  afterEach(() => {
    vi.mocked(fetchProfile).mockReset();
    authState.set('bootstrapping');
  });

  it('keeps the danger notice and the focus on its busy Retry until the answer', async () => {
    let answer!: (value: PublicProfile) => void;
    authState.set('authenticated');
    vi.mocked(fetchProfile)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user } = renderDom(UserProfile, { username: 'ann' });
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('social.userLoadFailed'));
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    answer(closedProfile);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(screen.getByText(msg('social.userProfileClosed'))).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement?.closest('.profile')).not.toBeNull());
  });

  it('a retry that finds no such user lands the focus on the "not found" state', async () => {
    authState.set('authenticated');
    vi.mocked(fetchProfile).mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new AccountError('user_not_found'));
    const { user } = renderDom(UserProfile, { username: 'ann' });
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    within(notice).getByRole('button', { name: msg('common.retry') }).focus();

    await user.keyboard('{Enter}');

    const heading = await screen.findByRole('heading', { level: 2, name: msg('social.userNotFoundTitle') });
    await vi.waitFor(() => expect(document.activeElement).toBe(heading.closest('[tabindex="-1"]')));
    expect(document.activeElement).not.toBe(document.body);
  });

  it('a failure without a reason of its own is said once, not as a title and again as the text', async () => {
    authState.set('authenticated');
    vi.mocked(fetchProfile).mockRejectedValueOnce(new Error('offline'));
    renderDom(UserProfile, { username: 'ann' });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.split(msg('social.userLoadFailed')).length).toBe(2);
  });
});

describe("a user's page: a closed profile", () => {
  afterEach(() => {
    vi.mocked(fetchProfile).mockReset();
    authState.set('bootstrapping');
  });

  it('says so on the base notice, a standing fact read in the flow of the page', async () => {
    authState.set('authenticated');
    vi.mocked(fetchProfile).mockResolvedValue(closedProfile);
    renderDom(UserProfile, { username: 'ann' });

    const notice = (await screen.findByText(msg('social.userProfileClosed'))).closest('.notice');
    expect(notice?.getAttribute('data-tone')).toBe('info');
    expect(notice?.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });
});

describe("a user's page: all the games in common", () => {
  afterEach(() => vi.mocked(userCommon).mockReset());

  it('with none, the dialog says so with the base empty state', async () => {
    vi.mocked(userCommon).mockResolvedValue({ games: [], next: '' });
    renderDom(UserCommonModal, { open: true, username: 'ann', name: 'Ann' });

    expect((await screen.findByText(msg('social.userCommonEmpty'))).closest('.empty.inline')).not.toBeNull();
  });

  it('a failed first page: the danger notice keeps its busy Retry and the focus until the answer', async () => {
    let answer!: (value: Awaited<ReturnType<typeof userCommon>>) => void;
    vi.mocked(userCommon)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user } = renderDom(UserCommonModal, { open: true, username: 'ann', name: 'Ann' });
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    answer({ games: [], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(screen.getByText(msg('social.userCommonEmpty'))).toBeTruthy();
  });

  it('a failed later page keeps the games and retries that page from its notice', async () => {
    let answer!: (value: Awaited<ReturnType<typeof userCommon>>) => void;
    const hades = { igdbId: 1, title: 'Hades', coverUrl: '', viewerOwned: true, targetOwned: true } as unknown as CommonGame;
    vi.mocked(userCommon)
      .mockResolvedValueOnce({ games: [hades], next: 'p2' })
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user } = renderDom(UserCommonModal, { open: true, username: 'ann', name: 'Ann' });
    await user.click(await screen.findByRole('button', { name: msg('social.userCommonMore') }));
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    expect(screen.queryByRole('button', { name: msg('social.userCommonMore') })).toBeNull();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(vi.mocked(userCommon).mock.calls.at(-1)).toEqual(['ann', 'p2']);
    expect(screen.getByText('Hades')).toBeTruthy();

    answer({ games: [], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
  });
});
