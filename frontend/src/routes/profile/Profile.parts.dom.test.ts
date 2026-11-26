import { tick } from 'svelte';
import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));
vi.mock('../../lib/stores/user', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/user')>()),
  saveProfile: vi.fn(),
}));
vi.mock('../../lib/stores/metadata', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(async () => {}),
}));

import { locale, msg } from '../../lib/i18n';
import { DEFAULT_PROFILE, type CurrentUser, type ProfileBlock, type ProfileSettings } from '../../lib/services/account';
import type { LibraryGame } from '../../lib/services/library';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot, type GameRef, type ProfileSnapshot } from '../../lib/services/profile';
import { libraryGames } from '../../lib/stores/library';
import { gameArt } from '../../lib/stores/metadata';
import { profileDraft, profileSnapshot } from '../../lib/stores/profile';
import { confirmLeave, leaveRequest, navigate, resetHistory, route, stayHere } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser, saveProfile } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import Profile from './Profile.svelte';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';

// The page keeps the profile, the draft and the snapshot's derived state; ProfileExternalBlock draws the bodies
// of the blocks BlockGrid leaves to the page (playing, recent, activity, stats, about). Held here: the block
// type each body is drawn for, the game being played and its open, the recent row following the snapshot, the
// art and the library stores, the stats block's hidden badge on first render, the bio being there; and,
// through the page, the leave guard (ask, stay, leave, the draft kept and restored), the save flow (the patch
// without the showcase, edit mode left) and the preview read with its failure notice. Not held: the activity
// and stats bodies following the snapshot store, the hidden types and the bio changing after the first render,
// the hidden badges of the playing and activity blocks, the cover of the game being played, the each items'
// reference equality that made the part read the snapshot store itself (invisible in the DOM), the block
// editor's own controls (Profile.blocks.dom.test.ts) and the appearance panel (ProfileAppearancePanel.*).

const ref = (id: string, title: string, extra: Partial<GameRef> = {}): GameRef => ({
  id, title, cover: `${id}.jpg`, canonicalGameId: `c-${id}`, playtimeSeconds: 3600, status: '', ...extra,
});
const block = (type: string, width: ProfileBlock['width'] = 'full'): ProfileBlock => ({ id: `b-${type}`, type, width, config: {} });
const SNAPSHOT: ProfileSnapshot = {
  ...EMPTY_SNAPSHOT,
  running: [ref('g1', 'Hades')],
  playing: [{ game: ref('g2', 'Celeste'), recentSeconds: 60 }],
  activity: [{ date: '2026-11-20', entries: [{ game: ref('g2', 'Celeste'), seconds: 3600 }] }],
  stats: { ...EMPTY_SNAPSHOT.stats, games: 12 },
};

function signIn(profile: Partial<ProfileSettings> = {}, bio = 'Hello there') {
  const me: CurrentUser = {
    id: 'u1', username: 'ann', displayName: 'Ann', email: 'ann@example.com', avatarUrl: '', bio,
    profile: { ...DEFAULT_PROFILE, layout: { version: 1, blocks: ['playing', 'recent', 'activity', 'stats', 'about'].map((t) => block(t)) }, ...profile },
    createdAt: '2025-01-01T00:00:00Z',
  };
  currentUser.set(me);
}

// The card of a block of the grid, found by its title (the header has stats and a bio of its own).
const blockCard = (title: string) =>
  [...document.querySelectorAll<HTMLElement>('.blocks .card')].find((card) => card.firstElementChild?.textContent?.trim() === title) ?? null;
const recentCard = () => document.querySelector('.recent-item') as HTMLElement | null;

beforeEach(() => {
  resetBindings();
  locale.set('en');
  authState.set('authenticated');
  vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
  vi.mocked(getProfilePreview).mockResolvedValue(SNAPSHOT);
  vi.mocked(saveProfile).mockReset();
  navigate('profile');
});

afterEach(() => {
  expect(calls).toEqual([]);
  // Unmounted first: a page left mounted would answer the next test's store changes.
  cleanup();
  stayHere();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  profileDraft.set(null);
  libraryGames.set([]);
  gameArt.set({});
  toasts.set([]);
  vi.mocked(getProfileSnapshot).mockReset();
  vi.mocked(getProfilePreview).mockReset();
  document.body.replaceChildren();
});

describe('Profile: the bodies of the page’s own blocks', () => {
  it('each block type gets its own body: the game being played, the recent row, the activity, the stats, the bio', async () => {
    signIn();
    renderDom(Profile);

    const playing = await screen.findByRole('region', { name: msg('social.nowPlayingTitle') });
    expect(within(playing).getByText('Hades')).toBeTruthy();
    expect(within(recentCard() as HTMLElement).getByText('Celeste')).toBeTruthy();
    expect(screen.getByText(msg('social.recentlyPlayedTitle'))).toBeTruthy();
    expect(blockCard(msg('profile.blockStats'))).not.toBeNull();
    expect(blockCard(msg('profile.blockAbout'))?.querySelector('.bio')?.textContent).toBe('Hello there');
  });

  it('opening the game being played goes to that game; an archived one cannot be opened', async () => {
    signIn();
    const { user } = renderDom(Profile);
    const playing = await screen.findByRole('region', { name: msg('social.nowPlayingTitle') });

    await user.click(within(playing).getByRole('button'));

    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
    profileSnapshot.set({ ...SNAPSHOT, running: [ref('g1', 'Hades', { archived: true })] });
    await tick();
    expect((within(playing).getByRole('button') as HTMLButtonElement).disabled).toBe(true);
  });

  it('a block hidden from others carries the badge: the page works the hidden types out, the body shows them', async () => {
    signIn({ showStats: false });
    renderDom(Profile);

    await vi.waitFor(() => expect(blockCard(msg('profile.blockStats'))).not.toBeNull());
    expect(blockCard(msg('profile.blockStats'))?.querySelector('.hidden-flag')).not.toBeNull();
  });

  it('the recent row follows the snapshot, the art and the library stores', async () => {
    signIn();
    renderDom(Profile);
    await vi.waitFor(() => expect(recentCard()).not.toBeNull());
    const card = recentCard() as HTMLElement;
    expect(card.textContent).not.toContain(msg('format.today'));

    libraryGames.set([{ id: 'g2', title: 'Celeste', lastPlayed: new Date().toISOString() } as unknown as LibraryGame]);
    await tick();
    expect(recentCard()?.textContent).toContain(msg('format.today'));

    gameArt.set({ 'c-g2': { cover: 'art-g2.jpg', hero: '', heroIsPoster: false } });
    await tick();
    expect(recentCard()?.innerHTML).toContain('art-g2.jpg');

    profileSnapshot.set({ ...SNAPSHOT, playing: [{ game: ref('g3', 'Inside'), recentSeconds: 60 }] });
    await tick();
    expect(recentCard()?.textContent).toContain('Inside');
  });
});

describe('Profile: through the page', () => {
  async function dirtyEditor() {
    signIn();
    const view = renderDom(Profile);
    await view.user.click(await screen.findByRole('button', { name: msg('profileStyle.edit') }));
    const themes = screen.getByRole('radiogroup', { name: msg('profile.theme') });
    const other = within(themes).getAllByRole('radio').find((radio) => radio.getAttribute('aria-checked') !== 'true') as HTMLElement;
    await view.user.click(other);
    return view;
  }

  it('the leave guard asks while the draft is unsaved, lets the player stay, and keeps the draft for the next visit', async () => {
    const view = await dirtyEditor();

    navigate('catalog');
    expect(get(leaveRequest)?.prompt().title).toBe(msg('profile.leaveTitle'));
    stayHere();
    expect(get(route).name).toBe('profile');
    navigate('catalog');
    confirmLeave();
    expect(get(route).name).toBe('catalog');

    view.unmount();
    expect(get(profileDraft)?.owner).toBe('u1');
    renderDom(Profile);
    await tick();
    expect(screen.getByRole('button', { name: msg('common.save') })).toBeTruthy();
    expect(get(profileDraft)).toBeNull();
    expect(get(toasts).map((t) => t.message)).toContain(msg('profile.draftRestored'));
  });

  it('Save sends the draft without the showcase and leaves edit mode', async () => {
    vi.mocked(saveProfile).mockResolvedValue(true);
    const { user } = await dirtyEditor();

    await user.click(screen.getByRole('button', { name: msg('common.save') }));

    expect(saveProfile).toHaveBeenCalledTimes(1);
    expect(vi.mocked(saveProfile).mock.calls[0][0].profile).not.toHaveProperty('showcase');
    await vi.waitFor(() => expect(screen.queryByRole('button', { name: msg('common.save') })).toBeNull());
  });

  it('the auto theme from the most played reads the preview, and says so when it fails', async () => {
    vi.mocked(getProfilePreview).mockRejectedValue(new Error('offline'));
    signIn({ appearance: { theme: 'auto', autoSource: 'most_played' } as ProfileSettings['appearance'] });
    renderDom(Profile);

    expect(await screen.findByText(msg('profile.previewLoadFailed'))).toBeTruthy();
    expect(getProfilePreview).toHaveBeenCalledTimes(1);
  });
});
