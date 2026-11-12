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
