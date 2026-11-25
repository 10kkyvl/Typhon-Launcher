import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { proposeLibraryPath, selectFolder, type Settings } from '../services/settings';
import { getStorageInfoFor } from '../services/system';
import { createLibrary } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { bytesLabel } from '../utils/format';
import LibrarySetupModal from './LibrarySetupModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
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
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
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
    expect(dialog.querySelector(`[placeholder="${msg('modals.librarySetupFolderPlaceholder')}"]`) ?? within(dialog).queryByText(msg('modals.librarySetupFolderPlaceholder'))).not.toBeNull();
    expect(create()).toHaveProperty('disabled', true);
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
    expect(create()).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(browse);
  });

  it('creates the library in the parent folder the player picked', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('D:/Games');
    vi.mocked(proposeLibraryPath).mockResolvedValueOnce(LONG);
    vi.mocked(createLibrary).mockResolvedValueOnce({} as Settings);
    const { dialog, keys, browse, create } = open();
    await keys.click(browse);
    await vi.waitFor(() => expect(create()).toHaveProperty('disabled', false));

    await keys.click(create());

    expect(vi.mocked(createLibrary)).toHaveBeenCalledWith('D:/Games');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('changes nothing when the picker is closed without a folder', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('');
    const { keys, browse, create } = open();

    await keys.click(browse);

    expect(vi.mocked(proposeLibraryPath)).not.toHaveBeenCalled();
    expect(create()).toHaveProperty('disabled', true);
    expect(get(toasts)).toEqual([]);
  });

  it('says in a toast that the picker could not open', async () => {
    vi.mocked(selectFolder).mockRejectedValueOnce(new Error('no dialog'));
    const { keys, browse, create } = open();

    await keys.click(browse);

    await vi.waitFor(() => expect(get(toasts).map(({ message, kind }) => [message, kind])).toEqual([[msg('modals.librarySetupFolderDialogFailed'), 'danger']]));
    expect(create()).toHaveProperty('disabled', true);
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
    expect(create()).toHaveProperty('disabled', false);
  });
});
