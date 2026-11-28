import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../i18n';
import { imageTitle, listPlatforms, selectImageFile, type PlatformStatus } from '../services/emulators';
import { addEmulatedGame, findEmulatedCover, type LibraryGame } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import AddImageModal from './AddImageModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  imageTitle: vi.fn(),
  imageConsoles: vi.fn(async () => []),
  selectImageFile: vi.fn(),
}));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  addEmulatedGame: vi.fn(),
  findEmulatedCover: vi.fn(),
}));

const ps2 = { id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2', homepage: '', extensions: ['.iso'], imageNames: [], path: '', status: 'ready', reason: '', warnings: [] } as PlatformStatus;
const added = { id: 'e1', title: 'Okami' } as LibraryGame;

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
  locale.set('ru');
  vi.clearAllMocks();
  vi.restoreAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const toastTexts = () => get(toasts).map((item) => [item.kind, item.message]);

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

async function filled(onadded = vi.fn()) {
  const view = render(AddImageModal, { target: app, props: { open: true, onadded } });
  const dialog = screen.getByRole('dialog', { name: msg('emul.addTitle') });
  const keys = user();
  await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());
  const image = within(dialog).getByRole('textbox', { name: msg('emul.addImageLabel') });
  const title = within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') });
  await keys.type(image, 'C:/roms/okami.iso');
  await keys.type(title, 'Okami');
  const add = within(dialog).getByRole('button', { name: msg('common.add') });
  await vi.waitFor(() => expect(add).toHaveProperty('disabled', false));
  return { view, dialog, keys, image, title, add, onadded };
}

describe('AddImageModal: the form', () => {
  it('puts the image on the kit path field and the title on the kit text field, each described by its hint', async () => {
    const { image, title } = await filled();

    expect(image.closest('.path-field')).not.toBeNull();
    expect(title.closest('.text-field')).not.toBeNull();
    expect(describedBy(title)).toContain(msg('emul.addTitleHint'));
  });

  it('writes nothing before Add: the console is chosen from the file, the title typed, the cover asked for', async () => {
    const { dialog, keys } = await filled();
    await keys.click(within(dialog).getByRole('switch', { name: msg('emul.addFindCover') }));

    expect(within(dialog).getByRole('combobox', { name: msg('emul.addConsoleLabel') }).textContent).toContain('PlayStation 2');
    expect(vi.mocked(addEmulatedGame)).not.toHaveBeenCalled();
    expect(vi.mocked(findEmulatedCover)).not.toHaveBeenCalled();
  });

  it('adds once per press, busy, and the cover is looked up only after it was added', async () => {
    const adding = held<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    vi.mocked(findEmulatedCover).mockResolvedValueOnce(added);
    const { dialog, keys, add, onadded } = await filled();
    await keys.click(within(dialog).getByRole('switch', { name: msg('emul.addFindCover') }));
    add.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(addEmulatedGame).mock.calls).toEqual([['C:/roms/okami.iso', 'ps2', 'Okami']]);
    expect(vi.mocked(findEmulatedCover)).not.toHaveBeenCalled();
    adding.resolve(added);
    await vi.waitFor(() => expect(vi.mocked(findEmulatedCover).mock.calls).toEqual([['e1']]));
    expect(onadded).toHaveBeenCalledWith(added);
  });

  it('keeps the title it sends while it adds: the field is read-only and typing changes nothing', async () => {
    const adding = held<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    const { keys, title, add } = await filled();

    await keys.click(add);
    expect(title).toHaveProperty('readOnly', true);
    await keys.type(title, ' 2');

    expect((title as HTMLInputElement).value).toBe('Okami');
    expect(vi.mocked(addEmulatedGame).mock.calls).toEqual([['C:/roms/okami.iso', 'ps2', 'Okami']]);
    adding.resolve(added);
    await vi.waitFor(() => expect(title.isConnected).toBe(false));
  });

  it('says a refused image at the image field and gives it the focus', async () => {
    vi.mocked(addEmulatedGame).mockRejectedValueOnce(new Error('typhon:emulators.image_missing: gone'));
    const { keys, image, add } = await filled();

    await keys.click(add);

    await vi.waitFor(() => expect(image.getAttribute('aria-invalid')).toBe('true'));
    expect(describedBy(image)).toContain(msg('common.failedReason', { action: msg('emul.addFailed'), reason: msg('emulators.image_missing') }));
    expect(document.activeElement).toBe(image);
    expect(toastTexts()).toEqual([]);
  });

  it('says a refused title at the title field and gives it the focus', async () => {
    vi.mocked(addEmulatedGame).mockRejectedValueOnce(new Error('typhon:emulators.title_empty: empty'));
    const { keys, title, add } = await filled();

    await keys.click(add);

    await vi.waitFor(() => expect(title.getAttribute('aria-invalid')).toBe('true'));
    expect(describedBy(title)).toContain(msg('common.failedReason', { action: msg('emul.addFailed'), reason: msg('emulators.title_empty') }));
    expect(document.activeElement).toBe(title);
  });

  it('says a picker that failed to open in the dialog, not in a toast', async () => {
    vi.mocked(selectImageFile).mockRejectedValueOnce(new Error('typhon:emulators.unknown_platform: x'));
    render(AddImageModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('emul.addTitle') });
    const keys = user();

    await keys.click(within(dialog).getByRole('button', { name: msg('games.installedBrowseButton') }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(
      msg('common.failedReason', { action: msg('games.installedOpenDialogError'), reason: msg('emulators.unknown_platform') }),
    );
    expect(toastTexts()).toEqual([]);
  });
});

describe('AddImageModal: gone with its screen while it adds', () => {
  it('a late success is said once, calls no callback and closes nothing', async () => {
    const adding = held<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    const { view, keys, add, onadded } = await filled();
    await keys.click(add);
    view.unmount();

    adding.resolve(added);

    await vi.waitFor(() => expect(toastTexts()).toEqual([['success', msg('games.installedGameAddedToast', { title: 'Okami' })]]));
    expect(onadded).not.toHaveBeenCalled();
  });

  it('a late refusal is said once in a danger toast naming the game with the reason', async () => {
    const adding = held<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    const { view, keys, add } = await filled();
    await keys.click(add);
    view.unmount();

    adding.reject(new Error('typhon:emulators.image_missing: gone'));

    await vi.waitFor(() =>
      expect(toastTexts()).toEqual([['danger', msg('common.failedReason', { action: msg('emul.addFailedFor', { title: 'Okami' }), reason: msg('emulators.image_missing') })]]),
    );
  });

  it('a late refusal that is a whole sentence is said after the game it concerns', async () => {
    const adding = held<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    const { view, keys, add } = await filled();
    await keys.click(add);
    view.unmount();

    adding.reject(new Error('typhon:emulators.pkg_invalid: bad'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', 'Не удалось добавить «Okami». Файл .pkg повреждён или это не пакет PS3']]));
  });
});

describe('AddImageModal: the cover looked up after the dialog closed', () => {
  it('says a failed look-up once, naming the game, its reason with a colon its own sentence', async () => {
    locale.set('en');
    vi.mocked(addEmulatedGame).mockResolvedValueOnce(added);
    vi.mocked(findEmulatedCover).mockRejectedValueOnce(new Error('typhon:emulators.cover_lookup_failed: down'));
    const { dialog, keys, add } = await filled();
    await keys.click(within(dialog).getByRole('switch', { name: msg('emul.addFindCover') }));

    await keys.click(add);

    await vi.waitFor(() =>
      expect(toastTexts()).toEqual([
        ['success', msg('games.installedGameAddedToast', { title: 'Okami' })],
        ['info', 'Could not look for a cover for "Okami". Could not look for a cover: the search is unavailable'],
      ]),
    );
  });
});
