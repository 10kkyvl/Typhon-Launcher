import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { getSourceDetails, queryReleases, type ReleasePage, type Source, type SourceDetails } from '../services/sources';
import { sources } from '../stores/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import SourceDetailsModal from './SourceDetailsModal.svelte';

const source = {
  id: 's1',
  name: 'FitGirl',
  type: 'url',
  url: 'https://feed.test/fitgirl.json',
  enabled: true,
  status: 'ready',
  health: 'ok',
  lastUpdatedAt: null,
  lastError: '',
  fingerprint: '',
  feedVersion: 1,
  entries: 3,
} as unknown as Source;

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getSourceDetails: vi.fn(),
  queryReleases: vi.fn(),
  getRelease: vi.fn(async () => null),
}));

type Page = ReleasePage;
const page = (total = 0): Page => ({ items: [], total, page: 1, pageSize: 50 });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const details = { source, total: 3, available: 3, removed: 0, new: 0, ignored: 0 } as unknown as SourceDetails;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  sources.set([source]);
  vi.mocked(getSourceDetails).mockResolvedValue(details);
  vi.mocked(queryReleases).mockResolvedValue(page());
});

afterEach(() => {
  toasts.set([]);
  sources.set([]);
  vi.mocked(getSourceDetails).mockReset();
  vi.mocked(queryReleases).mockReset();
  document.body.replaceChildren();
});

async function openDialog() {
  render(SourceDetailsModal, { target: app, props: { open: true, sourceId: 's1' } });
  return screen.findByRole('dialog', { name: 'FitGirl' });
}

describe('SourceDetailsModal: retrying what failed to load', () => {
  it('keeps the failed details and the focus on their busy Retry until the answer', async () => {
    const again = deferred<SourceDetails>();
    vi.mocked(getSourceDetails).mockRejectedValueOnce(new Error('ipc')).mockReturnValueOnce(again.promise);
    const dialog = await openDialog();
    const keys = user();
    const text = await within(dialog).findByText(msg('modals.sourceDetailsLoadFailed'));
    const notice = text.closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    again.resolve(details);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
  });

  it('keeps the failed release list and the focus on its busy Retry until the answer', async () => {
    const again = deferred<Page>();
    vi.mocked(queryReleases).mockRejectedValueOnce(new Error('ipc')).mockReturnValueOnce(again.promise);
    const dialog = await openDialog();
    const keys = user();
    const text = await within(dialog).findByText(msg('modals.sourceDetailsReleasesFailed'));
    const notice = text.closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    again.resolve(page());
    await within(dialog).findByText(msg('modals.sourceDetailsNoReleases'));
    expect(notice.isConnected).toBe(false);
  });
});

describe('SourceDetailsModal: the rest of the dialog', () => {
  it('says the list is loading with a labelled spinner', async () => {
    const pending = deferred<Page>();
    vi.mocked(queryReleases).mockReturnValueOnce(pending.promise);
    const dialog = await openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.sourceDetailsLoading'));
    pending.resolve(page());
  });

  it('states the source\'s last error in a danger notice read with the dialog', async () => {
    sources.set([{ ...source, lastError: 'typhon:sources.fetch_failed: 503' } as Source]);
    const dialog = await openDialog();

    const notice = dialog.querySelector('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="alert"], [role="status"]')).toBeNull();
  });

  it('pages the list with named icon buttons', async () => {
    vi.mocked(queryReleases).mockResolvedValue(page(120));
    const dialog = await openDialog();
    const keys = user();
    const next = await within(dialog).findByRole('button', { name: msg('ui.next') });
    expect(within(dialog).getByRole('button', { name: msg('ui.previous') })).toBeTruthy();
    expect(next.classList.contains('icon-btn')).toBe(true);

    await keys.click(next);

    expect(vi.mocked(queryReleases)).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }));
  });
});
