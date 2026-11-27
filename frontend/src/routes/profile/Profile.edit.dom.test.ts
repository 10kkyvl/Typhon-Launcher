import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser, type ProfileAppearance } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot } from '../../lib/services/profile';
import { layoutErrorText } from '../../lib/profile/layout';
import { profileDraft, profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory, stayHere } from '../../lib/stores/router';
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

// Edit mode of the own profile (layout and appearance): the bar's Save, Cancel and Reset with their
// questions, where focus goes when the editor closes, and the appearance panel's controls on the kit.

const button = (name: string) => screen.getByRole('button', { name });
const editControl = () => button(msg('profileStyle.edit'));
// A cover that does not follow the pointer: the parallax asks for matchMedia, which jsdom lacks.
const still = (extra: Partial<ProfileAppearance> = {}): CurrentUser => player({ appearance: { parallax: false, ...extra } as ProfileAppearance });

async function editor(me: CurrentUser = still()) {
  authState.set('authenticated');
  currentUser.set(me);
  const view = renderDom(Profile);
  await view.user.click(await screen.findByRole('button', { name: msg('profileStyle.edit') }));
  return view;
}

// Any change to the draft: another profile theme.
async function change(user: Awaited<ReturnType<typeof editor>>['user']) {
  const themes = screen.getByRole('radiogroup', { name: msg('profile.theme') });
  await user.click(within(themes).getAllByRole('radio').find((radio) => radio.getAttribute('aria-checked') !== 'true') as HTMLElement);
}

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
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

describe("the edit bar's Save", () => {
  it('stays focused and busy until the answer and sends once; then focus goes to the edit control', async () => {
    const me = still();
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = await editor(me);
    await change(user);
    const save = button(msg('common.save'));

    await user.click(save);
    await user.click(save);

    expect(document.activeElement).toBe(save);
    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(save.matches(':disabled')).toBe(false);
    expect(updateProfile).toHaveBeenCalledTimes(1);

    answer.resolve(me);

    await waitFor(() => expect(screen.queryByRole('button', { name: msg('common.save') })).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(editControl()));
  });

  it('a failed save is an alert in the bar, in catalogued words, and Save keeps focus', async () => {
    vi.mocked(updateProfile).mockRejectedValue(new AccountError('network'));
    const { user } = await editor();
    await change(user);
    const save = button(msg('common.save'));

    await user.click(save);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(accountErrorText(new AccountError('network'), msg('profile.saveFailed')));
    expect(alert.closest('.topbar')).not.toBeNull();
    expect(document.activeElement).toBe(save);
  });

  // Chromium drops focus to <body> from a control that turns natively disabled; jsdom keeps it, so the
  // tests read the attributes that decide it.
  it('the connection going while the save waits: the failed save leaves Save focused, refused with its reason', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = await editor();
    await change(user);
    const save = button(msg('common.save'));

    await user.click(save);
    authState.set('offline');
    answer.reject(new AccountError('network'));

    await waitFor(() => expect(save.getAttribute('aria-busy')).toBeNull());
    expect(document.activeElement).toBe(save);
    expect({ disabled: save.matches(':disabled'), ariaDisabled: save.getAttribute('aria-disabled') }).toEqual({ disabled: false, ariaDisabled: 'true' });
    expect(description(save)).toBe(msg('social.editRequiresConnection'));
  });

  it('the connection going while Save holds focus: it stays focused, refused with its reason, and a press sends nothing', async () => {
    const { user } = await editor();
    await change(user);
    const save = button(msg('common.save'));
    save.focus();

    authState.set('offline');

    await waitFor(() => expect(save.getAttribute('aria-disabled')).toBe('true'));
    expect(save.matches(':disabled')).toBe(false);
    expect(document.activeElement).toBe(save);
    expect(description(save)).toBe(msg('social.editRequiresConnection'));
    await user.keyboard('{Enter}');
    expect(updateProfile).not.toHaveBeenCalled();
  });

  it('with nothing changed, online, Save is natively disabled and says no reason', async () => {
    await editor();
    const save = button(msg('common.save'));

    expect(save.matches(':disabled')).toBe(true);
    expect(save.hasAttribute('aria-disabled')).toBe(false);
    expect(description(save)).toBe('');
  });
});

describe("the edit bar's Cancel and Reset", () => {
  it('Cancel with nothing changed leaves at once and focus goes to the edit control', async () => {
    const { user } = await editor();

    await user.click(button(msg('common.cancel')));

    expect(screen.queryByRole('button', { name: msg('common.save') })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(editControl()));
  });

  it('Cancel with changes asks; "Keep editing" stays on Cancel, "Discard changes" leaves and focus goes to the edit control', async () => {
    const { user } = await editor();
    await change(user);
    const cancel = button(msg('common.cancel'));

    await user.click(cancel);
    await user.click(within(screen.getByRole('dialog', { name: msg('profile.discardTitle') })).getByRole('button', { name: msg('profile.keepEditing') }));

    await waitFor(() => expect(document.activeElement).toBe(cancel));
    expect(button(msg('common.save'))).toBeTruthy();

    await user.click(cancel);
    await user.click(within(screen.getByRole('dialog', { name: msg('profile.discardTitle') })).getByRole('button', { name: msg('profile.discardConfirm') }));

    await waitFor(() => expect(screen.queryByRole('button', { name: msg('common.save') })).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(editControl()));
  });

  it('Reset asks before it puts the layout back', async () => {
    const { user } = await editor();

    await user.click(button(msg('profile.resetLayout')));

    expect(screen.getByRole('dialog', { name: msg('profile.resetLayoutTitle') })).toBeTruthy();
  });
});

describe("the appearance panel's controls", () => {
  it('the status text is a text field named by its heading, and a bad text is said at it', async () => {
    const { user } = await editor();
    const status = screen.getByRole('textbox', { name: msg('profileStyle.status') });
    expect(status.closest('.text-field')).not.toBeNull();

    await user.type(status, 'a‮b');

    expect(status.getAttribute('aria-invalid')).toBe('true');
    expect(description(status)).toContain(layoutErrorText({ code: 'bad_chars', field: 'statusText' }));
  });

  it('the gradient angle and the cover dimming are sliders the arrows move', async () => {
    const { user } = await editor(still({ theme: 'custom', customAngle: 90, coverDim: 40 }));
    const angle = screen.getByRole('slider', { name: msg('profileStyle.angle') });
    const dim = screen.getByRole('slider', { name: msg('profile.dim') });
    expect(angle.getAttribute('aria-valuenow')).toBe('90');
    expect(dim.getAttribute('aria-valuenow')).toBe('40');

    angle.focus();
    await user.keyboard('{ArrowRight}');
    dim.focus();
    await user.keyboard('{ArrowLeft}');

    expect(angle.getAttribute('aria-valuenow')).toBe('91');
    expect(dim.getAttribute('aria-valuenow')).toBe('39');
  });

  it('a drag along the angle slider changes the draft at each step, before the release', async () => {
    await editor(still({ theme: 'custom', customAngle: 90 }));
    const angle = screen.getByRole('slider', { name: msg('profileStyle.angle') });
    const box = angle.closest('.slider') as HTMLElement;
    const bar = box.querySelector('.bar') as HTMLElement;
    // jsdom lays out nothing: a track 360 px wide from x = 0, one pixel a degree.
    (box.querySelector('.track') as HTMLElement).getBoundingClientRect = () => ({ left: 0, width: 360, right: 360, top: 0, bottom: 4, height: 4, x: 0, y: 0, toJSON: () => ({}) });
    bar.setPointerCapture = () => {};
    const swatch = () => (screen.getByRole('radio', { name: msg('profileStyle.themeCustom') }).querySelector('.theme-swatch') as HTMLElement).style.background;

    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: 120 });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: 200 });

    expect(angle.getAttribute('aria-valuetext')).toBe('200°');
    expect(swatch()).toContain('200deg');
    await fireEvent.pointerUp(bar, { pointerId: 1, clientX: 200 });
    expect(angle.getAttribute('aria-valuetext')).toBe('200°');
  });

  it('removing the cover hands focus to Upload cover, as the remove button goes', async () => {
    const { user } = await editor(still({ coverUrl: 'https://cdn.example/old.jpg' }));

    await user.click(button(msg('profile.removeCover')));
    await user.click(within(screen.getByRole('dialog', { name: msg('profile.removeCover') })).getByRole('button', { name: msg('profile.removeCover') }));

    await waitFor(() => expect(screen.queryByRole('button', { name: msg('profile.removeCover') })).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(button(msg('profile.upload'))));
  });
});
