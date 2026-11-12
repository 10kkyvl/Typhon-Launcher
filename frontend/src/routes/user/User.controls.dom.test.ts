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

  it('a retry that fails again keeps the notice, says it again and keeps the focus', async () => {
    authState.set('authenticated');
    vi.mocked(fetchProfile).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(UserProfile, { username: 'ann' });
    const region = await screen.findByRole('alert');
    const notice = region.closest('.notice') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent).toContain(msg('social.userLoadFailed'));
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

  type CommonPage = Awaited<ReturnType<typeof userCommon>>;
  const common = (igdbId: number, title: string) => ({ igdbId, title, coverUrl: '', viewerOwned: true, targetOwned: true }) as unknown as CommonGame;
  const pending = () => {
    let resolve!: (value: CommonPage) => void;
    const promise = new Promise<CommonPage>((res) => (resolve = res));
    return { promise, resolve };
  };
  const mount = () => renderDom(UserCommonModal, { open: true, username: 'ann', name: 'Ann' });
  const rowOf = (title: string) => screen.getByRole('button', { name: (name) => name.startsWith(title) });

  it('with none, the dialog says so with the base empty state', async () => {
    vi.mocked(userCommon).mockResolvedValue({ games: [], next: '' });
    mount();

    expect((await screen.findByText(msg('social.userCommonEmpty'))).closest('.empty.inline')).not.toBeNull();
  });

  it('shows the base spinner, named, while its first page loads', async () => {
    vi.mocked(userCommon).mockReturnValue(new Promise(() => {}));
    mount();

    expect((await screen.findByText(msg('social.loadingEllipsis'))).closest('.spinner[role="status"]')).not.toBeNull();
  });

  it('a failed first page: the notice keeps its busy Retry and the focus, then focus goes to the first game', async () => {
    const again = pending();
    vi.mocked(userCommon).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = mount();
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve({ games: [common(1, 'Hades')], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(rowOf('Hades')));
  });

  it('a retry that finds no games keeps the focus inside the dialog', async () => {
    vi.mocked(userCommon).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ games: [], next: '' });
    const { user } = mount();
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    within(notice).getByRole('button', { name: msg('common.retry') }).focus();

    await user.keyboard('{Enter}');

    await screen.findByText(msg('social.userCommonEmpty'));
    await vi.waitFor(() => expect(document.activeElement).not.toBe(document.body));
    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true);
  });

  it('a first page whose Retry fails again: the notice stays, says it again and keeps the focus', async () => {
    vi.mocked(userCommon).mockRejectedValue(new Error('offline'));
    const { user } = mount();
    const region = await screen.findByRole('alert');
    const notice = region.closest('.notice') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent).toContain(msg('social.userCommonFailed'));
  });

  it('a later page whose Retry fails again: the games stay, the notice says it again and keeps the focus', async () => {
    vi.mocked(userCommon).mockResolvedValueOnce({ games: [common(1, 'Hades')], next: 'p2' }).mockRejectedValue(new Error('offline'));
    const { user } = mount();
    (await screen.findByRole('button', { name: msg('social.userCommonMore') })).focus();
    await user.keyboard('{Enter}');
    const region = await screen.findByRole('alert');
    const notice = region.closest('.notice') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    await vi.waitFor(() => expect(document.activeElement).toBe(retry));

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(rowOf('Hades')).toBeTruthy();
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent).toContain(msg('social.userCommonFailed'));
  });

  it('"Show more" is busy while its page loads, keeps its words and the focus', async () => {
    vi.mocked(userCommon).mockResolvedValueOnce({ games: [common(1, 'Hades')], next: 'p2' }).mockReturnValueOnce(new Promise(() => {}));
    const { user } = mount();
    const more = await screen.findByRole('button', { name: msg('social.userCommonMore') });
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-busy')).toBe('true');
    expect(more.hasAttribute('disabled')).toBe(false);
    expect(more.textContent?.trim()).toBe(msg('social.userCommonMore'));
    expect(document.activeElement).toBe(more);
  });

  it('"Show more" pressed from the keyboard and failed: focus goes to the Retry of the notice in its place', async () => {
    vi.mocked(userCommon).mockResolvedValueOnce({ games: [common(1, 'Hades')], next: 'p2' }).mockRejectedValueOnce(new Error('offline'));
    const { user } = mount();
    const more = await screen.findByRole('button', { name: msg('social.userCommonMore') });
    more.focus();

    await user.keyboard('{Enter}');

    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    expect(more.isConnected).toBe(false);
    expect(rowOf('Hades')).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement).toBe(within(notice).getByRole('button', { name: msg('common.retry') })));
  });

  it('a failed later page is retried from its notice, then focus goes to the first game it brought', async () => {
    const again = pending();
    vi.mocked(userCommon)
      .mockResolvedValueOnce({ games: [common(1, 'Hades')], next: 'p2' })
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(again.promise);
    const { user } = mount();
    (await screen.findByRole('button', { name: msg('social.userCommonMore') })).focus();
    await user.keyboard('{Enter}');
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    await vi.waitFor(() => expect(document.activeElement).toBe(retry));

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(vi.mocked(userCommon).mock.calls.at(-1)).toEqual(['ann', 'p2']);

    again.resolve({ games: [common(2, 'Celeste')], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(rowOf('Celeste')));
  });
});
