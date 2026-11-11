import type { GameHit, ReleaseHit, SearchResult } from '../services/search';
import { errorCode, hasMessage, msg } from '../i18n';

export const debounceMs = 220;

/**
 * What the keys walk through, top to bottom: a row per game, "more in the catalog" when the catalog
 * holds more, then a row per release. A game that is not installed has a second cell, its watch
 * toggle. `key` names the result itself, so the active row survives a refresh of the results.
 */
export type Row =
  | { kind: 'game'; key: string; cells: 1 | 2; hit: GameHit }
  | { kind: 'more'; key: 'more'; cells: 1 }
  | { kind: 'release'; key: string; cells: 1; hit: ReleaseHit };

type Results = Pick<OverlayState, 'games' | 'releases' | 'moreGames'>;

export function rowsOf({ games, releases, moreGames }: Results): Row[] {
  const rows: Row[] = games.map((hit) => ({ kind: 'game', key: `game:${hit.id}`, cells: hit.installed ? 1 : 2, hit }));
  if (games.length > 0 && moreGames > 0) rows.push({ kind: 'more', key: 'more', cells: 1 });
  for (const hit of releases) rows.push({ kind: 'release', key: `release:${hit.sourceId}:${hit.id}`, cells: 1, hit });
  return rows;
}

export interface OverlayState {
  query: string;
  open: boolean;
  loading: boolean;
  error: string;
  searched: boolean;
  games: GameHit[];
  releases: ReleaseHit[];
  moreGames: number;
  moreReleases: number;
  /** Index into `rowsOf(state)` of the row the keys are on, -1 while they are in the text. */
  active: number;
  /** The cell of the active row: 0 the result, 1 its watch toggle. */
  cell: number;
}

export interface OverlayOptions {
  search: (query: string) => Promise<SearchResult>;
  onState: (state: OverlayState) => void;
  delay?: number;
  errorText?: string;
}

const blank = (): OverlayState => ({
  query: '',
  open: false,
  loading: false,
  error: '',
  searched: false,
  games: [],
  releases: [],
  moreGames: 0,
  moreReleases: 0,
  active: -1,
  cell: 0,
});

export function initialState(): OverlayState {
  return blank();
}

export class SearchOverlay {
  private state = blank();
  private timer: ReturnType<typeof setTimeout> | undefined;
  private token = 0;
  private requested = '';
  private readonly search: (query: string) => Promise<SearchResult>;
  private readonly emit: (state: OverlayState) => void;
  private readonly delay: number;
  private readonly errorText: string;

  constructor(options: OverlayOptions) {
    this.search = options.search;
    this.emit = options.onState;
    this.delay = options.delay ?? debounceMs;
    this.errorText = options.errorText ?? msg('search.unavailable');
  }

  get snapshot(): OverlayState {
    return this.state;
  }

  get rows(): Row[] {
    return rowsOf(this.state);
  }

  setQuery(value: string) {
    this.patch({ query: value, open: true });
    this.schedule(value);
  }

  focus() {
    this.patch({ open: true });
    const trimmed = this.state.query.trim();
    if (trimmed !== '' && trimmed !== this.requested && this.timer === undefined) {
      this.schedule(this.state.query);
    }
  }

  close() {
    if (!this.state.open) return;
    this.patch({ open: false, active: -1, cell: 0 });
  }

  /** Up or down a row, wrapping; every row starts at its first cell. */
  move(delta: number) {
    const total = this.rows.length;
    if (!this.state.open || total === 0) return;
    const next = this.state.active < 0 && delta < 0 ? total - 1 : this.state.active + delta;
    this.patch({ active: ((next % total) + total) % total, cell: 0 });
  }

  /**
   * Left or right within the active row, stopping at its ends. False with no row active: the key is
   * then the text's own (the caret).
   */
  moveCell(delta: number): boolean {
    const row = this.activeRow();
    if (!this.state.open || !row) return false;
    const cell = Math.min(Math.max(this.state.cell + delta, 0), row.cells - 1);
    if (cell !== this.state.cell) this.patch({ cell });
    return true;
  }

  /** The pointer's row and cell; one that is not there is ignored. */
  setActive(index: number, cell = 0) {
    const row = this.rows[index];
    if (!this.state.open || !row || cell < 0 || cell >= row.cells) return;
    if (index !== this.state.active || cell !== this.state.cell) this.patch({ active: index, cell });
  }

  /** Back from the rows to the text, the results staying open. */
  leave() {
    if (this.state.active >= 0) this.patch({ active: -1, cell: 0 });
  }

  activeRow(): Row | undefined {
    return this.state.active < 0 ? undefined : this.rows[this.state.active];
  }

  destroy() {
    clearTimeout(this.timer);
    this.timer = undefined;
    this.token++;
  }

  private schedule(value: string) {
    this.token++;
    clearTimeout(this.timer);
    this.timer = undefined;
    const trimmed = value.trim();
    if (trimmed === '') {
      this.token++;
      this.requested = '';
      this.patch({
        loading: false,
        error: '',
        searched: false,
        games: [],
        releases: [],
        moreGames: 0,
        moreReleases: 0,
        active: -1,
        cell: 0,
      });
      return;
    }
    this.patch({ loading: true, error: '' });
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.run(trimmed);
    }, this.delay);
  }

  private async run(query: string) {
    const token = ++this.token;
    this.requested = query;
    try {
      const result = await this.search(query);
      if (token !== this.token) return;
      const held = this.activeRow();
      const fresh = rowsOf(result);
      const active = held ? fresh.findIndex((row) => row.key === held.key) : -1;
      this.patch({
        loading: false,
        error: '',
        searched: true,
        games: result.games,
        releases: result.releases,
        moreGames: result.moreGames,
        moreReleases: result.moreReleases,
        active,
        cell: active >= 0 && this.state.cell < fresh[active].cells ? this.state.cell : 0,
      });
    } catch (err) {
      if (token !== this.token) return;
      const code = errorCode(err);
      this.patch({
        loading: false,
        searched: true,
        error: hasMessage(code) ? msg(code) : this.errorText,
        games: [],
        releases: [],
        moreGames: 0,
        moreReleases: 0,
        active: -1,
        cell: 0,
      });
    }
  }

  private patch(next: Partial<OverlayState>) {
    this.state = { ...this.state, ...next };
    this.emit(this.state);
  }
}
