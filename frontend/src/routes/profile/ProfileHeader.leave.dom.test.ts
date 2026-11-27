import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser, type ProfileAppearance } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot } from '../../lib/services/profile';
import { profileDraft, profileSnapshot } from '../../lib/stores/profile';
import { confirmLeave, leaveRequest, navigate, resetHistory, route, stayHere } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser, savingProfile } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
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

// Leaving the page while the header's own editor (More → Edit) holds typed text: the page asks through
// the one leave guard it shares with the edit bar, and a save that fails once the page is gone is said
// once, in a toast. The page is left by the router; the test unmounts it, as the app's view does.

// A cover that does not follow the pointer: the parallax asks for matchMedia, which jsdom lacks.
const me: CurrentUser = player({ appearance: { parallax: false } as ProfileAppearance });
const other: CurrentUser = player({ appearance: { parallax: false } as ProfileAppearance }, { id: 'u2', username: 'bob', displayName: 'Bob', email: 'bob@example.com' });
const button = (name: string) => screen.getByRole('button', { name });
const about = () => screen.getByRole('textbox', { name: msg('social.aboutTitle') }) as HTMLTextAreaElement;
const said = () => get(toasts).map((toast) => [toast.kind, toast.message]);

async function headerEditor() {
  const view = renderDom(Profile);
  await view.user.click(button(msg('social.moreLabel')));
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
  vi.mocked(getProfilePreview).mockResolvedValue(SNAPSHOT);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [getProfileSnapshot, getProfilePreview, updateProfile]) vi.mocked(mock).mockReset();
  stayHere();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  savingProfile.set(false);
  profileSnapshot.set(EMPTY_SNAPSHOT);
  profileDraft.set(null);
  toasts.set([]);
  document.body.replaceChildren();
});

describe('leaving with typed text in the header editor', () => {
  it('asks first, saying the typed text is lost; Stay keeps it, Leave goes', async () => {
    const { user } = await headerEditor();
    await user.type(about(), 'Hi');

    navigate('catalog');

    expect(get(route).name).toBe('profile');
    expect(get(leaveRequest)?.prompt()).toEqual({
      title: msg('profile.leaveTitle'),
      text: msg('profile.leaveHeaderText'),
      confirm: msg('profile.leaveConfirm'),
      cancel: msg('profile.leaveStay'),
    });
    stayHere();
    expect(get(route).name).toBe('profile');
    expect(about().value).toBe('Hi');

    navigate('catalog');
    confirmLeave();
    expect(get(route).name).toBe('catalog');
  });

  it('with nothing typed it lets the page go without a question', async () => {
    await headerEditor();

    navigate('catalog');

    expect(get(leaveRequest)).toBeNull();
    expect(get(route).name).toBe('catalog');
  });

  it('with the edit bar holding changes too, asks one question that names both', async () => {
    const { user } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('profileStyle.edit')));
    const themes = screen.getByRole('radiogroup', { name: msg('profile.theme') });
    await user.click(within(themes).getAllByRole('radio').find((radio) => radio.getAttribute('aria-checked') !== 'true') as HTMLElement);

    navigate('catalog');

    expect(get(leaveRequest)?.prompt().text).toBe(msg('profile.leaveBothText'));
    confirmLeave();
    expect(get(route).name).toBe('catalog');
    expect(get(leaveRequest)).toBeNull();
  });
});

describe('a header save still on its way when the page is left', () => {
  it('failing, is said once in a toast naming what was not saved, in catalogued words', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.save')));

    navigate('catalog');
    confirmLeave();
    unmount();
    answer.reject(new AccountError('network_error'));

    const failed = msg('profile.headerNotSaved', { reason: accountErrorText(new AccountError('network_error')) });
    await waitFor(() => expect(said()).toContainEqual(['danger', failed]));
    expect(said().filter(([, message]) => message === failed)).toHaveLength(1);
    expect(said().filter(([kind]) => kind === 'danger')).toHaveLength(1);
  });

  it('answering, says the profile was updated and nothing failed', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.save')));

    navigate('catalog');
    confirmLeave();
    unmount();
    answer.resolve({ ...me, bio: 'Hi' });

    await waitFor(() => expect(said()).toContainEqual(['success', msg('social.profileUpdated')]));
    expect(said().filter(([kind]) => kind === 'danger')).toEqual([]);
  });
});

describe('a header save answered after another account signed in', () => {
  // A pressed Save and left; A signed out and B signed in; then the server answers A's save.
  async function answeredUnderAnother(answer: (late: ReturnType<typeof pending<CurrentUser>>) => void) {
    const late = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(late.promise);
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.save')));

    navigate('catalog');
    confirmLeave();
    unmount();
    currentUser.set(other);
    answer(late);

    await waitFor(() => expect(get(savingProfile)).toBe(false));
    await new Promise((settled) => setTimeout(settled, 0));
  }

  it('saved by the server, says nothing to the other account, and never that it was not saved', async () => {
    await answeredUnderAnother((late) => late.resolve({ ...me, bio: 'Hi' }));

    expect(said()).toEqual([]);
  });

  it('failed, says nothing about the first account to the other one', async () => {
    await answeredUnderAnother((late) => late.reject(new AccountError('network_error')));

    expect(said()).toEqual([]);
  });
});

describe('a failed header save on the page', () => {
  it('is said in the editor, not in a toast', async () => {
    vi.mocked(updateProfile).mockRejectedValue(new AccountError('network_error'));
    const { user } = await headerEditor();
    await user.type(about(), 'Hi');

    await user.click(button(msg('common.save')));

    expect(await screen.findByText(accountErrorText(new AccountError('network_error'), msg('social.saveFailed')))).toBeTruthy();
    expect(said().filter(([kind]) => kind === 'danger')).toEqual([]);
  });
});
