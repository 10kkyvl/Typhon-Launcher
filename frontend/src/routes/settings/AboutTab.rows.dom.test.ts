import { render, screen, waitFor, within } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The about tab in the desktop app, every backend call answered here: the diagnostics upload and the
// launcher update never reach a real server.
vi.mock('@wailsio/runtime', async () => ({
  ...(await import('../../lib/testing/dom')).wailsRuntime(),
  Clipboard: { SetText: vi.fn(async () => {}) },
}));
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getAppInfo: vi.fn(),
  getSystemInfo: vi.fn(),
  getWineStatus: vi.fn(),
  exportLogs: vi.fn(),
}));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  openFolder: vi.fn(),
}));
vi.mock('../../lib/services/legal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/legal')>()),
  listLegalDocuments: vi.fn(),
  getLegalDocument: vi.fn(),
}));
const uploadListeners: ((status: LogUploadStatus) => void)[] = [];
vi.mock('../../lib/services/logsUpload', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/logsUpload')>()),
  onLogUploadStatus: (handler: (status: LogUploadStatus) => void) => {
    uploadListeners.push(handler);
    return () => uploadListeners.splice(uploadListeners.indexOf(handler), 1);
  },
  sendLogs: vi.fn(),
}));
vi.mock('../../lib/services/overlay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/overlay')>()),
  overlayStatus: vi.fn(async () => ({ supported: false, enabled: false, hotkey: '', error: '' })),
  onOverlayStatus: () => () => {},
}));
vi.mock('../../lib/stores/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/cloudsaves')>()),
  refreshCloudStatus: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/selfupdate')>()),
  checkForUpdate: vi.fn(),
}));
vi.mock('../../lib/stores/router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/router')>()),
  navigate: vi.fn(),
}));

import { Clipboard } from '@wailsio/runtime';
import { msg, type MessageKey } from '../../lib/i18n';
import { getLegalDocument, listLegalDocuments } from '../../lib/services/legal';
import { sendLogs, type LogUploadStatus } from '../../lib/services/logsUpload';
import { checkForUpdate, emptyReleaseNotes, noRollback, type SelfUpdateStatus } from '../../lib/services/selfupdate';
import { rollbackReason, updateReason } from '../../lib/services/selfupdateMessages';
import { openFolder } from '../../lib/services/settings';
import { exportLogs, getAppInfo, getSystemInfo, getWineStatus, type AppInfo, type SystemInfo } from '../../lib/services/system';
import { navigate } from '../../lib/stores/router';
import { releaseNotes, rollbackInfo, selfUpdateChecking, selfUpdateStatus } from '../../lib/stores/selfupdate';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import { bytesLabel } from '../../lib/utils/format';
import about from './AboutTab.svelte?raw';
import Settings from './Settings.svelte';

const APP: AppInfo = { version: '1.4.2', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' };
const SYSTEM: SystemInfo = {
  os: 'Windows 11 Pro',
  osUnknown: false,
  arch: 'amd64',
  cpu: 'Ryzen 7 5800X',
  cpuUnknown: false,
  cores: 16,
  physicalCores: 8,
  physicalCoresUnknown: false,
  ramBytes: 32 * 1024 ** 3,
};
const BUNDLE = { path: '/home/p/Downloads/typhon-logs.zip', name: 'typhon-logs.zip', dir: '/home/p/Downloads', sizeBytes: 2048 };

let app: HTMLDivElement;

function later<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function open() {
  render(Settings, { target: app, props: { tab: 'about' } });
}

const button = (name: string) => screen.getByRole('button', { name }) as HTMLButtonElement;
const toastTexts = () => get(toasts).map((item) => item.message);
const squeeze = (text: string | null | undefined) => (text ?? '').replace(/\s+/g, ' ').trim();

/** What an info row says beside its label: the words of the label's own block, the label left out. */
async function said(label: MessageKey | string): Promise<string> {
  const name = label.includes('.') ? msg(label as MessageKey) : label;
  const element = (await screen.findAllByText(name, { exact: true })).find((found) => !found.closest('button'));
  if (!element) throw new Error(`no label ${name} outside a button`);
  const block = element.parentElement as HTMLElement;
  return squeeze(squeeze(block.textContent).slice(squeeze(element.textContent).length));
}

function emit(status: LogUploadStatus) {
  flushSync(() => uploadListeners.forEach((handler) => handler(status)));
}

async function confirmSend() {
  await user().click(button(msg('settings.aboutLogsSendButton')));
  const dialog = await screen.findByRole('dialog');
  await user().click(within(dialog).getByRole('button', { name: msg('settings.aboutLogsSendConfirmButton') }));
  await waitFor(() => expect(sendLogs).toHaveBeenCalledOnce());
}

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
  vi.mocked(getAppInfo).mockResolvedValue(APP);
  vi.mocked(getSystemInfo).mockResolvedValue(SYSTEM);
  vi.mocked(getWineStatus).mockResolvedValue({ required: false, installed: false, version: '' });
  vi.mocked(listLegalDocuments).mockResolvedValue([
    { id: 'terms', title: 'Terms of use' },
    { id: 'privacy', title: 'Privacy policy' },
  ]);
  vi.mocked(getLegalDocument).mockResolvedValue({ id: 'terms', title: 'Terms of use', body: 'Text' });
  vi.mocked(openFolder).mockResolvedValue();
});

afterEach(() => {
  document.body.replaceChildren();
  toasts.set([]);
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  selfUpdateChecking.set(false);
  releaseNotes.set(emptyReleaseNotes());
  rollbackInfo.set(noRollback());
  for (const fn of [getAppInfo, getSystemInfo, getWineStatus, exportLogs, openFolder, listLegalDocuments, getLegalDocument, sendLogs, checkForUpdate, navigate]) {
    vi.mocked(fn).mockReset();
  }
  vi.mocked(Clipboard.SetText).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('the about tab: the version and the machine', () => {
  it('says the version, the platform and the build', async () => {
    open();
    expect(await screen.findByText(`${msg('settings.aboutVersionLabel', { version: '1.4.2' })} · windows/amd64`)).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Typhon Launcher' })).toBeTruthy();
  });

  it('marks a beta build and a devmock build', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, channel: 'beta', devMock: true, platform: 'darwin', arch: 'arm64' });
    open();
    expect(await screen.findByText(`${msg('settings.aboutVersionBetaLabel', { version: '1.4.2' })} · darwin/arm64 · devmock`)).toBeTruthy();
  });

  it('marks the beta channel the update status reports', async () => {
    selfUpdateStatus.set({ state: 'idle', currentVersion: '1.4.2', channel: 'beta' } as SelfUpdateStatus);
    open();
    expect(await screen.findByText(`${msg('settings.aboutVersionBetaLabel', { version: '1.4.2' })} · windows/amd64`)).toBeTruthy();
  });

  it('says where a portable build keeps its data, and nothing of it otherwise', async () => {
    vi.mocked(getAppInfo).mockResolvedValue({ ...APP, portable: true, dataDir: 'D:\\Typhon\\data' });
    open();
    expect(await said('settings.aboutPortableLabel')).toBe(msg('settings.aboutPortableDataDir', { path: 'D:\\Typhon\\data' }));
  });

  it('has no portable row for an installed build', async () => {
    open();
    await screen.findByText(msg('settings.aboutSystemLabel'));
    expect(screen.queryByText(msg('settings.aboutPortableLabel'))).toBeNull();
  });

  it('says the system, the processor and the memory', async () => {
    open();
    expect(await said('settings.aboutSystemLabel')).toBe('Windows 11 Pro');
    expect(await said('settings.aboutCpuLabel')).toBe(
      `Ryzen 7 5800X · ${msg('settings.aboutCpuCores', { cores: 8 })} · ${msg('settings.aboutCpuThreads', { cores: 16 })}`,
    );
    expect(await said('settings.aboutMemoryLabel')).toBe(bytesLabel(32 * 1024 ** 3));
  });

  it('says unknown for each fact the system could not tell', async () => {
    vi.mocked(getSystemInfo).mockResolvedValue({ ...SYSTEM, osUnknown: true, cpuUnknown: true, physicalCoresUnknown: true, ramBytes: 0 });
    open();
    expect(await said('settings.aboutSystemLabel')).toBe(msg('settings.aboutUnknown'));
    expect(await said('settings.aboutCpuLabel')).toBe(`${msg('settings.aboutUnknown')} · ${msg('settings.aboutCpuThreads', { cores: 16 })}`);
    expect(await said('settings.aboutMemoryLabel')).toBe(msg('settings.aboutUnknown'));
  });

  it('says the CrossOver version where games need it, and that it is missing', async () => {
    vi.mocked(getWineStatus).mockResolvedValue({ required: true, installed: true, version: '24.0.5' });
    open();
    expect(await said('settings.aboutRuntimeLabel')).toBe('CrossOver 24.0.5');
  });

  it('says CrossOver is missing', async () => {
    vi.mocked(getWineStatus).mockResolvedValue({ required: true, installed: false, version: '' });
    open();
    expect(await said('settings.aboutRuntimeLabel')).toBe(msg('settings.aboutRuntimeMissing'));
  });

  it('has no runtime row where games need no CrossOver', async () => {
    open();
    await screen.findByText(msg('settings.aboutSystemLabel'));
    expect(screen.queryByText(msg('settings.aboutRuntimeLabel'))).toBeNull();
  });

  it('says the license and offers the community', async () => {
    open();
    expect(await said('settings.aboutLicenseLabel')).toBe(msg('settings.aboutLicenseText'));
    expect(await said('discord.communityLabel')).toBe(msg('discord.communitySub'));
    expect(button(msg('discord.joinButton'))).toBeTruthy();
  });
});

// The page reads the app facts, the system, CrossOver and the legal list once on opening: each read
// fails on its own and the others still show.
describe('the about tab: its first reads failing one by one', () => {
  it('the app facts: the rows that need them say the read failed, and the rest still shows', async () => {
    vi.mocked(getAppInfo).mockRejectedValue(new Error('app info down'));
    open();
    expect(await said('settings.aboutSystemLabel')).toBe('Windows 11 Pro');
    expect(await screen.findByText('Terms of use')).toBeTruthy();
    const heading = screen.getByRole('heading', { name: 'Typhon Launcher' });
    expect(squeeze(heading.parentElement?.textContent)).toBe(`Typhon Launcher ${msg('settings.aboutSystemFailed')}`);
    expect(await said('settings.aboutCheckUpdatesLabel')).toBe(msg('settings.aboutSystemFailed'));
    expect(description(button(msg('settings.aboutCheckButtonLabel')))).toBe(msg('settings.aboutSystemFailed'));
  });

  it('the app facts failing still says the version the update status knows', async () => {
    vi.mocked(getAppInfo).mockRejectedValue(new Error('app info down'));
    selfUpdateStatus.set({ state: 'idle', currentVersion: '1.4.2' });
    open();
    await screen.findByText(msg('settings.aboutSystemFailed'));
    expect(await said('settings.aboutCheckUpdatesLabel')).toBe(msg('settings.aboutInstalledVersion', { version: '1.4.2' }));
  });

  it('the app facts pending: a dash for the version, no platform yet', () => {
    vi.mocked(getAppInfo).mockReturnValue(new Promise(() => {}));
    open();
    const heading = screen.getByRole('heading', { name: 'Typhon Launcher' });
    expect(squeeze(heading.parentElement?.textContent)).toBe(`Typhon Launcher ${msg('settings.aboutVersionLabel', { version: '—' })}`);
  });

  it('the system: one row says it failed and the rest still shows', async () => {
    vi.mocked(getSystemInfo).mockRejectedValue(new Error('no system info'));
    open();
    expect(await said('settings.aboutSystemLabel')).toBe(msg('settings.aboutSystemFailed'));
    expect(screen.queryByText(msg('settings.aboutCpuLabel'))).toBeNull();
    expect(screen.queryByText(msg('settings.aboutMemoryLabel'))).toBeNull();
    expect(await screen.findByText('Terms of use')).toBeTruthy();
  });

  it('CrossOver: no runtime row and the rest still shows', async () => {
    vi.mocked(getWineStatus).mockRejectedValue(new Error('wine down'));
    open();
    expect(await screen.findByText('Terms of use')).toBeTruthy();
    expect(screen.queryByText(msg('settings.aboutRuntimeLabel'))).toBeNull();
  });

  it('the legal list: a danger notice in place of the documents', async () => {
    vi.mocked(listLegalDocuments).mockRejectedValue(new Error('broken'));
    open();
    const alert = await waitFor(() => {
      const found = screen.getAllByRole('alert').find((el) => el.textContent?.includes(msg('settings.aboutLegalLoadError')));
      expect(found).toBeDefined();
      return found as HTMLElement;
    });
    expect(alert.closest('[data-tone]')?.getAttribute('data-tone')).toBe('danger');
    expect(screen.queryByRole('button', { name: msg('common.open') })).toBeNull();
    expect(await said('settings.aboutSystemLabel')).toBe('Windows 11 Pro');
  });
});

describe('the about tab: the launcher update rows', () => {
  it('says the installed version and when it was last checked', async () => {
    selfUpdateStatus.set({ state: 'idle', currentVersion: '1.4.2', checkedAt: new Date().toISOString() });
    open();
    const words = await said('settings.aboutCheckUpdatesLabel');
    expect(words.startsWith(`${msg('settings.aboutInstalledVersion', { version: '1.4.2' })} · `)).toBe(true);
    expect(words).toContain(msg('settings.aboutCheckedAt', { date: '' }).trim());
  });

  it('checks for an update on a press, and says it is checking', async () => {
    const check = later<SelfUpdateStatus>();
    vi.mocked(checkForUpdate).mockReturnValueOnce(check.promise);
    open();
    const press = button(msg('settings.aboutCheckButtonLabel'));
    await user().click(press);
    await waitFor(() => expect(checkForUpdate).toHaveBeenCalledOnce());
    expect(press.textContent).toContain(msg('settings.aboutCheckingEllipsis'));
    expect(await said('settings.aboutCheckUpdatesLabel')).toBe(msg('settings.aboutCheckUpdatesChecking'));
    check.resolve({ state: 'idle', currentVersion: '1.4.2' });
    await waitFor(() => expect(press.textContent).toContain(msg('settings.aboutCheckButtonLabel')));
  });

  it('says a failed check in catalogued words', async () => {
    const failure = new Error('typhon:selfupdate.stalled: read tcp 10.0.0.1:443');
    vi.mocked(checkForUpdate).mockRejectedValueOnce(failure);
    open();
    await user().click(button(msg('settings.aboutCheckButtonLabel')));
    await waitFor(() => expect(toastTexts()).toEqual([updateReason(failure)]));
    expect(toastTexts()[0]).not.toContain('read tcp');
  });

  it('keeps the update history shut until there are notes, and opens it once there are', async () => {
    open();
    const history = button(msg('settings.aboutHistoryButtonLabel'));
    expect(history.disabled).toBe(true);
    expect(await said('settings.aboutHistoryLabel')).toBe(`${msg('settings.aboutHistoryEmpty')} ${msg('settings.aboutHistoryHasNotes')}`);
    releaseNotes.set({ currentVersion: '1.4.2', unseen: [], history: [{ version: '1.4.2', publishedAt: '2026-11-01', changes: [] }] });
    await waitFor(() => expect(history.disabled).toBe(false));
    expect(await said('settings.aboutHistoryLabel')).toBe(msg('settings.aboutHistoryHasNotes'));
    await user().click(history);
    expect(await screen.findByRole('dialog', { name: msg('settings.aboutHistoryLabel') })).toBeTruthy();
  });

  it('shows the previous version row only when there is one or a reason to say', async () => {
    open();
    await screen.findByText(msg('settings.aboutSystemLabel'));
    expect(screen.queryByText(msg('settings.aboutRollbackLabel'))).toBeNull();
    rollbackInfo.set({ available: false, version: '', reason: '', reasonCode: 'selfupdate.rollback_unsupported' });
    await Promise.resolve();
    expect(screen.queryByText(msg('settings.aboutRollbackLabel'))).toBeNull();

    rollbackInfo.set({ available: true, version: '1.4.1', reason: '', reasonCode: '' });
    expect(await said('settings.aboutRollbackLabel')).toBe(msg('settings.aboutRollbackAvailable', { version: '1.4.1' }));
    expect(button(msg('ui.rollbackButton', { version: '1.4.1' }))).toBeTruthy();

    const blocked = { available: false, version: '', reason: '', reasonCode: 'selfupdate.rollback_mismatch' };
    rollbackInfo.set(blocked);
    await waitFor(() => expect(screen.queryByRole('button', { name: msg('ui.rollbackButton', { version: '1.4.1' }) })).toBeNull());
    expect(await said('settings.aboutRollbackLabel')).toBe(rollbackReason(blocked));
  });
});

describe('the about tab: saving the logs', () => {
  it('saves the archive, opens its folder and says where it is', async () => {
    vi.mocked(exportLogs).mockResolvedValueOnce(BUNDLE);
    open();
    expect(await said('settings.aboutLogsLabel')).toBe(msg('settings.aboutLogsSaveHint'));
    await user().click(button(msg('settings.aboutLogsDownloadButton')));
    await waitFor(() => expect(openFolder).toHaveBeenCalledWith(BUNDLE.dir));
    expect(toastTexts()).toEqual([msg('settings.aboutLogsSavedToast')]);
    expect(await said('settings.aboutLogsLabel')).toBe(`typhon-logs.zip · ${bytesLabel(2048)} ${msg('settings.aboutLogsAttachHint')}`);
  });

  it('says the path when the folder cannot be opened', async () => {
    vi.mocked(exportLogs).mockResolvedValueOnce(BUNDLE);
    vi.mocked(openFolder).mockRejectedValueOnce(new Error('no shell'));
    open();
    await user().click(button(msg('settings.aboutLogsDownloadButton')));
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.aboutLogsSavedToast'), BUNDLE.path]));
  });

  it('says a failed save in catalogued words', async () => {
    vi.mocked(exportLogs).mockRejectedValueOnce(new Error('open C:\\Users\\p\\x: access is denied'));
    open();
    await user().click(button(msg('settings.aboutLogsDownloadButton')));
    await waitFor(() => expect(toastTexts()).toEqual([msg('state.logsReasonGeneric')]));
    expect(await said('settings.aboutLogsLabel')).toBe(msg('settings.aboutLogsSaveHint'));
  });

  it('says it is saving while the archive is made', async () => {
    const save = later<typeof BUNDLE>();
    vi.mocked(exportLogs).mockReturnValueOnce(save.promise);
    open();
    const press = button(msg('settings.aboutLogsDownloadButton'));
    await user().click(press);
    await waitFor(() => expect(press.textContent).toContain(msg('settings.aboutLogsSavingEllipsis')));
    save.resolve(BUNDLE);
    await waitFor(() => expect(press.textContent).toContain(msg('settings.aboutLogsDownloadButton')));
  });
});

describe('the about tab: sending the logs', () => {
  it('asks first, then says each stage up to the archive id', async () => {
    const send = later<{ id: string; dropped: string[] }>();
    vi.mocked(sendLogs).mockReturnValueOnce(send.promise);
    open();
    expect(await said('settings.aboutLogsSendButton')).toContain(msg('settings.aboutLogsSendConfirmNote'));
    await confirmSend();
    expect(screen.getByText(msg('settings.aboutLogsPreparingEllipsis'))).toBeTruthy();
    expect(screen.getByRole('progressbar').getAttribute('aria-busy')).toBe('true');

    emit({ state: 'sending', sentBytes: 42, totalBytes: 100 });
    expect(screen.getByText(msg('settings.aboutLogsSendingProgress', { percent: 42 }))).toBeTruthy();
    expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBe('42');

    emit({ state: 'waiting' });
    expect(screen.getByText(msg('settings.aboutLogsWaitingEllipsis'))).toBeTruthy();

    send.resolve({ id: 'AB12CD', dropped: ['typhon.1.log'] });
    emit({ state: 'success' });
    expect(await screen.findByText(msg('settings.aboutLogsSentStatus'))).toBeTruthy();
    expect(screen.queryByRole('progressbar')).toBeNull();
    expect(await said('settings.aboutLogsSendResultLabel')).toBe('AB12CD');
    expect(screen.getByText(msg('settings.aboutLogsSendDroppedNote', { files: 'typhon.1.log' }))).toBeTruthy();
    await user().click(button(msg('settings.aboutLogsSendIdCopyLabel')));
    await waitFor(() => expect(Clipboard.SetText).toHaveBeenCalledWith('AB12CD'));
    expect(toastTexts()).toEqual([msg('settings.aboutLogsSendIdCopiedToast')]);
  });

  it('sends nothing when the question is cancelled', async () => {
    open();
    await user().click(await screen.findByRole('button', { name: msg('settings.aboutLogsSendButton') }));
    const dialog = await screen.findByRole('dialog');
    await user().keyboard('{Escape}');
    await waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(sendLogs).not.toHaveBeenCalled();
  });

  it('says a failed send in catalogued words, and a second press asks and sends again', async () => {
    vi.mocked(sendLogs).mockRejectedValueOnce(new Error('typhon:diagnostics.log_upload_network: dial tcp 10.0.0.1:443'));
    open();
    await confirmSend();
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain(msg('errLogs.uploadNetwork'));
    expect(alert.textContent).toContain(msg('settings.aboutLogsSendFailedHint'));
    expect(alert.textContent).not.toContain('dial tcp');
    expect(screen.queryByRole('progressbar')).toBeNull();

    vi.mocked(sendLogs).mockResolvedValueOnce({ id: 'EF34', dropped: [] });
    await user().click(button(msg('settings.aboutLogsSendButton')));
    const dialog = await screen.findByRole('dialog');
    expect(screen.queryByText(msg('errLogs.uploadNetwork'))).toBeNull();
    await user().click(within(dialog).getByRole('button', { name: msg('settings.aboutLogsSendConfirmButton') }));
    await waitFor(() => expect(sendLogs).toHaveBeenCalledTimes(2));
    emit({ state: 'success' });
    expect(await screen.findByText(msg('settings.aboutLogsSentStatus'))).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('says it is sending on the button while the archive goes', async () => {
    const send = later<{ id: string; dropped: string[] }>();
    vi.mocked(sendLogs).mockReturnValueOnce(send.promise);
    open();
    const press = await screen.findByRole('button', { name: msg('settings.aboutLogsSendButton') });
    await confirmSend();
    expect(press.textContent).toContain(msg('settings.aboutLogsSendingEllipsis'));
    send.resolve({ id: 'AB12CD', dropped: [] });
    await waitFor(() => expect(press.textContent).toContain(msg('settings.aboutLogsSendButton')));
  });
});

describe('the about tab: support and the legal documents', () => {
  it('opens a new support request about the launcher', async () => {
    open();
    expect(await said('tickets.settingsLabel')).toBe(msg('tickets.settingsHint'));
    await user().click(button(msg('tickets.writeSupport')));
    expect(navigate).toHaveBeenCalledWith('support', { view: 'new', topic: 'launcher' });
  });

  it('opens each legal document by its title', async () => {
    open();
    await screen.findByText('Privacy policy');
    const opens = screen.getAllByRole('button', { name: new RegExp(msg('common.open')) });
    expect(opens).toHaveLength(3);
    await user().click(opens[1]);
    expect(await screen.findByRole('dialog', { name: 'Privacy policy' })).toBeTruthy();
    await waitFor(() => expect(getLegalDocument).toHaveBeenCalledWith('privacy'));
  });

  it('opens the sources notice for review', async () => {
    open();
    expect(await said('settings.aboutSourcesNoticeLabel')).toBe(msg('settings.aboutSourcesNoticeSub'));
    const opens = screen.getAllByRole('button', { name: new RegExp(msg('common.open')) });
    await user().click(opens[opens.length - 1]);
    expect(await screen.findByRole('dialog', { name: msg('modals.sourcesNoticeReviewTitle') })).toBeTruthy();
  });
});

// Each control is named by its own words and described by its row's hint, the way the other tabs'
// rows do it: a screen reader hears what the button does and the row's sentence about it.
describe('the about tab: each control described by its row', () => {
  it('describes the update, history, logs, support and community buttons by their rows', async () => {
    selfUpdateStatus.set({ state: 'idle', currentVersion: '1.4.2' });
    open();
    await screen.findByText('Terms of use');
    expect(description(button(msg('settings.aboutCheckButtonLabel')))).toBe(msg('settings.aboutInstalledVersion', { version: '1.4.2' }));
    expect(description(button(msg('settings.aboutHistoryButtonLabel')))).toBe(`${msg('settings.aboutHistoryEmpty')} ${msg('settings.aboutHistoryHasNotes')}`);
    expect(description(button(msg('settings.aboutLogsDownloadButton')))).toBe(msg('settings.aboutLogsSaveHint'));
    expect(description(button(msg('settings.aboutLogsSendButton')))).toBe(msg('settings.aboutLogsSendConfirmNote'));
    expect(description(button(msg('tickets.writeSupport')))).toBe(msg('tickets.settingsHint'));
    expect(description(button(msg('discord.joinButton')))).toBe(msg('discord.communitySub'));
  });

  it('names each legal opener by its document, and describes the sources notice opener by its row', async () => {
    open();
    await screen.findByText('Terms of use');
    for (const title of ['Terms of use', 'Privacy policy']) {
      const opener = button(msg('settings.aboutLegalOpenNamed', { title }));
      expect(opener.textContent?.trim()).toBe(msg('common.open'));
    }
    const notice = button(msg('settings.aboutLegalOpenNamed', { title: msg('settings.aboutSourcesNoticeLabel') }));
    expect(description(notice)).toBe(msg('settings.aboutSourcesNoticeSub'));
  });

  it('describes the copy button by the archive id it copies', async () => {
    vi.mocked(sendLogs).mockResolvedValueOnce({ id: 'AB12CD', dropped: [] });
    open();
    await confirmSend();
    emit({ state: 'success' });
    expect(description(await screen.findByRole('button', { name: msg('settings.aboutLogsSendIdCopyLabel') }))).toBe('AB12CD');
  });

  it('says the stage of a running send under the row, not in the button\'s description', async () => {
    const sending = later<{ id: string; dropped: string[] }>();
    vi.mocked(sendLogs).mockReturnValueOnce(sending.promise);
    open();
    await confirmSend();
    emit({ state: 'sending', sentBytes: 42, totalBytes: 100 });
    const said42 = await screen.findByText(msg('settings.aboutLogsSendingProgress', { percent: 42 }));

    const send = document.querySelector<HTMLButtonElement>('button[aria-busy="true"]')!;
    expect(description(send)).toBe(msg('settings.aboutLogsSendConfirmNote'));
    expect(description(send)).not.toContain(said42.textContent?.trim() ?? '');
    expect(send.closest('.settings-row')?.contains(said42)).toBe(true);

    sending.resolve({ id: 'AB12CD', dropped: [] });
    emit({ state: 'success' });
    await screen.findByRole('button', { name: msg('settings.aboutLogsSendIdCopyLabel') });
  });

  it('keeps the send stage, the failure and the progress under the send row, its hint the same', async () => {
    vi.mocked(sendLogs).mockRejectedValueOnce(new Error('typhon:diagnostics.log_upload_network: dial tcp'));
    open();
    await confirmSend();
    const alert = await screen.findByRole('alert');
    const send = button(msg('settings.aboutLogsSendButton'));
    expect(description(send)).toBe(msg('settings.aboutLogsSendConfirmNote'));
    // The row holds the send button and its failure, and not the save button of the row above.
    let row: HTMLElement | null = alert.parentElement;
    while (row && !row.contains(send)) row = row.parentElement;
    expect(row?.contains(button(msg('settings.aboutLogsDownloadButton')))).toBe(false);
  });
});

// A source tripwire: the about tab builds no sentence in its markup and draws every row with the
// shared settings row.
describe('the about tab: markup', () => {
  const markup = about.replace(/<script[\s\S]*?<\/script>|<style[\s\S]*?<\/style>/g, '');
  // What the markup writes itself: its catalog keys and other quoted strings left out.
  const written = markup.replace(/<!--[\s\S]*?-->/g, '').replace(/'[^']*'/g, "''");
  const script = /<script[^>]*>([\s\S]*?)<\/script>/.exec(about)?.[1] ?? '';

  it('glues no words together: separators, dashes and product words come from the catalog', () => {
    expect(written).not.toMatch(/·|—|CrossOver|devmock/);
    expect(script).not.toMatch(/['`][^'`]*(·|—)[^'`]*['`]/);
  });

  it('has no local row of its own', () => {
    expect(about).not.toMatch(/class="rows?\b|row-text|row-label|row-sub/);
    expect((markup.match(/<SettingsRow\b/g) ?? []).length).toBeGreaterThanOrEqual(14);
  });
});

// A button its own press puts to work keeps focus: busy, not disabled. A button that is off says why,
// the reason first.
describe('the about tab: buttons at work and buttons that are off', () => {
  const atWork = (target: HTMLButtonElement) => [target.disabled, target.getAttribute('aria-busy'), document.activeElement === target];

  it('keeps Check in focus and busy while it checks', async () => {
    const check = later<SelfUpdateStatus>();
    vi.mocked(checkForUpdate).mockReturnValueOnce(check.promise);
    open();
    const press = button(msg('settings.aboutCheckButtonLabel'));
    press.focus();
    await user().keyboard('{Enter}');
    await waitFor(() => expect(checkForUpdate).toHaveBeenCalledOnce());
    expect(atWork(press)).toEqual([false, 'true', true]);
    check.resolve({ state: 'idle', currentVersion: '1.4.2' });
    await waitFor(() => expect(press.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(press);
  });

  it('keeps Download logs in focus and busy while it saves', async () => {
    const save = later<typeof BUNDLE>();
    vi.mocked(exportLogs).mockReturnValueOnce(save.promise);
    open();
    const press = button(msg('settings.aboutLogsDownloadButton'));
    press.focus();
    await user().keyboard('{Enter}');
    await waitFor(() => expect(exportLogs).toHaveBeenCalledOnce());
    expect(atWork(press)).toEqual([false, 'true', true]);
    save.resolve(BUNDLE);
    await waitFor(() => expect(press.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(press);
  });

  it('returns focus to Send to us after the question, busy while it sends, and a press there tries again after a failure', async () => {
    const send = later<{ id: string; dropped: string[] }>();
    vi.mocked(sendLogs).mockReturnValueOnce(send.promise);
    open();
    const press = await screen.findByRole('button', { name: msg('settings.aboutLogsSendButton') }) as HTMLButtonElement;
    press.focus();
    await user().keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    await user().click(within(dialog).getByRole('button', { name: msg('settings.aboutLogsSendConfirmButton') }));
    await waitFor(() => expect(sendLogs).toHaveBeenCalledOnce());
    await waitFor(() => expect(dialog.isConnected).toBe(false));
    await waitFor(() => expect(document.activeElement).toBe(press));
    expect(atWork(press)).toEqual([false, 'true', true]);

    send.reject(new Error('typhon:diagnostics.log_upload_network: dial tcp'));
    await screen.findByRole('alert');
    expect(atWork(press)).toEqual([false, null, true]);
    await user().keyboard('{Enter}');
    expect(await screen.findByRole('dialog')).toBeTruthy();
    expect(sendLogs).toHaveBeenCalledOnce();
  });

  it('says why the update history is off, the reason first', async () => {
    open();
    const history = button(msg('settings.aboutHistoryButtonLabel'));
    expect(history.disabled).toBe(true);
    expect(description(history)).toBe(`${msg('settings.aboutHistoryEmpty')} ${msg('settings.aboutHistoryHasNotes')}`);
    releaseNotes.set({ currentVersion: '1.4.2', unseen: [], history: [{ version: '1.4.2', publishedAt: '2026-11-01', changes: [] }] });
    await waitFor(() => expect(history.disabled).toBe(false));
    expect(description(history)).toBe(msg('settings.aboutHistoryHasNotes'));
  });
});
