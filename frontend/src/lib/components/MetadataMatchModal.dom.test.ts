import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { applyMetadataMatch, findMetadataCandidates, searchMetadataCandidates, type MetadataCandidate, type MetadataView } from '../services/metadata';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import MetadataMatchModal from './MetadataMatchModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/metadata')>()),
  findMetadataCandidates: vi.fn(),
  searchMetadataCandidates: vi.fn(),
  applyMetadataMatch: vi.fn(),
}));

const hades: MetadataCandidate = { providerId: 'igdb:1', title: 'Hades', releaseYear: 2020, thumb: 'https://img.test/hades.jpg', confidence: 0.9 };
const celeste: MetadataCandidate = { providerId: 'igdb:2', title: 'Celeste', releaseYear: 2018, thumb: 'https://img.test/celeste.jpg', confidence: 0.9 };
const hadesTwo: MetadataCandidate = { providerId: 'igdb:3', title: 'Hades II', releaseYear: 2024, thumb: 'https://img.test/hades2.jpg', confidence: 0.7 };

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
  vi.useRealTimers();
  vi.resetAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function openDialog(props: Record<string, unknown> = {}) {
  const result = render(MetadataMatchModal, { target: app, props: { open: true, gameId: 'g1', gameTitle: 'Hades', ...props } });
  const title = props.mode === 'change' ? msg('modals.metadataMatchChangeTitle') : msg('modals.metadataMatchFindTitle');
  return { ...result, dialog: screen.getByRole('dialog', { name: title }) };
}

describe('MetadataMatchModal: the candidates', () => {
  it('says it is looking for them with a labelled spinner and what it matches against, the focus in the search field', async () => {
    const found = held<MetadataCandidate[]>();
    vi.mocked(findMetadataCandidates).mockReturnValueOnce(found.promise);
    const { dialog } = openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.metadataMatchSearchingCandidates'));
    expect(within(dialog).getByText(msg('modals.metadataMatchSearchingCandidatesDesc', { gameTitle: 'Hades' }))).toBeTruthy();
    expect(document.activeElement).toBe(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }));
    found.resolve([]);
  });

  it('says none were found in words of their own', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([]);
    const { dialog } = openDialog();

    expect(await within(dialog).findByText(msg('modals.metadataMatchNoCandidatesTitle'))).toBeTruthy();
  });

  it('are a radio group named by its heading, each row named once by its text, the cover adding no second name', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades, hadesTwo]);
    const { dialog } = openDialog();

    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.metadataMatchCandidates') });
    const radios = within(group).getAllByRole('radio');
    expect(radios).toHaveLength(2);
    expect(radios[0].getAttribute('aria-checked')).toBe('false');
    expect(within(group).getByRole('radio', { name: /^Hades \(2020\)/ }).querySelector('img[alt]:not([alt=""])')).toBeNull();
  });

  it('applies nothing while the arrows walk the list; Enter applies the focused one, once', async () => {
    const applied = held<MetadataView>();
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades, hadesTwo]);
    vi.mocked(applyMetadataMatch).mockReturnValueOnce(applied.promise);
    const { dialog } = openDialog();
    const keys = user();
    const first = await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    first.focus();

    await keys.keyboard('{ArrowDown}');
    expect(applyMetadataMatch).not.toHaveBeenCalled();
    const second = within(dialog).getByRole('radio', { name: /^Hades II \(2024\)/ });
    expect(document.activeElement).toBe(second);

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(applyMetadataMatch).toHaveBeenCalledExactlyOnceWith('g1', 'igdb:3');
    expect(second).toHaveProperty('disabled', false);
    expect(second.closest('[role="radiogroup"]')?.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(second);
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(document.getElementById(cancel.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('modals.actionRunning'));
    applied.resolve({ game: { title: 'Hades II' } } as MetadataView);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('says a failed look-up in the dialog, never as "none found", and Retry looks again', async () => {
    vi.mocked(findMetadataCandidates).mockRejectedValueOnce(new Error('typhon:metadata.rate_limited: 429')).mockResolvedValueOnce([hades]);
    const { dialog } = openDialog();
    const keys = user();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errMetadata.metadataRateLimited'));
    expect(within(dialog).queryByText(msg('modals.metadataMatchNoCandidatesTitle'))).toBeNull();
    expect(get(toasts)).toEqual([]);

    await keys.click(within(dialog).getByRole('button', { name: msg('common.retry') }));

    expect(await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ })).toBeTruthy();
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });
});

describe('MetadataMatchModal: the search by name', () => {
  it('shows what it found as a second list', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([]);
    vi.mocked(searchMetadataCandidates).mockResolvedValueOnce([celeste]);
    const { dialog } = openDialog();
    const keys = user();

    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }), 'Celeste');
    await vi.advanceTimersByTimeAsync(300);

    expect(searchMetadataCandidates).toHaveBeenCalledExactlyOnceWith('Celeste');
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.metadataMatchManualSearch') });
    expect(within(group).getByRole('radio', { name: /^Celeste \(2018\)/ })).toBeTruthy();
  });

  it('says a search that found nothing in words of its own', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([]);
    vi.mocked(searchMetadataCandidates).mockResolvedValueOnce([]);
    const { dialog } = openDialog();
    const keys = user();

    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }), 'Zzz');
    await vi.advanceTimersByTimeAsync(300);

    expect(await within(dialog).findByText(msg('modals.metadataMatchSearchEmpty', { query: 'Zzz' }))).toBeTruthy();
  });

  it('says a failed search in the dialog, and Retry searches again', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([]);
    vi.mocked(searchMetadataCandidates).mockRejectedValueOnce(new Error('typhon:metadata.not_configured: no key')).mockResolvedValueOnce([celeste]);
    const { dialog } = openDialog();
    const keys = user();

    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }), 'Celeste');
    await vi.advanceTimersByTimeAsync(300);

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errMetadata.metadataNotConfigured'));
    expect(get(toasts)).toEqual([]);

    await keys.click(within(dialog).getByRole('button', { name: msg('common.retry') }));

    expect(await within(dialog).findByRole('radio', { name: /^Celeste \(2018\)/ })).toBeTruthy();
    expect(searchMetadataCandidates).toHaveBeenLastCalledWith('Celeste');
  });

  it('does not search for a query typed before the dialog was opened again for another game', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(findMetadataCandidates).mockResolvedValue([]);
    vi.mocked(searchMetadataCandidates).mockResolvedValue([celeste]);
    const { dialog, rerender } = openDialog();
    const keys = user();
    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }), 'Celeste');

    await rerender({ open: true, gameId: 'g2', gameTitle: 'Celeste' });
    await vi.advanceTimersByTimeAsync(300);

    expect(searchMetadataCandidates).not.toHaveBeenCalled();
    expect(within(screen.getByRole('dialog')).queryByRole('radio', { name: /^Celeste \(2018\)/ })).toBeNull();
  });

  it('paints nothing of a search answer that lands after the dialog was opened again for another game', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const late = held<MetadataCandidate[]>();
    vi.mocked(findMetadataCandidates).mockImplementation(async (id: string) => (id === 'g2' ? [hadesTwo] : []));
    vi.mocked(searchMetadataCandidates).mockReturnValueOnce(late.promise);
    const { dialog, rerender } = openDialog();
    const keys = user();
    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }), 'Celeste');
    await vi.advanceTimersByTimeAsync(300);
    expect(searchMetadataCandidates).toHaveBeenCalledTimes(1);

    await rerender({ open: true, gameId: 'g2', gameTitle: 'Hades II' });
    await within(dialog).findByRole('radio', { name: /^Hades II \(2024\)/ });
    late.resolve([celeste]);
    await vi.advanceTimersByTimeAsync(0);

    expect(within(dialog).queryByRole('radio', { name: /^Celeste \(2018\)/ })).toBeNull();
    expect(within(dialog).queryByRole('status', { name: msg('search.searching') })).toBeNull();
  });
});

describe('MetadataMatchModal: changing a match from the keyboard', () => {
  it('moves the focus into the question a row opens, and back to the row on Cancel', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    const { dialog } = openDialog({ mode: 'change' });
    const keys = user();
    const first = await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    first.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement?.closest('.confirm')).not.toBeNull());
    expect(document.activeElement?.textContent).toContain('Hades');
    expect(applyMetadataMatch).not.toHaveBeenCalled();

    within(dialog).getByRole('button', { name: msg('common.cancel') }).focus();
    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('radio', { name: /^Hades \(2020\)/ })));
  });

  it('leaves the focus alone for presses that did not hold it', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    const { dialog } = openDialog({ mode: 'change' });
    const first = await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    (document.activeElement as HTMLElement | null)?.blur();
    expect(document.activeElement).toBe(document.body);

    first.click();
    await vi.waitFor(() => expect(dialog.querySelector('.confirm')).not.toBeNull());
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(document.body);

    within(dialog).getByRole('button', { name: msg('common.cancel') }).click();
    await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.activeElement).toBe(document.body);
  });

  it('moves the focus the search field held to the question when a row is pressed by pointer', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    const { dialog } = openDialog({ mode: 'change' });
    const first = await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    const search = within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') });
    search.focus();

    first.click();

    await vi.waitFor(() => expect(search.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement?.closest('.confirm')).not.toBeNull());
    expect(document.activeElement).not.toBe(document.body);
  });

  it('keeps the focus on "replace", busy, while the new match is applied', async () => {
    const applied = held<MetadataView>();
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    vi.mocked(applyMetadataMatch).mockReturnValueOnce(applied.promise);
    const { dialog } = openDialog({ mode: 'change' });
    const keys = user();
    await keys.click(await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ }));
    const replace = within(dialog).getByRole('button', { name: msg('modals.metadataMatchReplace') });
    replace.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.metadataMatchApplying') });
    expect(busy).toBe(replace);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    applied.resolve({ game: { title: 'Hades' } } as MetadataView);
  });

  it('says a refused replacement in the dialog, in the launcher\'s words, and moves the focus there', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    vi.mocked(applyMetadataMatch).mockRejectedValueOnce(new Error('typhon:metadata.busy: already running'));
    const { dialog } = openDialog({ mode: 'change' });
    const keys = user();
    await keys.click(await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ }));
    within(dialog).getByRole('button', { name: msg('modals.metadataMatchReplace') }).focus();

    await keys.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errMetadata.metadataBusy'));
    expect(dialog.textContent).not.toContain('already running');
    expect(get(toasts)).toEqual([]);
    expect(within(dialog).getByRole('radio', { name: /^Hades \(2020\)/ })).toBeTruthy();
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
  });
});

describe('MetadataMatchModal: opened for another game before the first answer', () => {
  it('shows the candidates of the game it is open for, not a late answer for the one before', async () => {
    const first = held<MetadataCandidate[]>();
    vi.mocked(findMetadataCandidates).mockImplementation((id: string) => (id === 'g1' ? first.promise : Promise.resolve([celeste])));
    const { dialog, rerender } = openDialog();
    await rerender({ open: true, gameId: 'g2', gameTitle: 'Celeste' });
    await vi.waitFor(() => expect(within(dialog).queryAllByText('Celeste').length).toBeGreaterThan(0));

    first.resolve([hades]);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryAllByText('Hades')).toEqual([]);
    expect(within(dialog).queryAllByText('Celeste').length).toBeGreaterThan(0);
  });
});

describe('MetadataMatchModal: a match applied for one game, answered after the dialog turned to another', () => {
  async function applyThenTurn() {
    const applied = held<MetadataView>();
    const onapplied = vi.fn();
    vi.mocked(findMetadataCandidates).mockImplementation(async (id: string) => (id === 'g1' ? [hades] : [celeste]));
    vi.mocked(applyMetadataMatch).mockReturnValueOnce(applied.promise);
    const { dialog, rerender } = openDialog({ onapplied });
    const keys = user();
    const row = await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ });
    row.focus();
    await keys.keyboard('{Enter}');
    expect(applyMetadataMatch).toHaveBeenCalledExactlyOnceWith('g1', 'igdb:1');

    await rerender({ open: true, gameId: 'g2', gameTitle: 'Celeste', onapplied });
    const celesteRow = await within(dialog).findByRole('radio', { name: /^Celeste \(2018\)/ });
    return { applied, onapplied, dialog, celesteRow };
  }

  it('for the game still on screen, a success closes the dialog, names the game and hands the view to the page', async () => {
    const onapplied = vi.fn();
    const view = { game: { id: 'g1', title: 'Hades' } } as MetadataView;
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    vi.mocked(applyMetadataMatch).mockResolvedValueOnce(view);
    const { dialog } = openDialog({ onapplied });

    await user().click(await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(onapplied).toHaveBeenCalledExactlyOnceWith(view);
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[msg('modals.metadataMatchApplied', { title: 'Hades' }), 'success']]);
  });

  it('a success keeps the new game\'s dialog open and live, hands nothing to the page, and only names the game it was about', async () => {
    const { applied, onapplied, dialog, celesteRow } = await applyThenTurn();

    applied.resolve({ game: { id: 'g1', title: 'Hades' } } as MetadataView);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(dialog.isConnected).toBe(true);
    expect(onapplied).not.toHaveBeenCalled();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[msg('modals.metadataMatchApplied', { title: 'Hades' }), 'success']]);
    expect(celesteRow.isConnected).toBe(true);
    expect(celesteRow.closest('[role="radiogroup"]')?.getAttribute('aria-disabled')).not.toBe('true');
    expect(within(dialog).getByRole('button', { name: msg('common.cancel') }).getAttribute('aria-disabled')).toBeNull();
  });

  it('a refusal says nothing in the new game\'s dialog and leaves it live', async () => {
    const { applied, onapplied, dialog, celesteRow } = await applyThenTurn();

    applied.reject(new Error('typhon:metadata.busy: already running'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(get(toasts)).toEqual([]);
    expect(onapplied).not.toHaveBeenCalled();
    expect(celesteRow.isConnected).toBe(true);
    expect(celesteRow.closest('[role="radiogroup"]')?.getAttribute('aria-disabled')).not.toBe('true');
  });
});

describe('MetadataMatchModal: turned to another game, an earlier game\'s state and answers stay behind', () => {
  it('a failed look-up for the game before, answered after the new game\'s candidates, says nothing', async () => {
    const first = held<MetadataCandidate[]>();
    vi.mocked(findMetadataCandidates).mockImplementation((id: string) => (id === 'g1' ? first.promise : Promise.resolve([celeste])));
    const { dialog, rerender } = openDialog();
    await rerender({ open: true, gameId: 'g2', gameTitle: 'Celeste' });
    const row = await within(dialog).findByRole('radio', { name: /^Celeste \(2018\)/ });

    first.reject(new Error('typhon:metadata.rate_limited: 429'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(row.isConnected).toBe(true);
  });

  it('a failed search for the game before, answered after the new game\'s candidates, says nothing', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const late = held<MetadataCandidate[]>();
    vi.mocked(findMetadataCandidates).mockImplementation(async (id: string) => (id === 'g2' ? [hadesTwo] : []));
    vi.mocked(searchMetadataCandidates).mockReturnValueOnce(late.promise);
    const { dialog, rerender } = openDialog();
    const keys = user();
    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.metadataMatchManualSearch') }), 'Celeste');
    await vi.advanceTimersByTimeAsync(300);
    expect(searchMetadataCandidates).toHaveBeenCalledTimes(1);

    await rerender({ open: true, gameId: 'g2', gameTitle: 'Hades II' });
    await within(dialog).findByRole('radio', { name: /^Hades II \(2024\)/ });
    late.reject(new Error('typhon:metadata.not_configured: no key'));
    await vi.advanceTimersByTimeAsync(0);

    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('in change mode, does not ask the new game about the match picked for the game before', async () => {
    vi.mocked(findMetadataCandidates).mockImplementation(async (id: string) => (id === 'g1' ? [hades] : [celeste]));
    const { dialog, rerender } = openDialog({ mode: 'change' });
    await user().click(await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ }));
    expect(within(dialog).getByRole('button', { name: msg('modals.metadataMatchReplace') })).toBeTruthy();

    await rerender({ open: true, gameId: 'g2', gameTitle: 'Celeste', mode: 'change' });

    await within(dialog).findByRole('radio', { name: /^Celeste \(2018\)/ });
    expect(within(dialog).queryByRole('button', { name: msg('modals.metadataMatchReplace') })).toBeNull();
    expect(dialog.querySelector('.confirm')).toBeNull();
    expect(applyMetadataMatch).not.toHaveBeenCalled();
  });

  it('does not say the game before\'s refusal for the new game', async () => {
    vi.mocked(findMetadataCandidates).mockImplementation(async (id: string) => (id === 'g1' ? [hades] : [celeste]));
    vi.mocked(applyMetadataMatch).mockRejectedValueOnce(new Error('typhon:metadata.busy: already running'));
    const { dialog, rerender } = openDialog();
    await user().click(await within(dialog).findByRole('radio', { name: /^Hades \(2020\)/ }));
    await within(dialog).findByRole('alert');

    await rerender({ open: true, gameId: 'g2', gameTitle: 'Celeste' });

    await within(dialog).findByRole('radio', { name: /^Celeste \(2018\)/ });
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });
});
