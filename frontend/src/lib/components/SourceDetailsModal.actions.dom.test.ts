import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import {
  getRelease,
  getSourceDetails,
  listSources,
  queryReleases,
  refreshSource,
  removeSource,
  setSourceEnabled,
  type RefreshSummary,
  type ReleaseView,
  type Source,
  type SourceDetails,
} from '../services/sources';
import { sources } from '../stores/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import SourceDetailsModal from './SourceDetailsModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getSourceDetails: vi.fn(),
  queryReleases: vi.fn(),
  getRelease: vi.fn(),
  getMatchCandidates: vi.fn(),
  listSources: vi.fn(),
  refreshSource: vi.fn(),
  setSourceEnabled: vi.fn(),
  removeSource: vi.fn(),
}));

const LONG_URL = `https://mirror.example.test/${'a-very-long-path-segment/'.repeat(8)}feed.json`;
const LONG_TITLE = `Hades ${'Deluxe Collector Edition '.repeat(6)}v1.0.0`;

const source = (id: string, name: string, patch: Partial<Source> = {}) =>
  ({ id, name, type: 'url', url: LONG_URL, enabled: true, status: 'active', health: 'healthy', lastUpdatedAt: null, lastError: '', fingerprint: '', feedVersion: 1, entries: 3, ...patch }) as unknown as Source;
const fitgirl = source('s1', 'FitGirl');
const dodi = source('s2', 'DODI', { url: 'https://feed.test/dodi.json' });
const details = (of: Source) => ({ source: of, total: 3, available: 3, removed: 0, new: 0, ignored: 0 }) as unknown as SourceDetails;
const releaseRow = {
  release: { id: 'r1', rawTitle: LONG_TITLE, normalizedTitle: 'hades', size: 1024, uploadedAt: '2026-11-01T10:00:00Z', matchStatus: 'unmatched', matchConfidence: 0, availability: 'available', new: false },
  sourceName: 'FitGirl',
  gameTitle: '',
} as unknown as ReleaseView;
const summary = { entries: 3, added: 0, review: 0 } as unknown as RefreshSummary;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  sources.set([fitgirl, dodi]);
  vi.mocked(getSourceDetails).mockImplementation(async (id: string) => details(id === 's1' ? fitgirl : dodi));
  vi.mocked(queryReleases).mockResolvedValue({ items: [releaseRow], total: 1, page: 1, pageSize: 50 });
  vi.mocked(getRelease).mockResolvedValue(null);
  vi.mocked(listSources).mockImplementation(async () => get(sources));
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  sources.set([]);
  vi.useRealTimers();
  vi.resetAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

async function openDialog(sourceId = 's1') {
  const result = render(SourceDetailsModal, { target: app, props: { open: true, sourceId } });
  const dialog = await screen.findByRole('dialog', { name: sourceId === 's1' ? 'FitGirl' : 'DODI' });
  return { ...result, dialog, keys: user() };
}

const button = (dialog: HTMLElement, name: string) => within(dialog).getByRole('button', { name });
const describedBy = (element: Element) =>
  (element.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());
const heldWithReason = (control: HTMLElement) => {
  expect(control.getAttribute('aria-disabled')).toBe('true');
  expect(control.hasAttribute('disabled')).toBe(false);
  expect(describedBy(control)).toContain(msg('modals.actionRunning'));
};

describe('SourceDetailsModal: opening', () => {
  it('never puts the focus on Delete, and shows a long URL and a long release title whole, with no native title', async () => {
    const { dialog } = await openDialog();
    await within(dialog).findByText(LONG_TITLE);

    expect(document.activeElement).not.toBe(button(dialog, msg('common.delete')));
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(within(dialog).getByText(LONG_URL)).toBeTruthy();
    // The pager's icon buttons at an end are natively disabled, and the kit's IconButton then names
    // them by a native title (it takes no focus, so no tooltip): the kit's rule, not this dialog's text.
    expect([...dialog.querySelectorAll('[title]')].filter((element) => !element.classList.contains('icon-btn'))).toEqual([]);
  });

  it('says the source details are loading with a labelled spinner, then shows the counters', async () => {
    const pending = held<SourceDetails>();
    vi.mocked(getSourceDetails).mockReturnValueOnce(pending.promise);
    const { dialog } = await openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.sourceDetailsLoadingDetails'));

    pending.resolve(details(fitgirl));
    expect(await within(dialog).findByText(msg('modals.sourceDetailsCounterTotal'))).toBeTruthy();
    expect(within(dialog).queryByText(msg('modals.sourceDetailsLoadingDetails'))).toBeNull();
  });
});

describe('SourceDetailsModal: refresh, enable and remove', () => {
  it('refreshes once for two presses, keeps the focus on Refresh, and holds the other actions with a reason', async () => {
    const refreshing = held<RefreshSummary>();
    vi.mocked(refreshSource).mockReturnValueOnce(refreshing.promise);
    const { dialog, keys } = await openDialog();
    const refresh = button(dialog, msg('common.refresh'));
    refresh.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(refreshSource).mock.calls).toEqual([['s1']]);
    expect(refresh.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(refresh);
    heldWithReason(button(dialog, msg('modals.sourceDetailsDisable')));
    heldWithReason(button(dialog, msg('common.delete')));
    await keys.click(button(dialog, msg('modals.sourceDetailsDisable')));
    await keys.click(button(dialog, msg('common.delete')));
    expect(vi.mocked(setSourceEnabled)).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog', { name: msg('transfers.sourcesRemoveAction') })).toBeNull();

    refreshing.resolve(summary);
    await vi.waitFor(() => expect(refresh.getAttribute('aria-busy')).toBeNull());
    expect(button(dialog, msg('common.delete')).getAttribute('aria-disabled')).toBeNull();
  });

  it('turns the source off once for two presses, the button busy and focused, the others held', async () => {
    const switching = held<void>();
    vi.mocked(setSourceEnabled).mockReturnValueOnce(switching.promise);
    const { dialog, keys } = await openDialog();
    const toggle = button(dialog, msg('modals.sourceDetailsDisable'));
    toggle.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(setSourceEnabled).mock.calls).toEqual([['s1', false]]);
    expect(toggle.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(toggle);
    heldWithReason(button(dialog, msg('common.refresh')));
    heldWithReason(button(dialog, msg('common.delete')));

    sources.set([{ ...fitgirl, enabled: false } as Source, dodi]);
    switching.resolve();
    await vi.waitFor(() => expect(toggle.getAttribute('aria-busy')).toBeNull());
    expect(toggle.textContent?.trim()).toBe(msg('modals.sourceDetailsEnable'));
    expect(document.activeElement).toBe(toggle);
  });

  it('asks before removing, says plainly what goes and what stays, with Cancel as the default, and removes once', async () => {
    const removing = held<void>();
    vi.mocked(removeSource).mockReturnValueOnce(removing.promise);
    const { dialog, keys } = await openDialog();

    await keys.click(button(dialog, msg('common.delete')));

    const confirm = screen.getByRole('dialog', { name: msg('transfers.sourcesRemoveAction') });
    expect(confirm.textContent).toContain(msg('modals.sourceDetailsRemoveText', { name: 'FitGirl' }));
    expect(document.activeElement).toBe(within(confirm).getByRole('button', { name: msg('common.cancel') }));
    const remove = within(confirm).getByRole('button', { name: msg('transfers.sourcesRemoveAction') });
    remove.focus();
    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    expect(vi.mocked(removeSource).mock.calls).toEqual([['s1']]);
    removing.resolve();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});

describe('SourceDetailsModal: an action answering after the dialog was opened for another source', () => {
  it('leaves the new source’s Refresh ready and reads nothing for it again', async () => {
    const refreshing = held<RefreshSummary>();
    vi.mocked(refreshSource).mockReturnValueOnce(refreshing.promise);
    const { dialog, keys, rerender } = await openDialog();
    await keys.click(button(dialog, msg('common.refresh')));
    await rerender({ open: false, sourceId: 's1' });
    await rerender({ open: true, sourceId: 's2' });
    const next = await screen.findByRole('dialog', { name: 'DODI' });
    await within(next).findByText(LONG_TITLE);

    expect(button(next, msg('common.refresh')).getAttribute('aria-busy')).toBeNull();
    expect(button(next, msg('common.delete')).getAttribute('aria-disabled')).toBeNull();
    const reads = vi.mocked(getSourceDetails).mock.calls.length;

    refreshing.resolve(summary);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(vi.mocked(getSourceDetails).mock.calls.length).toBe(reads);
    expect(button(next, msg('common.refresh')).getAttribute('aria-busy')).toBeNull();
  });
});
