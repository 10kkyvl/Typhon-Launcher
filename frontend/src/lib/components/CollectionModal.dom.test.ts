import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { msg } from '../i18n';
import { allTags, deleteCollection, saveCollection, type LibraryCollection } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import CollectionModal from './CollectionModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  allTags: vi.fn(),
  saveCollection: vi.fn(async (collection: LibraryCollection) => collection),
  deleteCollection: vi.fn(async () => {}),
}));

const evenings: LibraryCollection = { id: 'c1', name: 'Evenings', tags: ['coop'], genres: [], status: '', createdAt: '', updatedAt: '' };

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(allTags).mockResolvedValue([]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function openEditor(props: Record<string, unknown> = {}) {
  render(CollectionModal, { target: app, props: { open: true, collection: evenings, genres: ['Roguelike'], ...props } });
  return screen.getByRole('dialog', { name: msg('modals.collectionEditTitle') });
}

describe('CollectionModal: the status field', () => {
  it('is named by its label, chosen from the keyboard inside the dialog, and saved', async () => {
    render(CollectionModal, { target: app, props: { open: true, genres: [] } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });
    const status = screen.getByRole('combobox', { name: msg('modals.collectionStatusLabel') });
    expect(status.textContent?.trim()).toBe(msg('modals.collectionStatusAny'));

    status.focus();
    await keys.keyboard('{ArrowDown}');
    const list = screen.getByRole('listbox', { name: msg('modals.collectionStatusLabel') });
    expect(dialog.contains(list)).toBe(false);
    await keys.keyboard('{ArrowDown}{Enter}');
    expect(status.textContent?.trim()).toBe(msg('games.statusPlaying'));

    await keys.keyboard('{ArrowDown}{Escape}');
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(screen.getByRole('dialog')).toBe(dialog);

    await keys.type(screen.getByPlaceholderText(msg('modals.collectionNamePlaceholder')), 'Evenings');
    await keys.click(screen.getByRole('button', { name: msg('common.save') }));
    expect(saveCollection).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ name: 'Evenings', status: 'playing' }));
  });
});

describe('CollectionModal: the genres', () => {
  it('says there are no genres to choose with an inline empty state', () => {
    render(CollectionModal, { target: app, props: { open: true, genres: [] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });

    const text = within(dialog).getByText(msg('modals.collectionGenresEmpty'));
    expect(text.closest('.empty')).not.toBeNull();
  });

  it('groups the genre chips under their label', () => {
    render(CollectionModal, { target: app, props: { open: true, genres: ['Roguelike', 'Platformer'] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });

    const group = within(dialog).getByRole('group', { name: msg('modals.collectionGenresLabel') });
    expect(within(group).getAllByRole('button')).toHaveLength(2);
  });
});

describe('CollectionModal: the name field', () => {
  it('is a text field named by its label, which has the focus when the dialog opens', () => {
    render(CollectionModal, { target: app, props: { open: true, genres: [] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });

    const field = within(dialog).getByRole('textbox', { name: msg('modals.collectionNameLabel') });
    expect(field.getAttribute('maxlength')).toBe('48');
    expect(document.activeElement).toBe(field);
  });

  it('holds Save with the reason while the name is empty', async () => {
    render(CollectionModal, { target: app, props: { open: true, genres: ['Roguelike'] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: 'Roguelike' }));
    const save = within(dialog).getByRole('button', { name: msg('common.save') });

    expect(save).toHaveProperty('disabled', false);
    expect(save.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(save)).toContain(msg('modals.collectionNameRequired'));
    await keys.click(save);
    expect(saveCollection).not.toHaveBeenCalled();
  });

  it('says a refused name at the field, which takes the focus Save held', async () => {
    vi.mocked(saveCollection).mockRejectedValueOnce(new Error('typhon:library.collection_name_invalid: имя коллекции пустое'));
    const editor = openEditor();
    const save = within(editor).getByRole('button', { name: msg('common.save') });
    save.focus();

    await user().keyboard('{Enter}');

    const field = within(editor).getByRole('textbox', { name: msg('modals.collectionNameLabel') });
    await vi.waitFor(() => expect(field.getAttribute('aria-invalid')).toBe('true'));
    expect(describedBy(field)).toContain(msg('library.collection_name_invalid'));
    await vi.waitFor(() => expect(document.activeElement).toBe(field));
    expect(get(toasts)).toEqual([]);
  });

  it('says another refusal in the dialog\'s notice, which takes the focus Save held', async () => {
    vi.mocked(saveCollection).mockRejectedValueOnce(new Error('typhon:library.too_many_collections: limit'));
    const editor = openEditor();
    const save = within(editor).getByRole('button', { name: msg('common.save') });
    save.focus();

    await user().keyboard('{Enter}');

    const alert = await within(editor).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('library.too_many_collections'));
    const field = within(editor).getByRole('textbox', { name: msg('modals.collectionNameLabel') });
    expect(field.getAttribute('aria-invalid')).not.toBe('true');
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
  });
});

describe('CollectionModal: while a save or a delete runs', () => {
  it('holds Cancel and Delete with the reason, keeps the name field focusable, and Escape does not close', async () => {
    const call = held<LibraryCollection>();
    vi.mocked(saveCollection).mockReturnValueOnce(call.promise);
    const editor = openEditor();
    const keys = user();
    await keys.click(within(editor).getByRole('button', { name: msg('common.save') }));

    for (const name of [msg('common.cancel'), msg('common.delete')]) {
      const button = within(editor).getByRole('button', { name });
      expect(button).toHaveProperty('disabled', false);
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(describedBy(button)).toContain(msg('modals.actionRunning'));
    }
    const field = within(editor).getByRole('textbox', { name: msg('modals.collectionNameLabel') });
    expect(field).toHaveProperty('disabled', false);
    expect(field).toHaveProperty('readOnly', true);
    await keys.click(within(editor).getByRole('button', { name: msg('common.cancel') }));
    await keys.keyboard('{Escape}');
    expect(editor.isConnected).toBe(true);
    expect(screen.queryByRole('dialog', { name: msg('library.collectionDeleteTitle') })).toBeNull();

    call.resolve(evenings);
    await vi.waitFor(() => expect(editor.isConnected).toBe(false));
  });

  it('holds the genre chips with the reason and the status field, so nothing changes under the save', async () => {
    const call = held<LibraryCollection>();
    vi.mocked(saveCollection).mockReturnValueOnce(call.promise);
    const editor = openEditor();
    const keys = user();
    await keys.click(within(editor).getByRole('button', { name: msg('common.save') }));

    const chip = within(within(editor).getByRole('group', { name: msg('modals.collectionGenresLabel') })).getByRole('button', { name: 'Roguelike' });
    expect(chip).toHaveProperty('disabled', false);
    expect(chip.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(chip)).toContain(msg('modals.actionRunning'));
    await keys.click(chip);
    expect(chip.getAttribute('aria-pressed')).toBe('false');
    const status = within(editor).getByRole('combobox', { name: msg('modals.collectionStatusLabel') });
    expect(status).toHaveProperty('disabled', true);

    call.resolve(evenings);
    await vi.waitFor(() => expect(editor.isConnected).toBe(false));
    expect(saveCollection).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ genres: [], status: '' }));
  });

  it('gives the genre chips and the status field back when the save is refused', async () => {
    vi.mocked(saveCollection).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась'));
    const editor = openEditor();
    const keys = user();
    await keys.click(within(editor).getByRole('button', { name: msg('common.save') }));
    await within(editor).findByRole('alert');

    const chip = within(within(editor).getByRole('group', { name: msg('modals.collectionGenresLabel') })).getByRole('button', { name: 'Roguelike' });
    expect(chip.getAttribute('aria-disabled')).toBeNull();
    await keys.click(chip);
    expect(chip.getAttribute('aria-pressed')).toBe('true');
    expect(within(editor).getByRole('combobox', { name: msg('modals.collectionStatusLabel') })).toHaveProperty('disabled', false);
  });
});

describe('CollectionModal: the tag suggestions', () => {
  it('says a failed read in the dialog with Retry, which reads them again', async () => {
    vi.mocked(allTags).mockReset();
    vi.mocked(allTags).mockRejectedValueOnce(new Error('typhon:library.something_new: чтение не удалось')).mockResolvedValueOnce(['rpg']);
    const editor = openEditor();
    const keys = user();

    const alert = await within(editor).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.tagSuggestionsFailed'));
    expect(get(toasts)).toEqual([]);

    await keys.click(within(editor).getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect([...editor.querySelectorAll('datalist option')].map((option) => option.getAttribute('value'))).toEqual(['rpg']));
    expect(within(editor).queryByRole('alert')).toBeNull();
  });

  it('says nothing about a read that fails after the editor closed', async () => {
    const read = held<string[]>();
    vi.mocked(allTags).mockReset();
    vi.mocked(allTags).mockReturnValueOnce(read.promise);
    const { rerender } = render(CollectionModal, { target: app, props: { open: true, collection: evenings, genres: [] } });
    const editor = screen.getByRole('dialog', { name: msg('modals.collectionEditTitle') });

    await user().keyboard('{Escape}');
    await vi.waitFor(() => expect(editor.isConnected).toBe(false));
    read.reject(new Error('typhon:library.something_new: чтение не удалось'));
    await new Promise((resolve) => setTimeout(resolve, 0));
    await rerender({ open: true });

    expect(get(toasts)).toEqual([]);
    expect(within(screen.getByRole('dialog', { name: msg('modals.collectionEditTitle') })).queryByRole('alert')).toBeNull();
  });
});

describe('CollectionModal: the tags', () => {
  it('names the tag field by its heading', () => {
    const editor = openEditor();
    expect(within(editor).getByRole('combobox', { name: msg('modals.collectionTagsLabel') })).toBeTruthy();
  });
});
