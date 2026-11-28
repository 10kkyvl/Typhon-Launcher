import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { proposeLibraryPath, selectFolder, type Settings } from '../services/settings';
import { getStorageInfoFor } from '../services/system';
import { createLibrary } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import { description } from '../testing/hints';
import { bytesLabel } from '../utils/format';
import LibrarySetupModal from './LibrarySetupModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  selectFolder: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));
vi.mock('../services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/system')>()),
  getStorageInfoFor: vi.fn(),
}));
vi.mock('../stores/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/settings')>()),
  createLibrary: vi.fn(),
}));

const LONG = 'D:/Games/A folder whose name runs on for long enough to need a second line/Typhon Library';
const DISK = { volume: 'D', filesystem: 'NTFS', freeBytes: 1_000_000_000, totalBytes: 2_000_000_000 };

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(getStorageInfoFor).mockResolvedValue(DISK as Awaited<ReturnType<typeof getStorageInfoFor>>);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  // A press the dialog failed to refuse leaves answers queued for the next test.
  for (const mock of [selectFolder, proposeLibraryPath, getStorageInfoFor, createLibrary]) vi.mocked(mock).mockReset();
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function open() {
  render(LibrarySetupModal, { target: app, props: { open: true } });
  const dialog = screen.getByRole('dialog', { name: msg('modals.librarySetupDefaultTitle') });
  return {
    dialog,
    keys: user(),
    browse: within(dialog).getByRole('button', { name: msg('modals.librarySetupBrowse') }),
    create: () => within(dialog).getByRole('button', { name: msg('modals.librarySetupCreate') }),
  };
}

// The words the player sees, whether they sit in a field's value or in the page's text.
function shows(dialog: HTMLElement, text: string): boolean {
  return [...dialog.querySelectorAll('input')].some((input) => input.value === text) || (dialog.textContent ?? '').includes(text);
}

describe('LibrarySetupModal: the folder', () => {
  it('names the folder and says none is chosen yet; Create waits for one', () => {
    const { dialog, create } = open();
    expect(within(dialog).getByText(msg('modals.librarySetupFolderLabel'))).toBeTruthy();
    expect(within(dialog).getByText(msg('modals.librarySetupFolderLabel')).closest('label')).toBeNull();
    expect(dialog.querySelector(`[placeholder="${msg('modals.librarySetupFolderPlaceholder')}"]`) ?? within(within(dialog).getByRole('group', { name: msg('modals.librarySetupFolderLabel') })).queryByText(msg('modals.librarySetupFolderPlaceholder'))).not.toBeNull();
    expect(create().getAttribute('aria-disabled')).toBe('true');
  });

  it('asks the picker for a folder, shows the library path it proposes and the drive under it; Create is ready', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    const { dialog, keys, browse, create } = open();

    await keys.click(browse);

    expect(vi.mocked(selectFolder)).toHaveBeenCalledWith(msg('modals.librarySetupChooseFolder'));
    expect(vi.mocked(proposeLibraryPath)).toHaveBeenCalledWith('D:/Games');
    await vi.waitFor(() => expect(shows(dialog, LONG)).toBe(true));
    expect(vi.mocked(getStorageInfoFor)).toHaveBeenCalledWith('D:/Games');
    await within(dialog).findByText(`${msg('modals.librarySetupDiskName', { volume: 'D' })} · NTFS`);
    expect(within(dialog).getByText(msg('modals.librarySetupDiskFree', { free: bytesLabel(DISK.freeBytes), total: bytesLabel(DISK.totalBytes) }))).toBeTruthy();
    expect(create().getAttribute('aria-disabled')).toBeNull();
    expect(document.activeElement).toBe(browse);
  });

  it('creates the library in the parent folder the player picked', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    vi.mocked(createLibrary).mockResolvedValueOnce({} as Settings);
    const { dialog, keys, browse, create } = open();
    await keys.click(browse);
    await vi.waitFor(() => expect(create().getAttribute('aria-disabled')).toBeNull());

    await keys.click(create());

    expect(vi.mocked(createLibrary)).toHaveBeenCalledWith('D:/Games');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('changes nothing when the picker is closed without a folder', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('');
    const { keys, browse, create } = open();

    await keys.click(browse);

    expect(vi.mocked(proposeLibraryPath)).not.toHaveBeenCalled();
    expect(create().getAttribute('aria-disabled')).toBe('true');
    expect(get(toasts)).toEqual([]);
  });

  it('says in the dialog that the picker could not open, not in a toast', async () => {
    vi.mocked(selectFolder).mockRejectedValueOnce(new Error('no dialog'));
    const { dialog, keys, browse, create } = open();

    await keys.click(browse);

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.librarySetupFolderDialogFailed'));
    expect(get(toasts)).toEqual([]);
    expect(create().getAttribute('aria-disabled')).toBe('true');
  });

  it('shows the path as text read whole in a group named by the folder label; Browse is described by the path and the drive', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    const { dialog, keys, browse } = open();
    const group = within(dialog).getByRole('group', { name: msg('modals.librarySetupFolderLabel') });
    expect(within(group).queryByRole('textbox')).toBeNull();
    expect(description(browse)).toBe(msg('modals.librarySetupFolderPlaceholder'));

    await keys.click(browse);

    const drive = `${msg('modals.librarySetupDiskName', { volume: 'D' })} · NTFS`;
    await within(group).findByText(drive);
    expect(within(group).getByText(LONG).tagName).toBe('P');
    expect(description(browse)).toBe(`${LONG} ${drive} ${msg('modals.librarySetupDiskFree', { free: bytesLabel(DISK.freeBytes), total: bytesLabel(DISK.totalBytes) })}`);
    expect(dialog.querySelectorAll('[title]')).toHaveLength(0);
  });

  it('keeps Browse busy and focused while the picker is up, and opens no second picker', async () => {
    let answer!: (path: string) => void;
    vi.mocked(selectFolder).mockReturnValueOnce(new Promise<string>((resolve) => (answer = resolve)));
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    const { keys, browse } = open();
    browse.focus();

    await keys.keyboard('{Enter}');
    expect(browse.getAttribute('aria-busy')).toBe('true');
    await keys.keyboard('{Enter}');
    await keys.click(browse);
    expect(vi.mocked(selectFolder)).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(browse);

    answer('D:/Games');
    await vi.waitFor(() => expect(browse.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(browse);
  });

  it('keeps the path and says so when the free space cannot be read', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    vi.mocked(getStorageInfoFor).mockRejectedValueOnce(new Error('no volume'));
    const { dialog, keys, browse, create } = open();

    await keys.click(browse);

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.librarySetupFreeSpaceUnknown'));
    expect(shows(dialog, LONG)).toBe(true);
    expect(within(dialog).queryByText(`${msg('modals.librarySetupDiskName', { volume: 'D' })} · NTFS`)).toBeNull();
    expect(create().getAttribute('aria-disabled')).toBeNull();
  });
});

describe('LibrarySetupModal: held controls and late answers', () => {
  const describedBy = (el: Element) =>
    (el.getAttribute('aria-describedby') ?? '')
      .split(' ')
      .filter(Boolean)
      .map((id) => document.getElementById(id)?.textContent?.trim());

  async function picked() {
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    const opened = open();
    await opened.keys.click(opened.browse);
    await vi.waitFor(() => expect(opened.create().getAttribute('aria-disabled')).toBeNull());
    return opened;
  }

  it('holds Create with a reason until a folder is chosen', () => {
    const { create } = open();
    expect(create().getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(create())).toContain(msg('modals.librarySetupFolderPlaceholder'));
  });

  it('holds Cancel and Browse with a reason while the library is made, and sends one Create', async () => {
    const making = held<Settings>();
    vi.mocked(createLibrary).mockReturnValueOnce(making.promise);
    const { dialog, keys, browse, create } = await picked();
    create().focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(createLibrary).mock.calls).toEqual([['D:/Games']]);
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    for (const button of [cancel, browse]) {
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(describedBy(button)).toContain(msg('modals.actionRunning'));
    }
    making.resolve({} as Settings);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('drops a folder picked for an earlier opening: the dialog opened again starts empty', async () => {
    const picking = held<string>();
    vi.mocked(selectFolder).mockReturnValueOnce(picking.promise);
    vi.mocked(proposeLibraryPath).mockResolvedValue(LONG);
    const view = render(LibrarySetupModal, { target: app, props: { open: true } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: msg('modals.librarySetupBrowse') }));
    await view.rerender({ open: false });
    await view.rerender({ open: true });

    picking.resolve('D:/Games');
    await new Promise((resolve) => setTimeout(resolve, 0));

    const dialog = screen.getByRole('dialog');
    expect(vi.mocked(proposeLibraryPath)).not.toHaveBeenCalled();
    expect(within(within(dialog).getByRole('group', { name: msg('modals.librarySetupFolderLabel') })).getByText(msg('modals.librarySetupFolderPlaceholder'))).toBeTruthy();
  });

  it('a library made after the dialog was closed and opened again closes nothing and calls nothing back', async () => {
    const making = held<Settings>();
    vi.mocked(createLibrary).mockReturnValueOnce(making.promise);
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    const onready = vi.fn();
    const view = render(LibrarySetupModal, { target: app, props: { open: true, onready } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: msg('modals.librarySetupBrowse') }));
    const create = screen.getByRole('button', { name: msg('modals.librarySetupCreate') });
    await vi.waitFor(() => expect(create.getAttribute('aria-disabled')).toBeNull());
    await keys.click(create);
    await view.rerender({ open: false });
    await view.rerender({ open: true });

    making.resolve({} as Settings);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(onready).not.toHaveBeenCalled();
  });
});
