import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, type CurrentUser, type ProfilePatch } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot } from '../../lib/services/profile';
import { uploadProfileCover } from '../../lib/services/profileCover';
import { updateProfile } from '../../lib/services/account';
import { profileDraft, profileFailed, profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory, stayHere } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
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

// What the own profile page does today and keeps doing through the stage: the header's own editor
// saved and cancelled, the cover uploaded and removed in the draft, the guest's header, the figures
// once the snapshot has come, the privacy dialog saving a switch and saying a failure, and a draft of
// another player not restored. The leave guard's question and the draft restored after a visit are
// held in Profile.parts.dom.test.ts and Profile.leave.test.ts.

const said = () => get(toasts).map((toast) => [toast.kind, toast.message]);
const button = (name: string) => screen.getByRole('button', { name });

function signIn(me: CurrentUser = player()) {
  authState.set('authenticated');
  currentUser.set(me);
}

// The server's answer to a profile update: the player as the patch leaves them.
function saves(me: CurrentUser) {
  vi.mocked(updateProfile).mockImplementation(async (patch: ProfilePatch) => ({
    ...me,
    ...(patch.displayName !== undefined ? { displayName: patch.displayName } : {}),
    ...(patch.username !== undefined ? { username: patch.username } : {}),
    ...(patch.bio !== undefined ? { bio: patch.bio } : {}),
    profile: patch.profile ? { ...me.profile, ...patch.profile, showcase: me.profile.showcase } : me.profile,
  }));
}

async function headerEditor() {
  const view = renderDom(Profile);
  await view.user.click(button(msg('social.moreLabel')));
  await view.user.click(screen.getByRole('menuitem', { name: msg('social.editProfileLabel') }));
  return view;
}

async function privacy() {
  const view = renderDom(Profile);
  await view.user.click(button(msg('social.moreLabel')));
  await view.user.click(screen.getByRole('menuitem', { name: msg('profile.privacy') }));
  return { ...view, dialog: screen.getByRole('dialog', { name: msg('social.profileSettingsTitle') }) };
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
  for (const mock of [getProfileSnapshot, getProfilePreview, updateProfile, uploadProfileCover]) vi.mocked(mock).mockReset();
  vi.unstubAllGlobals();
  stayHere();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  profileFailed.set(false);
  profileDraft.set(null);
  toasts.set([]);
  document.body.replaceChildren();
});

describe("the header's own editor", () => {
  it('saves the changed name alone, closes and says the profile was updated', async () => {
    const me = player();
    signIn(me);
    saves(me);
    const { user } = await headerEditor();
    const name = screen.getByRole('textbox', { name: msg('social.displayNameLabel') });

    await user.clear(name);
    await user.type(name, 'Anna');
    await user.click(button(msg('common.save')));

    await waitFor(() => expect(screen.queryByRole('textbox', { name: msg('social.displayNameLabel') })).toBeNull());
    expect(vi.mocked(updateProfile).mock.calls).toEqual([[{ displayName: 'Anna' }]]);
    expect(said()).toContainEqual(['success', msg('social.profileUpdated')]);
    expect(screen.getByRole('heading', { name: 'Anna' })).toBeTruthy();
  });

  it('Cancel closes the editor and sends nothing', async () => {
    signIn();
    const { user } = await headerEditor();
    await user.type(screen.getByRole('textbox', { name: msg('social.aboutTitle') }), 'Hi');

    await user.click(button(msg('common.cancel')));

    expect(screen.queryByRole('textbox', { name: msg('social.aboutTitle') })).toBeNull();
    expect(updateProfile).not.toHaveBeenCalled();
  });
});

describe('the cover in the appearance panel', () => {
  // jsdom decodes no picture: this one says it loaded, at a size the panel takes.
  class LoadedImage {
    naturalWidth = 1600;
    naturalHeight = 400;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(_url: string) {
      queueMicrotask(() => this.onload?.());
    }
  }

  // A cover that does not follow the pointer: the parallax asks for matchMedia, which jsdom lacks.
  const still = (coverUrl = '') => player({ appearance: { coverUrl, parallax: false } as CurrentUser['profile']['appearance'] });

  async function editor() {
    const view = renderDom(Profile);
    await view.user.click(await screen.findByRole('button', { name: msg('profileStyle.edit') }));
    return view;
  }

  it('an animated cover is uploaded as it is and stands in the draft until Save', async () => {
    vi.stubGlobal('Image', LoadedImage);
    signIn(still());
    vi.mocked(uploadProfileCover).mockResolvedValue('https://cdn.example/cover.gif');
    const { user, container } = await editor();
    const input = container.ownerDocument.querySelector<HTMLInputElement>('input[type="file"]') as HTMLInputElement;

    await user.upload(input, new File(['GIF89a'], 'cover.gif', { type: 'image/gif' }));

    await waitFor(() => expect(uploadProfileCover).toHaveBeenCalledTimes(1));
    expect(vi.mocked(uploadProfileCover).mock.calls[0][0]).toBe(btoa('GIF89a'));
    expect(await screen.findByRole('button', { name: msg('profile.removeCover') })).toBeTruthy();
    expect(updateProfile).not.toHaveBeenCalled();
  });

  it('Remove cover asks, then takes the cover out of the draft only; Save sends it without one', async () => {
    const me = still('https://cdn.example/old.jpg');
    signIn(me);
    saves(me);
    const { user } = await editor();

    await user.click(button(msg('profile.removeCover')));
    const ask = screen.getByRole('dialog', { name: msg('profile.removeCover') });
    await user.click(within(ask).getByRole('button', { name: msg('profile.removeCover') }));

    await waitFor(() => expect(screen.queryByRole('button', { name: msg('profile.removeCover') })).toBeNull());
    expect(updateProfile).not.toHaveBeenCalled();
    expect(uploadProfileCover).not.toHaveBeenCalled();

    await user.click(button(msg('common.save')));

    await waitFor(() => expect(updateProfile).toHaveBeenCalledTimes(1));
    expect(vi.mocked(updateProfile).mock.calls[0][0].profile?.appearance?.coverUrl).toBe('');
  });
});

describe('the guest', () => {
  it('sees the guest header with sign in and create account, and no figures, blocks or editor', async () => {
    authState.set('guest');
    renderDom(Profile);

    expect(screen.getByRole('heading', { name: msg('social.guestName') })).toBeTruthy();
    expect(button(msg('social.signInButton'))).toBeTruthy();
    expect(button(msg('social.createAccountButton'))).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('profileStyle.edit') })).toBeNull();
    expect(screen.queryByText(msg('social.statGames'))).toBeNull();
    expect(document.querySelector('.blocks')).toBeNull();
  });
});

describe('the first read', () => {
  it("once the snapshot has come the header shows the player's figures", async () => {
    signIn();
    renderDom(Profile);

    const games = await screen.findByText('12');
    expect(games.closest('.tile')?.textContent).toContain(msg('social.statGames'));
    expect(screen.getByText('340').closest('.tile')?.textContent).toContain(msg('social.statHours'));
    expect(screen.getByText('5').closest('.tile')?.textContent).toContain(msg('social.statCompleted'));
  });
});

describe('the privacy dialog', () => {
  it('a switch turned off is saved, the dialog closes and says so', async () => {
    const me = player({ showOnline: true });
    signIn(me);
    saves(me);
    const { user, dialog } = await privacy();

    await user.click(within(dialog).getByRole('switch', { name: msg('social.flagOnlineLabel') }));
    await user.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    await waitFor(() => expect(screen.queryByRole('dialog', { name: msg('social.profileSettingsTitle') })).toBeNull());
    expect(updateProfile).toHaveBeenCalledTimes(1);
    expect(vi.mocked(updateProfile).mock.calls[0][0].profile?.showOnline).toBe(false);
    expect(said()).toContainEqual(['success', msg('social.settingsSaved')]);
  });

  it('a second Save while the first is on its way sends nothing more', async () => {
    const me = player();
    signIn(me);
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user, dialog } = await privacy();
    const save = within(dialog).getByRole('button', { name: msg('common.save') });

    await user.click(save);
    await user.click(within(dialog).getByRole('button', { name: msg('social.saving') }));

    expect(updateProfile).toHaveBeenCalledTimes(1);
    answer.resolve(me);
    await waitFor(() => expect(screen.queryByRole('dialog', { name: msg('social.profileSettingsTitle') })).toBeNull());
  });

  it('a failed save is said in the dialog in catalogued words, and the dialog stays open', async () => {
    signIn();
    vi.mocked(updateProfile).mockRejectedValue(new AccountError('network'));
    const { user, dialog } = await privacy();

    await user.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(accountErrorText(new AccountError('network'), msg('social.saveFailed')));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(screen.getByRole('dialog', { name: msg('social.profileSettingsTitle') })).toBe(dialog);
  });
});

describe('the draft kept between visits', () => {
  it("is not restored for another player: the page opens out of the editor and leaves the other's draft", async () => {
    signIn();
    const draft = { owner: 'u2', draft: player().profile, reset: false };
    profileDraft.set(draft);

    renderDom(Profile);

    expect(await screen.findByRole('button', { name: msg('profileStyle.edit') })).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('common.save') })).toBeNull();
    expect(get(profileDraft)).toBe(draft);
  });
});
