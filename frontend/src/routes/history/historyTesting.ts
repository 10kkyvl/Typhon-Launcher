import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { msg } from '../../lib/i18n';
import { Kind, type Record as HistoryRecord } from '../../lib/services/history';
import type { LibraryGame } from '../../lib/services/library';
import { history, historyLoaded, historyLoadError, historyStatus } from '../../lib/stores/history';
import { libraryGames } from '../../lib/stores/library';
import { gameArt } from '../../lib/stores/metadata';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';

// Fixtures of the history page's dom tests. Each test file mocks the services itself (vi.mock is
// hoisted per file); this holds the records, the games and the queries they share.

// The markup the screen draws a record in, and the parts of it the tests read.
export const ROW = '[data-history-row]';
export const TITLE = '.title';
export const DETAIL = '.detail';
export const TIME = '.when [aria-hidden="true"]';

export const NOW = new Date(2026, 10, 23, 15, 0);
export const at = (daysAgo: number, hours: number, minutes = 0) => new Date(2026, 10, 23 - daysAgo, hours, minutes).toISOString();
export const GB = 1024 ** 3;

export function record(id: string, kind: Kind, title: string, when: string, patch: Partial<HistoryRecord> = {}): HistoryRecord {
  return { id, kind, at: when, title, bytesKnown: false, ...patch };
}

// Newest first, as the store keeps them; the record with no readable time last.
export const RECORDS: HistoryRecord[] = [
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

export function game(id: string, patch: Partial<LibraryGame> = {}): LibraryGame {
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

export const words = (el: Element | null | undefined) => (el?.textContent ?? '').replace(/\s+/g, ' ').trim();
export const rows = () => Array.from(document.querySelectorAll<HTMLElement>(ROW));
export const titles = () => rows().map((row) => words(row.querySelector(TITLE)));
export const rowOf = (title: string) => {
  const found = rows().find((row) => words(row.querySelector(TITLE)) === title);
  if (!found) throw new Error(`no row titled ${title}; rows: ${titles().join(' | ')}`);
  return found;
};
export const detailOf = (title: string) => words(rowOf(title).querySelector(DETAIL));
export const timeOf = (title: string) => words(rowOf(title).querySelector(TIME));
export const dayHeadings = () => screen.queryAllByRole('heading', { level: 2 }).map(words);
export const segments = () => screen.queryAllByRole('radio').map(words);
export const search = () => screen.getByRole('textbox', { name: msg('transfers.historySearchPlaceholder') });
export const clearButton = () => screen.getByRole('button', { name: msg('transfers.historyClearAction') });
export const toastsSaid = () => get(toasts).map((item) => [item.kind, item.message]);

export const installed = (title: string) => msg('transfers.historyInstalledTitle', { title });

/** The stores the screen reads, back as they start; the page empties. */
export function resetScreen(): void {
  history.set([]);
  historyLoaded.set(false);
  historyLoadError.set(null);
  historyStatus.set({ degraded: false, message: '' });
  libraryGames.set([]);
  gameArt.set({});
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
}
