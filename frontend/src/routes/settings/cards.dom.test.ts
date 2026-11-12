import { screen, waitFor, within } from '@testing-library/svelte';
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
import { listPlatforms, type PlatformStatus } from '../../lib/services/emulators';
import type { DiscordApi } from '../../lib/services/discord';
import { testProxy } from '../../lib/services/network';
import { moveLibrary } from '../../lib/services/relocate';
import { libraryFolders, saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { settings } from '../../lib/stores/settings';
import { renderDom } from '../../lib/testing/dom';
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
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement?.isConnected).toBe(true);
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
  // A saved proxy with nothing filled in: choosing the proxy mode on the card matches what is saved.
  beforeEach(() => {
    settings.set({ networkMode: 'proxy', proxyType: 'socks5', proxyHost: '', proxyPort: 0, proxyUsername: '' } as unknown as SettingsData);
  });

  async function proxyMode() {
    const view = renderDom(NetworkSettingsCard, {});
    await view.user.click(screen.getByRole('radio', { name: msg('settings.networkModeProxy') }));
    return view;
  }

  it('marks a bad port on its field and says why there', async () => {
    const { user } = await proxyMode();
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

  it('shows the proxy warning on a warning notice', async () => {
    await proxyMode();
    const plate = screen.getByText(msg('settings.networkProxyWarning')).closest('[data-tone]');
    expect(plate?.getAttribute('data-tone')).toBe('warning');
  });

  it('announces the result of a proxy check', async () => {
    vi.mocked(testProxy).mockResolvedValueOnce();
    const { user } = await proxyMode();
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
