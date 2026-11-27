import { cleanup, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfileSnapshot } from '../../lib/services/profile';
import { profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser, savingProfile } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Profile from './Profile.svelte';
import { player, SNAPSHOT } from './profileFixtures';
import { pending } from './profileTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

// The header's own editor (More → Edit): name, username and about on the base text fields, a Save
// that keeps focus while it waits, focus back on the control that opened the editor once it closes,
// and the connection going while the player types.

const me: CurrentUser = player();
const button = (name: string) => screen.getByRole('button', { name });
const field = (name: string) => screen.getByRole('textbox', { name });
const opener = () => button(msg('social.moreLabel'));
// The three fields refuse edits by being read-only, never disabled: a field that turns disabled drops
// the focus it holds in Chromium (jsdom keeps it, so the attributes are read).
const fields = () => [msg('social.displayNameLabel'), msg('social.usernameLabel'), msg('social.aboutTitle')].map((name) => field(name) as HTMLInputElement);
const refusing = () => fields().map((input) => ({ readOnly: input.readOnly, disabled: input.matches(':disabled') }));
const REFUSING = Array(3).fill({ readOnly: true, disabled: false });

async function headerEditor() {
  const view = renderDom(Profile);
  await view.user.click(opener());
  await view.user.click(screen.getByRole('menuitem', { name: msg('social.editProfileLabel') }));
  return view;
}

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
  authState.set('authenticated');
  currentUser.set(me);
  vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(getProfileSnapshot).mockReset();
  vi.mocked(updateProfile).mockReset();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  // A save a failed test left waiting would hold the next test's buttons.
  savingProfile.set(false);
  profileSnapshot.set(EMPTY_SNAPSHOT);
  toasts.set([]);
  document.body.replaceChildren();
});

describe("the header's editor on the base fields", () => {
  it('draws the name and the username as text fields and the about as a text area counting against its limit', async () => {
    const { user } = await headerEditor();
    const about = field(msg('social.aboutTitle'));

    expect(field(msg('social.displayNameLabel')).closest('.text-field')).not.toBeNull();
    expect(field(msg('social.usernameLabel')).closest('.text-field')).not.toBeNull();
    expect(about.closest('.text-area')).not.toBeNull();
    expect(about.closest('.text-area')?.querySelector('.counter')?.textContent).toBe(msg('profile.bioCounter', { count: 0, max: 150 }));

    await user.type(about, 'Hi');

    expect(about.closest('.text-area')?.querySelector('.counter')?.textContent).toBe(msg('profile.bioCounter', { count: 2, max: 150 }));
  });

  it('counts a character outside the basic plane as two, as the field\'s maxlength does', async () => {
    const { user } = await headerEditor();
    const about = field(msg('social.aboutTitle'));

    await user.type(about, '🎮');

    // The browser's maxlength counts UTF-16 units and refuses at the same count the counter shows; the
    // server counts runes and would accept more, so the field is the stricter of the two.
    expect(about.closest('.text-area')?.querySelector('.counter')?.textContent).toBe(msg('profile.bioCounter', { count: 2, max: 150 }));
  });

  it('describes the username by how others will see it, following what is typed', async () => {
    const { user } = await headerEditor();
    const username = field(msg('social.usernameLabel'));
    expect(description(username)).toContain(msg('profile.usernameHint', { username: 'ann' }));

    await user.type(username, 'a');

    expect(description(username)).toContain(msg('profile.usernameHint', { username: 'anna' }));
  });
});

describe('Save and Cancel', () => {
  it('Save stays focused and busy until the answer, sends once, and focus goes back to the opener', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = await headerEditor();
    await user.type(field(msg('social.aboutTitle')), 'Hi');
    const save = button(msg('common.save'));

    await user.click(save);
    await user.click(save);

    expect(document.activeElement).toBe(save);
    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(updateProfile).toHaveBeenCalledTimes(1);
    expect(refusing()).toEqual(REFUSING);

    answer.resolve({ ...me, bio: 'Hi' });

    await waitFor(() => expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull());
    expect(document.activeElement).toBe(opener());
  });

  it('Cancel closes the editor and focus goes back to the opener', async () => {
    const { user } = await headerEditor();

    await user.click(button(msg('common.cancel')));

    expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(opener()));
  });

  it('a refused username is said at that field, which takes focus', async () => {
    vi.mocked(updateProfile).mockRejectedValue(new AccountError('username_taken'));
    const { user } = await headerEditor();
    const username = field(msg('social.usernameLabel'));
    await user.clear(username);
    await user.type(username, 'neo');

    await user.click(button(msg('common.save')));

    await waitFor(() => expect(document.activeElement).toBe(username));
    expect(username.getAttribute('aria-invalid')).toBe('true');
    expect(description(username)).toContain(accountErrorText(new AccountError('username_taken')));
  });
});

describe('the connection going while the player edits', () => {
  it('the form says so, keeps what was typed, and the field keeps its focus', async () => {
    const { user } = await headerEditor();
    const about = field(msg('social.aboutTitle'));
    await user.type(about, 'Hi');

    authState.set('offline');

    const notice = await screen.findByText(msg('profile.editOffline'));
    expect(notice.closest('[data-tone="warning"]')).not.toBeNull();
    expect((field(msg('social.aboutTitle')) as HTMLTextAreaElement).value).toBe('Hi');
    expect(document.activeElement).toBe(about);
    const save = button(msg('common.save'));
    expect({ ariaDisabled: save.getAttribute('aria-disabled'), disabled: save.matches(':disabled') }).toEqual({ ariaDisabled: 'true', disabled: false });
    expect(description(save)).toBe(msg('social.editRequiresConnection'));
    expect(refusing()).toEqual(REFUSING);
  });
});
