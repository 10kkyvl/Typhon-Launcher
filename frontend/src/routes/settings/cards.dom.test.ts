import { screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
// The library rows and the move row act only in the desktop app.
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
  libraryFolders: vi.fn(),
}));
vi.mock('../../lib/services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/emulators')>()),
  listPlatforms: vi.fn(),
  detectEmulators: vi.fn(),
  setEmulatorPath: vi.fn(),
}));
vi.mock('../../lib/services/network', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/network')>()),
  hasProxyPassword: vi.fn(async () => false),
  testProxy: vi.fn(),
}));
vi.mock('../../lib/services/relocate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/relocate')>()),
  selectMoveTargetFolder: vi.fn(async () => '/new'),
  moveLibrary: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { detectEmulators, listPlatforms, setEmulatorPath, type PlatformStatus } from '../../lib/services/emulators';
import type { DiscordApi } from '../../lib/services/discord';
import { testProxy } from '../../lib/services/network';
import { moveLibrary } from '../../lib/services/relocate';
import { libraryFolders, saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import DiscordCard from './DiscordCard.svelte';
import EmulatorsCard from './EmulatorsCard.svelte';
import LibraryFoldersRow from './LibraryFoldersRow.svelte';
import LibraryLocationRow from './LibraryLocationRow.svelte';
import NetworkSettingsCard from './NetworkSettingsCard.svelte';

const PLATFORM: PlatformStatus = {
  id: 'ps2',
  name: 'PlayStation 2',
  emulator: 'PCSX2',
  homepage: '',
  extensions: [],
  imageNames: [],
  path: '',
  status: 'not_set',
  reason: '',
  warnings: [],
};

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  settings.set({ libraryPath: '/games', gamesPath: '/games/g', extraLibraryPaths: [], networkMode: 'direct' } as unknown as SettingsData);
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);
});

afterEach(() => {
  document.body.replaceChildren();
  settings.set(null);
  vi.restoreAllMocks();
  vi.mocked(listPlatforms).mockReset();
  vi.mocked(libraryFolders).mockReset();
  vi.mocked(testProxy).mockReset();
  vi.mocked(moveLibrary).mockReset();
  vi.mocked(saveSettingsPatch).mockReset();
});

describe('EmulatorsCard', () => {
  it('says it is loading through a named spinner', async () => {
    const list = deferred<PlatformStatus[]>();
    vi.mocked(listPlatforms).mockReturnValue(list.promise);
    renderDom(EmulatorsCard, {});
    expect(screen.getByRole('status').textContent?.trim()).toBe(msg('emul.loading'));
    list.resolve([PLATFORM]);
    await waitFor(() => expect(screen.queryByRole('status')).toBeNull());
  });

  it('keeps the load error and focus on the busy Retry until the list arrives', async () => {
    vi.mocked(listPlatforms).mockRejectedValueOnce(new Error('down'));
    const { user } = renderDom(EmulatorsCard, {});
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('emul.loadFailed'));
    const retry = screen.getByRole('button', { name: msg('common.retry') });

    const list = deferred<PlatformStatus[]>();
    vi.mocked(listPlatforms).mockReturnValueOnce(list.promise);
    retry.focus();
    await user.keyboard('{Enter}');

    expect(retry.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    expect(screen.getByRole('alert').textContent).toContain(msg('emul.loadFailed'));

    list.resolve([PLATFORM]);
    await screen.findByText(`${PLATFORM.name} · ${PLATFORM.emulator}`);
    expect(screen.queryByRole('alert')).toBeNull();
    // Focus moves once the rows that replaced the notice are drawn, to their first control.
    await waitFor(() => expect(document.activeElement).toBe(document.querySelector('.rows')?.querySelector('button')));
    // The notice went before its retry answered: the card still learns the retry is over.
    expect((screen.getByRole('button', { name: msg('emul.detect') }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('shows a new error in place when the retry fails again', async () => {
    vi.mocked(listPlatforms).mockRejectedValue(new Error('down'));
    const { user } = renderDom(EmulatorsCard, {});
    await screen.findByRole('alert');
    const retry = screen.getByRole('button', { name: msg('common.retry') });
    retry.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(retry.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    // Said again though the words are the same: they come again into the alert.
    const again = screen.getByRole('alert').querySelector('[data-part="notice-again"]');
    await waitFor(() => expect(again?.textContent).toBe(msg('emul.loadFailed')));
  });
});

describe('EmulatorsCard: the found emulators', () => {
  it('ticks each found emulator for a later Apply, as a checkbox described by its path', async () => {
    vi.mocked(listPlatforms).mockResolvedValue([PLATFORM]);
    vi.mocked(detectEmulators).mockResolvedValueOnce({ found: [{ platform: 'ps2', path: '/apps/pcsx2' }], unreadable: 0 });
    vi.mocked(setEmulatorPath).mockResolvedValue({ ...PLATFORM, path: '/apps/pcsx2', status: 'ready' });
    const { user } = renderDom(EmulatorsCard, {});
    await screen.findByText(`${PLATFORM.name} · ${PLATFORM.emulator}`);
    await user.click(screen.getByRole('button', { name: msg('emul.detect') }));

    const dialog = await screen.findByRole('dialog', { name: msg('emul.detectTitle') });
    const box = within(dialog).getByRole('checkbox', { name: 'PCSX2' });
    expect(box.getAttribute('aria-checked')).toBe('true');
    expect(description(box)).toBe('/apps/pcsx2');
    const apply = within(dialog).getByRole('button', { name: msg('emul.detectApply') }) as HTMLButtonElement;

    box.focus();
    await user.keyboard(' ');
    expect(box.getAttribute('aria-checked')).toBe('false');
    expect(apply.disabled).toBe(true);
    await user.keyboard(' ');
    await user.click(apply);
    expect(setEmulatorPath).toHaveBeenCalledWith('ps2', '/apps/pcsx2');
  });
});

describe('DiscordCard', () => {
  const api = (getLink: DiscordApi['getLink']): DiscordApi => ({
    getLink,
    createLinkCode: vi.fn(),
    unlink: vi.fn(),
  });

  it('says it is checking the link through a named spinner', () => {
    renderDom(DiscordCard, { api: api(() => new Promise(() => {})) });
    expect(screen.getByRole('status').textContent?.trim()).toBe(msg('discord.loading'));
  });

  it('keeps the load error and focus on the busy Retry until the answer arrives', async () => {
    const answer = deferred<Awaited<ReturnType<DiscordApi['getLink']>>>();
    const getLink = vi.fn<DiscordApi['getLink']>().mockRejectedValueOnce(new Error('down')).mockReturnValueOnce(answer.promise);
    const { user } = renderDom(DiscordCard, { api: api(getLink) });
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('discord.loadFailed'));
    const retry = screen.getByRole('button', { name: msg('discord.retry') });

    retry.focus();
    await user.keyboard('{Enter}');

    expect(retry.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    expect(screen.queryByText(msg('discord.loading'))).toBeNull();

    answer.resolve({ linked: false, discordId: '', discordUsername: '', linkedAt: '' });
    await screen.findByRole('button', { name: msg('discord.linkButton') });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement?.isConnected).toBe(true);
  });
});

describe('NetworkSettingsCard: the proxy', () => {
  const PROXY = { networkMode: 'proxy', proxyType: 'http', proxyHost: 'proxy.lan', proxyPort: 3128, proxyUsername: 'me' };
  beforeEach(() => {
    settings.set({ ...PROXY } as unknown as SettingsData);
  });

  it('opens on the saved proxy, its fields filled and nothing to apply', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    expect(screen.getByRole('radio', { name: msg('settings.networkModeProxy') }).getAttribute('aria-checked')).toBe('true');
    expect((screen.getByRole('textbox', { name: msg('settings.networkProxyHostLabel') }) as HTMLInputElement).value).toBe('proxy.lan');
    expect((screen.getByRole('textbox', { name: msg('settings.networkProxyPortLabel') }) as HTMLInputElement).value).toBe('3128');
    expect((screen.getByRole('textbox', { name: msg('settings.networkProxyUsernameLabel') }) as HTMLInputElement).value).toBe('me');
    const apply = screen.getByRole('button', { name: msg('settings.networkApply') }) as HTMLButtonElement;
    expect(apply.disabled).toBe(true);
    await user.click(apply);
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });

  it('follows a save made elsewhere while the card is untouched, and keeps an edit', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    const host = screen.getByRole('textbox', { name: msg('settings.networkProxyHostLabel') }) as HTMLInputElement;
    settings.set({ ...PROXY, proxyHost: 'other.lan' } as unknown as SettingsData);
    await waitFor(() => expect(host.value).toBe('other.lan'));

    const port = screen.getByRole('textbox', { name: msg('settings.networkProxyPortLabel') }) as HTMLInputElement;
    await user.clear(port);
    await user.type(port, '8080');
    settings.set({ ...PROXY, proxyHost: 'third.lan' } as unknown as SettingsData);
    await tick();
    expect(port.value).toBe('8080');
    expect(host.value).toBe('other.lan');
  });

  it('marks a bad port on its field and says why there', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    const port = screen.getByRole('textbox', { name: msg('settings.networkProxyPortLabel') });
    expect(port.hasAttribute('aria-invalid')).toBe(false);
    await user.clear(port);
    await user.type(port, 'abc');

    await user.click(screen.getByRole('button', { name: msg('settings.networkApply') }));

    expect(port.getAttribute('aria-invalid')).toBe('true');
    const said = document.getElementById(port.getAttribute('aria-describedby') ?? '');
    expect(said?.textContent).toBe(msg('settings.proxy_port_invalid'));
    expect(said?.getAttribute('role')).toBe('alert');
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });

  it('clears the bad port mark once the port is valid, and says it again on the next refusal', async () => {
    const { user } = renderDom(NetworkSettingsCard, {});
    const port = screen.getByRole('textbox', { name: msg('settings.networkProxyPortLabel') });
    const apply = screen.getByRole('button', { name: msg('settings.networkApply') });
    await user.clear(port);
    await user.type(port, 'abc');
    await user.click(apply);
    const first = screen.getByRole('alert');

    await user.click(apply);
    const again = screen.getByRole('alert');
    expect(again).not.toBe(first);
    expect(first.isConnected).toBe(false);

    await user.clear(port);
    await user.type(port, '8080');
    expect(port.hasAttribute('aria-invalid')).toBe(false);
    expect(port.hasAttribute('aria-describedby')).toBe(false);
    expect(screen.queryByRole('alert')).toBeNull();

    await user.clear(port);
    await user.type(port, 'x');
    expect(port.hasAttribute('aria-invalid')).toBe(false);
    await user.click(apply);
    expect(port.getAttribute('aria-invalid')).toBe('true');
    expect(screen.getByRole('alert').textContent).toBe(msg('settings.proxy_port_invalid'));
  });

  it('shows the proxy warning on a warning notice', async () => {
    renderDom(NetworkSettingsCard, {});
    const plate = screen.getByText(msg('settings.networkProxyWarning')).closest('[data-tone]');
    expect(plate?.getAttribute('data-tone')).toBe('warning');
  });

  it('announces the result of a proxy check', async () => {
    vi.mocked(testProxy).mockResolvedValueOnce();
    const { user } = renderDom(NetworkSettingsCard, {});
    const check = screen.getByRole('button', { name: msg('settings.networkProxyTest') });
    check.focus();
    await user.keyboard('{Enter}');
    const result = await screen.findByText(msg('settings.networkProxyTestOk'));
    expect(result.closest('[role="status"]')).not.toBeNull();
    expect(result.closest('[data-tone]')?.getAttribute('data-tone')).toBe('success');
  });
});

describe('the library rows', () => {
  it('report a folder list that cannot be read on a danger notice', async () => {
    vi.mocked(libraryFolders).mockRejectedValue(new Error('down'));
    renderDom(LibraryFoldersRow, {});
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('settings.libraryFoldersLoadFailed'));
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
  });

  it('report a move that cannot start in an alert', async () => {
    vi.mocked(moveLibrary).mockRejectedValueOnce(new Error('busy'));
    const { user } = renderDom(LibraryLocationRow, {});
    const move = screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') });
    move.focus();
    await user.keyboard('{Enter}');
    const alert = await screen.findByRole('alert');
    expect(within(alert).getByText(msg('settings.generalLibraryMoveStartFailed'))).toBeTruthy();
  });
});
