import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/profile', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/profile')>()),
  getProfileSnapshot: vi.fn(),
  igdbIdsOf: vi.fn(),
}));

import { locale } from '../../lib/i18n';
import { DEFAULT_PROFILE, type CurrentUser, type ProfileBlock } from '../../lib/services/account';
import type { LibraryGame } from '../../lib/services/library';
import { EMPTY_SNAPSHOT, getProfileSnapshot, igdbIdsOf } from '../../lib/services/profile';
import { libraryGames } from '../../lib/stores/library';
import { profileDraft, profileSnapshot } from '../../lib/stores/profile';
import { authState, currentUser } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import Profile from './Profile.svelte';

// The profile page of a signed-in user whose layout is given, with its editor of blocks: the grid, the
// picker of new blocks and the settings of the selected block.
function signIn(blocks: ProfileBlock[]) {
  const me: CurrentUser = {
    id: 'u1',
    username: 'ann',
    displayName: 'Ann',
    email: 'ann@example.com',
    avatarUrl: '',
    bio: '',
    profile: { ...DEFAULT_PROFILE, layout: { version: 1, blocks } },
    createdAt: '2025-01-01T00:00:00Z',
  };
  currentUser.set(me);
}

const pinned = (igdbId: number): ProfileBlock => ({ id: 'p1', type: 'pinned', width: 'full', config: { igdbId, caption: '' } });
const game = (id: string, title: string) => ({ id, title, canonicalGameId: `c-${id}`, hidden: false }) as unknown as LibraryGame;

beforeEach(() => {
  locale.set('en');
  authState.set('authenticated');
  vi.mocked(getProfileSnapshot).mockResolvedValue(EMPTY_SNAPSHOT);
  vi.mocked(igdbIdsOf).mockReset();
  vi.mocked(igdbIdsOf).mockResolvedValue({});
});

afterEach(() => {
  currentUser.set(null);
  authState.set('bootstrapping');
  libraryGames.set([]);
  profileSnapshot.set(EMPTY_SNAPSHOT);
  profileDraft.set(null);
  document.body.replaceChildren();
});

async function editor() {
  const view = renderDom(Profile);
  await view.user.click(screen.getByRole('button', { name: 'Customize profile' }));
  return view;
}

async function addBlock(name: RegExp) {
  const view = await editor();
  await view.user.click(screen.getByRole('button', { name: 'Add block' }));
  await view.user.click(screen.getByRole('button', { name }));
  return view;
}

async function pinnedSettings() {
  signIn([pinned(0)]);
  const view = await editor();
  await view.user.click(screen.getByRole('button', { name: 'Block settings: Pinned game' }));
  return view;
}

describe('the settings of a block', () => {
  it('a field of the text block is named by its label alone; a refused character marks it and is tied to it', async () => {
    signIn([]);
    const { user } = await addBlock(/^Text/);
    const title = screen.getByRole('textbox', { name: 'Title' });
    expect(screen.getByRole('textbox', { name: 'Text' })).toBeTruthy();

    await user.click(title);
    await user.paste('Hi‮');

    expect(title.getAttribute('aria-invalid')).toBe('true');
    const error = document.getElementById(title.getAttribute('aria-describedby') ?? '');
    expect(error?.getAttribute('role')).toBe('alert');
    expect(error?.textContent).toMatch(/^The text has characters that are not allowed/);
  });

  it('a custom collection without games shows the empty state in its settings', async () => {
    signIn([]);
    await addBlock(/^Custom collection/);

    const settings = screen.getByRole('region', { name: 'Block settings: Collection' });
    expect(within(settings).getByText('Add at least one game').closest('.empty')).not.toBeNull();
  });
});

describe('the game picker of a block', () => {
  it('a list being loaded is the spinner, named "Loading…"', async () => {
    libraryGames.set([game('g1', 'Hades')]);
    vi.mocked(igdbIdsOf).mockReturnValue(new Promise(() => {}));
    await pinnedSettings();

    const status = screen.getAllByRole('status').filter((element) => element.textContent?.trim() === 'Loading…');
    expect(status).toHaveLength(1);
    expect(status[0].classList.contains('spinner')).toBe(true);
  });

  it('no game to pick and no match are empty states', async () => {
    const { user } = await pinnedSettings();
    expect((await screen.findByText(/^No game in your library was found/)).closest('.empty')).not.toBeNull();

    libraryGames.set([game('g1', 'Hades')]);
    vi.mocked(igdbIdsOf).mockResolvedValue({ 'c-g1': '7' });
    await screen.findByRole('button', { name: 'Hades' });
    await user.type(screen.getByRole('textbox', { name: 'Find a game in your library' }), 'zzz');

    expect(screen.getByText('Nothing found').closest('.empty')).not.toBeNull();
  });

  it('Retry keeps the failed notice on screen, busy and focused, until the list answers', async () => {
    libraryGames.set([game('g1', 'Hades')]);
    let answer: (found: Record<string, string>) => void = () => {};
    vi.mocked(igdbIdsOf)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user } = await pinnedSettings();
    const alert = await screen.findByRole('alert');
    const notice = alert.closest('[data-tone="danger"]') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: 'Retry' });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(igdbIdsOf).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(within(notice).getByRole('button', { name: 'Retry' })).toBe(retry);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);

    answer({ 'c-g1': '7' });

    await waitFor(() => expect(notice.isConnected).toBe(false));
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Hades' }));
  });

  it('a Retry that fails again keeps the notice and its focus, and says the failure again', async () => {
    libraryGames.set([game('g1', 'Hades')]);
    vi.mocked(igdbIdsOf).mockRejectedValue(new Error('offline'));
    const { user } = await pinnedSettings();
    const alert = await screen.findByRole('alert');
    const before = alert.firstElementChild;
    const notice = alert.closest('[data-tone="danger"]') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: 'Retry' });
    retry.focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(retry.getAttribute('aria-busy')).toBeNull());
    expect(igdbIdsOf).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    // Said again: the message comes into the alert again, as new nodes, once the retry has failed.
    await waitFor(() => expect(alert.firstElementChild).not.toBe(before));
    expect(alert.textContent).toContain('Could not load the list of games');
  });
});

describe('the blocks of the profile', () => {
  it('a block whose game could not be loaded says so in a danger notice inside its card', async () => {
    signIn([pinned(42)]);
    vi.mocked(getProfileSnapshot).mockResolvedValue({ ...EMPTY_SNAPSHOT, layoutGames: { '42': { error: 'unavailable' } } });
    renderDom(Profile);

    const text = await screen.findByText('Could not load');
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('a block that still needs its game shows the empty state and an "Incomplete" badge in the editor', async () => {
    signIn([pinned(0)]);
    await editor();

    const empty = screen.getByText('Pick a game in the block settings');
    expect(empty.closest('.empty')).not.toBeNull();
    expect(screen.getByText('Incomplete').classList.contains('badge')).toBe(true);
  });

  it('a full layout holding blocks of a newer launcher says so in a standing warning notice', async () => {
    signIn([...Array.from({ length: 15 }, (_, index) => ({ id: `t${index}`, type: 'text', width: 'half' as const, config: { title: '', body: '' } })), { id: 'f', type: 'future', width: 'full', config: {} }]);
    await editor();

    const notice = screen.getByText(/^Some blocks were created by a newer version/).closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });
});
