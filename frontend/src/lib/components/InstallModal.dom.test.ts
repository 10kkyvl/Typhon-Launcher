import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { installErrorText } from '../install/installErrors';
import { confirmExecutable, inspectDownload, planSpace, type Installation, type PlanInfo } from '../services/install';
import { installations } from '../stores/install';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import InstallModal from './InstallModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  inspectDownload: vi.fn(),
  planSpace: vi.fn(),
  confirmExecutable: vi.fn(),
}));

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

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  installations.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

function openDialog() {
  render(InstallModal, { target: app, props: { open: true, downloadId: 'd1' } });
  return screen.getByRole('dialog', { name: msg('modals.installTitle') });
}

describe('InstallModal: choosing the game\'s executable', () => {
  it('is a radio group with one Tab stop whose arrows move the choice; "confirm" stays focused and busy while it is sent', async () => {
    installations.set([installation()]);
    const sent = deferred<void>();
    vi.mocked(confirmExecutable).mockReturnValueOnce(sent.promise);
    const dialog = openDialog();
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

    expect(vi.mocked(confirmExecutable)).toHaveBeenCalledWith('i1', 'C:/Games/Hades/x64/Hades.exe');
    expect(confirm.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(confirm);
    sent.resolve();
  });
});

describe('InstallModal: messages', () => {
  it('says why an install stopped in a danger notice', () => {
    installations.set([installation({ status: 'failed', error: 'typhon:install.disk_full: full' })]);
    const dialog = openDialog();

    const text = within(dialog).getByText(installErrorText('typhon:install.disk_full: full'));
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('says a failed space check by the destination field, which is named by its label and marked wrong', async () => {
    vi.mocked(inspectDownload).mockResolvedValue({
      plan: { type: 'archive_zip', sourcePath: 'D:/dl/hades.zip', destination: 'C:/Games/Hades', installerPath: '', candidates: null } as unknown as PlanInfo['plan'],
      downloadId: 'd1',
      name: 'Hades',
      requiredBytes: 10,
      freeBytes: 100,
      seeding: false,
      mode: 'copy',
      sameVolume: false,
      roots: null,
    });
    vi.mocked(planSpace).mockRejectedValue(new Error('typhon:install.destination_invalid: bad'));
    const dialog = openDialog();

    const field = await within(dialog).findByRole('textbox', { name: msg('modals.installDestinationFolder') });
    const alert = await within(dialog).findByRole('alert');
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect((field.getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
  });
});
