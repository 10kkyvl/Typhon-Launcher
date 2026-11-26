import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { allTags, setTags, type LibraryGame } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import GameTagsModal from './GameTagsModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  allTags: vi.fn(),
  setTags: vi.fn(),
}));

const hades = { id: 'g1', title: 'Hades', tags: ['coop'] } as unknown as LibraryGame;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(allTags).mockResolvedValue(['rpg']);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  vi.mocked(allTags).mockReset();
  vi.mocked(setTags).mockReset();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function openDialog(props: Record<string, unknown> = {}) {
  const result = render(GameTagsModal, { target: app, props: { open: true, game: hades, ...props } });
  return { ...result, dialog: screen.getByRole('dialog', { name: msg('modals.gameTagsTitle') }) };
}

describe('GameTagsModal: saving', () => {
  it('sends the tags with the one still typed, and closes', async () => {
    vi.mocked(setTags).mockResolvedValueOnce(hades);
    const { dialog } = openDialog();
    const keys = user();
    await keys.type(within(dialog).getByPlaceholderText(msg('modals.gameTagsPlaceholder')), 'indie');

    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    expect(setTags).toHaveBeenCalledExactlyOnceWith('g1', ['coop', 'indie']);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('keeps the focus on Save, busy, while the tags are saved, and a second press sends nothing', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setTags).mockReturnValueOnce(saved.promise);
    const { dialog } = openDialog();
    const keys = user();
    const save = within(dialog).getByRole('button', { name: msg('common.save') });
    save.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(save);
    expect(setTags).toHaveBeenCalledTimes(1);
    saved.resolve(hades);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('holds Cancel with the reason while the tags are saved, and Escape does not close', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setTags).mockReturnValueOnce(saved.promise);
    const { dialog } = openDialog();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });

    expect(cancel).toHaveProperty('disabled', false);
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(document.getElementById(cancel.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('modals.actionRunning'));
    await keys.click(cancel);
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);

    saved.resolve(hades);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it.each([
    ['a code with words', new Error('typhon:library.game_not_found: игра не найдена'), () => msg('library.game_not_found')],
    ['no code', new Error('rename tags.json.tmp: access is denied'), () => msg('modals.gameTagsSaveFailed')],
  ])('says a failed save (%s) in the dialog, in the launcher\'s words, and moves the focus there', async (_case, err, words) => {
    vi.mocked(setTags).mockRejectedValueOnce(err);
    const { dialog } = openDialog();
    const keys = user();
    const save = within(dialog).getByRole('button', { name: msg('common.save') });
    save.focus();

    await keys.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(words());
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(dialog.textContent).not.toContain(err.message);
    expect(get(toasts)).toEqual([]);
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
    expect(save.getAttribute('aria-busy')).toBeNull();
  });
});

describe('GameTagsModal: the tag suggestions', () => {
  it('offers the tags of the library', async () => {
    const { dialog } = openDialog();

    await vi.waitFor(() => expect([...dialog.querySelectorAll('datalist option')].map((option) => option.getAttribute('value'))).toEqual(['rpg']));
  });

  it('says a failed read in the dialog with Retry, which reads them again', async () => {
    vi.mocked(allTags).mockReset();
    vi.mocked(allTags).mockRejectedValueOnce(new Error('typhon:library.something_new: чтение не удалось')).mockResolvedValueOnce(['rpg']);
    const { dialog } = openDialog();
    const keys = user();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.tagSuggestionsFailed'));
    expect(get(toasts)).toEqual([]);

    await keys.click(within(dialog).getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect([...dialog.querySelectorAll('datalist option')].map((option) => option.getAttribute('value'))).toEqual(['rpg']));
    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(allTags).toHaveBeenCalledTimes(2);
  });

  it('says nothing about a read that fails after the dialog closed', async () => {
    const read = held<string[]>();
    vi.mocked(allTags).mockReset();
    vi.mocked(allTags).mockReturnValueOnce(read.promise);
    const { dialog } = openDialog();

    await user().keyboard('{Escape}');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    read.reject(new Error('typhon:library.something_new: чтение не удалось'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(get(toasts)).toEqual([]);
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
