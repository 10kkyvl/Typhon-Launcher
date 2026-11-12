import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/stores/user', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/user')>()),
  saveProfile: vi.fn(),
}));

import { locale } from '../../lib/i18n';
import { AccountError, DEFAULT_PROFILE, type CurrentUser } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { renderDom } from '../../lib/testing/dom';
import { authState, currentUser, saveProfile } from '../../lib/stores/user';
import Profile from './Profile.svelte';

// The profile page of a signed-in user with the header's own editor open (More → Edit): name, username,
// about.
const me: CurrentUser = {
  id: 'u1',
  username: 'ann',
  displayName: 'Ann',
  email: 'ann@example.com',
  avatarUrl: '',
  bio: '',
  profile: DEFAULT_PROFILE,
  createdAt: '2025-01-01T00:00:00Z',
};

beforeEach(() => {
  locale.set('en');
  authState.set('authenticated');
  currentUser.set(me);
  vi.mocked(saveProfile).mockReset();
});

afterEach(() => {
  currentUser.set(null);
  authState.set('bootstrapping');
  document.body.replaceChildren();
});

async function headerEditor() {
  const view = renderDom(Profile);
  await view.user.click(screen.getByRole('button', { name: 'More' }));
  await view.user.click(screen.getByRole('menuitem', { name: 'Edit' }));
  return view;
}

describe('the profile header editor', () => {
  it('names each field by its label alone, and a refused username marks that field and is tied to it', async () => {
    vi.mocked(saveProfile).mockRejectedValue(new AccountError('username_taken'));
    const { user } = await headerEditor();
    const username = screen.getByRole('textbox', { name: 'Username' });
    const displayName = screen.getByRole('textbox', { name: 'Display name' });
    expect(screen.getByRole('textbox', { name: 'About' })).toBeTruthy();

    await user.clear(username);
    await user.type(username, 'neo');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    const text = accountErrorText(new AccountError('username_taken'));
    expect(screen.getByRole('textbox', { name: 'Username' })).toBe(username);
    expect(username.getAttribute('aria-invalid')).toBe('true');
    const error = document.getElementById(username.getAttribute('aria-describedby') ?? '');
    expect(error?.textContent).toBe(text);
    expect(error?.getAttribute('role')).toBe('alert');
    expect(displayName.getAttribute('aria-invalid')).not.toBe('true');
    expect(displayName.hasAttribute('aria-describedby')).toBe(false);
  });

  it('a save that is refused as a whole says so in a danger notice', async () => {
    vi.mocked(saveProfile).mockResolvedValue(false);
    const { user } = await headerEditor();
    await user.type(screen.getByRole('textbox', { name: 'About' }), 'Hi');

    await user.click(screen.getByRole('button', { name: 'Save' }));

    const alert = screen.getByRole('alert');
    expect(alert.textContent?.trim()).toBe('The previous save is still running. Try again in a moment.');
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});
