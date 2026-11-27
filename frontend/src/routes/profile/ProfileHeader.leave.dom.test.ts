import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser, type ProfileAppearance } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot } from '../../lib/services/profile';
import { profileDraft, profileHeaderDraft, profileSnapshot } from '../../lib/stores/profile';
import { confirmLeave, leaveRequest, navigate, resetHistory, route, stayHere } from '../../lib/stores/router';
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

// Leaving the page while the header's own editor (More → Edit) holds typed text: the page asks through
// the one leave guard it shares with the edit bar, and a save that fails once the page is gone is said
// once, in a toast. The page is left by the router; the test unmounts it, as the app's view does.

// A cover that does not follow the pointer: the parallax asks for matchMedia, which jsdom lacks.
const me: CurrentUser = player({ appearance: { parallax: false } as ProfileAppearance });
const other: CurrentUser = player({ appearance: { parallax: false } as ProfileAppearance }, { id: 'u2', username: 'bob', displayName: 'Bob', email: 'bob@example.com' });
const button = (name: string) => screen.getByRole('button', { name });
const about = () => screen.getByRole('textbox', { name: msg('social.aboutTitle') }) as HTMLTextAreaElement;
const aboutOrNull = () => screen.queryByRole('textbox', { name: msg('social.aboutTitle') });
const headerSave = () => screen.getAllByRole('button', { name: msg('common.save') }).find((save) => !save.closest('.topbar')) as HTMLElement;
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
  profileHeaderDraft.set(null);
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

  it('typed, then Cancel: the closed editor asks nothing when the page is left', async () => {
    const { user } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.cancel')));

    navigate('catalog');

    expect(get(leaveRequest)).toBeNull();
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
  // Nothing asks while the header's own save runs: its answer is still to come, and a failure is said
  // in a toast once the page is gone.
  it('failing, is said once in a toast naming what was not saved, in catalogued words', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.save')));

    navigate('catalog');
    expect(get(leaveRequest)).toBeNull();
    expect(get(route).name).toBe('catalog');
    unmount();
    answer.reject(new AccountError('network_error'));

    const failed = msg('profile.headerNotSavedKept', { reason: accountErrorText(new AccountError('network_error')) });
    await waitFor(() => expect(said()).toContainEqual(['danger', failed]));
    expect(said().filter(([, message]) => message === failed)).toHaveLength(1);
    expect(said().filter(([kind]) => kind === 'danger')).toHaveLength(1);
  });

  it('refused, keeps the typed text: the toast says where, and the next opening of the page shows it', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user, unmount } = await headerEditor();
    const username = screen.getByRole('textbox', { name: msg('social.usernameLabel') });
    await user.clear(username);
    await user.type(username, 'neo');
    await user.click(button(msg('common.save')));

    navigate('catalog');
    expect(get(leaveRequest)).toBeNull();
    unmount();
    answer.reject(new AccountError('username_taken'));

    const refused = msg('profile.headerNotSavedKept', { reason: accountErrorText(new AccountError('username_taken')) });
    await waitFor(() => expect(said()).toEqual([['danger', refused]]));
    navigate('profile');
    renderDom(Profile);
    expect((screen.getByRole('textbox', { name: msg('social.usernameLabel') }) as HTMLInputElement).value).toBe('neo');
    expect(headerSave().getAttribute('aria-disabled')).toBeNull();
    navigate('catalog');
    expect(get(leaveRequest)?.prompt().text).toBe(msg('profile.leaveHeaderText'));
  });

  // The page opened again before the late refusal came, and the player settled newer text there.
  for (const settle of ['saved', 'cancelled'] as const) {
    it(`refused while the page is open again, then the newer text ${settle}: the next opening shows no older text`, async () => {
      const late = pending<CurrentUser>();
      vi.mocked(updateProfile).mockReturnValueOnce(late.promise).mockResolvedValueOnce({ ...me, bio: 'Hello' });
      const first = await headerEditor();
      await first.user.type(about(), 'Hi');
      await first.user.click(button(msg('common.save')));
      navigate('catalog');
      first.unmount();
      navigate('profile');
      const second = renderDom(Profile);
      await second.user.click(button(msg('social.moreLabel')));
      await second.user.click(screen.getByRole('menuitem', { name: msg('social.editProfileLabel') }));
      late.reject(new AccountError('network_error'));
      await waitFor(() => expect(get(savingProfile)).toBe(false));
      await waitFor(() => expect(about().readOnly).toBe(false));
      await second.user.type(about(), 'Hello');

      if (settle === 'saved') {
        await second.user.click(headerSave());
        await waitFor(() => expect(get(currentUser)?.bio).toBe('Hello'));
      } else {
        await second.user.click(button(msg('common.cancel')));
      }
      await waitFor(() => expect(aboutOrNull()).toBeNull());
      navigate('catalog');
      second.unmount();
      navigate('profile');
      renderDom(Profile);

      expect(aboutOrNull()).toBeNull();
    });
  }

  it('answering, says the profile was updated and nothing failed', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.save')));

    navigate('catalog');
    expect(get(leaveRequest)).toBeNull();
    expect(get(route).name).toBe('catalog');
    unmount();
    answer.resolve({ ...me, bio: 'Hi' });

    await waitFor(() => expect(said()).toContainEqual(['success', msg('social.profileUpdated')]));
    expect(said().filter(([kind]) => kind === 'danger')).toEqual([]);
  });
});

describe("leaving while the header's own save runs and the edit bar holds changes", () => {
  it("asks about the bar's changes only", async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('profileStyle.edit')));
    const themes = screen.getByRole('radiogroup', { name: msg('profile.theme') });
    await user.click(within(themes).getAllByRole('radio').find((radio) => radio.getAttribute('aria-checked') !== 'true') as HTMLElement);
    await user.click(screen.getAllByRole('button', { name: msg('common.save') }).find((save) => !save.closest('.topbar')) as HTMLElement);

    navigate('catalog');

    expect(get(route).name).toBe('profile');
    expect(get(leaveRequest)?.prompt().text).toBe(msg('profile.leaveText'));
    stayHere();
    answer.resolve({ ...me, bio: 'Hi' });
    await waitFor(() => expect(get(savingProfile)).toBe(false));
  });
});

// The bar's save is its own: while it runs, the header's typed text is still unsaved and still the
// player's to lose or keep.
describe("the edit bar's save running while the header editor holds typed text", () => {
  async function barSaving() {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const view = await headerEditor();
    await view.user.type(about(), 'Hi');
    await view.user.click(button(msg('profileStyle.edit')));
    const themes = screen.getByRole('radiogroup', { name: msg('profile.theme') });
    await view.user.click(within(themes).getAllByRole('radio').find((radio) => radio.getAttribute('aria-checked') !== 'true') as HTMLElement);
    await view.user.click(screen.getAllByRole('button', { name: msg('common.save') }).find((save) => save.closest('.topbar')) as HTMLElement);
    expect(get(savingProfile)).toBe(true);
    return { ...view, answer };
  }

  it('leaving asks about both editors', async () => {
    const { answer } = await barSaving();

    navigate('catalog');

    expect(get(route).name).toBe('profile');
    expect(get(leaveRequest)?.prompt().text).toBe(msg('profile.leaveBothText'));
    stayHere();
    answer.resolve(me);
    await waitFor(() => expect(get(savingProfile)).toBe(false));
  });

  it('Big Picture entered keeps the header text for the next opening', async () => {
    const { answer, unmount } = await barSaving();

    unmount();
    renderDom(Profile);

    expect(about().value).toBe('Hi');
    answer.resolve(me);
    await waitFor(() => expect(get(savingProfile)).toBe(false));
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

  it('failed, says nothing about the first account to the other one, and keeps nothing', async () => {
    await answeredUnderAnother((late) => late.reject(new AccountError('network_error')));

    expect(said()).toEqual([]);
    currentUser.set(me);
    navigate('profile');
    renderDom(Profile);
    expect(aboutOrNull()).toBeNull();
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

// Big Picture takes the desktop shell away while the route stays on the profile: no leave question is
// asked, so the typed text is kept and the editor opens with it when the page is next shown.
describe('the page taken away while the player is still on it (Big Picture)', () => {
  it('keeps the typed text, and the next opening of the page shows the editor with it', async () => {
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');

    unmount();
    renderDom(Profile);

    expect(about().value).toBe('Hi');
    expect(updateProfile).not.toHaveBeenCalled();
    navigate('catalog');
    expect(get(leaveRequest)?.prompt().text).toBe(msg('profile.leaveHeaderText'));
  });

  it('keeps it for that account only', async () => {
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    unmount();

    currentUser.set(other);
    const second = renderDom(Profile);

    expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull();
    second.unmount();
    currentUser.set(me);
    renderDom(Profile);
    expect(about().value).toBe('Hi');
  });

  it('keeps nothing once the player left through the question', async () => {
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    navigate('catalog');
    confirmLeave();
    unmount();

    navigate('profile');
    renderDom(Profile);

    expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull();
  });

  it('keeps nothing while its save is on its way', async () => {
    vi.mocked(updateProfile).mockReturnValue(pending<CurrentUser>().promise);
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.save')));

    unmount();
    savingProfile.set(false);
    renderDom(Profile);

    expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull();
  });

  it('keeps nothing the player cancelled', async () => {
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    await user.click(button(msg('common.cancel')));

    unmount();
    renderDom(Profile);

    expect(aboutOrNull()).toBeNull();
  });

  it('keeps nothing from an editor opened and left as it was', async () => {
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'x');
    await user.type(about(), '{Backspace}');

    unmount();
    renderDom(Profile);

    expect(aboutOrNull()).toBeNull();
  });
});

// The kept text is taken where the editor opens, and only once: an opening of the page, offline too, or
// a fresh More → Edit. Once taken it is nowhere else, so it never comes back over newer text.
describe('the kept text taken by the next opening of the editor', () => {
  async function keptHi() {
    const { user, unmount } = await headerEditor();
    await user.type(about(), 'Hi');
    unmount();
  }

  it('opens with it while offline, read-only and saying so, and Save is free once the connection is back', async () => {
    await keptHi();
    authState.set('offline');

    renderDom(Profile);

    expect(about().value).toBe('Hi');
    expect(about().readOnly).toBe(true);
    expect(screen.getByText(msg('profile.editOffline'))).toBeTruthy();
    expect(description(headerSave())).toBe(msg('social.editRequiresConnection'));
    authState.set('authenticated');
    await waitFor(() => expect(headerSave().getAttribute('aria-disabled')).toBeNull());
    expect(about().value).toBe('Hi');
  });

  it('is taken by a fresh More → Edit when the page opened under another account', async () => {
    await keptHi();
    currentUser.set(other);
    const { user } = renderDom(Profile);
    expect(aboutOrNull()).toBeNull();

    currentUser.set(me);
    await user.click(button(msg('social.moreLabel')));
    await user.click(screen.getByRole('menuitem', { name: msg('social.editProfileLabel') }));

    expect(about().value).toBe('Hi');
  });

  it('taken, then left through the question: the next opening does not bring it back', async () => {
    await keptHi();
    const second = renderDom(Profile);
    expect(about().value).toBe('Hi');

    navigate('catalog');
    confirmLeave();
    second.unmount();
    navigate('profile');
    renderDom(Profile);

    expect(aboutOrNull()).toBeNull();
  });

  it('opened offline, then newer text saved: the next opening does not bring the older text back', async () => {
    await keptHi();
    authState.set('offline');
    const second = renderDom(Profile);
    expect(about().value).toBe('Hi');
    authState.set('authenticated');
    await waitFor(() => expect(about().readOnly).toBe(false));
    vi.mocked(updateProfile).mockResolvedValue({ ...me, bio: 'Hello' });
    await second.user.clear(about());
    await second.user.type(about(), 'Hello');
    await second.user.click(headerSave());
    await waitFor(() => expect(aboutOrNull()).toBeNull());

    navigate('catalog');
    second.unmount();
    navigate('profile');
    renderDom(Profile);

    expect(aboutOrNull()).toBeNull();
    expect(get(currentUser)?.bio).toBe('Hello');
  });
});
