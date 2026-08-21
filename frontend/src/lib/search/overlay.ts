import type { GameHit, ReleaseHit, SearchResult } from '../services/search';

export const debounceMs = 220;

export type ActiveHit =
  | { kind: 'game'; hit: GameHit }
  | { kind: 'release'; hit: ReleaseHit };

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
  active: number;
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
    this.errorText = options.errorText ?? 'Поиск недоступен';
  }

  get snapshot(): OverlayState {
    return this.state;
  }

  get total(): number {
    return this.state.games.length + this.state.releases.length;
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
    this.patch({ open: false, active: -1 });
  }

  move(delta: number) {
    const total = this.total;
    if (!this.state.open || total === 0) return;
    const next = this.state.active < 0 && delta < 0 ? total - 1 : this.state.active + delta;
    this.patch({ active: ((next % total) + total) % total });
  }

  activeHit(): ActiveHit | undefined {
    const { active, games, releases } = this.state;
    if (active < 0) return undefined;
    if (active < games.length) return { kind: 'game', hit: games[active] };
    const release = releases[active - games.length];
    return release ? { kind: 'release', hit: release } : undefined;
  }

  destroy() {
    clearTimeout(this.timer);
    this.timer = undefined;
    this.token++;
  }

  private schedule(value: string) {
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
      this.patch({
        loading: false,
        error: '',
        searched: true,
        games: result.games,
        releases: result.releases,
        moreGames: result.moreGames,
        moreReleases: result.moreReleases,
        active: -1,
      });
    } catch (err) {
      if (token !== this.token) return;
      this.patch({
        loading: false,
        searched: true,
        error: err instanceof Error && err.message ? err.message : this.errorText,
        games: [],
        releases: [],
        moreGames: 0,
        moreReleases: 0,
        active: -1,
      });
    }
  }

  private patch(next: Partial<OverlayState>) {
    this.state = { ...this.state, ...next };
    this.emit(this.state);
  }
}
