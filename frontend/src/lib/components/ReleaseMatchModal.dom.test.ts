import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { confirmMatch, getMatchCandidates, searchGames, type CatalogGame, type MatchCandidate, type ReleaseView } from '../services/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import ReleaseMatchModal from './ReleaseMatchModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getMatchCandidates: vi.fn(),
  searchGames: vi.fn(),
  confirmMatch: vi.fn(),
  ignoreRelease: vi.fn(),
}));

const release = {
  release: { id: 'r1', rawTitle: 'Hades v1.0', normalizedTitle: 'hades', size: 1024, uploadedAt: '2026-11-01T10:00:00Z' },
  sourceName: 'FitGirl',
} as unknown as ReleaseView;

const candidate = (gameId: string, title: string, patch: Partial<MatchCandidate> = {}): MatchCandidate => ({ gameId, title, score: 0.9, method: 'title', source: 'local', ...patch });
const game = (id: string, title: string): CatalogGame => ({ id, title, sortTitle: title }) as CatalogGame;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  toasts.set([]);
  vi.mocked(getMatchCandidates).mockReset();
  vi.mocked(searchGames).mockReset();
  vi.mocked(confirmMatch).mockReset();
  document.body.replaceChildren();
});

function openDialog() {
  render(ReleaseMatchModal, { target: app, props: { open: true, release } });
  return screen.getByRole('dialog', { name: msg('modals.releaseMatchTitle') });
}

describe('ReleaseMatchModal: the candidates', () => {
  it('are a radio group named by their heading: one Tab stop; the arrows choose, and reach one not in the catalog without choosing it', async () => {
    vi.mocked(getMatchCandidates).mockResolvedValue([
      candidate('a', 'Hades'),
      candidate('b', 'Hades II', { inCatalog: false }),
      candidate('c', 'Hades Demo'),
    ]);
    const dialog = openDialog();
    const keys = user();
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.releaseMatchCandidates') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false']);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([0, -1, -1]);
    expect(radios[1].getAttribute('aria-disabled')).toBe('true');

    radios[0].focus();
    await keys.keyboard('{ArrowDown}');

    expect(document.activeElement).toBe(radios[1]);
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false']);

    await keys.keyboard('{ArrowDown}');

    expect(radios[2].getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(radios[2]);
    expect(radios[2].tabIndex).toBe(0);
  });

  it('say they are being looked up with a labelled spinner, and that there are none with an inline empty state', async () => {
    const found = deferred<MatchCandidate[]>();
    vi.mocked(getMatchCandidates).mockReturnValueOnce(found.promise);
    const dialog = openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.releaseMatchSearchingCandidates'));

    found.resolve([]);
    const empty = await within(dialog).findByText(msg('modals.releaseMatchNoCandidates'));
    expect(empty.closest('.empty')).not.toBeNull();
  });
});

describe('ReleaseMatchModal: the manual search', () => {
  it('is a search field named by its heading whose failure stays, its Retry busy and focused, until the answer', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const again = deferred<CatalogGame[]>();
    vi.mocked(getMatchCandidates).mockResolvedValue([]);
    vi.mocked(searchGames).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const dialog = openDialog();
    const keys = user();
    const field = within(dialog).getByRole('textbox', { name: msg('modals.releaseMatchManualSearch') });

    await keys.type(field, 'hades');
    await vi.advanceTimersByTimeAsync(300);
    const alert = await within(dialog).findByRole('alert');
    const notice = alert.closest('[data-tone="danger"]') as HTMLElement;
    expect(notice).not.toBeNull();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');
    await vi.advanceTimersByTimeAsync(300);

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve([game('g1', 'Hades')]);
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.releaseMatchManualSearch') });
    expect(within(group).getByRole('radio', { name: /Hades/ })).toBeTruthy();
    expect(notice.isConnected).toBe(false);
  });
});

describe('ReleaseMatchModal: confirming', () => {
  it('keeps the focus on "confirm", busy, while the match is saved', async () => {
    const saved = deferred<void>();
    vi.mocked(getMatchCandidates).mockResolvedValue([candidate('a', 'Hades')]);
    vi.mocked(confirmMatch).mockReturnValueOnce(saved.promise);
    const dialog = openDialog();
    const keys = user();
    await keys.click(await within(dialog).findByRole('radio', { name: /Hades/ }));
    const confirm = within(dialog).getByRole('button', { name: msg('modals.releaseMatchConfirm') });
    confirm.focus();

    await keys.keyboard('{Enter}');

    expect(confirm.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(confirm);
    expect(within(dialog).getByRole('button', { name: msg('modals.releaseMatchIgnore') })).toHaveProperty('disabled', true);
    saved.resolve();
  });
});
