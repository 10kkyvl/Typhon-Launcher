import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { allTags, setTags, type LibraryGame } from '../../lib/services/library';
import { applyMetadataMatch, findMetadataCandidates, type MetadataCandidate, type MetadataView } from '../../lib/services/metadata';
import { toasts } from '../../lib/stores/toasts';
import { resetBindings } from '../../lib/testing/fakeBinding';
import { held, sentToGo } from '../../lib/testing/gameDialogsTesting';
import type { GamePageData } from './gamePageData.svelte';
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
  toasts.set([]);
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

describe('GameDialogs: the page turned to another game while a match was applied', () => {
  it('keeps the page\'s metadata and the open dialog of the game now shown', async () => {
    const hadesMatch: MetadataCandidate = { providerId: 'igdb:10', title: 'Hades', releaseYear: 2020, thumb: '', confidence: 0.9 };
    const celesteMatch: MetadataCandidate = { providerId: 'igdb:20', title: 'Celeste', releaseYear: 2018, thumb: '', confidence: 0.9 };
    const applied = held<MetadataView>();
    vi.mocked(findMetadataCandidates).mockImplementation(async (id: string) => (id === 'igdb:1' ? [hadesMatch] : [celesteMatch]));
    vi.mocked(applyMetadataMatch).mockReturnValueOnce(applied.promise);
    const data = { metaView: null } as unknown as GamePageData;
    const { rerender } = render(GameDialogsHarness, { target: app, props: { game: hades, data } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Open match' }));
    const dialog = await screen.findByRole('dialog', { name: msg('modals.metadataMatchFindTitle') });
    const row = await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    row.focus();
    await keys.keyboard('{Enter}');
    expect(applyMetadataMatch).toHaveBeenCalledExactlyOnceWith('igdb:1', 'igdb:10');

    const celeste = { id: 'g2', title: 'Celeste', status: '', tags: [] } as unknown as LibraryGame;
    await rerender({ game: celeste, data, canonicalId: 'igdb:2', title: 'Celeste' });
    await within(dialog).findByRole('radio', { name: /^Celeste \(2018\)/ });
    applied.resolve({ game: { id: 'igdb:1', title: 'Hades' } } as MetadataView);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(data.metaView).toBeNull();
    expect(dialog.isConnected).toBe(true);
    expect(openDialogs()).toEqual([msg('modals.metadataMatchFindTitle')]);
  });
});

describe('GameDialogs: the page turned to another game while the tags dialog was open', () => {
  it('shows the new game\'s tags and saves them to the new game; a late save for the game before closes nothing', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setTags).mockReturnValueOnce(saved.promise).mockResolvedValueOnce({} as LibraryGame);
    const { rerender } = render(GameDialogsHarness, { target: app, props: { game: hades } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Open tags' }));
    let dialog = await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') });
    await within(dialog).findByText('coop');
    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));
    expect(setTags).toHaveBeenCalledExactlyOnceWith('g1', ['coop']);

    const celeste = { id: 'g2', title: 'Celeste', status: '', tags: ['solo'] } as unknown as LibraryGame;
    await rerender({ game: celeste, canonicalId: 'igdb:2', title: 'Celeste' });
    dialog = await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') });
    await within(dialog).findByText('solo');
    expect(within(dialog).queryByText('coop')).toBeNull();
    saved.resolve({} as LibraryGame);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(openDialogs()).toEqual([msg('modals.gameTagsTitle')]);
    expect(dialog.isConnected).toBe(true);
    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));
    expect(setTags).toHaveBeenLastCalledWith('g2', ['solo']);
  });

  it('keeps the open tags dialog, and what was typed in it, when the library sends the same game again', async () => {
    const { rerender } = render(GameDialogsHarness, { target: app, props: { game: hades } });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Open tags' }));
    const dialog = await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') });
    const field = within(dialog).getByPlaceholderText(msg('modals.gameTagsPlaceholder'));
    await keys.type(field, 'ind');

    await rerender({ game: { ...hades, tags: ['coop'] } as unknown as LibraryGame });

    expect(dialog.isConnected).toBe(true);
    expect(field.isConnected).toBe(true);
    expect((field as HTMLInputElement).value).toBe('ind');
  });
});
