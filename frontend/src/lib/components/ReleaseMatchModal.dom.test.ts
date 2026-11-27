import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../i18n';
import { confirmMatch, getMatchCandidates, ignoreRelease, searchGames, type CatalogGame, type MatchCandidate, type ReleaseView } from '../services/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import ReleaseMatchModal from './ReleaseMatchModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getMatchCandidates: vi.fn(),
  searchGames: vi.fn(),
  confirmMatch: vi.fn(),
  ignoreRelease: vi.fn(),
}));

const release = {
  release: { id: 'r1', rawTitle: 'Hades v1.0', normalizedTitle: 'hades', size: 1024, uploadedAt: '2026-11-01T10:00:00Z' },
  sourceName: 'FitGirl',
} as unknown as ReleaseView;
const other = { ...release, release: { ...release.release, id: 'r2', rawTitle: 'Celeste v2' } } as ReleaseView;

const candidate = (gameId: string, title: string, patch: Partial<MatchCandidate> = {}): MatchCandidate => ({ gameId, title, score: 0.9, method: 'fuzzy', source: 'local', ...patch });
const game = (id: string, title: string): CatalogGame => ({ id, title, sortTitle: title }) as CatalogGame;
// Go's own words after the code: none of them may reach the screen.
const goError = (code: string) => new Error(`typhon:${code}: GO TEXT`);
// A row is named by its title (a row of the list before the kit read on into its platforms and score).
const named = (title: string) => new RegExp(`^${title}(?=$| ${msg('modals.releaseMatchMethodFuzzy')}| Windows)`);
const goText = () => document.body.textContent?.includes('GO TEXT') || get(toasts).some((toast) => toast.message.includes('GO TEXT'));

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
  const result = render(ReleaseMatchModal, { target: app, props: { open: true, release, ...props } });
  return { ...result, dialog: screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') }) };
}

const confirmButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.releaseMatchConfirm') });
const ignoreButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.releaseMatchIgnore') });
const cancelButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('common.cancel') });

describe('ReleaseMatchModal: opening', () => {
  it('puts the focus in the search field, never on Ignore, and shows no native title', async () => {
    vi.mocked(getMatchCandidates).mockResolvedValue([candidate('a', 'Hades', { platforms: ['Windows', 'Linux'] })]);
    const { dialog } = openDialog();

    expect(document.activeElement).toBe(within(dialog).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') }));
    await within(dialog).findByRole('radio', { name: named('Hades') });
    expect(dialog.querySelectorAll('[title]').length).toBe(0);
    expect(document.activeElement).not.toBe(ignoreButton(dialog));
  });
});

describe('ReleaseMatchModal: the candidates', () => {
  it('are an option list named by their heading: the arrows only walk it, Space chooses, and one not in the catalog says why and is passed over', async () => {
    vi.mocked(getMatchCandidates).mockResolvedValue([candidate('a', 'Hades'), candidate('b', 'Hades II', { inCatalog: false }), candidate('c', 'Hades Demo')]);
    const { dialog } = openDialog();
    const keys = user();
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.releaseMatchCandidates') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false']);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([0, -1, -1]);
    expect(radios[1].getAttribute('aria-disabled')).toBe('true');
    expect(radios[1].getAttribute('aria-describedby')).toBeTruthy();
    const why = document.getElementById(radios[1].getAttribute('aria-describedby')?.split(' ').pop() ?? '');
    expect(why?.textContent).toBe(msg('modals.releaseMatchNotInCatalog'));
    expect(radios[1].contains(why)).toBe(true);
    expect(radios[1].hasAttribute('title')).toBe(false);
    await keys.click(radios[1]);
    expect(radios[1].getAttribute('aria-checked')).toBe('false');

    radios[0].focus();
    await keys.keyboard('{ArrowDown}');

    expect(document.activeElement).toBe(radios[2]);
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false']);
    expect(confirmButton(dialog)).toHaveProperty('disabled', true);

    await keys.keyboard(' ');

    expect(radios[2].getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(radios[2]);
    expect(vi.mocked(confirmMatch)).not.toHaveBeenCalled();
  });

  it('say they are being looked up with a labelled spinner, and that there are none with an inline empty state', async () => {
    const found = held<MatchCandidate[]>();
    vi.mocked(getMatchCandidates).mockReturnValueOnce(found.promise);
    const { dialog } = openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.releaseMatchSearchingCandidates'));

    found.resolve([]);
    const empty = await within(dialog).findByText(msg('modals.releaseMatchNoCandidates'));
    expect(empty.closest('.empty')).not.toBeNull();
  });

  it('say a failed look-up in the dialog by its code, never Go text and never as "none", and Retry finds them', async () => {
    vi.mocked(getMatchCandidates).mockRejectedValueOnce(goError('sources.catalog_unavailable')).mockResolvedValueOnce([candidate('a', 'Hades')]);
    const { dialog } = openDialog();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.releaseMatchCandidatesFailed', { reason: msg('errSources.srcCatalogUnavailable') }));
    expect(within(dialog).queryByText(msg('modals.releaseMatchNoCandidates'))).toBeNull();
    expect(goText()).toBe(false);
    expect(get(toasts)).toEqual([]);

    await user().click(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') }));

    expect(await within(dialog).findByRole('radio', { name: named('Hades') })).toBeTruthy();
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('give a failure with no code the dialog’s own reason', async () => {
    vi.mocked(getMatchCandidates).mockRejectedValueOnce(new Error('dial tcp: GO TEXT'));
    const { dialog } = openDialog();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.releaseMatchCandidatesFailed', { reason: msg('modals.releaseMatchUnknownReason') }));
    expect(goText()).toBe(false);
  });
});

describe('ReleaseMatchModal: how a candidate was found', () => {
  it.each<[string, MessageKey]>([
    ['external_id', 'modals.releaseMatchMethodExternalId'],
    ['override', 'modals.releaseMatchMethodOverride'],
    ['exact_title', 'modals.releaseMatchMethodExactTitle'],
    ['alias', 'modals.releaseMatchMethodAlias'],
    ['fuzzy', 'modals.releaseMatchMethodFuzzy'],
    ['provisional', 'modals.releaseMatchMethodProvisional'],
    ['server_title', 'modals.releaseMatchMethodServerTitle'],
    ['none', 'modals.releaseMatchMethodUnknown'],
    ['someday_new', 'modals.releaseMatchMethodUnknown'],
  ])('says %s in words with the score, never the code', async (method, words) => {
    vi.mocked(getMatchCandidates).mockResolvedValue([candidate('a', 'Hades', { method, score: 0.72, year: 2020 })]);
    const { dialog } = openDialog();

    const row = await within(dialog).findByRole('radio', { name: /^Hades/ });

    expect(row.textContent).toContain(`2020 · ${msg(words)} · 72%`);
    expect(row.textContent).not.toContain(method);
  });

  it('says a candidate from the server catalog by where it came from, with no score', async () => {
    vi.mocked(getMatchCandidates).mockResolvedValue([candidate('a', 'Hades', { method: 'none', source: 'server' })]);
    const { dialog } = openDialog();

    const row = await within(dialog).findByRole('radio', { name: /^Hades/ });

    expect(row.textContent).toContain(msg('modals.releaseMatchFromCatalog'));
    expect(row.textContent).not.toContain('%');
  });
});

describe('ReleaseMatchModal: opened for another release before the first answer', () => {
  it('shows and saves the candidates of the release it is open for, not a late answer for the one before', async () => {
    const first = held<MatchCandidate[]>();
    vi.mocked(getMatchCandidates).mockImplementation((id: string) => (id === 'r1' ? first.promise : Promise.resolve([candidate('celeste', 'Celeste')])));
    vi.mocked(confirmMatch).mockResolvedValue(undefined);
    const { rerender } = render(ReleaseMatchModal, { target: app, props: { open: true, release } });
    await rerender({ open: true, release: other });
    const dialog = screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') });
    await within(dialog).findByRole('radio', { name: named('Celeste') });

    first.resolve([candidate('hades', 'Hades')]);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByRole('radio', { name: named('Hades') })).toBeNull();
    const keys = user();
    await keys.click(within(dialog).getByRole('radio', { name: named('Celeste') }));
    await keys.click(confirmButton(dialog));
    expect(vi.mocked(confirmMatch)).toHaveBeenCalledWith('r2', 'celeste');
  });
});

describe('ReleaseMatchModal: the manual search', () => {
  it('is a search field named by its heading whose failure stays, its Retry busy and focused, until the answer', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const again = held<CatalogGame[]>();
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(searchGames).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { dialog } = openDialog();
    const keys = user();
    const field = within(dialog).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') });

    await keys.type(field, 'hades');
    await vi.advanceTimersByTimeAsync(300);
    const alert = await within(dialog).findByRole('alert');
    const notice = alert.closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');
    await vi.advanceTimersByTimeAsync(300);

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve([game('g1', 'Hades')]);
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.releaseMatchManualSearch') });
    expect(within(group).getByRole('radio', { name: named('Hades') })).toBeTruthy();
    expect(notice.isConnected).toBe(false);
  });

  async function failedSearch() {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    const { dialog } = openDialog();
    const keys = user();
    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') }), 'hades');
    await vi.advanceTimersByTimeAsync(300);
    const region = await within(dialog).findByRole('alert');
    const retry = within(region.closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') });
    retry.focus();
    return { dialog, keys, region, retry };
  }

  it('says a failed search by its code, a code with no words of its own by the dialog’s reason, never Go text', async () => {
    vi.mocked(searchGames).mockRejectedValueOnce(goError('catalog.backend_outdated'));
    const { region } = await failedSearch();

    expect(region.textContent?.trim()).toBe(msg('modals.releaseMatchSearchFailed', { reason: msg('modals.releaseMatchUnknownReason') }));
    expect(goText()).toBe(false);
  });

  it('sends nothing while the name is typed or pasted, only after the pause', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(searchGames).mockResolvedValue([]);
    const { dialog } = openDialog();
    const keys = user();
    const field = within(dialog).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') });

    await keys.type(field, 'had');
    field.focus();
    await keys.paste('es');

    expect(vi.mocked(searchGames)).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(300);
    expect(vi.mocked(searchGames).mock.calls).toEqual([['hades']]);
  });

  it('asks again at once, not after the typing pause, and moves the focus to the result it found', async () => {
    vi.mocked(searchGames).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([game('g1', 'Hades')]);
    const { dialog, keys } = await failedSearch();

    await keys.keyboard('{Enter}');

    expect(vi.mocked(searchGames)).toHaveBeenCalledTimes(2);
    const result = await within(dialog).findByRole('radio', { name: named('Hades') });
    await vi.waitFor(() => expect(document.activeElement).not.toBe(document.body));
    expect(document.activeElement).toBe(result);
  });

  it('says a search that fails again in the same words again, and keeps the focus on its Retry', async () => {
    vi.mocked(searchGames).mockRejectedValue(new Error('offline'));
    const { keys, region, retry } = await failedSearch();
    const before = region.firstElementChild;

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent?.trim()).toBe(before?.textContent?.trim());
    expect(document.activeElement).toBe(retry);
  });
});

describe('ReleaseMatchModal: closed and opened again while a search Retry runs', () => {
  it('opens without the old failure: no empty notice with a busy Retry', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const again = held<CatalogGame[]>();
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(searchGames).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { rerender } = render(ReleaseMatchModal, { target: app, props: { open: true, release } });
    const keys = user();
    await keys.type(within(screen.getByRole('dialog')).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') }), 'hades');
    await vi.advanceTimersByTimeAsync(300);
    await keys.click(within((await screen.findByRole('alert')).closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') }));

    await rerender({ open: false, release });
    await rerender({ open: true, release });

    const dialog = await screen.findByRole('dialog', { name: msg('modals.releaseMatchTitle') });
    expect(dialog.querySelectorAll('.notice').length).toBe(0);
    again.resolve([]);
  });
});

describe('ReleaseMatchModal: an old search Retry answering after the dialog was opened again', () => {
  it('leaves the new Retry busy, and a second press sends nothing', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const old = held<CatalogGame[]>();
    const never = new Promise<never>(() => {});
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(searchGames).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(old.promise).mockRejectedValueOnce(new Error('offline')).mockReturnValue(never);
    const { rerender } = render(ReleaseMatchModal, { target: app, props: { open: true, release } });
    const keys = user();
    const failedRetry = async () => {
      await keys.type(within(screen.getByRole('dialog')).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') }), 'hades');
      await vi.advanceTimersByTimeAsync(300);
      return within((await screen.findByRole('alert')).closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') });
    };
    await keys.click(await failedRetry());
    await rerender({ open: false, release });
    await rerender({ open: true, release });
    const retry = await failedRetry();
    await keys.click(retry);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    const sent = vi.mocked(searchGames).mock.calls.length;

    old.resolve([]);
    await vi.advanceTimersByTimeAsync(0);

    expect(retry.getAttribute('aria-busy')).toBe('true');
    await keys.click(retry);
    expect(vi.mocked(searchGames).mock.calls.length).toBe(sent);
  });
});

async function chosen(props: Record<string, unknown> = {}) {
  vi.mocked(getMatchCandidates).mockImplementation(async (id: string) => (id === 'r1' ? [candidate('a', 'Hades'), candidate('b', 'Hades II')] : [candidate('celeste', 'Celeste')]));
  const opened = openDialog(props);
  const keys = user();
  await keys.click(await within(opened.dialog).findByRole('radio', { name: named('Hades') }));
  return { ...opened, keys };
}

describe('ReleaseMatchModal: confirming', () => {
  it('keeps the focus on "confirm", busy, holds the list, Ignore and Cancel with a reason, and sends one match for two presses', async () => {
    const saved = held<void>();
    vi.mocked(confirmMatch).mockReturnValueOnce(saved.promise);
    const { dialog, keys } = await chosen();
    const confirm = confirmButton(dialog);
    confirm.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(confirm);

    expect(vi.mocked(confirmMatch).mock.calls).toEqual([['r1', 'a']]);
    expect(confirm.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(confirm);
    const group = within(dialog).getByRole('radiogroup', { name: msg('modals.releaseMatchCandidates') });
    expect(group.getAttribute('aria-disabled')).toBe('true');
    for (const button of [ignoreButton(dialog), cancelButton(dialog)]) {
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(button.hasAttribute('disabled')).toBe(false);
      expect(button.getAttribute('aria-describedby')).toBeTruthy();
    }
    await keys.click(within(group).getByRole('radio', { name: named('Hades II') }));
    expect(within(group).getByRole('radio', { name: named('Hades II') }).getAttribute('aria-checked')).toBe('false');
    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeNull();
    saved.resolve();
  });

  it('holds the search results too while a match found by name is saved', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const saved = held<void>();
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(searchGames).mockResolvedValue([game('g1', 'Hades'), game('g2', 'Hades II')]);
    vi.mocked(confirmMatch).mockReturnValueOnce(saved.promise);
    const { dialog } = openDialog();
    const keys = user();
    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') }), 'hades');
    await vi.advanceTimersByTimeAsync(300);
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.releaseMatchManualSearch') });
    await keys.click(within(group).getByRole('radio', { name: named('Hades') }));

    await keys.click(confirmButton(dialog));
    await keys.click(within(group).getByRole('radio', { name: named('Hades II') }));

    expect(group.getAttribute('aria-disabled')).toBe('true');
    expect(within(group).getByRole('radio', { name: named('Hades II') }).getAttribute('aria-checked')).toBe('false');
    expect(vi.mocked(confirmMatch).mock.calls).toEqual([['r1', 'g1']]);
    saved.resolve();
  });

  it('closes on success, toasts it and tells the page', async () => {
    vi.mocked(confirmMatch).mockResolvedValueOnce(undefined);
    const onchanged = vi.fn();
    const { dialog, keys } = await chosen({ onchanged });

    await keys.click(confirmButton(dialog));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([[msg('modals.releaseMatchSaved'), 'success']]);
    expect(onchanged).toHaveBeenCalledTimes(1);
  });

  it('says a refusal in the dialog by its code, never Go text, keeps the dialog open and moves the focus to the words', async () => {
    vi.mocked(confirmMatch).mockRejectedValueOnce(goError('catalog.game_not_found'));
    const onchanged = vi.fn();
    const { dialog, keys } = await chosen({ onchanged });
    const confirm = confirmButton(dialog);
    confirm.focus();

    await keys.keyboard('{Enter}');

    const words = msg('modals.releaseMatchConfirmFailed', { reason: msg('errMetadata.catalogGameNotFound') });
    const alert = await within(dialog).findByText(words);
    await vi.waitFor(() => expect(document.activeElement).toBe(alert.closest('.notice')));
    expect(goText()).toBe(false);
    expect(get(toasts)).toEqual([]);
    expect(onchanged).not.toHaveBeenCalled();
    expect(confirm.getAttribute('aria-busy')).toBeNull();
    expect(within(dialog).getByRole('radiogroup', { name: msg('modals.releaseMatchCandidates') }).getAttribute('aria-disabled')).toBeNull();
  });

  it('says a refused ignore in the dialog by its code', async () => {
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(ignoreRelease).mockRejectedValueOnce(goError('sources.release_not_found'));
    const { dialog } = openDialog();
    const keys = user();
    const ignore = ignoreButton(dialog);
    ignore.focus();

    await keys.keyboard('{Enter}');

    const words = msg('modals.releaseMatchIgnoreFailed', { reason: msg('errSources.srcReleaseNotFound') });
    const alert = await within(dialog).findByText(words);
    await vi.waitFor(() => expect(document.activeElement).toBe(alert.closest('.notice')));
    expect(vi.mocked(ignoreRelease).mock.calls).toEqual([['r1', true]]);
    expect(goText()).toBe(false);
    expect(get(toasts)).toEqual([]);
  });
});

describe('ReleaseMatchModal: an answer for an opening the dialog has left', () => {
  it('a match saved after the dialog was opened for another release closes nothing and paints nothing there', async () => {
    const saved = held<void>();
    vi.mocked(confirmMatch).mockReturnValueOnce(saved.promise);
    const onchanged = vi.fn();
    const { dialog, keys, rerender } = await chosen({ onchanged });
    await keys.click(confirmButton(dialog));
    await rerender({ open: false, release, onchanged });
    await rerender({ open: true, release: other, onchanged });
    const next = screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') });
    await within(next).findByRole('radio', { name: named('Celeste') });

    saved.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') })).toBe(next);
    expect(within(next).getByText('Celeste v2')).toBeTruthy();
    expect(within(next).queryByRole('alert')).toBeNull();
    expect(confirmButton(next).getAttribute('aria-busy')).toBeNull();
    expect(onchanged).not.toHaveBeenCalled();
    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([[msg('modals.releaseMatchSaved'), 'success']]);
  });

  it('a refused match after the dialog was opened for another release is one toast naming the release, by code, and nothing in the dialog', async () => {
    const saved = held<void>();
    vi.mocked(confirmMatch).mockReturnValueOnce(saved.promise);
    const { dialog, keys, rerender } = await chosen();
    await keys.click(confirmButton(dialog));
    await rerender({ open: false, release });
    await rerender({ open: true, release: other });
    const next = screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') });
    await within(next).findByRole('radio', { name: named('Celeste') });

    saved.reject(goError('sources.release_not_found'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([
      [msg('modals.releaseMatchFailedFor', { title: 'Hades v1.0', reason: msg('errSources.srcReleaseNotFound') }), 'danger'],
    ]);
    expect(within(next).queryByRole('alert')).toBeNull();
    expect(screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') })).toBe(next);
    expect(goText()).toBe(false);
  });

  it('a refused ignore after the dialog was closed is one toast naming the release', async () => {
    const ignored = held<void>();
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(ignoreRelease).mockReturnValueOnce(ignored.promise);
    const { dialog, rerender } = openDialog();
    await user().click(ignoreButton(dialog));
    await rerender({ open: false, release });

    ignored.reject(new Error('save ignored release: GO TEXT'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([
      [msg('modals.releaseMatchIgnoreFailedFor', { title: 'Hades v1.0', reason: msg('modals.releaseMatchUnknownReason') }), 'danger'],
    ]);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('a refusal for a dialog taken off the page is one toast naming the release', async () => {
    const saved = held<void>();
    vi.mocked(confirmMatch).mockReturnValueOnce(saved.promise);
    const { dialog, keys, unmount } = await chosen();
    await keys.click(confirmButton(dialog));
    unmount();

    saved.reject(goError('sources.catalog_unavailable'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(get(toasts).map((toast) => toast.message)).toEqual([msg('modals.releaseMatchFailedFor', { title: 'Hades v1.0', reason: msg('errSources.srcCatalogUnavailable') })]);
  });
});
