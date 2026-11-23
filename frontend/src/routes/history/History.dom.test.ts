import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { installErrorText } from '../../lib/install/installErrors';
import { platformBadge } from '../../lib/library/emulation';
import { clearHistory as clearRequest, getHistoryStatus, Kind, listHistory, type Record as HistoryRecord } from '../../lib/services/history';
import type { LibraryGame } from '../../lib/services/library';
import { history, historyLoaded } from '../../lib/stores/history';
import { libraryGames } from '../../lib/stores/library';
import { gameArt, requestArt } from '../../lib/stores/metadata';
import { route } from '../../lib/stores/router';
import { renderDom } from '../../lib/testing/dom';
import { updateErrorText } from '../../lib/updates/updateErrors';
import { bytesSize, clockTime, longDate } from '../../lib/utils/format';
import History from './History.svelte';
import {
  at,
  dayHeadings,
  detailOf,
  game,
  GB,
  installed,
  NOW,
  RECORDS,
  record,
  resetScreen,
  ROW,
  rowOf,
  rows,
  search,
  segments,
  timeOf,
  TITLE,
  titles,
  words,
} from './historyTesting';

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

  // The name stops being a button when its game leaves the library: the focused button leaves the page,
  // as in a browser, and jsdom, as a browser, leaves focus on <body> then.
  it("keeps focus in the row when the game whose name holds it leaves the library", async () => {
    libraryGames.set([game('lib-alpha', { title: 'Alpha' }), game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma' })]);
    history.set(linked);
    renderDom(History);
    const row = rowOf(installed('Alpha'));
    within(row).getByRole('button', { name: 'Alpha' }).focus();

    libraryGames.set([game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma' })]);

    await waitFor(() => expect(within(row).queryByRole('button')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(row));
    expect(row.getAttribute('tabindex')).toBe('-1');
  });

  it('leaves focus where it is when a game leaves the library while focus is elsewhere', async () => {
    libraryGames.set([game('lib-alpha', { title: 'Alpha' }), game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma' })]);
    history.set(linked);
    renderDom(History);
    const gamma = within(rowOf(msg('transfers.historyUpdatedTitle', { title: 'Gamma' }))).getByRole('button', { name: 'Gamma' });
    gamma.focus();

    libraryGames.set([game('lib-gamma', { title: 'Gamma', canonicalGameId: 'cat-gamma' })]);

    await waitFor(() => expect(within(rowOf(installed('Alpha'))).queryByRole('button')).toBeNull());
    await Promise.resolve();
    expect(document.activeElement).toBe(gamma);
  });

  it('leaves the name plain for a game the library no longer holds or a record of no game', () => {
    libraryGames.set([game('lib-alpha', { title: 'Alpha' })]);
    history.set(linked);
    renderDom(History);

    expect(within(rowOf(msg('transfers.historyRemovedTitle', { title: 'Epsilon' }))).queryByRole('button')).toBeNull();
    expect(within(rowOf(msg('transfers.historyDownloadedTitle', { title: 'Nu' }))).queryByRole('button')).toBeNull();
  });
});
