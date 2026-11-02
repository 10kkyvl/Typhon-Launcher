import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/sources', () => ({ queryCatalogGames: vi.fn() }));

import { CatalogController } from './catalogController';
import type { CatalogPage } from '../services/sources';

const filters = { search: 'old title', genre: 'Action', sort: 'popular' };

function page(pageNumber: number, snapshot: string, revision: number): CatalogPage {
  return { items: [], total: 70, page: pageNumber, pageSize: 24, snapshot, revision };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((yes) => { resolve = yes; });
  return { promise, resolve };
}

describe('CatalogController', () => {
  it('retries the latest search and filters at page one after a failed filter change', async () => {
    const fetchPage = vi.fn()
      .mockResolvedValueOnce(page(3, 'old-snapshot', 8))
      .mockRejectedValueOnce(new Error('typhon:source.offline'))
      .mockResolvedValueOnce(page(1, 'new-snapshot', 9));
    const controller = new CatalogController(fetchPage);
    await controller.load(filters, 3);

    const changedFilters = { search: 'new title', genre: '', sort: 'rating' };
    await expect(controller.load(changedFilters, 1)).resolves.toMatchObject({ kind: 'error', catalogChanged: false });
    await expect(controller.retry()).resolves.toMatchObject({ kind: 'page', firstPage: true });

    expect(fetchPage).toHaveBeenNthCalledWith(2,
      { ...changedFilters, page: 1, pageSize: 24, snapshot: '', revision: 0 },
      expect.any(AbortSignal),
    );
    expect(fetchPage).toHaveBeenNthCalledWith(3,
      { ...changedFilters, page: 1, pageSize: 24, snapshot: '', revision: 0 },
      expect.any(AbortSignal),
    );
  });

  it('discards a changed catalog snapshot and retries from page one', async () => {
    const fetchPage = vi.fn()
      .mockResolvedValueOnce(page(1, 'snapshot-a', 2))
      .mockResolvedValueOnce(page(2, 'snapshot-a', 2))
      .mockRejectedValueOnce(new Error('typhon:catalog.changed'))
      .mockResolvedValueOnce(page(1, 'snapshot-b', 3));
    const controller = new CatalogController(fetchPage);
    await controller.load(filters, 1);
    await controller.load(filters, 2);
    await expect(controller.load(filters, 3)).resolves.toMatchObject({ kind: 'error', catalogChanged: true });
    await controller.retry();

    expect(fetchPage).toHaveBeenNthCalledWith(2,
      { ...filters, page: 2, pageSize: 24, snapshot: 'snapshot-a', revision: 2 },
      expect.any(AbortSignal),
    );
    expect(fetchPage).toHaveBeenNthCalledWith(4,
      { ...filters, page: 1, pageSize: 24, snapshot: '', revision: 0 },
      expect.any(AbortSignal),
    );
  });

  it('keeps the latest response and its snapshot when earlier catalog requests finish late', async () => {
    const oldRequest = deferred<CatalogPage>();
    const currentRequest = deferred<CatalogPage>();
    const fetchPage = vi.fn().mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(currentRequest.promise).mockResolvedValueOnce(page(2, 'current-snapshot', 5));
    const controller = new CatalogController(fetchPage);
    const oldLoad = controller.load(filters, 1);
    const currentFilters = { ...filters, search: 'current title' };
    const currentLoad = controller.load(currentFilters, 1);

    currentRequest.resolve(page(1, 'current-snapshot', 5));
    await expect(currentLoad).resolves.toMatchObject({ kind: 'page' });
    oldRequest.resolve(page(1, 'old-snapshot', 2));
    await expect(oldLoad).resolves.toEqual({ kind: 'stale' });
    await controller.load(currentFilters, 2);

    expect(fetchPage).toHaveBeenLastCalledWith(
      { ...currentFilters, page: 2, pageSize: 24, snapshot: 'current-snapshot', revision: 5 },
      expect.any(AbortSignal),
    );
  });

  it('aborts the previous in-flight request when a new one starts', async () => {
    const first = deferred<CatalogPage>();
    const fetchPage = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValueOnce(page(2, 'snapshot', 1));
    const controller = new CatalogController(fetchPage);

    void controller.load(filters, 1);
    const [, firstSignal] = fetchPage.mock.calls[0] as [unknown, AbortSignal];
    expect(firstSignal.aborted).toBe(false);

    await controller.load(filters, 2);

    expect(firstSignal.aborted).toBe(true);
    first.resolve(page(1, 'irrelevant', 1));
  });

  it('does not surface a cancelled request as a visible error', async () => {
    const { CancelError } = await import('@wailsio/runtime');
    const fetchPage = vi.fn((_query, signal?: AbortSignal) => new Promise<CatalogPage>((_resolve, reject) => {
      signal?.addEventListener('abort', () => reject(new CancelError('Promise cancelled.')));
    }));
    const controller = new CatalogController(fetchPage);

    const stalePage = controller.load(filters, 1);
    const [, firstSignal] = fetchPage.mock.calls[0] as [unknown, AbortSignal];
    void controller.load(filters, 2);

    expect(firstSignal.aborted).toBe(true);
    await expect(stalePage).resolves.toEqual({ kind: 'stale' });
  });
});
