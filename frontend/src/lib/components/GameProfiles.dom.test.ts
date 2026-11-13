import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { Candidate } from '../services/install';
import { addProfile, createProfileShortcut, getProfiles, type LibraryGame, type ProfileView } from '../services/library';
import { toasts } from '../stores/toasts';
import { get } from 'svelte/store';
import { renderDom } from '../testing/dom';
import GameProfiles from './GameProfiles.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  getProfiles: vi.fn(),
  profileSteamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
  addProfile: vi.fn(),
  createProfileShortcut: vi.fn(),
}));

const game = { id: 'g1', title: 'Hades', installDir: '/games/hades', executable: '/games/hades/Hades.exe', profiles: [] } as unknown as LibraryGame;

function profile(patch: Partial<ProfileView> = {}): ProfileView {
  return { id: 'p1', name: 'Vulkan', executable: '/games/hades/Hades.exe', argsLine: '', runAsAdmin: false, isDefault: false, isLast: false, ...patch };
}

const candidates: Candidate[] = [
  { path: '/games/hades/Hades.exe' } as Candidate,
  { path: '/games/hades/x64/Hades.exe' } as Candidate,
  { path: '/games/hades/tools/Config.exe' } as Candidate,
];

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

afterEach(() => {
  vi.mocked(getProfiles).mockReset();
  vi.mocked(addProfile).mockReset();
  vi.mocked(createProfileShortcut).mockReset();
  toasts.set([]);
  document.body.replaceChildren();
});

describe('GameProfiles: a profile row', () => {
  it('edits and deletes with named icon buttons that Enter presses', async () => {
    vi.mocked(getProfiles).mockResolvedValue([profile()]);
    const { user } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const edit = await screen.findByRole('button', { name: msg('modals.profilesEdit') });
    expect(screen.getByRole('button', { name: msg('modals.profilesDelete') })).toBeTruthy();
    expect(edit.classList.contains('icon-btn')).toBe(true);

    edit.focus();
    await user.keyboard('{Enter}');

    expect((screen.getByRole('textbox', { name: msg('modals.profilesName') }) as HTMLInputElement).value).toBe('Vulkan');
  });

  it('marks the last used profile and one run as administrator with status badges', async () => {
    vi.mocked(getProfiles).mockResolvedValue([profile({ isLast: true, runAsAdmin: true })]);
    renderDom(GameProfiles, { game, candidates: [], adminShown: true });

    for (const label of [msg('modals.profilesLast'), msg('modals.propertiesAdminTitle')]) {
      const badge = await screen.findByText(label, { selector: 'li *' });
      expect(badge.classList.contains('badge')).toBe(true);
    }
  });
});

describe('GameProfiles: opening and closing the editor from the keyboard', () => {
  it('Edit puts the focus in the editor, and Cancel gives it back to Edit', async () => {
    vi.mocked(getProfiles).mockResolvedValue([profile()]);
    const { user } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const edit = await screen.findByRole('button', { name: msg('modals.profilesEdit') });
    edit.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('textbox', { name: msg('modals.profilesName') })));

    screen.getByRole('button', { name: msg('common.cancel') }).focus();
    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('modals.profilesEdit') })));
  });

  it('"Add profile" puts the focus in the editor, and a save gives it back to "Add profile"', async () => {
    vi.mocked(getProfiles).mockResolvedValue([]);
    vi.mocked(addProfile).mockResolvedValue(profile());
    const { user } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const add = await screen.findByRole('button', { name: msg('modals.profilesAdd') });
    add.focus();

    await user.keyboard('{Enter}');

    const name = screen.getByRole('textbox', { name: msg('modals.profilesName') });
    await vi.waitFor(() => expect(document.activeElement).toBe(name));
    await user.type(name, 'Safe');
    screen.getByRole('button', { name: msg('modals.profilesSave') }).focus();
    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('modals.profilesAdd') })));
  });
});

describe('GameProfiles: presses that did not hold the focus', () => {
  it('open and close the editor without moving the focus', async () => {
    vi.mocked(getProfiles).mockResolvedValue([profile()]);
    renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const edit = await screen.findByRole('button', { name: msg('modals.profilesEdit') });
    expect(document.activeElement).toBe(document.body);

    edit.click();
    const name = await screen.findByRole('textbox', { name: msg('modals.profilesName') });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(document.body);
    expect(name.isConnected).toBe(true);

    screen.getByRole('button', { name: msg('common.cancel') }).click();
    await vi.waitFor(() => expect(name.isConnected).toBe(false));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(document.body);
  });
});

describe('GameProfiles: the edited profile deleted under its editor', () => {
  it('Cancel gives the focus to "Add profile" when the Edit that opened the editor is gone', async () => {
    vi.mocked(getProfiles).mockResolvedValueOnce([profile()]).mockResolvedValue([]);
    const { user, rerender } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const edit = await screen.findByRole('button', { name: msg('modals.profilesEdit') });
    edit.focus();
    await user.keyboard('{Enter}');
    await rerender({ game: { ...game, lastProfile: 'gone' } as unknown as LibraryGame, candidates: [], adminShown: false });
    await vi.waitFor(() => expect(screen.queryByRole('button', { name: msg('modals.profilesEdit') })).toBeNull());

    screen.getByRole('button', { name: msg('common.cancel') }).focus();
    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('modals.profilesAdd') })));
  });
});

describe('GameProfiles: "Add profile" while the list reloads after a save', () => {
  const changed = { ...game, profiles: [{ id: 'p9', name: 'Safe' }] } as unknown as LibraryGame;

  async function fillNewProfile() {
    vi.mocked(getProfiles).mockResolvedValueOnce([]).mockReturnValue(new Promise(() => {}));
    const view = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const add = await screen.findByRole('button', { name: msg('modals.profilesAdd') });
    add.focus();
    await view.user.keyboard('{Enter}');
    await view.user.type(screen.getByRole('textbox', { name: msg('modals.profilesName') }), 'Safe');
    screen.getByRole('button', { name: msg('modals.profilesSave') }).focus();
    return view;
  }

  it('takes the focus back though the library\'s update came before the save\'s answer', async () => {
    const saving = deferred<ProfileView>();
    vi.mocked(addProfile).mockReturnValueOnce(saving.promise);
    const { user, rerender } = await fillNewProfile();
    await user.keyboard('{Enter}');

    await rerender({ game: changed, candidates: [], adminShown: false });
    saving.resolve(profile({ id: 'p9', name: 'Safe' }));

    const add = screen.getByRole('button', { name: msg('modals.profilesAdd') });
    await vi.waitFor(() => expect(document.activeElement).toBe(add));
    expect(add).toHaveProperty('disabled', false);
  });

  it('keeps the focus though the library\'s update comes after the save\'s answer', async () => {
    vi.mocked(addProfile).mockResolvedValueOnce(profile({ id: 'p9', name: 'Safe' }));
    const { user, rerender } = await fillNewProfile();
    await user.keyboard('{Enter}');
    const add = screen.getByRole('button', { name: msg('modals.profilesAdd') });
    await vi.waitFor(() => expect(document.activeElement).toBe(add));

    await rerender({ game: changed, candidates: [], adminShown: false });

    expect(add).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(add);
  });
});

describe('GameProfiles: messages', () => {
  it('says a failed load in a danger notice', async () => {
    vi.mocked(getProfiles).mockRejectedValue(new Error('disk'));
    renderDom(GameProfiles, { game, candidates: [], adminShown: false });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.profilesLoadFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('says a failed save in a danger notice inside the editor', async () => {
    vi.mocked(getProfiles).mockResolvedValue([]);
    vi.mocked(addProfile).mockRejectedValue(new Error('disk'));
    const { user } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    await user.click(await screen.findByRole('button', { name: msg('modals.profilesAdd') }));
    await user.type(screen.getByRole('textbox', { name: msg('modals.profilesName') }), 'Safe');

    await user.click(screen.getByRole('button', { name: msg('modals.profilesSave') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.profilesSaveFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('keeps the focus on the shortcut button, busy, while the shortcut is made', async () => {
    const made = deferred<void>();
    vi.mocked(getProfiles).mockResolvedValue([profile()]);
    vi.mocked(createProfileShortcut).mockReturnValueOnce(made.promise);
    const { user } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });
    const shortcut = await screen.findByRole('button', { name: msg('modals.profilesShortcut') });
    shortcut.focus();

    await user.keyboard('{Enter}');

    expect(shortcut.getAttribute('aria-busy')).toBe('true');
    expect(shortcut).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(shortcut);
    made.resolve();
    await vi.waitFor(() => expect(shortcut.getAttribute('aria-busy')).toBeNull());
  });

  it('says a failed shortcut once, in the toast; the notice in the list stays without saying it again', async () => {
    vi.mocked(getProfiles).mockResolvedValue([profile()]);
    vi.mocked(createProfileShortcut).mockRejectedValueOnce(new Error('disk'));
    const { user } = renderDom(GameProfiles, { game, candidates: [], adminShown: false });

    await user.click(await screen.findByRole('button', { name: msg('modals.profilesShortcut') }));

    const text = await screen.findByText(msg('modals.profilesShortcutFailed'), { selector: '[data-tone="danger"] *' });
    expect(text.closest('[role="alert"]')).toBeNull();
    expect(get(toasts).map((item) => item.message)).toContain(msg('modals.profilesShortcutFailed'));
  });
});

describe('GameProfiles: the executable of a profile', () => {
  it('is chosen from the found files in a radio group named by its label: the arrows move the choice', async () => {
    vi.mocked(getProfiles).mockResolvedValue([]);
    const { user } = renderDom(GameProfiles, { game, candidates, adminShown: false });
    await user.click(await screen.findByRole('button', { name: msg('modals.profilesAdd') }));

    const group = screen.getByRole('radiogroup', { name: msg('modals.profilesExe') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['true', 'false', 'false']);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([0, -1, -1]);

    radios[0].focus();
    await user.keyboard('{ArrowDown}');

    expect(radios[1].getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(radios[1]);
    expect(screen.getByText('/games/hades/x64/Hades.exe', { selector: 'p' })).toBeTruthy();
  });
});
