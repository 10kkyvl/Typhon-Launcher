import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { addGame, selectExecutable, type LibraryGame } from '../../lib/services/library';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import { resetBindings } from '../../lib/testing/fakeBinding';
import { held, sentToGo } from '../../lib/testing/gameDialogsTesting';
import InstalledAddGame from './InstalledAddGame.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/gameDialogsTesting')).goRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  addGame: vi.fn(),
  selectExecutable: vi.fn(),
}));

const coded = (code: string) => new Error(`typhon:${code}`);
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());
const EXE = 'C:\\Games\\Hades\\Hades.exe';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.mocked(addGame).mockReset();
  vi.mocked(selectExecutable).mockReset();
  toasts.set([]);
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

async function openDialog() {
  const onadded = vi.fn();
  const view = render(InstalledAddGame, { target: app, props: { uid: 'page', onadded } });
  view.component.show();
  await tick();
  const dialog = screen.getByRole('dialog', { name: msg('games.installedAddGameModalTitle') });
  return { view, dialog, onadded, keys: user() };
}

const exeField = (dialog: HTMLElement) => within(dialog).getByRole('textbox', { name: msg('games.executableLabel') });
const addButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('common.add') });

describe('InstalledAddGame: the fields', () => {
  it('names the executable field by its label, with Browse beside it, and the title field by its own', async () => {
    const { dialog } = await openDialog();
    expect(exeField(dialog).closest('label')).toBeNull();
    expect(within(dialog).getByRole('button', { name: msg('games.installedBrowseButton') }).closest('label')).toBeNull();
    expect(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') })).toBeTruthy();
  });

  it('fills the title from a browsed executable', async () => {
    vi.mocked(selectExecutable).mockResolvedValueOnce(EXE);
    const { dialog, keys } = await openDialog();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.installedBrowseButton') }));
    await vi.waitFor(() => expect(exeField(dialog)).toHaveProperty('value', EXE));
    expect(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') })).toHaveProperty('value', 'Hades');
  });

  it('holds Add with a reason until an executable is given', async () => {
    const { dialog, keys } = await openDialog();
    expect(addButton(dialog).getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(addButton(dialog))).toContain(msg('games.installedSelectExeDialog'));
    await keys.type(exeField(dialog), EXE);
    expect(addButton(dialog).getAttribute('aria-disabled')).toBeNull();
  });

  it('says a picker that would not open at the executable field', async () => {
    vi.mocked(selectExecutable).mockRejectedValueOnce(new Error('dialog failed'));
    const { dialog, keys } = await openDialog();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.installedBrowseButton') }));
    await vi.waitFor(() => expect(describedBy(exeField(dialog))).toContain(msg('games.installedOpenDialogError')));
    expect(get(toasts)).toEqual([]);
  });
});

describe('InstalledAddGame: adding', () => {
  it('sends one AddGame with the trimmed values for any number of presses, busy, and holds the rest', async () => {
    const adding = held<LibraryGame>();
    vi.mocked(addGame).mockReturnValueOnce(adding.promise);
    const { dialog, keys, onadded } = await openDialog();
    await keys.type(exeField(dialog), `  ${EXE} `);
    await keys.type(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') }), ' Hades ');
    const add = addButton(dialog);
    add.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(addGame).mock.calls).toEqual([[EXE, 'Hades']]);
    expect(add.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(add);
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    expect(within(dialog).getByRole('button', { name: msg('games.installedBrowseButton') }).getAttribute('aria-disabled')).toBe('true');
    expect(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') })).toHaveProperty('readOnly', true);

    adding.resolve({ id: 'g1', title: 'Hades' } as LibraryGame);
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(onadded.mock.calls).toEqual([[{ id: 'g1', title: 'Hades' }]]);
    expect(get(toasts).map((t) => t.message)).toEqual([msg('games.installedGameAddedToast', { title: 'Hades' })]);
  });

  it('says a refusal at the executable field by its code, not in a toast', async () => {
    vi.mocked(addGame).mockRejectedValueOnce(coded('library.already_added'));
    const { dialog, keys } = await openDialog();
    await keys.type(exeField(dialog), EXE);
    await keys.click(addButton(dialog));

    await vi.waitFor(() => expect(exeField(dialog).getAttribute('aria-invalid')).toBe('true'));
    expect(describedBy(exeField(dialog))).toContain(msg('library.already_added'));
    expect(get(toasts)).toEqual([]);
  });
});

describe('InstalledAddGame: an answer after the dialog moved on', () => {
  async function addThenReopen() {
    const adding = held<LibraryGame>();
    vi.mocked(addGame).mockReturnValueOnce(adding.promise);
    const opened = await openDialog();
    await opened.keys.type(exeField(opened.dialog), EXE);
    await opened.keys.click(addButton(opened.dialog));
    opened.view.component.show();
    await tick();
    return { ...opened, adding };
  }

  it('a late success leaves the new opening open and empty, calls nothing back, and is said once', async () => {
    const { adding, onadded } = await addThenReopen();

    adding.resolve({ id: 'g1', title: 'Hades' } as LibraryGame);
    await settle();

    const dialog = screen.getByRole('dialog');
    expect(exeField(dialog)).toHaveProperty('value', '');
    expect(addButton(dialog).getAttribute('aria-busy')).toBeNull();
    expect(onadded).not.toHaveBeenCalled();
    expect(get(toasts).map((t) => t.message)).toEqual([msg('games.installedGameAddedToast', { title: 'Hades' })]);
  });

  it('a late refusal is said once in a toast naming the game, not in the new opening', async () => {
    const { adding } = await addThenReopen();

    adding.reject(coded('library.already_added'));
    await settle();

    const dialog = screen.getByRole('dialog');
    expect(exeField(dialog).getAttribute('aria-invalid')).toBeNull();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [msg('games.installedAddGameFailedFor', { title: 'Hades', reason: msg('library.already_added') }), 'danger'],
    ]);
  });
});
