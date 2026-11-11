import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { removeSource, type Source } from '../services/sources';
import { sourceErrorText } from '../sources/sourceErrors';
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
  getSourceDetails: vi.fn(async () => ({ source, total: 3, available: 3, removed: 0, new: 0, ignored: 0 })),
  queryReleases: vi.fn(async () => ({ items: [], total: 0 })),
  getRelease: vi.fn(async () => null),
  removeSource: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  sources.set([source]);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  toasts.set([]);
  sources.set([]);
  app.remove();
  vi.restoreAllMocks();
});

async function askToRemove() {
  render(SourceDetailsModal, { target: app, props: { open: true, sourceId: 's1' } });
  const keys = user();
  const details = await screen.findByRole('dialog', { name: 'FitGirl' });
  await keys.click(await within(details).findByRole('button', { name: msg('common.delete') }));
  const confirm = screen.getByRole('dialog', { name: msg('transfers.sourcesRemoveAction') });
  return { keys, details, confirm };
}

describe('removing a source from its details', () => {
  it('a refused removal keeps both dialogs open and says why in the confirmation, without a toast', async () => {
    const refusal = new Error('typhon:sources.source_busy: busy');
    vi.mocked(removeSource).mockRejectedValueOnce(refusal);
    const { keys, details, confirm } = await askToRemove();

    await keys.click(within(confirm).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }));

    expect((await within(confirm).findByRole('alert')).textContent?.trim()).toBe(sourceErrorText(refusal));
    expect(details.isConnected).toBe(true);
    expect(get(toasts)).toEqual([]);
  });

  it('a removal that went through closes the confirmation and the details', async () => {
    vi.mocked(removeSource).mockResolvedValueOnce(undefined);
    const { keys, confirm } = await askToRemove();

    await keys.click(within(confirm).getByRole('button', { name: msg('transfers.sourcesRemoveAction') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});
