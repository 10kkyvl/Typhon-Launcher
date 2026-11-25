import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import SavesCandidatesModal from './SavesCandidatesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

const SHORT = 'C:/Users/me/Saved Games/Hades';
const LONG = 'C:/Users/me/AppData/Local/Supergiant Games/A folder whose name runs on for long enough to be cut by the old list/Hades/Profiles';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.clearAllMocks();
  document.body.replaceChildren();
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

  it('a press on a folder uses it', async () => {
    const { dialog, onpick, keys } = open();

    await keys.click(within(dialog).getByRole('button', { name: SHORT }));

    expect(onpick).toHaveBeenCalledWith(SHORT);
    expect(onpick).toHaveBeenCalledTimes(1);
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
