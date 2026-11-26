import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { sentToGo } from '../testing/gameDialogsTesting';
import { description, tabbable } from '../testing/hints';
import SavesCandidatesModal from './SavesCandidatesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());

const SHORT = 'C:/Users/me/Saved Games/Hades';
const LONG = 'C:/Users/me/AppData/Local/Supergiant Games/A folder whose name runs on for long enough to be cut by the old list/Hades/Profiles';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function open() {
  const onpick = vi.fn();
  const onbrowse = vi.fn();
  render(SavesCandidatesModal, { target: app, props: { open: true, title: 'Hades', candidates: [SHORT, LONG], onpick, onbrowse } });
  const dialog = screen.getByRole('dialog', { name: msg('ui.savesDirTitle') });
  return { dialog, onpick, onbrowse, keys: user() };
}

// Every word of a candidate's path reaches the player: on the row, or in what the row is described by.
function reachable(dialog: HTMLElement, path: string): boolean {
  if ((dialog.textContent ?? '').includes(path)) return true;
  return [...dialog.querySelectorAll('[aria-describedby]')].some((element) =>
    (element.getAttribute('aria-describedby') ?? '').split(/\s+/).some((id) => document.getElementById(id)?.textContent?.includes(path)),
  );
}

describe('SavesCandidatesModal', () => {
  it('names the game it asks about and offers every folder found, the long one whole', () => {
    const { dialog } = open();
    expect(within(dialog).getByText(msg('ui.savesMultipleCandidates', { title: 'Hades' }))).toBeTruthy();
    expect(reachable(dialog, SHORT)).toBe(true);
    expect(reachable(dialog, LONG)).toBe(true);
  });

  it('offers the folders as one choice named by the question, each path written out whole with no native title', () => {
    const { dialog } = open();
    const group = within(dialog).getByRole('radiogroup', { name: msg('ui.savesMultipleCandidates', { title: 'Hades' }) });
    expect(within(group).getAllByRole('radio').map((row) => row.getAttribute('aria-checked'))).toEqual(['false', 'false']);
    expect(within(group).getByRole('radio', { name: LONG })).toBeTruthy();
    expect(dialog.querySelectorAll('[title]')).toHaveLength(0);
  });

  it('a folder is chosen, then used by Select, once', async () => {
    const { dialog, onpick, keys } = open();
    const select = within(dialog).getByRole('button', { name: msg('common.select') });
    expect(select.getAttribute('aria-disabled')).toBe('true');

    await keys.click(within(dialog).getByRole('radio', { name: LONG }));
    expect(select.hasAttribute('aria-disabled')).toBe(false);
    expect(onpick).not.toHaveBeenCalled();
    await keys.click(select);

    expect(onpick).toHaveBeenCalledWith(LONG);
    expect(onpick).toHaveBeenCalledTimes(1);
  });

  // An arrow chooses in a radio group: were a choice a use, walking the list would set the folder.
  it('arrows walk the folders and use none of them until Select', async () => {
    const { dialog, onpick, keys } = open();
    const first = within(dialog).getByRole('radio', { name: SHORT });
    first.focus();

    await keys.keyboard('{ArrowDown}{ArrowDown}{ArrowUp}');

    expect(onpick).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(within(dialog).getByRole('radio', { name: LONG }));
    await keys.click(within(dialog).getByRole('button', { name: msg('common.select') }));
    expect(onpick).toHaveBeenCalledWith(LONG);
  });

  // The game menu and the saves view keep the dialog mounted between openings.
  it('opens again with no folder chosen', async () => {
    const onpick = vi.fn();
    const props = { title: 'Hades', candidates: [SHORT, LONG], onpick, onbrowse: vi.fn() };
    const { rerender } = render(SavesCandidatesModal, { target: app, props: { ...props, open: true } });
    const keys = user();
    await keys.click(within(screen.getByRole('dialog')).getByRole('radio', { name: LONG }));

    await rerender({ ...props, open: false });
    await rerender({ ...props, open: true });

    const dialog = screen.getByRole('dialog', { name: msg('ui.savesDirTitle') });
    expect(within(dialog).getAllByRole('radio').map((row) => row.getAttribute('aria-checked'))).toEqual(['false', 'false']);
    expect(within(dialog).getByRole('button', { name: msg('common.select') }).getAttribute('aria-disabled')).toBe('true');
  });

  it('with no folder chosen, Select stays reachable, says why it is off and uses nothing', async () => {
    const { dialog, onpick, keys } = open();
    const select = within(dialog).getByRole('button', { name: msg('common.select') });

    expect(select).toHaveProperty('disabled', false);
    expect(tabbable(select)).toBe(true);
    expect(description(select)).toBe(msg('ui.savesChooseFirst'));

    select.focus();
    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');
    await keys.click(select);
    expect(onpick).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(select);

    await keys.click(within(dialog).getByRole('radio', { name: SHORT }));
    expect(select.hasAttribute('aria-describedby')).toBe(false);
  });

  it('"Choose another" hands the choice to the folder picker', async () => {
    const { dialog, onpick, onbrowse, keys } = open();

    await keys.click(within(dialog).getByRole('button', { name: msg('ui.pickAnotherFolder') }));

    expect(onbrowse).toHaveBeenCalledTimes(1);
    expect(onpick).not.toHaveBeenCalled();
  });

  it('Cancel closes it without using a folder', async () => {
    const { dialog, onpick, onbrowse, keys } = open();

    await keys.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(onpick).not.toHaveBeenCalled();
    expect(onbrowse).not.toHaveBeenCalled();
  });
});
