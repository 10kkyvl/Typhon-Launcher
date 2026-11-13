import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { deleteCollection, saveCollection, type LibraryCollection } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import CollectionModal from './CollectionModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  allTags: vi.fn(async () => []),
  saveCollection: vi.fn(async (collection: LibraryCollection) => collection),
  deleteCollection: vi.fn(async () => {}),
}));

const evenings: LibraryCollection = { id: 'c1', name: 'Evenings', tags: ['coop'], genres: [], status: '', createdAt: '', updatedAt: '' };

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function openEditor(props: Record<string, unknown> = {}) {
  render(CollectionModal, { target: app, props: { open: true, collection: evenings, genres: ['Roguelike'], ...props } });
  return screen.getByRole('dialog', { name: msg('modals.collectionEditTitle') });
}

describe('CollectionModal: deleting a collection', () => {
  it('asks first; cancel keeps the collection and the editor', async () => {
    const editor = openEditor();
    const keys = user();
    const remove = within(editor).getByRole('button', { name: msg('common.delete') });

    await keys.click(remove);

    const confirm = screen.getByRole('dialog', { name: msg('library.collectionDeleteTitle') });
    expect(confirm.textContent).toContain(msg('library.collectionDeleteText', { name: 'Evenings' }));
    expect(deleteCollection).not.toHaveBeenCalled();

    await keys.click(within(confirm).getByRole('button', { name: msg('common.cancel') }));

    expect(screen.queryByRole('dialog', { name: msg('library.collectionDeleteTitle') })).toBeNull();
    expect(editor.isConnected).toBe(true);
    expect(deleteCollection).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(remove);
  });

  it('deletes once confirmed, the pressed Delete busy and focused, not disabled, while the call runs', async () => {
    const call = deferred();
    vi.mocked(deleteCollection).mockReturnValueOnce(call.promise);
    const editor = openEditor();
    const keys = user();
    const remove = within(editor).getByRole('button', { name: msg('common.delete') });
    await keys.click(remove);

    await keys.click(within(screen.getByRole('dialog', { name: msg('library.collectionDeleteTitle') })).getByRole('button', { name: msg('common.delete') }));

    expect(deleteCollection).toHaveBeenCalledExactlyOnceWith('c1');
    await vi.waitFor(() => expect(remove.getAttribute('aria-busy')).toBe('true'));
    expect(remove).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(remove);

    call.resolve();
    await vi.waitFor(() => expect(editor.isConnected).toBe(false));
  });

  it('returns focus to the named place when what opened the editor left with the collection', async () => {
    const pencil = document.createElement('button');
    pencil.textContent = 'Edit Evenings';
    const heading = document.createElement('button');
    heading.textContent = 'All games';
    document.body.append(heading, pencil);
    pencil.focus();
    vi.mocked(deleteCollection).mockImplementationOnce(async () => {
      pencil.remove();
    });
    const editor = openEditor({ returnFocus: () => heading });
    const keys = user();
    await keys.click(within(editor).getByRole('button', { name: msg('common.delete') }));

    await keys.click(within(screen.getByRole('dialog', { name: msg('library.collectionDeleteTitle') })).getByRole('button', { name: msg('common.delete') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.activeElement).toBe(heading);
  });

  it.each([
    ['a code with text', new Error('typhon:library.collection_not_found: коллекция не найдена'), () => msg('library.collection_not_found')],
    ['no code', new Error('open collections.json: permission denied'), () => msg('modals.collectionDeleteFailed')],
  ])('a failed delete (%s) says it in the editor, in the launcher words', async (_case, err, text) => {
    vi.mocked(deleteCollection).mockRejectedValueOnce(err);
    const editor = openEditor();
    const keys = user();
    await keys.click(within(editor).getByRole('button', { name: msg('common.delete') }));

    await keys.click(within(screen.getByRole('dialog', { name: msg('library.collectionDeleteTitle') })).getByRole('button', { name: msg('common.delete') }));

    expect(await within(editor).findByText(text())).toBeTruthy();
    expect(editor.textContent).not.toContain(err.message);
    expect(get(toasts)).toEqual([]);
  });
});

describe('CollectionModal: saving', () => {
  it('refuses a collection with no tag, genre or status, says why next to the fields, and sends nothing', async () => {
    render(CollectionModal, { target: app, props: { open: true, genres: ['Roguelike'] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });
    const keys = user();
    await keys.type(screen.getByPlaceholderText(msg('modals.collectionNamePlaceholder')), 'Everything');

    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    const why = within(dialog).queryByText(msg('library.collectionNoCriteria'));
    expect(why?.closest('[role="alert"]')).toBeTruthy();
    expect(saveCollection).not.toHaveBeenCalled();
    expect(dialog.isConnected).toBe(true);

    await keys.click(within(dialog).getByRole('button', { name: 'Roguelike' }));
    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    expect(saveCollection).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ name: 'Everything', genres: ['Roguelike'] }));
  });

  it('keeps the pressed Save busy and focused, not disabled, while the call runs', async () => {
    const call = deferred();
    vi.mocked(saveCollection).mockReturnValueOnce(call.promise.then(() => evenings));
    const editor = openEditor();
    const save = within(editor).getByRole('button', { name: msg('common.save') });
    save.focus();

    await user().keyboard('{Enter}');

    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(save).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(save);
    call.resolve();
    await vi.waitFor(() => expect(editor.isConnected).toBe(false));
  });

  it.each([
    ['a code with text', new Error('typhon:library.collection_name_invalid: имя коллекции пустое'), () => msg('library.collection_name_invalid')],
    ['no code', new Error('rename collections.json.tmp: access is denied'), () => msg('modals.collectionSaveFailed')],
  ])('a failed save (%s) says it in the editor, which stays open', async (_case, err, text) => {
    vi.mocked(saveCollection).mockRejectedValueOnce(err);
    const editor = openEditor();

    await user().click(within(editor).getByRole('button', { name: msg('common.save') }));

    expect(await within(editor).findByText(text())).toBeTruthy();
    expect(editor.textContent).not.toContain(err.message);
    expect(editor.isConnected).toBe(true);
    expect(get(toasts)).toEqual([]);
  });
});
