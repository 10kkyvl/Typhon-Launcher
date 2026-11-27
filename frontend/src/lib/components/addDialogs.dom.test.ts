import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { imageTitle, listPlatforms, type PlatformStatus } from '../services/emulators';
import { addEmulatedGame, type LibraryGame } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { sentToGo } from '../testing/gameDialogsTesting';
import AddImageModal from './AddImageModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  imageTitle: vi.fn(),
  imageConsoles: vi.fn(async () => []),
}));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  addEmulatedGame: vi.fn(),
}));

const ps2 = { id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2', homepage: '', extensions: ['.iso'], imageNames: [], path: '', status: 'ready', reason: '', warnings: [] } as PlatformStatus;

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
  vi.mocked(listPlatforms).mockResolvedValue([ps2]);
  vi.mocked(imageTitle).mockResolvedValue('');
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  vi.clearAllMocks();
  vi.restoreAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

describe('AddImageModal', () => {
  async function openDialog() {
    render(AddImageModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('emul.addTitle') });
    const keys = user();
    return { dialog, keys, image: within(dialog).getByRole('textbox', { name: msg('emul.addImageLabel') }) };
  }

  it('names the image field by its label alone, and says by the field that no console takes the file', async () => {
    const { dialog, keys, image } = await openDialog();
    await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());

    await keys.type(image, 'C:/roms/game.xyz');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.closest('label')).toBeNull();
    expect(image.getAttribute('aria-invalid')).toBe('true');
    expect((image.getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
  });

  it('says a refused add in a danger notice, and keeps the focus on "add", busy, while it runs', async () => {
    const adding = deferred<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    const { dialog, keys, image } = await openDialog();
    await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());
    await keys.type(image, 'C:/roms/game.iso');
    await keys.type(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') }), 'Game');
    const add = within(dialog).getByRole('button', { name: msg('common.add') });
    await vi.waitFor(() => expect(add).toHaveProperty('disabled', false));
    add.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('games.installedAddingLabel') });
    expect(busy).toBe(add);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);

    adding.reject(new Error('exists'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('emul.addFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('AddImageModal: while it adds', () => {
  it('cannot be closed, so a refused add is seen in it', async () => {
    const adding = deferred<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    render(AddImageModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('emul.addTitle') });
    const keys = user();
    await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());
    await keys.type(within(dialog).getByRole('textbox', { name: msg('emul.addImageLabel') }), 'C:/roms/game.iso');
    await keys.type(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') }), 'Game');
    const add = within(dialog).getByRole('button', { name: msg('common.add') });
    await vi.waitFor(() => expect(add).toHaveProperty('disabled', false));
    await keys.click(add);

    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel).toHaveProperty('disabled', false);
    expect(cancel.getAttribute('aria-disabled')).toBe('true');

    adding.reject(new Error('exists'));
    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('emul.addFailed'));
  });
});
