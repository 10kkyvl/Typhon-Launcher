import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { getHistoryStatus, Kind, listHistory, type Record as HistoryRecord } from '../../lib/services/history';
import { history, historyRecent, initHistory } from '../../lib/stores/history';
import { renderDom } from '../../lib/testing/dom';
import History from './History.svelte';
import { at, clearButton, NOW, RECORDS, record, resetScreen, installed, rows, titles } from './historyTesting';

// Go's events as the store hears them: the handlers the store subscribed, called by the test.
const handlers: { [name: string]: (event: { data: unknown }) => void } = {};

vi.mock('@wailsio/runtime', async () => {
  const runtime = (await import('../../lib/testing/dom')).wailsRuntime();
  runtime.Events.On = vi.fn((name: string | undefined, handler: (event: unknown) => void) => {
    handlers[name ?? ''] = handler as (event: { data: unknown }) => void;
    return () => {};
  }) as typeof runtime.Events.On;
  return runtime;
});
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
// The art store follows the locale into Go when it runs in Wails.
vi.mock('../../../bindings/typhon/internal/metadata', () => ({ Service: { SetLanguage: vi.fn(async () => {}) } }));
vi.mock('../../lib/stores/metadata', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/metadata')>()),
  requestArt: vi.fn(),
}));
// Go's history.Service: List and StatusOf answer as each test says.
vi.mock('../../lib/services/history', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/history')>()),
  listHistory: vi.fn(),
  getHistoryStatus: vi.fn(),
  clearHistory: vi.fn(),
}));

const fresh = record('x', Kind.KindInstalled, 'Solo', at(0, 14));
const recorded = (item: HistoryRecord) => handlers['history:recorded']({ data: item });

/** The store's first read failed: the page starts from the failure with its Retry. */
async function failedFirstRead() {
  vi.mocked(listHistory).mockRejectedValueOnce(new Error('service not ready'));
  await initHistory();
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'], now: NOW });
  vi.mocked(getHistoryStatus).mockResolvedValue({ degraded: false, message: '' });
});

afterEach(() => {
  resetScreen();
  vi.mocked(listHistory).mockReset();
  vi.mocked(getHistoryStatus).mockReset();
});

describe('History: records Go sends before a whole list was read', () => {
  it('shows the failed read and its Retry alone, nothing that claims to be the history, when a record arrives after it', async () => {
    await failedFirstRead();
    recorded(fresh);
    renderDom(History);

    expect(screen.getByRole('alert').textContent).toContain(msg('transfers.historyLoadFailed'));
    expect(screen.getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(rows()).toEqual([]);
    expect(screen.queryByRole('radiogroup')).toBeNull();
    expect(screen.queryByRole('textbox', { name: msg('transfers.historySearchPlaceholder') })).toBeNull();
    const header = screen.getByRole('heading', { level: 1 }).closest('header') as HTMLElement;
    expect(within(header).queryByText(msg('transfers.historyCount', { count: 1 }))).toBeNull();
    expect((clearButton() as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeNull();
    // Kept by the store all the same, for the list to come and for the sidebar's recent list.
    expect(get(history).map((item) => item.id)).toEqual(['x']);
    expect(get(historyRecent).map((item) => item.id)).toEqual(['x']);
  });

  it('shows the loading state, not a one-record history, when a record arrives during the first read', async () => {
    let answer: (records: HistoryRecord[]) => void = () => {};
    vi.mocked(listHistory).mockImplementation(() => new Promise((resolve) => (answer = resolve)));
    const started = initHistory();
    recorded(fresh);
    renderDom(History);

    expect(screen.getByRole('heading', { name: msg('transfers.historyLoading') })).toBeTruthy();
    expect(rows()).toEqual([]);
    expect(screen.queryByRole('radiogroup')).toBeNull();
    expect((clearButton() as HTMLButtonElement).disabled).toBe(true);

    // The event came during the read: the store asks once more, and Go's second answer holds the record.
    answer(RECORDS);
    await waitFor(() => expect(listHistory).toHaveBeenCalledTimes(2));
    answer([fresh, ...RECORDS]);
    await started;

    await waitFor(() => expect(rows()).toHaveLength(13));
    expect(titles().filter((title) => title === installed('Solo'))).toHaveLength(1);
  });

  it('keeps the record that arrived and shows it once with the list the Retry read', async () => {
    await failedFirstRead();
    recorded(fresh);
    vi.mocked(listHistory).mockResolvedValueOnce([fresh, ...RECORDS]);
    const { user } = renderDom(History);

    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    await waitFor(() => expect(rows()).toHaveLength(13));
    expect(titles().filter((title) => title === installed('Solo'))).toHaveLength(1);
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByText(msg('transfers.historyCount', { count: 13 }))).toBeTruthy();
    expect((clearButton() as HTMLButtonElement).disabled).toBe(false);
  });
});
