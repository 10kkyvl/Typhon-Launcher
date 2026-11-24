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
vi.mock('../../lib/services/system', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../lib/services/system')>();
  return { ...original, getAppInfo: vi.fn(), getWineStatus: vi.fn(original.getWineStatus) };
});
vi.mock('../../lib/services/legal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/legal')>()),
  listLegalDocuments: vi.fn(async () => [{ id: 'terms', title: 'Terms of use' }]),
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
import { listLegalDocuments } from '../../lib/services/legal';
import { getAppInfo, getWineStatus, type AppInfo } from '../../lib/services/system';
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

const said = (...keys: MessageKey[]) => keys.map((key) => msg(key)).join(' ');
const button = (key: MessageKey) => screen.getByRole('button', { name: msg(key) }) as HTMLButtonElement;
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
  for (const fn of [listLegalDocuments, getWineStatus, saveSettingsPatch, getAppInfo, overlayStatus, syncNow, forgetRemote, importExternalGames, navigate, openFolder]) {
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

// A row's warning note is read with its hint.
const NOTES: Partial<Record<MessageKey, MessageKey>> = { 'settings.overlayEnabledLabel': 'settings.overlayExclusiveNote' };

describe('the general tab: every switch', () => {
  for (const [label, hint, patch] of SWITCHES) {
    it(`${label}: named and described by its row, saves ${JSON.stringify(patch)}`, async () => {
      open();
      // The power and priority rows come with the app facts.
      const control = await screen.findByRole('switch', { name: msg(label) });
      expect(control.hasAttribute('disabled')).toBe(false);
      expect(description(control)).toBe(NOTES[label] ? said(hint, NOTES[label]) : msg(hint));

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
      expect(description(control)).toBe(msg(hint));

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
    expect(description(startup)).toBe(said('settings.generalLaunchOnStartupPortable', 'settings.generalLaunchOnStartupSub'));
  });

  it('turns hiding during a game off without the tray', () => {
    open({ minimizeToTray: false, gameMode: { ...GAME_MODE, hideLauncher: true } });
    const hide = sw('settings.gameModeHideLabel');
    expect(hide.hasAttribute('disabled')).toBe(true);
    expect(hide.getAttribute('aria-checked')).toBe('false');
    expect(description(hide)).toBe(said('settings.gameModeHideNeedsTray', 'settings.gameModeHideSub'));
  });

  it('turns the account rows off without an account', () => {
    authState.set('unauthenticated');
    open();
    const account = 'settings.generalSyncNeedsAccountSub';
    const sync = sw('settings.generalSyncEnableLabel');
    expect(sync.hasAttribute('disabled')).toBe(true);
    expect(description(sync)).toBe(said(account, 'settings.generalSyncEnableSub'));
    const away = sw('settings.generalAutoAwayLabel');
    expect(away.hasAttribute('disabled')).toBe(true);
    expect(description(away)).toBe(said(account, 'settings.generalAutoAwaySub'));
    expect(button('settings.generalSyncNowLabel').disabled).toBe(true);
    expect(description(button('settings.generalSyncNowLabel'))).toBe(said(account, 'settings.generalSyncNowSub'));
    expect(button('settings.generalSyncForgetLabel').disabled).toBe(true);
    expect(description(button('settings.generalSyncForgetLabel'))).toBe(said(account, 'settings.generalSyncForgetSub'));
    expect(screen.queryByText(msg('discord.cardTitle'))).toBeNull();
  });

  it('turns auto-away and Sync now off while sync is off', () => {
    open({ accountSync: false });
    expect(sw('settings.generalSyncEnableLabel').hasAttribute('disabled')).toBe(false);
    const away = sw('settings.generalAutoAwayLabel');
    expect(away.hasAttribute('disabled')).toBe(true);
    expect(description(away)).toBe(said('settings.generalSyncOffReason', 'settings.generalAutoAwaySub'));
    expect(button('settings.generalSyncNowLabel').disabled).toBe(true);
    expect(description(button('settings.generalSyncNowLabel'))).toBe(said('settings.generalSyncOffReason', 'settings.generalSyncNowSub'));
    expect(button('settings.generalSyncForgetLabel').disabled).toBe(false);
    expect(description(button('settings.generalSyncForgetLabel'))).toBe(msg('settings.generalSyncForgetSub'));
  });

  it('hides the power and priority rows on a Mac', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, platform: 'darwin' });
    open();
    await waitFor(() => expect(screen.queryByRole('switch', { name: msg('settings.gameModePowerLabel') })).toBeNull());
    expect(screen.queryByRole('switch', { name: msg('settings.gameModePriorityLabel') })).toBeNull();
    expect(sw('settings.gameModeHideLabel')).toBeTruthy();
  });

  it('turns the import from other launchers off outside Windows and says so', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, platform: 'darwin' });
    open();
    await screen.findByText(msg('settings.extlibWindowsOnly'));
    const find = button('settings.extlibLabel');
    expect(find.disabled).toBe(true);
    expect(description(find)).toBe(said('settings.extlibWindowsOnly', 'settings.extlibSub'));
  });
});

// What only some platforms have waits for the app facts: a Mac must not see Windows rows while they
// are pending or after their read failed, and Windows must not hear it lacks Windows.
describe('the general tab: rows that depend on the platform', () => {
  const windowsRows = () => ['settings.gameModePowerLabel', 'settings.gameModePriorityLabel'].map((key) => screen.queryByRole('switch', { name: msg(key as MessageKey) }));

  it('draws no Windows row and offers no import while the app facts are pending', () => {
    vi.mocked(getAppInfo).mockReturnValue(new Promise(() => {}));
    open();
    expect(windowsRows()).toEqual([null, null]);
    const find = button('settings.extlibLabel');
    expect(find.disabled).toBe(true);
    expect(description(find)).toBe(msg('settings.extlibSub'));
    expect(screen.queryByText(msg('settings.extlibWindowsOnly'))).toBeNull();
    expect(sw('settings.gameModeHideLabel')).toBeTruthy();
  });

  it('draws no Windows row and offers no import when the app facts could not be read', async () => {
    vi.mocked(getAppInfo).mockRejectedValue(new Error('app info down'));
    open();
    await waitFor(() => expect(console.warn).toHaveBeenCalled());
    expect(windowsRows()).toEqual([null, null]);
    const find = button('settings.extlibLabel');
    expect(find.disabled).toBe(true);
    expect(description(find)).toBe(msg('settings.extlibSub'));
    expect(screen.queryByText(msg('settings.extlibWindowsOnly'))).toBeNull();
  });

  it('on a Mac: no Windows row, and the import off because it needs Windows', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, platform: 'darwin' });
    open();
    const find = button('settings.extlibLabel');
    await waitFor(() => expect(description(find)).toBe(said('settings.extlibWindowsOnly', 'settings.extlibSub')));
    expect(find.disabled).toBe(true);
    expect(windowsRows()).toEqual([null, null]);
  });

  it('on Windows: the power and priority switches, and the import on offer', async () => {
    open();
    const [power, priority] = await Promise.all(
      ['settings.gameModePowerLabel', 'settings.gameModePriorityLabel'].map((key) => screen.findByRole('switch', { name: msg(key as MessageKey) })),
    );
    expect([power.hasAttribute('disabled'), priority.hasAttribute('disabled')]).toEqual([false, false]);
    const find = button('settings.extlibLabel');
    expect(find.disabled).toBe(false);
    expect(description(find)).toBe(msg('settings.extlibSub'));
  });
});

describe('the general tab: the overlay status', () => {
  const region = () => screen.getByRole('status', { name: msg('settings.overlayStatusLabel') });

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

  // Go's own words never reach the player: a failure without a code says the place's own text.
  it('says an overlay failure without a code by the catalogued text, not Go\'s', async () => {
    vi.mocked(overlayStatus).mockResolvedValue({ supported: true, enabled: true, hotkey: 'Alt+`', error: 'register Alt+`: hotkey taken' });
    open();
    await waitFor(() => expect(region().textContent?.trim()).toBe(msg('settings.overlaySaveErrorPlain')));
    expect(region().querySelector('[data-tone="danger"]')).not.toBeNull();
  });

  it('says an overlay change that failed without a code by the catalogued text', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('locked'));
    open();
    await user().click(sw('settings.overlayEnabledLabel'));
    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('settings.overlaySaveErrorPlain'));
  });

  it('says a screenshot change that failed without a code by the catalogued text', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('hotkey taken'));
    open();
    await user().click(sw('settings.screenshotsEnabledLabel'));
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.screenshotsSaveErrorPlain')]));
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
  // An icon beside the words: drawn, never read, and off with its button.
  it('marks Sync now, Forget and the import with an icon a screen reader does not read', async () => {
    open();
    for (const key of ['settings.generalSyncNowLabel', 'settings.generalSyncForgetLabel', 'settings.extlibLabel'] as const) {
      const icons = [...button(key).querySelectorAll('svg')];
      expect(icons.map((icon) => icon.getAttribute('aria-hidden')), key).toEqual(['true']);
    }
  });

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
    const path = screen.getByRole('textbox', { name: msg('settings.generalLibraryPathLabel') }) as HTMLInputElement;
    expect([path.value, path.placeholder, path.readOnly]).toEqual(['', msg('settings.generalLibraryPathPlaceholder'), true]);
    // Off, and says why, but still reached by the keyboard.
    const openButton = button('settings.generalLibraryOpenFolderLabel');
    expect(openButton.getAttribute('aria-disabled')).toBe('true');
    expect(description(openButton)).toBe(msg('settings.generalLibraryOpenNoPathReason'));
    await user().click(openButton);
    expect(openFolder).not.toHaveBeenCalled();
  });

  it('with a library: the path and its three parts, each opened in the file manager', async () => {
    vi.mocked(openFolder).mockResolvedValue();
    open({ libraryPath: '/lib', gamesPath: '/lib/games', downloadsPath: '/lib/downloads', screenshotsPath: '/lib/shots' });
    const parts: [MessageKey, string][] = [
      ['settings.generalLibraryPathLabel', '/lib'],
      ['settings.generalLibraryFolderGames', '/lib/games'],
      ['settings.generalLibraryFolderDownloads', '/lib/downloads'],
      ['settings.generalLibraryFolderScreenshots', '/lib/shots'],
    ];
    // Each field is named by its row, each Open button by the folder it opens.
    for (const [label, value] of parts) expect((screen.getByRole('textbox', { name: msg(label) }) as HTMLInputElement).value).toBe(value);
    expect(screen.getByRole('button', { name: msg('common.edit') })).toBeTruthy();
    const opens = [
      button('settings.generalLibraryOpenFolderLabel'),
      ...parts.slice(1).map(([label]) => screen.getByRole('button', { name: msg('settings.generalLibraryOpenNamedFolderLabel', { folder: msg(label) }) })),
    ];
    for (const target of opens) await user().click(target);
    expect(vi.mocked(openFolder).mock.calls.map(([path]) => path)).toEqual(['/lib', '/lib/games', '/lib/downloads', '/lib/shots']);
  });

  it('says a folder that cannot be opened on a toast', async () => {
    vi.mocked(openFolder).mockRejectedValue(new Error('gone'));
    open({ libraryPath: '/lib', gamesPath: '/lib/games' });
    await user().click(screen.getAllByRole('button', { name: msg('settings.generalLibraryOpenFolderLabel') })[0]);
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.generalLibraryFolderUnavailableToast')]));
  });
});

// The page reads the app, the system, CrossOver and the legal list once on opening: one read that fails
// leaves the others to run, keeps its row's unknown state and is logged.
describe('the settings page: its first reads', () => {
  it('reads the system, CrossOver and the legal list when the app facts fail', async () => {
    const down = new Error('app info down');
    vi.mocked(getAppInfo).mockRejectedValue(down);
    open();
    await waitFor(() => expect(listLegalDocuments).toHaveBeenCalledOnce());
    expect(getWineStatus).toHaveBeenCalledOnce();
    expect(console.warn).toHaveBeenCalledWith(expect.any(String), down);
    // The unknown platform keeps the launch switch on offer, as before the read.
    expect(sw('settings.generalLaunchOnStartupLabel').hasAttribute('disabled')).toBe(false);
    await user().click(screen.getByRole('tab', { name: msg('settings.aboutTab') }));
    expect(await screen.findByText('Terms of use')).toBeTruthy();
  });

  it('reads the legal list when the CrossOver status fails', async () => {
    const down = new Error('wine down');
    vi.mocked(getWineStatus).mockRejectedValueOnce(down);
    open();
    await waitFor(() => expect(listLegalDocuments).toHaveBeenCalledOnce());
    expect(console.warn).toHaveBeenCalledWith(expect.any(String), down);
  });
});

// A button its own press puts to work keeps focus: busy, not disabled.
describe('the general tab: buttons at work', () => {
  function later() {
    let resolve: () => void = () => {};
    const promise = new Promise<void>((yes) => {
      resolve = yes;
    });
    return { promise, resolve };
  }

  const atWork = (target: HTMLButtonElement) => [target.disabled, target.getAttribute('aria-busy'), document.activeElement === target];

  it('keeps Sync now in focus and busy while it syncs', async () => {
    const run = later();
    vi.mocked(syncNow).mockReturnValueOnce(run.promise);
    open();
    const sync = button('settings.generalSyncNowLabel');
    sync.focus();
    await user().keyboard('{Enter}');
    await waitFor(() => expect(syncNow).toHaveBeenCalledOnce());
    const busy = screen.getByRole('button', { name: msg('settings.generalSyncNowRunning') }) as HTMLButtonElement;
    expect(busy).toBe(sync);
    expect(atWork(sync)).toEqual([false, 'true', true]);
    run.resolve();
    await waitFor(() => expect(sync.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(sync);
  });

  it('keeps Forget busy while it deletes, and returns focus to it when the dialog closes', async () => {
    const run = later();
    vi.mocked(forgetRemote).mockReturnValueOnce(run.promise);
    open();
    const forget = button('settings.generalSyncForgetLabel');
    forget.focus();
    await user().keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    const buttons = within(dialog).getAllByRole('button');
    await user().click(buttons[buttons.length - 1]);
    await waitFor(() => expect(forgetRemote).toHaveBeenCalledOnce());
    expect([forget.disabled, forget.getAttribute('aria-busy')]).toEqual([false, 'true']);
    run.resolve();
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(forget));
  });

  it('keeps the import in focus and busy while it searches', async () => {
    let finish: () => void = () => {};
    vi.mocked(importExternalGames).mockReturnValueOnce(
      new Promise((resolve) => {
        finish = () => resolve({ providers: [], added: 0, updated: 0, metadataErrors: null });
      }),
    );
    open();
    const find = await screen.findByRole('button', { name: msg('settings.extlibLabel') }) as HTMLButtonElement;
    await waitFor(() => expect(find.disabled).toBe(false));
    find.focus();
    await user().keyboard('{Enter}');
    await waitFor(() => expect(importExternalGames).toHaveBeenCalledOnce());
    expect(atWork(find)).toEqual([false, 'true', true]);
    finish();
    await waitFor(() => expect(find.hasAttribute('aria-busy')).toBe(false));
  });
});
