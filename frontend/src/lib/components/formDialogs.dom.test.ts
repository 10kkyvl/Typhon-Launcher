import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { proposeLibraryPath, selectFolder, type Settings } from '../services/settings';
import { createLibrary } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
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
  getStorageInfoFor: vi.fn(async () => ({ volume: 'D', filesystem: 'NTFS', freeBytes: 1, totalBytes: 2 })),
}));
vi.mock('../stores/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/settings')>()),
  createLibrary: vi.fn(),
}));

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
  toasts.set([]);
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe('LibrarySetupModal', () => {
  it('says an unsuitable folder in a danger notice; "create" stays focused and busy while the library is made', async () => {
    vi.mocked(selectFolder).mockResolvedValue('D:/Games');
    vi.mocked(proposeLibraryPath).mockRejectedValueOnce(new Error('nope')).mockResolvedValueOnce('D:/Games/Typhon');
    const created = deferred<Settings>();
    vi.mocked(createLibrary).mockReturnValueOnce(created.promise);
    render(LibrarySetupModal, { target: app, props: { open: true } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.librarySetupDefaultTitle') });
    const browse = within(dialog).getByRole('button', { name: msg('modals.librarySetupBrowse') });

    await keys.click(browse);
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.librarySetupFolderUnsuitable'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();

    await keys.click(browse);
    const create = within(dialog).getByRole('button', { name: msg('modals.librarySetupCreate') });
    await vi.waitFor(() => expect(create).toHaveProperty('disabled', false));
    create.focus();
    await keys.keyboard('{Enter}');

    expect(create.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(create);
    created.resolve({} as Settings);
  });
});

describe('LibrarySetupModal: the note it opens with', () => {
  it('is an info notice read with the dialog', () => {
    render(LibrarySetupModal, { target: app, props: { open: true, note: 'Games stay where they are.' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.librarySetupDefaultTitle') });

    const notice = within(dialog).getByText('Games stay where they are.').closest('[data-tone="info"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });
});
