import { cleanup, fireEvent, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfileSnapshot } from '../../lib/services/profile';
import { profileHeaderDraft, profileSnapshot } from '../../lib/stores/profile';
import { get } from 'svelte/store';
import { leaveRequest, navigate, resetHistory, route, stayHere } from '../../lib/stores/router';
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
  stayHere();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  // A save a failed test left waiting would hold the next test's buttons.
  savingProfile.set(false);
  profileSnapshot.set(EMPTY_SNAPSHOT);
  // The page unmounted on the profile route keeps the header's typed text for the next opening.
  profileHeaderDraft.set(null);
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

describe('opening the editor', () => {
  it('puts focus in its first field, the display name; Cancel gives it back to More', async () => {
    const { user } = await headerEditor();

    await waitFor(() => expect(document.activeElement).toBe(field(msg('social.displayNameLabel'))));

    await user.click(button(msg('common.cancel')));
    await waitFor(() => expect(document.activeElement).toBe(opener()));
  });

  it('opened by the keyboard from More, puts focus in the display name', async () => {
    const { user } = renderDom(Profile);
    opener().focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByRole('menuitem', { name: msg('social.editProfileLabel') })).toBeTruthy());
    screen.getByRole('menuitem', { name: msg('social.editProfileLabel') }).focus();
    await user.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(field(msg('social.displayNameLabel'))));
  });
});

describe('More → Edit while the editor is already open', () => {
  async function editAgain(user: Awaited<ReturnType<typeof headerEditor>>['user']) {
    await user.click(opener());
    await user.click(screen.getByRole('menuitem', { name: msg('social.editProfileLabel') }));
  }

  it('keeps the typed text and puts focus in the display name', async () => {
    const { user } = await headerEditor();
    await user.type(field(msg('social.aboutTitle')), 'Hi');

    await editAgain(user);

    expect((field(msg('social.aboutTitle')) as HTMLTextAreaElement).value).toBe('Hi');
    await waitFor(() => expect(document.activeElement).toBe(field(msg('social.displayNameLabel'))));
  });

  it('during a save that then fails, keeps the typed text, and leaving still asks', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = await headerEditor();
    await user.type(field(msg('social.aboutTitle')), 'Hi');
    await user.click(button(msg('common.save')));

    await editAgain(user);
    answer.reject(new AccountError('network_error'));

    await waitFor(() => expect(button(msg('common.save')).getAttribute('aria-busy')).toBeNull());
    expect((field(msg('social.aboutTitle')) as HTMLTextAreaElement).value).toBe('Hi');
    navigate('catalog');
    expect(get(route).name).toBe('profile');
    expect(get(leaveRequest)?.prompt().text).toBe(msg('profile.leaveHeaderText'));
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

  // A pointer press in the macOS WebKit view moves no focus: Cancel keeps the focus it had while the
  // save starts, and a natively disabled button would drop it to <body> in Chromium.
  it('Cancel holding focus while a save starts keeps it, refused with the saving words, and a press closes nothing', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = await headerEditor();
    await user.type(field(msg('social.aboutTitle')), 'Hi');
    const cancel = button(msg('common.cancel'));
    cancel.focus();

    await fireEvent.click(button(msg('common.save')));

    await waitFor(() => expect(cancel.getAttribute('aria-disabled')).toBe('true'));
    expect({ disabled: cancel.matches(':disabled'), reason: description(cancel) }).toEqual({ disabled: false, reason: msg('social.saving') });
    expect(document.activeElement).toBe(cancel);
    await user.keyboard('{Enter}');
    expect((field(msg('social.aboutTitle')) as HTMLTextAreaElement).value).toBe('Hi');

    answer.resolve({ ...me, bio: 'Hi' });
    await waitFor(() => expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull());
  });

  it('Cancel with no save running is free and says no reason', async () => {
    await headerEditor();
    const cancel = button(msg('common.cancel'));

    expect({ ariaDisabled: cancel.getAttribute('aria-disabled'), disabled: cancel.matches(':disabled'), reason: description(cancel) }).toEqual({ ariaDisabled: null, disabled: false, reason: '' });
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

describe("the editor's Save when it is refused", () => {
  const refusal = (save: HTMLElement) => ({ ariaDisabled: save.getAttribute('aria-disabled'), disabled: save.matches(':disabled'), reason: description(save) });

  it('with nothing typed, stays in the Tab order, refused, and says there is nothing to save', async () => {
    const { user } = await headerEditor();
    const save = button(msg('common.save'));

    expect(refusal(save)).toEqual({ ariaDisabled: 'true', disabled: false, reason: msg('state.accountErrorNoChanges') });
    save.focus();
    await user.keyboard('{Enter}');
    expect(updateProfile).not.toHaveBeenCalled();
  });

  it('unchanged and focused while the connection goes and comes back, keeps focus and changes only its reason', async () => {
    await headerEditor();
    authState.set('offline');
    const save = button(msg('common.save'));
    save.focus();

    await waitFor(() => expect(description(save)).toBe(msg('social.editRequiresConnection')));
    authState.set('authenticated');

    await waitFor(() => expect(description(save)).toBe(msg('state.accountErrorNoChanges')));
    expect(refusal(save)).toEqual({ ariaDisabled: 'true', disabled: false, reason: msg('state.accountErrorNoChanges') });
    expect(document.activeElement).toBe(save);
  });

  it('with text typed and online, is free and says no reason', async () => {
    const { user } = await headerEditor();
    await user.type(field(msg('social.aboutTitle')), 'Hi');

    expect(refusal(button(msg('common.save')))).toEqual({ ariaDisabled: null, disabled: false, reason: '' });
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
