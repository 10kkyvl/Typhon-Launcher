import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { installErrorText } from '../install/installErrors';
import { confirmExecutable, inspectDownload, planSpace, startInstall, type Installation, type PlanInfo } from '../services/install';
import type { Settings } from '../services/settings';
import { installations } from '../stores/install';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import InstallModal from './InstallModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  inspectDownload: vi.fn(),
  planSpace: vi.fn(),
  confirmExecutable: vi.fn(),
  startInstall: vi.fn(),
}));

const coded = (code: string) => new Error(`typhon:${code}`);
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function installation(patch: Partial<Installation> = {}): Installation {
  return {
    id: 'i1',
    downloadId: 'd1',
    gameId: '',
    name: 'Hades',
    type: 'exe_installer',
    status: 'waiting_for_user',
    mode: '',
    sourcePath: '',
    contentRoot: '',
    destination: 'C:/Games/Hades',
    installerPath: '',
    workingDir: '',
    archivePath: '',
    progress: 0,
    currentFile: '',
    bytesDone: 0,
    bytesTotal: 0,
    executable: '',
    candidates: [
      { path: 'C:/Games/Hades/Hades.exe', score: 1 },
      { path: 'C:/Games/Hades/x64/Hades.exe', score: 0.5 },
      { path: 'C:/Games/Hades/Config.exe', score: 0.1 },
    ],
    detectedVersion: '',
    versionSource: '',
    startedAt: '',
    completedAt: null,
    error: '',
    engine: '',
    silent: false,
    ...patch,
  };
}

const LONG_SOURCE = 'D:/Downloads/Hades Deluxe Edition with every update and the soundtrack/hades-setup.zip';

function planFor(id: string, name: string): PlanInfo {
  return {
    plan: { type: 'archive_zip', sourcePath: id === 'd1' ? LONG_SOURCE : `D:/dl/${name}.zip`, destination: `C:/Games/${name}`, installerPath: '', candidates: null } as unknown as PlanInfo['plan'],
    downloadId: id,
    name,
    requiredBytes: 10,
    freeBytes: 100,
    seeding: false,
    mode: 'copy',
    sameVolume: false,
    roots: null,
  };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(inspectDownload).mockImplementation(async (id: string) => planFor(id, id === 'd1' ? 'Hades' : 'Celeste'));
  vi.mocked(planSpace).mockImplementation(async () => ({ requiredBytes: 10, freeBytes: 100, sameVolume: false, mode: 'copy', enough: true, destinationBusy: false, suggested: '', reusable: false }));
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  installations.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function openDialog(downloadId = 'd1') {
  const view = render(InstallModal, { target: app, props: { open: true, downloadId } });
  return { view, dialog: screen.getByRole('dialog', { name: msg('modals.installTitle') }) };
}

async function openPlan() {
  const opened = openDialog();
  const install = await within(opened.dialog).findByRole('button', { name: msg('modals.installExtractAndInstall') });
  await vi.waitFor(() => expect(install.getAttribute('aria-disabled')).toBeNull());
  return { ...opened, install };
}

describe("InstallModal: choosing the game's executable", () => {
  it('is a radio group with one Tab stop whose arrows move the choice; "confirm" stays focused and busy while it is sent', async () => {
    installations.set([installation()]);
    const sent = held<void>();
    vi.mocked(confirmExecutable).mockReturnValueOnce(sent.promise);
    const { dialog } = openDialog();
    const keys = user();
    const group = within(dialog).getByRole('radiogroup', { name: msg('modals.installExecutablesAriaLabel') });
    const radios = within(group).getAllByRole('radio');
    await vi.waitFor(() => expect(radios[0].getAttribute('aria-checked')).toBe('true'));
    expect(radios.map((radio) => radio.tabIndex)).toEqual([0, -1, -1]);

    radios[0].focus();
    await keys.keyboard('{ArrowDown}');

    expect(radios[1].getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(radios[1]);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([-1, 0, -1]);

    const confirm = within(dialog).getByRole('button', { name: msg('modals.installConfirm') });
    confirm.focus();
    await keys.keyboard('{Enter}');

    expect(vi.mocked(confirmExecutable).mock.calls).toEqual([['i1', 'C:/Games/Hades/x64/Hades.exe']]);
    expect(confirm.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(confirm);
    sent.resolve();
  });

  it('reads each candidate with its whole path, with no native title', async () => {
    installations.set([installation()]);
    const { dialog } = openDialog();
    const radios = within(dialog).getAllByRole('radio');
    expect(describedBy(radios[1])).toEqual(['C:/Games/Hades/x64/Hades.exe']);
    expect(dialog.querySelector('[title]')).toBeNull();
  });

  it('holds the list, Close and the cancel with a reason while the choice is sent', async () => {
    installations.set([installation()]);
    const sent = held<void>();
    vi.mocked(confirmExecutable).mockReturnValueOnce(sent.promise);
    const { dialog } = openDialog();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('modals.installConfirm') }));

    expect(within(dialog).getByRole('radiogroup').getAttribute('aria-disabled')).toBe('true');
    for (const name of [msg('common.close'), msg('modals.installCancelInstall')]) {
      const button = within(dialog).getAllByRole('button', { name }).at(-1) as HTMLElement;
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(button).toHaveProperty('disabled', false);
      expect(describedBy(button)).toContain(msg('modals.actionRunning'));
    }
    sent.resolve();
  });

  it('says a refused choice in the dialog by its code, not in a toast', async () => {
    installations.set([installation()]);
    vi.mocked(confirmExecutable).mockRejectedValueOnce(coded('install.not_found'));
    const { dialog } = openDialog();
    const confirm = within(dialog).getByRole('button', { name: msg('modals.installConfirm') });
    confirm.focus();
    await user().keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errInstall.installNotFound'));
    await vi.waitFor(() => expect(document.activeElement).toBe(alert.closest('[data-tone="danger"]')));
    expect(get(toasts)).toEqual([]);
  });
});

describe('InstallModal: the plan', () => {
  it('shows the source whole', async () => {
    const { dialog } = await openPlan();
    expect(within(dialog).getByText(LONG_SOURCE)).toBeTruthy();
  });

  it('says a failed space check by the destination field, which is named by its label and marked wrong', async () => {
    vi.mocked(planSpace).mockRejectedValue(new Error('typhon:install.destination_invalid: bad'));
    const { dialog } = openDialog();

    const field = await within(dialog).findByRole('textbox', { name: msg('modals.installDestinationFolder') });
    const alert = await within(dialog).findByRole('alert');
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect((field.getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
  });

  it('sends one Start with the plan for any number of presses', async () => {
    const started = held<Installation>();
    vi.mocked(startInstall).mockReturnValueOnce(started.promise);
    const { install } = await openPlan();
    install.focus();
    const keys = user();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(install);

    expect(vi.mocked(startInstall).mock.calls).toEqual([['d1', { destination: 'C:/Games/Hades', mode: '', type: 'archive_zip', installerPath: '' }]]);
    started.resolve(installation({ type: 'archive_zip', status: 'installing', candidates: null }));
  });

  it('holds Cancel and the destination with a reason while the install starts', async () => {
    const started = held<Installation>();
    vi.mocked(startInstall).mockReturnValueOnce(started.promise);
    const { dialog, install } = await openPlan();
    await user().click(install);

    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    const browse = within(dialog).getByRole('button', { name: msg('modals.installBrowse') });
    expect(browse.getAttribute('aria-disabled')).toBe('true');
    started.reject(new Error('stop'));
    await vi.waitFor(() => expect(cancel.getAttribute('aria-disabled')).toBeNull());
  });

  it('says a refused start in the dialog by its code, and focus goes there', async () => {
    vi.mocked(startInstall).mockRejectedValueOnce(coded('install.not_enough_space'));
    const { dialog, install } = await openPlan();
    install.focus();
    await user().keyboard('{Enter}');

    const alerts = await within(dialog).findAllByRole('alert');
    const alert = alerts.find((a) => a.textContent?.trim() === msg('errInstall.installNotEnoughSpace'));
    expect(alert).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement).toBe(alert?.closest('[data-tone="danger"]')));
    expect(get(toasts)).toEqual([]);
  });

  it('says a package it could not read in the dialog, with a Retry, and stays open', async () => {
    vi.mocked(inspectDownload).mockRejectedValueOnce(coded('install.unrecognized_source'));
    const { dialog } = openDialog();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent).toContain(msg('errInstall.installUnrecognizedSource'));
    expect(get(toasts)).toEqual([]);

    await user().click(within(dialog).getByRole('button', { name: msg('common.retry') }));

    await within(dialog).findByRole('button', { name: msg('modals.installExtractAndInstall') });
    expect(vi.mocked(inspectDownload).mock.calls).toEqual([['d1'], ['d1']]);
  });
});

describe('InstallModal: an answer after the dialog moved on', () => {
  it('never shows an install started for one download as the next one’s, and names the one it started', async () => {
    const started = held<Installation>();
    vi.mocked(startInstall).mockReturnValueOnce(started.promise);
    const { dialog, view, install } = await openPlan();
    await user().click(install);
    await view.rerender({ downloadId: 'd2' });
    await within(dialog).findByText('Celeste');

    started.resolve(installation({ type: 'archive_zip', status: 'installing', candidates: null }));
    await settle();

    expect(within(dialog).queryByRole('progressbar')).toBeNull();
    expect(within(dialog).getByText('Celeste')).toBeTruthy();
    expect(get(toasts).map((t) => t.message)).toEqual([msg('modals.installStartedFor', { title: 'Hades' })]);
  });

  it('says a start refused after the dialog moved on once, naming the package, with the reason by code', async () => {
    const started = held<Installation>();
    vi.mocked(startInstall).mockReturnValueOnce(started.promise);
    const { dialog, view, install } = await openPlan();
    await user().click(install);
    await view.rerender({ downloadId: 'd2' });
    await within(dialog).findByText('Celeste');

    started.reject(coded('install.game_busy'));
    await settle();

    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [msg('modals.installRefusedFor', { title: 'Hades', reason: msg('errInstall.installGameBusy') }), 'danger'],
    ]);
  });

  it('a destroyed dialog paints nothing and says a late start once', async () => {
    const started = held<Installation>();
    vi.mocked(startInstall).mockReturnValueOnce(started.promise);
    const { view, install } = await openPlan();
    await user().click(install);
    view.unmount();

    started.resolve(installation({ type: 'archive_zip', status: 'installing', candidates: null }));
    await settle();

    expect(get(toasts).map((t) => t.message)).toEqual([msg('modals.installStartedFor', { title: 'Hades' })]);
    expect(get(installations).map((i) => i.id)).toEqual(['i1']);
  });
});

describe('InstallModal: done', () => {
  it('opens on Done, never on deleting the downloaded files', async () => {
    settings.set({ installCleanupPolicy: 'ask' } as Settings);
    installations.set([installation({ status: 'completed', type: 'archive_zip', candidates: null })]);
    const { dialog } = openDialog();
    const done = within(dialog).getByRole('button', { name: msg('common.done') });
    expect(within(dialog).getByRole('button', { name: msg('modals.installDeleteDownloadedFiles') })).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement).toBe(done));
    settings.set(null);
  });
});

describe('InstallModal: messages', () => {
  it('says why an install stopped in a danger notice', () => {
    installations.set([installation({ status: 'failed', error: 'typhon:install.disk_full: full' })]);
    const { dialog } = openDialog();

    const text = within(dialog).getByText(installErrorText('typhon:install.disk_full: full'));
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();
  });
});
