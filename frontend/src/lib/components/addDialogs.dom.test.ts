import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { imageTitle, listPlatforms, type PlatformStatus } from '../services/emulators';
import { addEmulatedGame, allTags, type LibraryGame } from '../services/library';
import { listSentData, type SentDataEntry } from '../services/telemetryLog';
import { addSource, testSource, type Source, type SourcePreview } from '../services/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import AddImageModal from './AddImageModal.svelte';
import AddSourceModal from './AddSourceModal.svelte';
import CollectionModal from './CollectionModal.svelte';
import SentDataModal from './SentDataModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  testSource: vi.fn(),
  addSource: vi.fn(),
  listSources: vi.fn(async () => []),
}));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(),
  imageTitle: vi.fn(),
  imageConsoles: vi.fn(async () => []),
}));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  addEmulatedGame: vi.fn(),
  allTags: vi.fn(),
}));
vi.mock('../services/telemetryLog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/telemetryLog')>()),
  listSentData: vi.fn(),
}));

const preview = (patch: Partial<SourcePreview> = {}): SourcePreview => ({
  name: 'FitGirl',
  type: 'url',
  url: 'http://feed.test/feed.json',
  feedVersion: 1,
  entries: 3,
  invalid: 0,
  games: 3,
  known: 2,
  unknown: 1,
  fingerprint: '',
  duplicate: false,
  ...patch,
});

const ps2 = { id: 'ps2', name: 'PlayStation 2', emulator: 'PCSX2', homepage: '', extensions: ['.iso'], imageNames: [], path: '', status: 'ready', reason: '', warnings: [] } as PlatformStatus;

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
  vi.mocked(listPlatforms).mockResolvedValue([ps2]);
  vi.mocked(imageTitle).mockResolvedValue('');
  vi.mocked(allTags).mockResolvedValue([]);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  toasts.set([]);
  vi.clearAllMocks();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe('AddSourceModal', () => {
  async function check(result: () => Promise<SourcePreview>) {
    vi.mocked(testSource).mockImplementationOnce(result);
    render(AddSourceModal, { target: app, props: { open: true } });
    const keys = user();
    const dialog = screen.getByRole('dialog', { name: msg('modals.addSourceTitle') });
    await keys.type(within(dialog).getByRole('textbox', { name: msg('modals.addSourceUrlLabel') }), 'http://feed.test/feed.json');
    await keys.click(within(dialog).getByRole('button', { name: msg('modals.addSourceCheck') }));
    return { dialog, keys };
  }

  it('says a failed check in a danger notice', async () => {
    const { dialog } = await check(() => Promise.reject(new Error('unreachable')));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('states what is wrong with the checked feed in warning notices read with the preview', async () => {
    const { dialog } = await check(async () => preview({ insecure: true, duplicate: true }));

    for (const text of [msg('modals.addSourceInsecure'), msg('modals.addSourceDuplicate')]) {
      const notice = (await within(dialog).findByText(text)).closest('[data-tone="warning"]') as HTMLElement;
      expect(notice).not.toBeNull();
      expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
    }
  });

  it('keeps the focus on "add", busy, while the source is added', async () => {
    const adding = deferred<Source>();
    vi.mocked(addSource).mockReturnValueOnce(adding.promise);
    const { dialog, keys } = await check(async () => preview());
    const add = await within(dialog).findByRole('button', { name: msg('common.add') });
    add.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.addSourceAdding') });
    expect(busy).toBe(add);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    adding.resolve({ id: 's1', name: 'FitGirl' } as Source);
  });
});

describe('AddImageModal', () => {
  async function openDialog() {
    render(AddImageModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('emul.addTitle') });
    const keys = user();
    return { dialog, keys, image: within(dialog).getByRole('textbox', { name: msg('emul.addImageLabel') }) };
  }

  it('names the image field by its label alone, and says by the field that no console takes the file', async () => {
    const { dialog, keys, image } = await openDialog();
    await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());

    await keys.type(image, 'C:/roms/game.xyz');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.closest('label')).toBeNull();
    expect(image.getAttribute('aria-invalid')).toBe('true');
    expect((image.getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
  });

  it('says a refused add in a danger notice, and keeps the focus on "add", busy, while it runs', async () => {
    const adding = deferred<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    const { dialog, keys, image } = await openDialog();
    await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());
    await keys.type(image, 'C:/roms/game.iso');
    await keys.type(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') }), 'Game');
    const add = within(dialog).getByRole('button', { name: msg('common.add') });
    await vi.waitFor(() => expect(add).toHaveProperty('disabled', false));
    add.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('games.installedAddingLabel') });
    expect(busy).toBe(add);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);

    adding.reject(new Error('exists'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('emul.addFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('AddImageModal: while it adds', () => {
  it('cannot be closed, so a refused add is seen in it', async () => {
    const adding = deferred<LibraryGame>();
    vi.mocked(addEmulatedGame).mockReturnValueOnce(adding.promise);
    render(AddImageModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('emul.addTitle') });
    const keys = user();
    await vi.waitFor(() => expect(vi.mocked(listPlatforms)).toHaveBeenCalled());
    await keys.type(within(dialog).getByRole('textbox', { name: msg('emul.addImageLabel') }), 'C:/roms/game.iso');
    await keys.type(within(dialog).getByRole('textbox', { name: msg('games.installedTitleFieldLabel') }), 'Game');
    const add = within(dialog).getByRole('button', { name: msg('common.add') });
    await vi.waitFor(() => expect(add).toHaveProperty('disabled', false));
    await keys.click(add);

    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
    expect(within(dialog).getByRole('button', { name: msg('common.cancel') })).toHaveProperty('disabled', true);

    adding.reject(new Error('exists'));
    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('emul.addFailed'));
  });
});

describe('SentDataModal', () => {
  it('says it is loading with a labelled spinner, a failed load in a danger notice, and a cut entry in a warning notice', async () => {
    const loaded = deferred<SentDataEntry[]>();
    vi.mocked(listSentData).mockReturnValueOnce(loaded.promise);
    const { unmount } = render(SentDataModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.sentDataTitle') });
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('modals.sentDataLoading'));

    loaded.reject(new Error('disk'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.sentDataLoadFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    unmount();

    vi.mocked(listSentData).mockResolvedValueOnce([{ kind: 'diagnostics', sentAt: '2026-11-01T10:00:00Z', endpoint: 'https://t.test', payload: '{', formatted: false } as SentDataEntry]);
    render(SentDataModal, { target: app, props: { open: true } });
    const text = await screen.findByText(msg('modals.sentDataTruncatedWarning'));
    const notice = text.closest('[data-tone="warning"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });
});

describe('CollectionModal', () => {
  it('says there are no genres to choose with an inline empty state, under a group named by its label', async () => {
    render(CollectionModal, { target: app, props: { open: true, genres: [] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });

    const text = within(dialog).getByText(msg('modals.collectionGenresEmpty'));
    expect(text.closest('.empty')).not.toBeNull();
  });

  it('groups the genre chips under their label', async () => {
    render(CollectionModal, { target: app, props: { open: true, genres: ['Roguelike', 'Platformer'] } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.collectionNewTitle') });

    const group = within(dialog).getByRole('group', { name: msg('modals.collectionGenresLabel') });
    expect(within(group).getAllByRole('button')).toHaveLength(2);
  });
});
