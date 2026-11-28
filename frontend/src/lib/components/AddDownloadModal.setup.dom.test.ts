import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', async (actual) => ({
  ...(await actual<typeof import('../services/downloads')>()),
  fetchMetadata: vi.fn(),
  discardMetadata: vi.fn(async () => {}),
  cancelFetchMetadata: vi.fn(async () => {}),
  startDownloadFrom: vi.fn(),
}));
vi.mock('../services/system', async (actual) => ({
  ...(await actual<typeof import('../services/system')>()),
  appInfo: vi.fn(async () => ({ version: '1', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' })),
  getStorageInfoFor: vi.fn(async () => ({ volume: 'D', filesystem: 'NTFS', freeBytes: 1_000_000_000, totalBytes: 2_000_000_000 })),
}));
// The settings store is the real one: the library it makes is what turns the add dialog on.
vi.mock('../services/settings', async (actual) => ({
  ...(await actual<typeof import('../services/settings')>()),
  selectFolder: vi.fn(),
  proposeLibraryPath: vi.fn(),
  setupLibrary: vi.fn(),
}));

import { locale, msg } from '../i18n';
import { fetchMetadata, startDownloadFrom, type TorrentInfo } from '../services/downloads';
import { proposeLibraryPath, selectFolder, setupLibrary, type Settings } from '../services/settings';
import { settings } from '../stores/settings';
import { renderDom } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import AddDownloadModal from './AddDownloadModal.svelte';

const MAGNET = 'magnet:?xt=urn:btih:abc';
const TORRENT: TorrentInfo = {
  infoHash: 'abc',
  name: 'Celeste',
  totalBytes: 1,
  files: [{ path: 'Celeste/setup.exe', size: 1, selected: true, bytesDone: 0 }],
};
const BEFORE = { language: 'en', uiScale: 1, animationsEnabled: true, accentColor: '', libraryPath: '', downloadsPath: '', autoInstall: false, elevateAhead: false };
const MADE = { ...BEFORE, libraryPath: 'D:/Games/Typhon Library', downloadsPath: 'D:/Games/Typhon Library/Downloads' };

beforeEach(() => {
  locale.set('en');
  settings.set(BEFORE as unknown as Settings);
  vi.mocked(fetchMetadata).mockResolvedValue(TORRENT);
  vi.mocked(selectFolder).mockResolvedValue('D:/Games');
  vi.mocked(proposeLibraryPath).mockResolvedValue('D:/Games/Typhon Library');
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  settings.set(null);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

describe('AddDownloadModal with no library yet', () => {
  it('sets the library up first, then goes on with the same link into the new folder, once', async () => {
    const making = held<Settings>();
    vi.mocked(setupLibrary).mockReturnValueOnce(making.promise);
    const { user } = renderDom(AddDownloadModal, { open: true, initialSource: MAGNET });
    const setup = screen.getByRole('dialog', { name: msg('modals.librarySetupDefaultTitle') });
    expect(fetchMetadata).not.toHaveBeenCalled();

    await user.click(within(setup).getByRole('button', { name: msg('modals.librarySetupBrowse') }));
    const create = within(setup).getByRole('button', { name: msg('modals.librarySetupCreate') });
    await waitFor(() => expect(create.getAttribute('aria-disabled')).toBeNull());
    await user.click(create);
    await user.click(create);
    making.resolve(MADE as unknown as Settings);

    await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
    expect(setup.isConnected).toBe(false);
    expect(screen.getAllByRole('dialog')).toEqual([screen.getByRole('dialog', { name: msg('modals.addDownloadTitle') })]);
    const folder = screen.getByRole('group', { name: msg('modals.addDownloadDestinationLabel') });
    expect(within(folder).getByText(MADE.downloadsPath)).toBeTruthy();
    expect(vi.mocked(setupLibrary).mock.calls).toEqual([['D:/Games']]);
    expect(vi.mocked(fetchMetadata).mock.calls).toEqual([[MAGNET]]);
    expect(startDownloadFrom).not.toHaveBeenCalled();
  });
});
