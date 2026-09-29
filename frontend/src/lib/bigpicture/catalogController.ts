import { errorCode } from '../i18n';
import { queryCatalogGames, type CatalogPage } from '../services/sources';
import { LatestRequestGate } from './latestRequest';

export interface CatalogFilters {
  search: string;
  genre: string;
  sort: string;
}

export type CatalogLoadResult =
  | { kind: 'page'; page: CatalogPage; firstPage: boolean }
  | { kind: 'error'; error: unknown; catalogChanged: boolean }
  | { kind: 'stale' };

type CatalogFetcher = (query: {
  search: string;
  genre: string;
  sort: string;
  page: number;
  pageSize: number;
  snapshot: string;
  revision: number;
}, signal?: AbortSignal) => Promise<CatalogPage>;

/** Keeps the active catalog snapshot and exact retry request together. */
export class CatalogController {
  private readonly requests = new LatestRequestGate();
  private readonly fetchPage: CatalogFetcher;
  private filters: CatalogFilters = { search: '', genre: '', sort: 'popular' };
  private filterKey = '';
  private pageSize = 24;
  private snapshot = '';
  private revision = 0;
  private retryPage = 1;

  constructor(fetchPage: CatalogFetcher = queryCatalogGames) {
    this.fetchPage = fetchPage;
  }

  load(filters: CatalogFilters, page: number, pageSize = this.pageSize): Promise<CatalogLoadResult> {
    const key = JSON.stringify(filters);
    if (key !== this.filterKey) {
      this.filterKey = key;
      this.snapshot = '';
      this.revision = 0;
    }
    this.filters = { ...filters };
    this.pageSize = pageSize;
    this.retryPage = page;
    const firstPage = page === 1;
    const { ticket, signal } = this.requests.beginRequest();
    return this.finishRequest(ticket, firstPage, this.fetchPage({
      ...filters,
      page,
      pageSize,
      snapshot: firstPage ? '' : this.snapshot,
      revision: firstPage ? 0 : this.revision,
    }, signal));
  }

  retry(): Promise<CatalogLoadResult> {
    return this.load(this.filters, this.retryPage, this.pageSize);
  }

  invalidate(): void {
    this.requests.invalidate();
  }

  private async finishRequest(ticket: number, firstPage: boolean, request: Promise<CatalogPage>): Promise<CatalogLoadResult> {
    const response = await this.requests.settle(ticket, request);
    if (response.kind === 'stale') return { kind: 'stale' };
    if (response.kind === 'error') {
      const catalogChanged = errorCode(response.error) === 'catalog.changed';
      if (catalogChanged) {
        this.snapshot = '';
        this.revision = 0;
        this.retryPage = 1;
      }
      return { kind: 'error', error: response.error, catalogChanged };
    }
    this.snapshot = response.value.snapshot ?? '';
    this.revision = response.value.revision ?? 0;
    this.retryPage = response.value.page || this.retryPage;
    return { kind: 'page', page: response.value, firstPage };
  }
}
