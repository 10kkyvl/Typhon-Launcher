import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { AccountError } from '../services/account';
import { getLegalDocument, type LegalDocument } from '../services/legal';
import { proposeLibraryPath, selectFolder, type Settings } from '../services/settings';
import { createLibrary } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { authState, deleteAccount } from '../stores/user';
import { user } from '../testing/dom';
import AvatarCropModal from './AvatarCropModal.svelte';
import DeleteAccountModal from './DeleteAccountModal.svelte';
import LegalDocumentModal from './LegalDocumentModal.svelte';
import LibrarySetupModal from './LibrarySetupModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../stores/user', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/user')>()),
  deleteAccount: vi.fn(),
}));
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
vi.mock('../services/legal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/legal')>()),
  getLegalDocument: vi.fn(),
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
  authState.set('bootstrapping');
  toasts.set([]);
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe('DeleteAccountModal', () => {
  it('says a wrong password by its field and marks it wrong; the delete button stays focused and busy while it runs', async () => {
    authState.set('authenticated');
    const deletion = deferred<void>();
    vi.mocked(deleteAccount).mockReturnValueOnce(deletion.promise);
    render(DeleteAccountModal, { target: app, props: { onclose: vi.fn() } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('profile.deleteAccountConfirmTitle') });
    const password = within(dialog).getByLabelText(msg('profile.deleteAccountPassword'));
    await keys.type(password, 'secret');
    const remove = within(dialog).getByRole('button', { name: msg('profile.deleteAccountConfirmButton') });
    remove.focus();

    await keys.keyboard('{Enter}');

    expect(remove.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(remove);

    deletion.reject(new AccountError('invalid_credentials'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('profile.deleteAccountWrongPassword'));
    expect(password.getAttribute('aria-invalid')).toBe('true');
    expect((password.getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
  });

  it('says a failure that is not the password in a danger notice', async () => {
    authState.set('authenticated');
    vi.mocked(deleteAccount).mockRejectedValueOnce(new AccountError('server_error'));
    render(DeleteAccountModal, { target: app, props: { onclose: vi.fn() } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('profile.deleteAccountConfirmTitle') });
    const password = within(dialog).getByLabelText(msg('profile.deleteAccountPassword'));
    await keys.type(password, 'secret');

    await keys.click(within(dialog).getByRole('button', { name: msg('profile.deleteAccountConfirmButton') }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(password.getAttribute('aria-invalid')).not.toBe('true');
  });
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

describe('the crop dialogs', () => {
  it('AvatarCropModal says an image that cannot be read in a danger notice, and keeps "save" focused and busy while it saves', async () => {
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
    const { rerender } = render(AvatarCropModal, { target: app, props: { open: true, src: 'blob:broken', onsave: vi.fn() } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.avatarCropTitle') });
    (dialog.querySelector('img') as HTMLImageElement).dispatchEvent(new Event('error'));

    const text = await within(dialog).findByText(msg('modals.avatarCropLoadFailed'));
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();

    const save = within(dialog).getByRole('button', { name: msg('common.save') });
    save.focus();
    await rerender({ saving: true });
    expect(save.getAttribute('aria-busy')).toBe('true');
  });
});

describe('LegalDocumentModal', () => {
  it('says it is loading with a labelled spinner, and a failed load in a danger notice', async () => {
    const loaded = deferred<LegalDocument>();
    vi.mocked(getLegalDocument).mockReturnValueOnce(loaded.promise);
    render(LegalDocumentModal, { target: app, props: { open: true, documentId: 'privacy', title: 'Privacy' } });
    const dialog = screen.getByRole('dialog', { name: 'Privacy' });
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('modals.legalLoading'));

    loaded.reject(new Error('offline'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.legalLoadFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});
