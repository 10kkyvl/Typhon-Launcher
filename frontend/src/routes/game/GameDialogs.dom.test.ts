import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { allTags, type LibraryGame } from '../../lib/services/library';
import { findMetadataCandidates } from '../../lib/services/metadata';
import { resetBindings } from '../../lib/testing/fakeBinding';
import { sentToGo } from '../../lib/testing/gameDialogsTesting';
import { user } from '../../lib/testing/dom';
import GameDialogsHarness from '../../lib/testing/GameDialogsHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/gameDialogsTesting')).goRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  allTags: vi.fn(async () => []),
  setStatus: vi.fn(),
  setTags: vi.fn(),
}));
vi.mock('../../lib/services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/metadata')>()),
  findMetadataCandidates: vi.fn(async () => []),
  searchMetadataCandidates: vi.fn(async () => []),
  applyMetadataMatch: vi.fn(),
}));

const hades = { id: 'g1', title: 'Hades', status: '', tags: ['coop'] } as unknown as LibraryGame;

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

const TITLES = () => ({
  status: msg('modals.gameStatusTitle'),
  tags: msg('modals.gameTagsTitle'),
  match: msg('modals.metadataMatchFindTitle'),
  picker: msg('games.detailChooseDownloadTitle'),
});

function openDialogs() {
  return screen.queryAllByRole('dialog').map((dialog) => dialog.getAttribute('aria-labelledby') && document.getElementById(dialog.getAttribute('aria-labelledby') ?? '')?.textContent);
}

describe('GameDialogs: which dialog opens for which request', () => {
  it('opens nothing until asked', () => {
    render(GameDialogsHarness, { target: app, props: { game: hades } });
    expect(openDialogs()).toEqual([]);
  });

  it.each([
    ['Open status', 'status'],
    ['Open tags', 'tags'],
    ['Open match', 'match'],
  ] as const)('"%s" opens its dialog alone with the focus in it, and closing it gives the focus back to the opener', async (opener, which) => {
    render(GameDialogsHarness, { target: app, props: { game: hades } });
    const keys = user();
    const button = screen.getByRole('button', { name: opener });

    await keys.click(button);

    const dialog = await screen.findByRole('dialog', { name: TITLES()[which] });
    expect(openDialogs()).toEqual([TITLES()[which]]);
    expect(dialog.contains(document.activeElement)).toBe(true);

    await keys.keyboard('{Escape}');

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.activeElement).toBe(button);
  });

  it('"Open picker" opens the release picker alone, and closing it gives the focus back to the opener', async () => {
    render(GameDialogsHarness, { target: app, props: { game: hades } });
    const keys = user();
    const button = screen.getByRole('button', { name: 'Open picker' });

    await keys.click(button);

    await screen.findByRole('dialog', { name: TITLES().picker });
    expect(openDialogs()).toEqual([TITLES().picker]);

    await keys.keyboard('{Escape}');

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(document.activeElement).toBe(button);
  });

  it('opens the status and the tags dialogs only for a game of the library', async () => {
    render(GameDialogsHarness, { target: app, props: { game: undefined } });
    const keys = user();

    await keys.click(screen.getByRole('button', { name: 'Open status' }));
    await keys.click(screen.getByRole('button', { name: 'Open tags' }));

    expect(openDialogs()).toEqual([]);
  });

  it('reads the tag suggestions when the tags dialog opens, not before', async () => {
    render(GameDialogsHarness, { target: app, props: { game: hades } });
    expect(allTags).not.toHaveBeenCalled();

    await user().click(screen.getByRole('button', { name: 'Open tags' }));

    await within(await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') })).findByText('coop');
    expect(allTags).toHaveBeenCalledTimes(1);
  });

  it('looks for the match candidates of the page\'s game when the match dialog opens, not before', async () => {
    render(GameDialogsHarness, { target: app, props: { game: hades } });
    expect(findMetadataCandidates).not.toHaveBeenCalled();

    await user().click(screen.getByRole('button', { name: 'Open match' }));

    await vi.waitFor(() => expect(findMetadataCandidates).toHaveBeenCalledExactlyOnceWith('igdb:1'));
  });
});
