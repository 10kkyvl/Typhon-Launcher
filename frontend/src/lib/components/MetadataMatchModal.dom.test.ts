import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { applyMetadataMatch, findMetadataCandidates, type MetadataCandidate, type MetadataView } from '../services/metadata';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import MetadataMatchModal from './MetadataMatchModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/metadata')>()),
  findMetadataCandidates: vi.fn(),
  applyMetadataMatch: vi.fn(),
}));

const hades: MetadataCandidate = { providerId: 'igdb:1', title: 'Hades', releaseYear: 2020, thumb: 'https://img.test/hades.jpg', confidence: 0.9 };

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

describe('MetadataMatchModal: the candidates', () => {
  it('says it is looking for them with a labelled spinner and what it matches against', async () => {
    const found = deferred<MetadataCandidate[]>();
    vi.mocked(findMetadataCandidates).mockReturnValueOnce(found.promise);
    render(MetadataMatchModal, { target: app, props: { open: true, gameId: 'g1', gameTitle: 'Hades' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.metadataMatchFindTitle') });

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('modals.metadataMatchSearchingCandidates'));
    expect(within(dialog).getByText(msg('modals.metadataMatchSearchingCandidatesDesc', { gameTitle: 'Hades' }))).toBeTruthy();
    found.resolve([]);
  });

  it('are buttons named once by their text, the cover adding no second name', async () => {
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    render(MetadataMatchModal, { target: app, props: { open: true, gameId: 'g1', gameTitle: 'Hades' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.metadataMatchFindTitle') });

    const row = await within(dialog).findByRole('button', { name: /^Hades \(2020\)/ });
    expect(row.getAttribute('type')).toBe('button');
    expect(row.querySelector('img[alt]:not([alt=""])')).toBeNull();
  });

  it('keep the focus on the pressed candidate while the match is applied', async () => {
    const applied = deferred<MetadataView>();
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    vi.mocked(applyMetadataMatch).mockReturnValueOnce(applied.promise);
    render(MetadataMatchModal, { target: app, props: { open: true, gameId: 'g1', gameTitle: 'Hades' } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.metadataMatchFindTitle') });
    const row = await within(dialog).findByRole('button', { name: /^Hades \(2020\)/ });
    row.focus();

    await keys.keyboard('{Enter}');

    expect(vi.mocked(applyMetadataMatch)).toHaveBeenCalledTimes(1);
    expect(row).toHaveProperty('disabled', false);
    expect(row.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(row);
    await keys.keyboard('{Enter}');
    expect(vi.mocked(applyMetadataMatch)).toHaveBeenCalledTimes(1);
    applied.resolve({ game: { title: 'Hades' } } as MetadataView);
  });
});

describe('MetadataMatchModal: replacing a match', () => {
  it('keeps the focus on "replace", busy, while the new match is applied', async () => {
    const applied = deferred<MetadataView>();
    vi.mocked(findMetadataCandidates).mockResolvedValueOnce([hades]);
    vi.mocked(applyMetadataMatch).mockReturnValueOnce(applied.promise);
    render(MetadataMatchModal, { target: app, props: { open: true, gameId: 'g1', gameTitle: 'Hades', mode: 'change' } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.metadataMatchChangeTitle') });
    await keys.click(await within(dialog).findByRole('button', { name: /^Hades \(2020\)/ }));
    const replace = within(dialog).getByRole('button', { name: msg('modals.metadataMatchReplace') });
    replace.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.metadataMatchApplying') });
    expect(busy).toBe(replace);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    applied.resolve({ game: { title: 'Hades' } } as MetadataView);
  });
});
