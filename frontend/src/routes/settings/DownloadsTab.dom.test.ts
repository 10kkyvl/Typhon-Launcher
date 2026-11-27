import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
// A Windows build: the row that asks for admin rights ahead of time is shown there.
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getAppInfo: vi.fn(async () => ({ version: '2.0.0', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' })),
}));
vi.mock('../../lib/stores/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/selfupdate')>()),
  requestCheck: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/selfupdate')>()),
  checkForUpdate: vi.fn(),
}));
vi.mock('../../lib/services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/cloudsaves')>()),
  cloudForget: vi.fn(),
}));

import { msg, type MessageKey } from '../../lib/i18n';
import { cloudForget } from '../../lib/services/cloudsaves';
import { checkForUpdate } from '../../lib/services/selfupdate';
import { updateReason } from '../../lib/services/selfupdateMessages';
import { defaultPlayDownloadLimit, defaultPlayUploadLimit } from '../../lib/services/playThrottle';
import { saveSettingsPatch, type Settings as SettingsData } from '../../lib/services/settings';
import { cloudStatus } from '../../lib/stores/cloudsaves';
import { saveBackupsEnabled } from '../../lib/stores/savebackup';
import { requestCheck, selfUpdateStatus } from '../../lib/stores/selfupdate';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import type { CurrentUser } from '../../lib/services/account';
import { user } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import Settings from './Settings.svelte';

const MB = 1024 * 1024;

const SAVED = {
  language: 'system',
  uiScale: 1,
  downloadRateLimit: 0,
  uploadRateLimit: 0,
  playDownloadMode: 'off',
  maxActiveDownloads: 2,
  uploadWhileDownloading: false,
  seedAfterDownload: false,
  sourceRefreshInterval: '6h',
  updateCheckAutomatically: true,
  updateAutoDownload: false,
  updateBetaChannel: false,
  updateSaveBackup: true,
  saveBackupAfterSession: true,
  saveBackupBeforeLaunch: true,
  saveBackupLimit: 5,
  cloudSaves: false,
  keepPreviousVersion: 'first_launch',
  allowTorrentReuse: true,
  installCleanupPolicy: 'delete',
  autoInstall: false,
  elevateAhead: false,
  installSkipShortcuts: true,
  desktopShortcuts: true,
  installSkipExtras: true,
  verifyAfterInstall: true,
  installVerifyRepack: false,
  downloadSchedule: [],
} as unknown as SettingsData;

let app: HTMLDivElement;

beforeEach(() => {
  // jsdom has no ResizeObserver; the settings tabs measure their marker with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ ...SAVED });
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  authState.set('bootstrapping');
  currentUser.set(null);
  cloudStatus.set(null);
  saveBackupsEnabled.set(false);
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  document.body.replaceChildren();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.mocked(requestCheck).mockReset();
  vi.mocked(cloudForget).mockReset();
  vi.mocked(checkForUpdate).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function open() {
  render(Settings, { target: app, props: { tab: 'downloads' } });
  return user();
}

const toastTexts = () => get(toasts).map((item) => item.message);

// Every switch of the tab: its name, its description, the setting it writes.
const SWITCHES: Array<[label: MessageKey, sub: MessageKey, key: keyof SettingsData]> = [
  ['settings.downloadsUploadWhileDownloadingLabel', 'settings.downloadsUploadWhileDownloadingSub', 'uploadWhileDownloading'],
  ['settings.downloadsSeedAfterLabel', 'settings.downloadsSeedAfterSub', 'seedAfterDownload'],
  ['settings.downloadsUpdateCheckAutoLabel', 'settings.downloadsUpdateCheckAutoSub', 'updateCheckAutomatically'],
  ['settings.downloadsUpdateAutoDownloadLabel', 'settings.downloadsUpdateAutoDownloadSub', 'updateAutoDownload'],
  ['settings.downloadsSaveBackupLabel', 'settings.downloadsSaveBackupSub', 'updateSaveBackup'],
  ['settings.downloadsTorrentReuseLabel', 'settings.downloadsTorrentReuseSub', 'allowTorrentReuse'],
  ['settings.downloadsAutoInstallLabel', 'settings.downloadsAutoInstallSub', 'autoInstall'],
  ['settings.downloadsElevateAheadLabel', 'settings.downloadsElevateAheadSub', 'elevateAhead'],
  ['settings.downloadsSkipShortcutsLabel', 'settings.downloadsSkipShortcutsSub', 'installSkipShortcuts'],
  ['settings.downloadsDesktopShortcutsLabel', 'settings.downloadsDesktopShortcutsSub', 'desktopShortcuts'],
  ['settings.downloadsSkipExtrasLabel', 'settings.downloadsSkipExtrasSub', 'installSkipExtras'],
  ['settings.downloadsVerifyAfterInstallLabel', 'settings.downloadsVerifyAfterInstallSub', 'verifyAfterInstall'],
  ['settings.downloadsVerifyRepackLabel', 'settings.downloadsVerifyRepackSub', 'installVerifyRepack'],
];

// Every list of the tab: its name, its description, an option (a catalog key or the shown text) and
// the patch it writes. Keys are read at the test's time: the catalog's language is set by then.
const SELECTS: Array<[label: MessageKey, sub: MessageKey, option: MessageKey | string, patch: Partial<SettingsData>]> = [
  ['settings.downloadsRateLimitLabel', 'settings.downloadsRateLimitSub', '10 MB/s', { downloadRateLimit: 10 * MB }],
  ['settings.downloadsUploadLimitLabel', 'settings.downloadsUploadLimitSub', '5 MB/s', { uploadRateLimit: 5 * MB }],
  ['settings.downloadsPlayLabel', 'settings.downloadsPlaySub', 'settings.downloadsPlayPause', { playDownloadMode: 'pause' }],
  ['settings.downloadsMaxActiveLabel', 'settings.downloadsMaxActiveSub', '3', { maxActiveDownloads: 3 }],
  ['settings.downloadsSourceRefreshLabel', 'settings.downloadsSourceRefreshSub', 'settings.downloadsSourceRefreshDaily', { sourceRefreshInterval: '24h' }],
  ['settings.downloadsKeepPreviousLabel', 'settings.downloadsKeepPreviousSub', 'settings.downloadsKeepPrevious24h', { keepPreviousVersion: '24h' }],
  ['settings.downloadsAfterInstallLabel', 'settings.downloadsAfterInstallSub', 'settings.downloadsCleanupKeep', { installCleanupPolicy: 'keep' }],
];

describe('the downloads tab: switches', () => {
  it.each(SWITCHES)('%s is a switch named by its label, described by its text, writing its setting', async (label, sub, key) => {
    const keys = open();
    const toggle = await screen.findByRole('switch', { name: msg(label) });
    expect(description(toggle)).toBe(msg(sub));
    expect(toggle.hasAttribute('disabled')).toBe(false);
    const before = toggle.getAttribute('aria-checked') === 'true';
    expect(before).toBe(Boolean(SAVED[key]));

    await keys.click(toggle);

    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ [key]: !before }));
    expect(toggle.getAttribute('aria-checked')).toBe(String(!before));
  });

  it('shows the held state and says so when a switch is not saved', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('locked'));
    const keys = open();
    const toggle = screen.getByRole('switch', { name: msg('settings.downloadsSeedAfterLabel') });

    await keys.click(toggle);

    await vi.waitFor(() => expect(toastTexts()).toContain(msg('state.settingsSaveFailed')));
    expect(toggle.getAttribute('aria-checked')).toBe('false');
  });
});

describe('the downloads tab: lists', () => {
  it.each(SELECTS)('%s is a list named by its label, writing its choice', async (label, _sub, option, patch) => {
    const keys = open();
    const field = screen.getByRole('combobox', { name: msg(label) });
    expect(field.hasAttribute('disabled')).toBe(false);

    await keys.click(field);
    await keys.click(screen.getByRole('option', { name: option.startsWith('settings.') ? msg(option as MessageKey) : option }));

    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith(patch));
  });

  it('describes every list by the text of its row', () => {
    open();
    const described = SELECTS.map(([label]) => description(screen.getByRole('combobox', { name: msg(label) })));
    expect(described).toEqual(SELECTS.map(([, sub]) => msg(sub)));
  });
});

describe('the downloads tab: while playing', () => {
  it('adds the two in-game limits after the list when "limit" is chosen, writing the defaults, focus kept on the list', async () => {
    const keys = open();
    const mode = screen.getByRole('combobox', { name: msg('settings.downloadsPlayLabel') });
    expect(screen.queryByRole('combobox', { name: msg('settings.downloadsPlayLimitDownLabel') })).toBeNull();
    mode.focus();
    await keys.keyboard('{Enter}');
    await keys.click(screen.getByRole('option', { name: msg('settings.downloadsPlayLimit') }));

    await vi.waitFor(() =>
      expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({
        playDownloadMode: 'limit',
        playDownloadRateLimit: defaultPlayDownloadLimit,
        playUploadRateLimit: defaultPlayUploadLimit,
      }),
    );
    const down = await screen.findByRole('combobox', { name: msg('settings.downloadsPlayLimitDownLabel') });
    const up = screen.getByRole('combobox', { name: msg('settings.downloadsPlayLimitUpLabel') });
    expect(mode.compareDocumentPosition(down) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(down.compareDocumentPosition(up) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(document.activeElement).toBe(mode);
  });

  it('writes an in-game limit chosen from its list', async () => {
    settings.set({ ...SAVED, playDownloadMode: 'limit', playDownloadRateLimit: 2 * MB, playUploadRateLimit: 1 * MB } as SettingsData);
    const keys = open();

    await keys.click(screen.getByRole('combobox', { name: msg('settings.downloadsPlayLimitDownLabel') }));
    await keys.click(screen.getByRole('option', { name: '5 MB/s' }));

    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ playDownloadRateLimit: 5 * MB }));
  });
});

describe('the downloads tab: save backups', () => {
  it('shows the three backup rows after the switch while backups are on', async () => {
    saveBackupsEnabled.set(true);
    open();
    const main = screen.getByRole('switch', { name: msg('settings.downloadsSaveBackupLabel') });
    const after = screen.getByRole('switch', { name: msg('settings.downloadsSaveBackupAfterSessionLabel') });
    const before = screen.getByRole('switch', { name: msg('settings.downloadsSaveBackupBeforeLaunchLabel') });
    expect(description(after)).toBe(msg('settings.downloadsSaveBackupAfterSessionSub'));
    expect(description(before)).toBe(msg('settings.downloadsSaveBackupBeforeLaunchSub'));
    expect(main.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(after.compareDocumentPosition(before) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('adds the backup rows after the switch that holds focus, focus kept', async () => {
    open();
    const main = screen.getByRole('switch', { name: msg('settings.downloadsSaveBackupLabel') });
    main.focus();

    saveBackupsEnabled.set(true);

    const after = await screen.findByRole('switch', { name: msg('settings.downloadsSaveBackupAfterSessionLabel') });
    expect(main.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(document.activeElement).toBe(main);
  });

  it('names the limit field once, by the label of its row, and writes the typed number', async () => {
    saveBackupsEnabled.set(true);
    const keys = open();
    const limit = screen.getByRole('textbox', { name: msg('settings.downloadsSaveBackupLimitLabel') });
    expect(limit.hasAttribute('aria-label')).toBe(false);
    expect(description(limit)).toBe(msg('settings.downloadsSaveBackupLimitSub'));
    expect((limit as HTMLInputElement).value).toBe('5');

    await keys.clear(limit);
    await keys.type(limit, '9{Enter}');

    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ saveBackupLimit: 9 }));
  });

  it('writes the sub-switches', async () => {
    saveBackupsEnabled.set(true);
    const keys = open();
    await keys.click(screen.getByRole('switch', { name: msg('settings.downloadsSaveBackupAfterSessionLabel') }));
    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ saveBackupAfterSession: false }));
  });
});

describe('the downloads tab: the schedule', () => {
  it('is a group named and described by its row', () => {
    open();
    const editor = screen.getByRole('group', { name: msg('settings.downloadsScheduleLabel') });
    expect(description(editor)).toBe(msg('settings.downloadsScheduleSub'));
  });

  it('adds a window and writes the schedule', async () => {
    const keys = open();
    expect(screen.getByText(msg('settings.downloadsScheduleEmpty'))).toBeTruthy();

    await keys.click(screen.getByRole('button', { name: msg('settings.downloadsScheduleAdd') }));

    await vi.waitFor(() =>
      expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ downloadSchedule: [{ days: [0, 1, 2, 3, 4, 5, 6], from: '01:00', to: '07:00' }] }),
    );
  });

  it('says a schedule that was not saved under the editor, on a danger notice, in the catalog\'s words', async () => {
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('locked'));
    const keys = open();

    await keys.click(screen.getByRole('button', { name: msg('settings.downloadsScheduleAdd') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('settings.downloadsScheduleSaveErrorPlain'));
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
    const days = screen.getByRole('group', { name: msg('settings.downloadsScheduleDays') });
    expect(days.compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe('the downloads tab: cloud saves', () => {
  it('is out of reach without an account, and says why in its description', () => {
    authState.set('unauthenticated');
    open();
    const cloud = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });
    expect(cloud.hasAttribute('disabled')).toBe(true);
    expect(description(cloud)).toBe(`${msg('cloudsaves.settingsNeedsAccount')} ${msg('cloudsaves.settingsSub')}`);
  });

  it('stays in reach when on without an account, so it can be turned off, and says an account is needed', () => {
    authState.set('unauthenticated');
    settings.set({ ...SAVED, cloudSaves: true } as SettingsData);
    open();
    const cloud = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });
    expect(cloud.hasAttribute('disabled')).toBe(false);
    expect(description(cloud)).toBe(`${msg('cloudsaves.settingsSub')} ${msg('cloudsaves.settingsNeedsAccount')}`);
  });

  it('opens the consent dialog from the switch with an account, writing nothing yet', async () => {
    authState.set('authenticated');
    const keys = open();
    const cloud = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });
    expect(cloud.hasAttribute('disabled')).toBe(false);
    expect(description(cloud)).toBe(msg('cloudsaves.settingsSub'));

    await keys.click(cloud);

    expect(await screen.findByRole('dialog', { name: msg('cloudsaves.consentTitle') })).toBeTruthy();
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });

  it('shows the cloud usage and that copies are pending, after the switch', async () => {
    authState.set('authenticated');
    settings.set({ ...SAVED, cloudSaves: true } as SettingsData);
    open();
    cloudStatus.set({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', pending: true, games: [] });
    const usage = msg('cloudsaves.usageUnknown', { used: '0 B' });
    const line = await screen.findByText(`${usage} · ${msg('cloudsaves.pending')}`);
    const cloud = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });
    expect(cloud.compareDocumentPosition(line) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('offers to delete the cloud copies again after the deletion failed, the switch kept in its place', async () => {
    authState.set('authenticated');
    settings.set({ ...SAVED, cloudSaves: true } as SettingsData);
    vi.mocked(cloudForget).mockRejectedValue(new Error('down'));
    const keys = open();
    const cloud = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });

    await keys.click(cloud);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));

    await vi.waitFor(() => expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ cloudSaves: false }));
    const retry = await screen.findByRole('button', { name: msg('cloudsaves.retryForget') });
    expect(description(cloud)).toBe(`${msg('cloudsaves.settingsSub')} ${msg('cloudsaves.retryForgetHint')}`);
    expect(cloud.isConnected).toBe(true);
    expect(cloud.getAttribute('aria-checked')).toBe('false');
    // The switch keeps its place: the retry comes after it, never in front of it.
    expect(cloud.compareDocumentPosition(retry) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    let answer: () => void = () => {};
    vi.mocked(cloudForget).mockImplementationOnce(() => new Promise<void>((resolve) => (answer = resolve)));
    retry.focus();
    await keys.keyboard('{Enter}');
    await vi.waitFor(() => expect(cloudForget).toHaveBeenCalledTimes(2));
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);

    answer();
    await vi.waitFor(() => expect(screen.queryByRole('button', { name: msg('cloudsaves.retryForget') })).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(cloud));
    expect(get(toasts).filter(({ kind }) => kind === 'success').map(({ message }) => message)).toEqual([msg('cloudsaves.forgetDone')]);
  });
});

describe('the downloads tab: a retry of the cloud deletion that fails again', () => {
  it('keeps the retry and focus on it', async () => {
    authState.set('authenticated');
    settings.set({ ...SAVED, cloudSaves: true } as SettingsData);
    vi.mocked(cloudForget).mockRejectedValue(new Error('down'));
    const keys = open();
    await keys.click(screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') }));
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));
    const retry = await screen.findByRole('button', { name: msg('cloudsaves.retryForget') });

    retry.focus();
    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(cloudForget).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(retry.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });
});

describe('the downloads tab: a failed cloud deletion after signing out', () => {
  it('keeps the retry on screen but off, with the switch, until an account is back', async () => {
    authState.set('authenticated');
    settings.set({ ...SAVED, cloudSaves: true } as SettingsData);
    vi.mocked(cloudForget).mockRejectedValue(new Error('down'));
    const keys = open();
    await keys.click(screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') }));
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));
    const retry = (await screen.findByRole('button', { name: msg('cloudsaves.retryForget') })) as HTMLButtonElement;
    expect(retry.disabled).toBe(false);

    authState.set('unauthenticated');

    await vi.waitFor(() => expect(retry.disabled).toBe(true));
    expect(screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') }).hasAttribute('disabled')).toBe(true);
    await keys.click(retry);
    expect(cloudForget).toHaveBeenCalledTimes(1);

    authState.set('authenticated');

    await vi.waitFor(() => expect(retry.disabled).toBe(false));
  });
});

describe('the downloads tab: beta versions', () => {
  const TESTER = { tester: true } as CurrentUser;

  it('is offered to a signed-in tester only', () => {
    authState.set('authenticated');
    currentUser.set({ tester: false } as CurrentUser);
    open();
    expect(screen.queryByRole('switch', { name: msg('settings.downloadsBetaLabel') })).toBeNull();
  });

  it('saves the channel, then checks for an update', async () => {
    authState.set('authenticated');
    currentUser.set(TESTER);
    const keys = open();
    const beta = screen.getByRole('switch', { name: msg('settings.downloadsBetaLabel') });
    expect(description(beta)).toBe(msg('settings.downloadsBetaSub'));

    await keys.click(beta);

    await vi.waitFor(() => expect(requestCheck).toHaveBeenCalledTimes(1));
    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ updateBetaChannel: true });
  });

  // requestCheck says a failed check itself and never rejects: the tab adds nothing to it.
  it('keeps a saved channel when the check after it fails, said once, the way a failed check is said', async () => {
    authState.set('authenticated');
    currentUser.set(TESTER);
    const failure = new Error('typhon:selfupdate.stalled: read tcp');
    vi.mocked(checkForUpdate).mockRejectedValue(failure);
    const real = await vi.importActual<typeof import('../../lib/stores/selfupdate')>('../../lib/stores/selfupdate');
    vi.mocked(requestCheck).mockImplementation(real.requestCheck);
    const keys = open();
    const beta = screen.getByRole('switch', { name: msg('settings.downloadsBetaLabel') });

    await keys.click(beta);

    await vi.waitFor(() => expect(checkForUpdate).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(get(toasts)).toHaveLength(1));
    expect(get(toasts).map((item) => [item.kind, item.message])).toEqual([['danger', updateReason(failure)]]);
    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ updateBetaChannel: true });
    expect(get(settings)?.updateBetaChannel).toBe(true);
    expect(beta.getAttribute('aria-checked')).toBe('true');
  });

  it('says a channel that was not saved, and checks nothing', async () => {
    authState.set('authenticated');
    currentUser.set(TESTER);
    vi.mocked(saveSettingsPatch).mockRejectedValue(new Error('locked'));
    const keys = open();

    await keys.click(screen.getByRole('switch', { name: msg('settings.downloadsBetaLabel') }));

    await vi.waitFor(() => expect(toastTexts()).toContain(msg('state.settingsSaveFailed')));
    expect(requestCheck).not.toHaveBeenCalled();
  });

  it('names the channel in the label while the launcher is on beta', () => {
    authState.set('authenticated');
    currentUser.set(TESTER);
    settings.set({ ...SAVED, updateBetaChannel: true } as SettingsData);
    selfUpdateStatus.set({ state: 'idle', currentVersion: '2.0.0-beta.1', channel: 'beta' });
    open();
    const name = `${msg('settings.downloadsBetaLabel')} · ${msg('settings.downloadsBetaChannelMark')}`;
    expect(screen.getByRole('switch', { name }).getAttribute('aria-checked')).toBe('true');
  });
});
