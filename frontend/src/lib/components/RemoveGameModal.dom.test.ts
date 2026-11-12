import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import RemoveGameModal from './RemoveGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  inspectRemoval: vi.fn(),
  removeGame: vi.fn(),
}));

function info(patch: Partial<RemovalInfo> = {}): RemovalInfo {
  return {
    gameId: 'g1',
    title: 'Hades',
    installDir: '/games/hades',
    installType: 'portable',
    method: 'files',
    owned: true,
    uninstallUnknown: false,
    sizeBytes: 1024,
    sizeUnknown: false,
    dirMissing: false,
    quietUninstall: false,
    running: false,
    busy: false,
    downloadId: '',
    downloadPresent: false,
    downloadSeeding: false,
    downloadBytes: 0,
    downloadPath: '',
    ...patch,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.mocked(inspectRemoval).mockReset();
  vi.mocked(removeGame).mockReset();
  toasts.set([]);
  document.body.replaceChildren();
});

async function openDialog(patch: Partial<RemovalInfo> = {}) {
  vi.mocked(inspectRemoval).mockResolvedValue(info(patch));
  render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades' } });
  const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
  await within(dialog).findByText(msg('modals.removeGameLibrarySub'));
  return dialog;
}

describe('RemoveGameModal: where the game goes', () => {
  it('is one named radio group: Tab reaches the chosen tile, the arrows move the choice', async () => {
    const dialog = await openDialog();
    const keys = user();
    const group = within(dialog).getByRole('radiogroup', { name: msg('modals.removeGameTitle') });
    const disk = within(group).getByRole('radio', { name: new RegExp(msg('modals.removeGameModeDiskLabel')) });
    const library = within(group).getByRole('radio', { name: new RegExp(msg('modals.removeGameModeLibraryLabel')) });
    expect(disk.getAttribute('aria-checked')).toBe('true');
    expect(library.getAttribute('aria-checked')).toBe('false');
    expect(disk.tabIndex).toBe(0);
    expect(library.tabIndex).toBe(-1);

    disk.focus();
    await keys.keyboard('{ArrowRight}');

    expect(library.getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(library);
    expect(within(dialog).getByRole('button', { name: msg('modals.removeGameModeLibraryLabel') })).toBeTruthy();

    await keys.keyboard('{ArrowLeft}');
    expect(disk.getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(disk);
  });

  it('skips the disk tile with the arrows when the game cannot be uninstalled', async () => {
    const dialog = await openDialog({ owned: false, method: 'record' });
    const keys = user();
    const disk = within(dialog).getByRole('radio', { name: new RegExp(msg('modals.removeGameModeDiskLabel')) });
    const library = within(dialog).getByRole('radio', { name: new RegExp(msg('modals.removeGameModeLibraryLabel')) });
    await vi.waitFor(() => expect(library.getAttribute('aria-checked')).toBe('true'));
    expect(disk.getAttribute('aria-disabled')).toBe('true');

    library.focus();
    await keys.keyboard('{ArrowLeft}');

    expect(library.getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(library);
  });
});

describe('RemoveGameModal: messages', () => {
  it('states that the game is running in a warning notice that does not announce', async () => {
    const dialog = await openDialog({ running: true });
    const text = within(dialog).getByText(msg('modals.removeGameRunningWarning'));
    const notice = text.closest('[data-tone="warning"]');
    expect(notice).not.toBeNull();
    expect(notice?.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('says a failed removal in a danger notice inside the dialog', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(new Error('locked'));
    const dialog = await openDialog();
    const keys = user();

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.removeGameModeDiskLabel') }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.removeGameDeleteFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('says it is reading the game with a labelled spinner', async () => {
    const pending = deferred<RemovalInfo>();
    vi.mocked(inspectRemoval).mockReturnValueOnce(pending.promise);
    render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('modals.removeGameLoading', { title: 'Hades' }));
    pending.resolve(info());
    await within(dialog).findByText(msg('modals.removeGameLibrarySub'));
  });
});

describe('RemoveGameModal: removing', () => {
  it('keeps the focus on the pressed button, busy, while the removal runs', async () => {
    const pending = deferred<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const dialog = await openDialog();
    const keys = user();
    const remove = within(dialog).getByRole('button', { name: msg('modals.removeGameModeDiskLabel') });
    remove.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.removeGameWorking') });
    expect(busy).toBe(remove);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(busy).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(busy);
    pending.resolve();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});
