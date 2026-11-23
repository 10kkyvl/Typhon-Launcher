import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { installErrorText } from '../../lib/install/installErrors';
import { platformBadge } from '../../lib/library/emulation';
import { clearHistory as clearRequest, getHistoryStatus, Kind, listHistory, type Record as HistoryRecord } from '../../lib/services/history';
import type { LibraryGame } from '../../lib/services/library';
import { history, historyErrorText, historyLoaded, historyLoadError, historyStatus, initHistory } from '../../lib/stores/history';
import { libraryGames } from '../../lib/stores/library';
import { gameArt, requestArt } from '../../lib/stores/metadata';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { updateErrorText } from '../../lib/updates/updateErrors';
import { bytesSize, clockTime, longDate } from '../../lib/utils/format';
import History from './History.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// Go's history.Service: List, StatusOf and Clear answer as each test says.
// The art store's one request path, as the Downloads rows use it; the covers come through gameArt.
vi.mock('../../lib/stores/metadata', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/metadata')>()),
  requestArt: vi.fn(),
}));
vi.mock('../../lib/services/history', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/history')>()),
  listHistory: vi.fn(),
  getHistoryStatus: vi.fn(),
  clearHistory: vi.fn(),
}));

// The markup the screen draws a record in, and the parts of it this file reads.
const ROW = '[data-history-row]';
const TITLE = '.title';
const DETAIL = '.detail';
const TIME = '.when [aria-hidden="true"]';

const NOW = new Date(2026, 10, 23, 15, 0);
const at = (daysAgo: number, hours: number, minutes = 0) => new Date(2026, 10, 23 - daysAgo, hours, minutes).toISOString();
const GB = 1024 ** 3;

function record(id: string, kind: Kind, title: string, when: string, patch: Partial<HistoryRecord> = {}): HistoryRecord {
  return { id, kind, at: when, title, bytesKnown: false, ...patch };
}

// Newest first, as the store keeps them; the record with no readable time last.
const RECORDS: HistoryRecord[] = [
  record('a', Kind.KindInstalled, 'Alpha', at(0, 10, 5), { gameId: 'lib-alpha' }),
  record('b', Kind.KindUpdateFailed, 'Beta', at(0, 9, 30), { detail: 'typhon:updates.no_plan: nothing to apply' }),
  record('w', Kind.KindInstalled, 'Ведьмак 3', at(0, 8)),
  record('c', Kind.KindUpdated, 'Gamma', at(1, 20), { fromVersion: '1.0', toVersion: '1.1' }),
  record('d', Kind.KindRolledBack, 'Delta', at(1, 8), { toVersion: '1.0' }),
  record('e', Kind.KindRemoved, 'Epsilon', at(13, 12), { bytes: 2 * GB, bytesKnown: true }),
  record('f', Kind.KindUninstalled, 'Zeta', at(13, 11)),
  record('g', Kind.KindInstallFailed, 'Eta', at(13, 10), { detail: 'typhon:install.not_enough_space: 3 GB short' }),
  record('h', Kind.KindDownloaded, 'Theta', at(22, 10)),
  record('i', Kind.KindMoved, 'Iota', at(22, 9), { gameId: 'lib-ps2', detail: 'C:\\Games\\Iota → D:\\Games\\Iota' }),
  record('j', Kind.KindLanReceived, 'Kappa', at(22, 8)),
  record('k', Kind.KindInstalled, 'Lambda', 'not a date'),
];

function game(id: string, patch: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title: id,
    executable: '',
    installDir: '',
    cover: '',
    version: '',
    sizeBytes: 0,
    lastPlayed: null,
    playtimeSeconds: 0,
    installedAt: '',
    ...patch,
  };
}

const words = (el: Element | null | undefined) => (el?.textContent ?? '').replace(/\s+/g, ' ').trim();
const rows = () => Array.from(document.querySelectorAll<HTMLElement>(ROW));
const titles = () => rows().map((row) => words(row.querySelector(TITLE)));
const rowOf = (title: string) => {
  const found = rows().find((row) => words(row.querySelector(TITLE)) === title);
  if (!found) throw new Error(`no row titled ${title}; rows: ${titles().join(' | ')}`);
  return found;
};
const detailOf = (title: string) => words(rowOf(title).querySelector(DETAIL));
const timeOf = (title: string) => words(rowOf(title).querySelector(TIME));
const dayHeadings = () => screen.queryAllByRole('heading', { level: 2 }).map(words);
const segments = () => screen.queryAllByRole('radio').map(words);
const search = () => screen.getByRole('textbox', { name: msg('transfers.historySearchPlaceholder') });
const clearButton = () => screen.getByRole('button', { name: msg('transfers.historyClearAction') });
const toastsSaid = () => get(toasts).map((item) => [item.kind, item.message]);

const installed = (title: string) => msg('transfers.historyInstalledTitle', { title });

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'], now: NOW });
  historyLoaded.set(true);
});

afterEach(() => {
  history.set([]);
  historyLoaded.set(false);
  historyLoadError.set(null);
  historyStatus.set({ degraded: false, message: '' });
  libraryGames.set([]);
  gameArt.set({});
  vi.mocked(requestArt).mockReset();
  toasts.set([]);
  vi.mocked(listHistory).mockReset();
  vi.mocked(getHistoryStatus).mockReset();
  vi.mocked(clearRequest).mockReset();
  resetHistory();
  document.body.replaceChildren();
});

describe('History: the layout', () => {
  it('says how many records there are under the title, and nothing of the kind while there are none', () => {
    history.set(RECORDS);
    const view = renderDom(History);

    const header = screen.getByRole('heading', { level: 1 }).closest('header') as HTMLElement;
    expect(within(header).getByText(msg('transfers.historyCount', { count: 12 }))).toBeTruthy();

    history.set([]);
    view.rerender({});
    expect(within(header).queryByText(msg('transfers.historyCount', { count: 0 }))).toBeNull();
  });

  it('makes each day a section named by its heading, with its records as a list', () => {
    history.set(RECORDS);
    renderDom(History);

    const regions = screen.getAllByRole('region');
    expect(regions.map((region) => words(document.getElementById(region.getAttribute('aria-labelledby') ?? '')))).toEqual(dayHeadings());
    const today = regions[0];
    expect(within(within(today).getByRole('list')).getAllByRole('listitem').map((item) => words(item.querySelector(TITLE)))).toEqual([
      installed('Alpha'),
      msg('transfers.historyUpdateFailedTitle', { title: 'Beta' }),
      installed('Ведьмак 3'),
    ]);
  });
});

describe('History: the segments', () => {
  // Downloads and copies over the local network count as installs, and moves have a segment of their
  // own, as the prototype groups them.
  it('counts every record under All and the kinds of each segment under it', () => {
    history.set(RECORDS);
    renderDom(History);

    expect(segments()).toEqual([
      `${msg('transfers.historySegmentAll')} 12`,
      `${msg('transfers.historySegmentInstalls')} 6`,
      `${msg('transfers.historySegmentUpdates')} 3`,
      `${msg('transfers.historySegmentRemovals')} 2`,
      `${msg('transfers.historySegmentMoves')} 1`,
    ]);
    expect(rows()).toHaveLength(12);
  });

  it('shows installs, failed installs, downloads and copies from the local network under Installs', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    await user.click(screen.getByRole('radio', { name: new RegExp(msg('transfers.historySegmentInstalls')) }));

    expect(titles()).toEqual([
      installed('Alpha'),
      installed('Ведьмак 3'),
      msg('transfers.historyInstallFailedTitle', { title: 'Eta' }),
      msg('transfers.historyDownloadedTitle', { title: 'Theta' }),
      msg('transfers.historyLanReceivedTitle', { title: 'Kappa' }),
      installed('Lambda'),
    ]);
  });

  it('shows updates, failed updates and rollbacks under Updates', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    await user.click(screen.getByRole('radio', { name: new RegExp(msg('transfers.historySegmentUpdates')) }));

    expect(titles()).toEqual([
      msg('transfers.historyUpdateFailedTitle', { title: 'Beta' }),
      msg('transfers.historyUpdatedTitle', { title: 'Gamma' }),
      msg('transfers.historyRolledBackTitle', { title: 'Delta', version: '1.0' }),
    ]);
  });

  it('shows removals and uninstalls under Removals', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    await user.click(screen.getByRole('radio', { name: new RegExp(msg('transfers.historySegmentRemovals')) }));

    expect(titles()).toEqual([msg('transfers.historyRemovedTitle', { title: 'Epsilon' }), msg('transfers.historyRemovedTitle', { title: 'Zeta' })]);
  });

  it('shows moves under Moves', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    await user.click(screen.getByRole('radio', { name: new RegExp(msg('transfers.historySegmentMoves')) }));

    expect(titles()).toEqual([msg('transfers.historyMovedTitle', { title: 'Iota' })]);
  });

  it('puts every record of a known kind under exactly one segment besides All', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);
    const all = titles();
    const seen: string[] = [];

    for (const name of ['historySegmentInstalls', 'historySegmentUpdates', 'historySegmentRemovals', 'historySegmentMoves'] as const) {
      await user.click(screen.getByRole('radio', { name: new RegExp(msg(`transfers.${name}`)) }));
      seen.push(...titles());
    }

    expect(all).toHaveLength(RECORDS.length);
    expect([...seen].sort()).toEqual([...all].sort());
  });
});

describe('History: search', () => {
  it('finds by title whatever the case, Cyrillic included, and ignores the spaces around the query', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);
    const field = search();

    await user.type(field, 'ALPHA');
    expect(titles()).toEqual([installed('Alpha')]);

    await user.clear(field);
    await user.type(field, 'ведьМАК');
    expect(titles()).toEqual([installed('Ведьмак 3')]);

    await user.clear(field);
    await user.type(field, '  gamma  ');
    expect(titles()).toEqual([msg('transfers.historyUpdatedTitle', { title: 'Gamma' })]);
  });

  it('searches within the chosen segment', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    await user.type(search(), 'ta');
    expect(titles()).toEqual([
      msg('transfers.historyUpdateFailedTitle', { title: 'Beta' }),
      msg('transfers.historyRolledBackTitle', { title: 'Delta', version: '1.0' }),
      msg('transfers.historyRemovedTitle', { title: 'Zeta' }),
      msg('transfers.historyInstallFailedTitle', { title: 'Eta' }),
      msg('transfers.historyDownloadedTitle', { title: 'Theta' }),
      msg('transfers.historyMovedTitle', { title: 'Iota' }),
    ]);

    await user.click(screen.getByRole('radio', { name: new RegExp(msg('transfers.historySegmentUpdates')) }));
    expect(titles()).toEqual([
      msg('transfers.historyUpdateFailedTitle', { title: 'Beta' }),
      msg('transfers.historyRolledBackTitle', { title: 'Delta', version: '1.0' }),
    ]);
  });

  it('says nothing was found, with the filter still there to change', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    await user.type(search(), 'no such game');

    expect(rows()).toEqual([]);
    expect(screen.getByRole('heading', { name: msg('transfers.historyNoResultsTitle') })).toBeTruthy();
    expect(screen.getByText(msg('transfers.historyNoResultsDescription'))).toBeTruthy();
    expect(screen.getByRole('radiogroup')).toBeTruthy();
    expect(screen.queryByRole('heading', { name: msg('transfers.historyEmptyTitle') })).toBeNull();
  });
});

describe('History: days', () => {
  it('groups the records by day, newest first: today, yesterday, then the date, then a record with no time', () => {
    history.set(RECORDS);
    renderDom(History);

    expect(dayHeadings()).toEqual([
      msg('format.today'),
      msg('format.yesterday'),
      longDate(new Date(2026, 10, 10)),
      longDate(new Date(2026, 10, 1)),
      '—',
    ]);
    const day = (label: string) => screen.getByRole('heading', { level: 2, name: label }).closest('section') as HTMLElement;
    const inDay = (label: string) => Array.from(day(label).querySelectorAll(ROW)).map((row) => words(row.querySelector(TITLE)));
    expect(inDay(msg('format.yesterday'))).toEqual([
      msg('transfers.historyUpdatedTitle', { title: 'Gamma' }),
      msg('transfers.historyRolledBackTitle', { title: 'Delta', version: '1.0' }),
    ]);
    expect(inDay('—')).toEqual([installed('Lambda')]);
  });

  it('shows the time of day of each record, and none for a record whose time cannot be read', () => {
    history.set(RECORDS);
    renderDom(History);

    expect(timeOf(installed('Alpha'))).toBe(clockTime(new Date(2026, 10, 23, 10, 5)));
    expect(timeOf(msg('transfers.historyMovedTitle', { title: 'Iota' }))).toBe(clockTime(new Date(2026, 10, 1, 9, 0)));
    expect(timeOf(installed('Lambda'))).toBe('');
  });
});

describe('History: what a row says', () => {
  it('says each kind in its own words, with the detail it has', () => {
    history.set(RECORDS);
    renderDom(History);

    expect(detailOf(installed('Alpha'))).toBe('');
    expect(detailOf(msg('transfers.historyUpdatedTitle', { title: 'Gamma' }))).toBe(msg('transfers.historyVersionJump', { from: '1.0', to: '1.1' }));
    expect(detailOf(msg('transfers.historyRolledBackTitle', { title: 'Delta', version: '1.0' }))).toBe('');
    expect(detailOf(msg('transfers.historyRemovedTitle', { title: 'Epsilon' }))).toBe(msg('transfers.historyFreedSize', { size: bytesSize(2 * GB) }));
    expect(detailOf(msg('transfers.historyRemovedTitle', { title: 'Zeta' }))).toBe('');
    expect(detailOf(msg('transfers.historyDownloadedTitle', { title: 'Theta' }))).toBe('');
    expect(detailOf(msg('transfers.historyMovedTitle', { title: 'Iota' }))).toBe('C:\\Games\\Iota → D:\\Games\\Iota');
    expect(detailOf(msg('transfers.historyLanReceivedTitle', { title: 'Kappa' }))).toBe('');
  });

  it('says why an install or an update failed through the install and update messages, and marks those rows failed', () => {
    history.set(RECORDS);
    renderDom(History);
    const installFailed = msg('transfers.historyInstallFailedTitle', { title: 'Eta' });
    const updateFailed = msg('transfers.historyUpdateFailedTitle', { title: 'Beta' });

    expect(detailOf(installFailed)).toBe(installErrorText('typhon:install.not_enough_space: 3 GB short'));
    expect(detailOf(installFailed)).toBe(msg('errInstall.installNotEnoughSpace'));
    expect(detailOf(updateFailed)).toBe(updateErrorText('typhon:updates.no_plan: nothing to apply'));
    expect(detailOf(updateFailed)).toBe(msg('errUpdates.updatesNoPlan'));
    const failed = rows().filter((row) => row.classList.contains('failed')).map((row) => words(row.querySelector(TITLE)));
    expect(failed).toEqual([updateFailed, installFailed]);
  });

  it('names an untitled game "Game"', () => {
    history.set([record('u', Kind.KindInstalled, '', at(0, 9))]);
    renderDom(History);

    expect(titles()).toEqual([installed(msg('transfers.historyUntitledGame'))]);
  });

  it("tags a row with the console of a library game that runs in an emulator, and no other row", () => {
    libraryGames.set([game('lib-ps2', { emulation: { platform: 'ps2' } as LibraryGame['emulation'] }), game('lib-alpha')]);
    history.set(RECORDS);
    renderDom(History);

    const tagged = rows().filter((row) => within(row).queryByText(platformBadge('ps2')) !== null);
    expect(tagged.map((row) => words(row.querySelector(TITLE)))).toEqual([msg('transfers.historyMovedTitle', { title: 'Iota' })]);
  });

  // D5: no native `title` hints: the title and the detail are shown whole, the exact time is text for
  // assistive tech and a hint on hover.
  it('D5: says the exact time to assistive tech and on hover, and carries no native title', async () => {
    history.set(RECORDS);
    const { user } = renderDom(History);

    const row = rowOf(msg('transfers.historyUpdatedTitle', { title: 'Gamma' }));
    const when = new Date(2026, 10, 22, 20, 0);
    const exact = msg('transfers.historyExactTime', { date: longDate(when), time: clockTime(when) });
    expect(document.querySelector('[title]')).toBeNull();
    const time = row.querySelector('time');
    expect(time?.getAttribute('datetime')).toBe(when.toISOString());
    expect(time?.querySelector('.sr-only')?.textContent).toBe(exact);

    await user.hover(time as HTMLElement);
    await waitFor(() => expect(document.querySelector('[data-part="tooltip"]')?.textContent?.trim()).toBe(exact));
  });
});

describe('History: the game a row is about', () => {
  const linked: HistoryRecord[] = [
    record('a', Kind.KindInstalled, 'Alpha', at(0, 10), { gameId: 'lib-alpha' }),
    record('c', Kind.KindUpdated, 'Gamma', at(0, 9), { gameId: 'cat-gamma', fromVersion: '1', toVersion: '2' }),
    record('e', Kind.KindRemoved, 'Epsilon', at(0, 8), { gameId: 'lib-epsilon' }),
    record('n', Kind.KindDownloaded, 'Nu', at(0, 7), { gameId: 'cat-nu' }),
  ];

  // Go records the library id of the game (an install, a removal, a move, a LAN copy) or its catalog id
  // (a download, a failed install, an update, a rollback).
  it("links the game's name to its page while the library holds it, by its library or its catalog id", async () => {
    libraryGames.set([game('lib-alpha', { title: 'Alpha' }), game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma' })]);
    history.set(linked);
    const { user } = renderDom(History);

    const alpha = rowOf(installed('Alpha'));
    await user.click(within(alpha).getByRole('button', { name: 'Alpha' }));
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'lib-alpha' } });

    const gamma = rowOf(msg('transfers.historyUpdatedTitle', { title: 'Gamma' }));
    await user.click(within(gamma).getByRole('button', { name: 'Gamma' }));
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'lib-gamma' } });
  });

  it("tags a row found by the game's catalog id with its console too", () => {
    libraryGames.set([game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma', emulation: { platform: 'ps2' } as LibraryGame['emulation'] })]);
    history.set(linked);
    renderDom(History);

    const tagged = rows().filter((row) => within(row).queryByText(platformBadge('ps2')) !== null);
    expect(tagged.map((row) => words(row.querySelector(TITLE)))).toEqual([msg('transfers.historyUpdatedTitle', { title: 'Gamma' })]);
  });

  it("starts each row with the game's cover from the art store, else the library's own, asking the store once for the games it can name", () => {
    libraryGames.set([
      game('lib-alpha', { title: 'Alpha', canonicalGameId: 'cat-alpha', cover: 'file:///covers/alpha.jpg' }),
      game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma', cover: 'file:///covers/gamma-own.jpg' }),
    ]);
    gameArt.set({ 'cat-gamma': { cover: 'https://art.example/gamma.jpg', hero: '', heroIsPoster: false } });
    history.set(linked);
    renderDom(History);

    const cover = (title: string) => rowOf(title).querySelector('img')?.getAttribute('src') ?? null;
    expect(cover(installed('Alpha'))).toBe('file:///covers/alpha.jpg');
    expect(cover(msg('transfers.historyUpdatedTitle', { title: 'Gamma' }))).toBe('https://art.example/gamma.jpg');
    expect(cover(msg('transfers.historyRemovedTitle', { title: 'Epsilon' }))).toBeNull();
    expect(cover(msg('transfers.historyDownloadedTitle', { title: 'Nu' }))).toBeNull();
    // A removed game's library id names nothing in the catalog: it is not asked for.
    expect(vi.mocked(requestArt).mock.calls.flat(2).sort()).toEqual(['cat-alpha', 'cat-gamma', 'cat-nu']);
  });

  it("makes the cover of a library game a second way to its page for the pointer only, the name being the one stop", () => {
    libraryGames.set([game('lib-alpha', { title: 'Alpha', canonicalGameId: 'cat-alpha', cover: 'file:///covers/alpha.jpg' })]);
    history.set(linked);
    renderDom(History);

    const row = rowOf(installed('Alpha'));
    expect(row.querySelector('img')).not.toBeNull();
    const picture = row.querySelector('img')?.closest('button') as HTMLButtonElement;
    expect(picture.tabIndex).toBe(-1);
    expect(picture.getAttribute('aria-hidden')).toBe('true');
    expect(within(row).getAllByRole('button').map((button) => button.textContent?.trim())).toEqual(['Alpha']);
    expect(rowOf(msg('transfers.historyRemovedTitle', { title: 'Epsilon' })).querySelector('button')).toBeNull();
  });

  it('leaves the name plain for a game the library no longer holds or a record of no game', () => {
    libraryGames.set([game('lib-alpha', { title: 'Alpha' })]);
    history.set(linked);
    renderDom(History);

    expect(within(rowOf(msg('transfers.historyRemovedTitle', { title: 'Epsilon' }))).queryByRole('button')).toBeNull();
    expect(within(rowOf(msg('transfers.historyDownloadedTitle', { title: 'Nu' }))).queryByRole('button')).toBeNull();
  });
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
