import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { clearHistory as clearRequest, getHistoryStatus, Kind, listHistory, type Record as HistoryRecord } from '../../lib/services/history';
import { history, historyErrorText, historyLoaded, historyStatus, initHistory } from '../../lib/stores/history';
import { libraryGames } from '../../lib/stores/library';
import { requestArt } from '../../lib/stores/metadata';
import { renderDom } from '../../lib/testing/dom';
import History from './History.svelte';
import { clearButton, game, installed, NOW, RECORDS, record, resetScreen, rowOf, rows, search, toastsSaid } from './historyTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The art store's one request path, as the Downloads rows use it; the covers come through gameArt.
vi.mock('../../lib/stores/metadata', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/metadata')>()),
  requestArt: vi.fn(),
}));
// Go's history.Service: List, StatusOf and Clear answer as each test says.
vi.mock('../../lib/services/history', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/history')>()),
  listHistory: vi.fn(),
  getHistoryStatus: vi.fn(),
  clearHistory: vi.fn(),
}));

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'], now: NOW });
  historyLoaded.set(true);
});

afterEach(() => {
  resetScreen();
  vi.mocked(requestArt).mockReset();
  vi.mocked(listHistory).mockReset();
  vi.mocked(getHistoryStatus).mockReset();
  vi.mocked(clearRequest).mockReset();
});

describe('History: states', () => {
  it('says the history is empty, with nothing to filter or clear', () => {
    renderDom(History);

    expect(screen.getByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeTruthy();
    expect(screen.getByText(msg('transfers.historyEmptyDescription'))).toBeTruthy();
    expect(screen.queryByRole('radiogroup')).toBeNull();
    expect(screen.queryByRole('textbox', { name: msg('transfers.historySearchPlaceholder') })).toBeNull();
    expect((clearButton() as HTMLButtonElement).disabled).toBe(true);
  });

  it('shows that the history is not being saved as a danger notice that is not announced', () => {
    historyStatus.set({ degraded: true, message: 'disk is full' });
    history.set(RECORDS);
    renderDom(History);

    const text = screen.getByText(msg('transfers.historyDegradedBanner', { message: 'disk is full' }));
    const notice = text.closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');
    expect(notice.querySelector('[role="alert"], [role="status"], [aria-live]')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(rows()).toHaveLength(12);
  });

  it('shows a loading state, not an empty history, until the first read answers', () => {
    historyLoaded.set(false);
    vi.mocked(listHistory).mockReturnValueOnce(new Promise(() => {}));
    void initHistory();
    renderDom(History);

    expect(screen.getByRole('heading', { name: msg('transfers.historyLoading') })).toBeTruthy();
    expect(screen.queryByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  // D2: a first read that failed says so, with a Retry, instead of looking like an empty history.
  it('D2: says a failed first read with a Retry, and no empty state; the Retry reads again', async () => {
    historyLoaded.set(false);
    vi.mocked(listHistory).mockRejectedValueOnce(new Error('service not ready'));
    vi.mocked(getHistoryStatus).mockResolvedValue({ degraded: false, message: '' });
    await initHistory();
    vi.mocked(listHistory).mockResolvedValueOnce([]);
    const { user } = renderDom(History);

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain(msg('transfers.historyLoadFailed'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(screen.queryByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeNull();
    expect(screen.queryByRole('heading', { name: msg('transfers.historyLoading') })).toBeNull();

    const retry = screen.getByRole('button', { name: msg('common.retry') });
    retry.focus();
    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.getByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeTruthy());
    expect(listHistory).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole('alert')).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  it('D2: shows the list the Retry brought', async () => {
    historyLoaded.set(false);
    vi.mocked(listHistory).mockRejectedValueOnce(new Error('service not ready'));
    vi.mocked(getHistoryStatus).mockResolvedValue({ degraded: false, message: '' });
    await initHistory();
    vi.mocked(listHistory).mockResolvedValueOnce(RECORDS);
    const { user } = renderDom(History);

    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    await waitFor(() => expect(rows()).toHaveLength(12));
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('D2: keeps the failure and its Retry when the read fails again, says it again, focus staying on Retry', async () => {
    historyLoaded.set(false);
    vi.mocked(listHistory).mockRejectedValueOnce(new Error('service not ready'));
    vi.mocked(getHistoryStatus).mockResolvedValue({ degraded: false, message: '' });
    await initHistory();
    vi.mocked(listHistory).mockRejectedValueOnce(new Error('still not ready'));
    const { user } = renderDom(History);
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const region = notice.querySelector('.body') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(listHistory).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(region.textContent).toContain(msg('transfers.historyLoadFailed'));
    expect(document.activeElement).toBe(retry);
    expect(screen.queryByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeNull();
  });
});

describe('History: clearing', () => {
  const dialog = () => screen.getByRole('dialog', { name: msg('transfers.historyClearAction') });

  it('asks first, and clears only on the confirmation', async () => {
    history.set(RECORDS);
    vi.mocked(clearRequest).mockResolvedValue(undefined);
    const { user } = renderDom(History);

    await user.click(clearButton());
    expect(within(dialog()).getByText(msg('transfers.historyConfirmClearText'))).toBeTruthy();
    expect(clearRequest).not.toHaveBeenCalled();

    await user.click(within(dialog()).getByRole('button', { name: msg('common.cancel') }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(clearRequest).not.toHaveBeenCalled();
    expect(rows()).toHaveLength(12);
  });

  it('empties the list once Go cleared it, and says so', async () => {
    history.set(RECORDS);
    vi.mocked(clearRequest).mockResolvedValue(undefined);
    const { user } = renderDom(History);

    await user.click(clearButton());
    await user.click(within(dialog()).getByRole('button', { name: msg('transfers.historyClearAction') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(clearRequest).toHaveBeenCalledTimes(1);
    expect(rows()).toEqual([]);
    expect(screen.getByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeTruthy();
    expect((clearButton() as HTMLButtonElement).disabled).toBe(true);
    expect(toastsSaid()).toEqual([['success', msg('transfers.historyCleared')]]);
  });

  // D4: Clear, where the dialog would hand focus back, is disabled by the empty list: focus goes to the page heading.
  it('D4: hands focus to the page heading once the list is cleared', async () => {
    history.set(RECORDS);
    vi.mocked(clearRequest).mockResolvedValue(undefined);
    const { user } = renderDom(History);

    clearButton().focus();
    await user.keyboard('{Enter}');
    within(dialog()).getByRole('button', { name: msg('transfers.historyClearAction') }).focus();
    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  it('D4: gives focus back to Clear when the dialog is cancelled', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    clearButton().focus();
    await user.keyboard('{Enter}');
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(clearButton());
  });

  it('D4: hands focus to the page heading when the list emptied while the dialog was open and it is cancelled', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    clearButton().focus();
    await user.keyboard('{Enter}');
    history.set([]);
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  // D1: a failed clear is said once, in the dialog, which stays open for another try.
  it('D1: keeps the list and the dialog when Go could not clear, says why there, and nothing else', async () => {
    history.set(RECORDS);
    vi.mocked(clearRequest).mockRejectedValue(new Error('typhon:history.clear_failed: read-only disk'));
    const { user } = renderDom(History);

    await user.click(clearButton());
    await user.click(within(dialog()).getByRole('button', { name: msg('transfers.historyClearAction') }));

    await waitFor(() => expect(within(dialog()).getByRole('alert').textContent).toContain(msg('history.clear_failed')));
    expect(historyErrorText(new Error('typhon:history.clear_failed'))).toBe(msg('history.clear_failed'));
    expect(within(dialog()).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(rows()).toHaveLength(12);
    expect(toastsSaid()).toEqual([]);
  });
});

describe('History: a long history', () => {
  const name = (index: number) => `Game ${String(index).padStart(3, '0')}`;
  // Newest first, half an hour apart: a few days.
  const many = (count: number, kind: (index: number) => Kind = () => Kind.KindInstalled): HistoryRecord[] =>
    Array.from({ length: count }, (_, index) =>
      record(`m${index}`, kind(index), name(index), new Date(NOW.getTime() - (index + 1) * 30 * 60_000).toISOString(), { gameId: `lib-${index}` }),
    );
  const more = () => screen.queryByRole('button', { name: msg('transfers.historyShowMore') });

  it('shows sixty records, then sixty more at each press of Show more, which keeps focus while more are left', async () => {
    history.set(many(130));
    const { user } = renderDom(History);

    expect(rows()).toHaveLength(60);
    expect(screen.getByText(msg('transfers.historyShownOf', { shown: 60, total: 130 }))).toBeTruthy();

    (more() as HTMLElement).focus();
    await user.keyboard('{Enter}');

    expect(rows()).toHaveLength(120);
    expect(document.activeElement).toBe(more());
    expect(screen.getByText(msg('transfers.historyShownOf', { shown: 120, total: 130 }))).toBeTruthy();
  });

  it('hands focus to the first record the last press brought, from which Tab goes on through the new ones', async () => {
    libraryGames.set(Array.from({ length: 130 }, (_, index) => game(`lib-${index}`, { title: name(index) })));
    history.set(many(130));
    const { user } = renderDom(History);
    expect(rows()).toHaveLength(60);

    (more() as HTMLElement).focus();
    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(rows()).toHaveLength(130);
    expect(more()).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(within(rowOf(installed(name(120)))).getByRole('button', { name: name(120) })));
    await user.tab();
    expect(document.activeElement).toBe(within(rowOf(installed(name(121)))).getByRole('button', { name: name(121) }));
  });

  it('hands focus to the day of the first new record when it has nothing to focus', async () => {
    history.set(many(70));
    const { user } = renderDom(History);
    expect(rows()).toHaveLength(60);

    (more() as HTMLElement).focus();
    await user.keyboard('{Enter}');

    expect(rows()).toHaveLength(70);
    const heading = rowOf(installed(name(60))).closest('section')?.querySelector('h2');
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it('starts again from sixty when the segment or the search changes', async () => {
    history.set(many(130, (index) => (index % 2 === 0 ? Kind.KindInstalled : Kind.KindUpdated)));
    const { user } = renderDom(History);

    await user.click(more() as HTMLElement);
    expect(rows()).toHaveLength(120);

    await user.click(screen.getByRole('radio', { name: new RegExp(msg('transfers.historySegmentInstalls')) }));
    expect(rows()).toHaveLength(60);
    await user.click(more() as HTMLElement);
    expect(rows()).toHaveLength(65);

    await user.type(search(), 'game');
    expect(rows()).toHaveLength(60);
  });

  it('shows no Show more for a history that fits', () => {
    history.set(many(60));
    renderDom(History);

    expect(rows()).toHaveLength(60);
    expect(more()).toBeNull();
    expect(screen.queryByText(msg('transfers.historyShownOf', { shown: 60, total: 60 }))).toBeNull();
  });
});
