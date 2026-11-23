import { render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
  openFolder: vi.fn(),
}));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getAppInfo: vi.fn(),
}));
vi.mock('../../lib/services/overlay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/overlay')>()),
  overlayStatus: vi.fn(),
}));
vi.mock('../../lib/services/accountSync', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/accountSync')>()),
  syncNow: vi.fn(),
  forgetRemote: vi.fn(),
}));
vi.mock('../../lib/services/extlib', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/extlib')>()),
  importExternalGames: vi.fn(),
}));
vi.mock('../../lib/stores/router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/router')>()),
  navigate: vi.fn(),
}));
// The Discord card asks the account backend for the link: answered here as linked, so its rows show.
vi.mock('../../lib/services/discord', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../lib/services/discord')>();
  return {
    ...original,
    discordApi: { ...original.discordApi, getLink: async () => ({ linked: true, discordId: '1', discordUsername: 'player', linkedAt: '' }) },
  };
});

import { msg, type MessageKey } from '../../lib/i18n';
import { forgetRemote, syncNow } from '../../lib/services/accountSync';
import { importExternalGames } from '../../lib/services/extlib';
import { overlayStatus } from '../../lib/services/overlay';
import { openFolder, saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { getAppInfo, type AppInfo } from '../../lib/services/system';
import { navigate } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { user } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import Settings from './Settings.svelte';

const GAME_MODE = { highPerformance: false, hideLauncher: false, raisePriority: false };

const BASE = {
  language: 'system',
  uiScale: 1,
  animationsEnabled: true,
  hardwareAcceleration: true,
  minimizeToTray: true,
  discordRichPresence: false,
  launchOnStartup: false,
  gameMode: GAME_MODE,
  overlayEnabled: true,
  overlayHotkey: 'Alt+`',
  screenshotsEnabled: true,
  screenshotHotkey: 'F12',
  anonymousUsageStats: false,
  anonymousDiagnostics: false,
  sessionHeartbeat: true,
  accountSync: true,
  presenceAutoAway: true,
  lanSharing: false,
  libraryPath: '',
  discordRemoteControl: false,
  discordRemoteDownloads: false,
  discordNotifications: false,
};

const APP: AppInfo = { version: '1.0.0', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' };

let app: HTMLDivElement;

function open(patch: Partial<typeof BASE> & Record<string, unknown> = {}) {
  settings.set({ ...BASE, ...patch } as unknown as SettingsData);
  render(Settings, { target: app });
}

const toastTexts = () => get(toasts).map((item) => item.message);
const sw = (key: MessageKey) => screen.getByRole('switch', { name: msg(key) });
const field = (key: MessageKey) => screen.getByRole('combobox', { name: msg(key) });

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  vi.mocked(getAppInfo).mockResolvedValue(APP);
  vi.mocked(overlayStatus).mockResolvedValue({ supported: true, enabled: true, hotkey: 'Alt+`', error: '' });
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  authState.set('bootstrapping');
  document.body.replaceChildren();
  for (const fn of [saveSettingsPatch, getAppInfo, overlayStatus, syncNow, forgetRemote, importExternalGames, navigate, openFolder]) {
    vi.mocked(fn).mockReset();
  }
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// Each switch of the tab: its name, what describes it, and the one patch a press saves.
const SWITCHES: [label: MessageKey, hint: MessageKey, patch: Record<string, unknown>][] = [
  ['settings.generalLaunchOnStartupLabel', 'settings.generalLaunchOnStartupSub', { launchOnStartup: true }],
  ['settings.generalMinimizeToTrayLabel', 'settings.generalMinimizeToTraySub', { minimizeToTray: false }],
  ['settings.generalDiscordRpcLabel', 'settings.generalDiscordRpcSub', { discordRichPresence: true }],
  ['settings.gameModePowerLabel', 'settings.gameModePowerSub', { gameMode: { ...GAME_MODE, highPerformance: true } }],
  ['settings.gameModeHideLabel', 'settings.gameModeHideSub', { gameMode: { ...GAME_MODE, hideLauncher: true } }],
  ['settings.gameModePriorityLabel', 'settings.gameModePrioritySub', { gameMode: { ...GAME_MODE, raisePriority: true } }],
  ['settings.overlayEnabledLabel', 'settings.overlayEnabledSub', { overlayEnabled: false }],
  ['settings.screenshotsEnabledLabel', 'settings.screenshotsEnabledSub', { screenshotsEnabled: false }],
  ['settings.generalAnimationsLabel', 'settings.generalAnimationsSub', { animationsEnabled: false }],
  ['settings.generalHwAccelLabel', 'settings.generalHwAccelSub', { hardwareAcceleration: false }],
  ['settings.generalPrivacyStatsLabel', 'settings.generalPrivacyStatsSub', { anonymousUsageStats: true }],
  ['settings.generalPrivacyDiagLabel', 'settings.generalPrivacyDiagSub', { anonymousDiagnostics: true }],
  ['settings.generalPrivacyHeartbeatLabel', 'settings.generalPrivacyHeartbeatSub', { sessionHeartbeat: false }],
  ['settings.generalSyncEnableLabel', 'settings.generalSyncEnableSub', { accountSync: false }],
  ['settings.generalAutoAwayLabel', 'settings.generalAutoAwaySub', { presenceAutoAway: false }],
  ['settings.generalLanLabel', 'settings.generalLanSub', { lanSharing: true }],
];

describe('the general tab: every switch', () => {
  for (const [label, hint, patch] of SWITCHES) {
    it(`${label}: named and described by its row, saves ${JSON.stringify(patch)}`, async () => {
      open();
      const control = sw(label);
      expect(control.hasAttribute('disabled')).toBe(false);
      expect(description(control)).toBe(msg(hint));

      await user().click(control);

      await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith(patch));
    });
  }

  it('says a switch that was not saved on the shared toast and shows the held state', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('disk full'));
    open();
    const stats = sw('settings.generalPrivacyStatsLabel');

    await user().click(stats);

    await waitFor(() => expect(toastTexts()).toEqual([msg('state.settingsSaveFailed')]));
    expect(stats.getAttribute('aria-checked')).toBe('false');
  });

  it('says a launch-on-startup failure by its code, else the shared text', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('typhon:overlay.hotkey_unavailable'));
    open();
    const startup = sw('settings.generalLaunchOnStartupLabel');
    await user().click(startup);
    await waitFor(() => expect(toastTexts()).toEqual([msg('overlay.hotkey_unavailable')]));

    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('access denied'));
    await user().click(startup);
    await waitFor(() => expect(toastTexts()).toContain(msg('state.settingsSaveFailed')));
  });
});

describe('the general tab: the Discord card switches', () => {
  it('names and describes each, saves it, and says a failure on its own toast', async () => {
    open({ discordRemoteControl: true });
    const remote = await screen.findByRole('switch', { name: msg('discord.remoteLabel') });
    expect(description(remote)).toBe(msg('discord.remoteSub'));
    const downloads = sw('discord.remoteDownloadsLabel');
    expect(description(downloads)).toBe(msg('discord.remoteDownloadsSub'));
    const notify = sw('discord.notifyLabel');
    expect(description(notify)).toBe(msg('discord.notifySub'));

    await user().click(downloads);
    await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledWith({ discordRemoteDownloads: true }));
    await user().click(notify);
    await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledWith({ discordNotifications: true }));

    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('offline'));
    await user().click(remote);
    await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledWith({ discordRemoteControl: false }));
    await waitFor(() => expect(toastTexts()).toEqual([msg('discord.remoteEnableFailed')]));

    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('offline'));
    await user().click(sw('discord.notifyLabel'));
    await waitFor(() => expect(toastTexts()).toContain(msg('discord.settingFailed')));
  });

  it('shows the remote downloads switch only while remote control is on', async () => {
    open();
    await screen.findByRole('switch', { name: msg('discord.remoteLabel') });
    expect(screen.queryByRole('switch', { name: msg('discord.remoteDownloadsLabel') })).toBeNull();
  });
});

describe('the general tab: every select', () => {
  const SELECTS: [label: MessageKey, hint: MessageKey, option: string, patch: Record<string, unknown>][] = [
    ['settings.overlayHotkeyLabel', 'settings.overlayHotkeySub', 'Shift+F1', { overlayHotkey: 'Shift+F1' }],
    ['settings.screenshotHotkeyLabel', 'settings.screenshotHotkeySub', 'Ctrl+F12', { screenshotHotkey: 'Ctrl+F12' }],
    ['settings.generalUiScaleLabel', 'settings.generalUiScaleSub', '125%', { uiScale: 1.25 }],
    ['settings.language', 'settings.languageSub', 'English', { language: 'en' }],
  ];
  for (const [label, hint, option, patch] of SELECTS) {
    it(`${label}: named by its row, saves ${JSON.stringify(patch)}`, async () => {
      // Refused, so a language or a zoom chosen here does not stay on for the next case.
      vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('refused'));
      open();
      const control = field(label);
      expect(control.hasAttribute('aria-disabled') || control.hasAttribute('disabled')).toBe(false);
      expect(description(control), hint).toBe('');

      await user().click(control);
      await user().click(screen.getByRole('option', { name: option }));

      await waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith(patch));
    });
  }
});

describe('the general tab: disabled rows', () => {
  it('turns launch on startup off in the portable build and says so', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, portable: true });
    open({ launchOnStartup: true });
    const startup = sw('settings.generalLaunchOnStartupLabel');
    await waitFor(() => expect(startup.hasAttribute('disabled')).toBe(true));
    expect(startup.getAttribute('aria-checked')).toBe('false');
    expect(description(startup)).toContain(msg('settings.generalLaunchOnStartupPortable'));
  });

  it('turns hiding during a game off without the tray', () => {
    open({ minimizeToTray: false, gameMode: { ...GAME_MODE, hideLauncher: true } });
    const hide = sw('settings.gameModeHideLabel');
    expect(hide.hasAttribute('disabled')).toBe(true);
    expect(hide.getAttribute('aria-checked')).toBe('false');
    expect(screen.getByText(msg('settings.gameModeHideNeedsTray'))).toBeTruthy();
  });

  it('turns the account rows off without an account', () => {
    authState.set('unauthenticated');
    open();
    expect(sw('settings.generalSyncEnableLabel').hasAttribute('disabled')).toBe(true);
    expect(screen.getByText(msg('settings.generalSyncNeedsAccountSub'))).toBeTruthy();
    expect(sw('settings.generalAutoAwayLabel').hasAttribute('disabled')).toBe(true);
    expect((screen.getByRole('button', { name: msg('settings.generalSyncNowLabel') }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: msg('settings.generalSyncForgetLabel') }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByText(msg('discord.cardTitle'))).toBeNull();
  });

  it('turns auto-away and Sync now off while sync is off', () => {
    open({ accountSync: false });
    expect(sw('settings.generalSyncEnableLabel').hasAttribute('disabled')).toBe(false);
    expect(sw('settings.generalAutoAwayLabel').hasAttribute('disabled')).toBe(true);
    expect((screen.getByRole('button', { name: msg('settings.generalSyncNowLabel') }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: msg('settings.generalSyncForgetLabel') }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('hides the power and priority rows on a Mac', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, platform: 'darwin' });
    open();
    await waitFor(() => expect(screen.queryByRole('switch', { name: msg('settings.gameModePowerLabel') })).toBeNull());
    expect(screen.queryByRole('switch', { name: msg('settings.gameModePriorityLabel') })).toBeNull();
    expect(sw('settings.gameModeHideLabel')).toBeTruthy();
  });

  it('offers the import from other launchers on Windows only', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, platform: 'darwin' });
    open();
    await screen.findByText(msg('settings.extlibWindowsOnly'));
    expect(screen.queryByRole('button', { name: msg('settings.extlibLabel') })).toBeNull();
  });
});

describe('the general tab: the overlay status', () => {
  const region = () => screen.getByText(msg('settings.overlayStatusLabel')).parentElement?.querySelector('[role="status"]') as HTMLElement;

  it('says the overlay works with its hotkey', async () => {
    open();
    await waitFor(() => expect(region().textContent?.trim()).toBe(msg('settings.overlayStatusOn', { hotkey: 'Alt+`' })));
  });

  it('says it is off, or not supported here', async () => {
    vi.mocked(overlayStatus).mockResolvedValue({ supported: true, enabled: false, hotkey: 'Alt+`', error: '' });
    open();
    await waitFor(() => expect(region().textContent?.trim()).toBe(msg('settings.overlayStatusOff')));
    document.body.replaceChildren();
    app = document.createElement('div');
    document.body.append(app);
    vi.mocked(overlayStatus).mockResolvedValue({ supported: false, enabled: false, hotkey: 'Alt+`', error: '' });
    open();
    await waitFor(() => expect(region().textContent?.trim()).toBe(msg('settings.overlayStatusUnsupported')));
  });

  it('says a status it could not read on a danger plate inside the region', async () => {
    vi.mocked(overlayStatus).mockRejectedValue(new Error('down'));
    open();
    await waitFor(() => expect(region().textContent?.trim()).toBe(msg('settings.overlayStatusUnknown')));
    expect(region().querySelector('[data-tone="danger"]')).not.toBeNull();
  });

  it('says a coded overlay failure by its message', async () => {
    vi.mocked(overlayStatus).mockResolvedValue({ supported: true, enabled: true, hotkey: 'Alt+`', error: 'typhon:overlay.hotkey_unavailable: register Alt+`: taken' });
    open();
    await waitFor(() => expect(region().textContent?.trim()).toBe(msg('overlay.hotkey_unavailable')));
  });

  it('says an overlay change that failed by its code on a notice', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('typhon:overlay.hotkey_unavailable'));
    open();
    await user().click(sw('settings.overlayEnabledLabel'));
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('settings.overlaySaveError', { reason: msg('overlay.hotkey_unavailable') }));
  });

  it('says a screenshot change that failed by its code on a toast', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('typhon:overlay.hotkey_unavailable'));
    open();
    await user().click(sw('settings.screenshotsEnabledLabel'));
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.screenshotsSaveError', { reason: msg('overlay.hotkey_unavailable') })]));
  });
});

describe('the general tab: the buttons', () => {
  it('opens the sent data from Show sent data', async () => {
    open();
    await user().click(screen.getByRole('button', { name: msg('settings.generalPrivacyShowSentButton') }));
    expect(await screen.findByRole('dialog')).toBeTruthy();
  });

  it('runs Sync now and says it is done, or says the failure', async () => {
    vi.mocked(syncNow).mockResolvedValueOnce();
    open();
    const button = screen.getByRole('button', { name: msg('settings.generalSyncNowLabel') });
    await user().click(button);
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.generalSyncDoneToast')]));
    expect(syncNow).toHaveBeenCalledOnce();

    vi.mocked(syncNow).mockRejectedValueOnce(new Error('boom'));
    await user().click(button);
    await waitFor(() => expect(toastTexts()).toContain(msg('settings.generalSyncFailedToast')));
  });

  it('asks before it forgets the synced data, then forgets it', async () => {
    vi.mocked(forgetRemote).mockResolvedValueOnce();
    open();
    await user().click(screen.getByRole('button', { name: msg('settings.generalSyncForgetLabel') }));
    const dialog = await screen.findByRole('dialog');
    expect(forgetRemote).not.toHaveBeenCalled();
    const buttons = within(dialog).getAllByRole('button');
    await user().click(buttons[buttons.length - 1]);
    await waitFor(() => expect(forgetRemote).toHaveBeenCalledOnce());
    await waitFor(() => expect(toastTexts()).toContain(msg('settings.generalSyncForgetSuccessToast')));
  });

  it('imports games from other launchers and lists the details', async () => {
    vi.mocked(importExternalGames).mockResolvedValueOnce({
      providers: [{ provider: 'steam', found: 1, added: 0, updated: 0, skipped: [{ title: 'Game', reason: 'no exe' }], notInstalled: false, error: '' }],
      added: 0,
      updated: 0,
      metadataErrors: null,
    });
    open();
    const button = await screen.findByRole('button', { name: msg('settings.extlibLabel') });
    await user().click(button);
    await waitFor(() => expect(importExternalGames).toHaveBeenCalledOnce());
    const detail = await screen.findByText(msg('settings.extlibDetailSkipped', { provider: 'Steam', title: 'Game', reason: 'no exe' }));
    expect(detail.closest('details')?.textContent).toContain(msg('settings.extlibDetailsTitle'));
  });

  it('opens the disk usage page', async () => {
    open();
    await user().click(screen.getByRole('button', { name: msg('diskusage.settingsOpen') }));
    expect(navigate).toHaveBeenCalledWith('diskusage');
  });
});

describe('the general tab: status lines', () => {
  it('shows the LAN state only while LAN sharing is on', () => {
    open({ lanSharing: true });
    expect(screen.getByText(msg('settings.generalLanStateLabel'))).toBeTruthy();
    document.body.replaceChildren();
    app = document.createElement('div');
    document.body.append(app);
    open();
    expect(screen.queryByText(msg('settings.generalLanStateLabel'))).toBeNull();
  });

  it('says what the session state is, with a link to the privacy policy', () => {
    open();
    expect(screen.getByText(msg('settings.generalPrivacySessionStateLabel'))).toBeTruthy();
    expect(screen.getByText(msg('settings.generalPrivacySessionStateSub'))).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('settings.generalPrivacyPolicyLinkLabel') })).toBeTruthy();
  });
});

describe('the general tab: the library card', () => {
  it('without a library: an empty field, a Select that needs the desktop app, an Open that is off', async () => {
    open();
    const select = screen.getByRole('button', { name: msg('common.select') });
    await user().click(select);
    expect(toastTexts()).toEqual([msg('settings.generalLibraryDesktopOnlyToast')]);
    const opens = screen.getAllByRole('button', { name: msg('settings.generalLibraryOpenFolderLabel') });
    expect(opens.map((button) => (button as HTMLButtonElement).disabled)).toEqual([true]);
  });

  it('with a library: the path and its three parts, each opened in the file manager', async () => {
    vi.mocked(openFolder).mockResolvedValue();
    open({ libraryPath: '/lib', gamesPath: '/lib/games', downloadsPath: '/lib/downloads', screenshotsPath: '/lib/shots' });
    const values = screen.getAllByRole('textbox').map((box) => (box as HTMLInputElement).value);
    expect(values).toEqual(['/lib', '/lib/games', '/lib/downloads', '/lib/shots']);
    expect(screen.getByRole('button', { name: msg('common.edit') })).toBeTruthy();
    const opens = screen.getAllByRole('button', { name: msg('settings.generalLibraryOpenFolderLabel') });
    expect(opens).toHaveLength(4);
    for (const button of opens) await user().click(button);
    expect(vi.mocked(openFolder).mock.calls.map(([path]) => path)).toEqual(['/lib', '/lib/games', '/lib/downloads', '/lib/shots']);
  });

  it('says a folder that cannot be opened on a toast', async () => {
    vi.mocked(openFolder).mockRejectedValue(new Error('gone'));
    open({ libraryPath: '/lib', gamesPath: '/lib/games' });
    await user().click(screen.getAllByRole('button', { name: msg('settings.generalLibraryOpenFolderLabel') })[0]);
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.generalLibraryFolderUnavailableToast')]));
  });
});
