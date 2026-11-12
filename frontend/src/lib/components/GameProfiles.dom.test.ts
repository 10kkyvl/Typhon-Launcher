import { screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { Candidate } from '../services/install';
import { addProfile, createProfileShortcut, getProfiles, type LibraryGame, type ProfileView } from '../services/library';
import { toasts } from '../stores/toasts';
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
