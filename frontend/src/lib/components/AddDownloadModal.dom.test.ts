import { screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', async (actual) => ({
  ...(await actual<typeof import('../services/downloads')>()),
  fetchMetadata: vi.fn(),
  discardMetadata: vi.fn(async () => {}),
  cancelFetchMetadata: vi.fn(async () => {}),
  startDownloadFrom: vi.fn(async () => ({})),
}));
vi.mock('../services/system', async (actual) => ({
  ...(await actual<typeof import('../services/system')>()),
  appInfo: vi.fn(async () => ({ version: '1', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' })),
}));
vi.mock('../stores/settings', async (actual) => {
  const { writable } = await import('svelte/store');
  return {
    ...(await actual<typeof import('../stores/settings')>()),
    settings: writable({ libraryPath: '/games', downloadsPath: '/downloads', autoInstall: false, elevateAhead: false }),
  };
});

import { locale } from '../i18n';
import { fetchMetadata, startDownloadFrom, type TorrentInfo } from '../services/downloads';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import AddDownloadModal from './AddDownloadModal.svelte';

const file = (path: string, size: number) => ({ path, size, selected: true, bytesDone: 0 });
const TORRENT: TorrentInfo = {
  infoHash: 'abc',
  name: 'Celeste',
  totalBytes: 3_000_000,
  files: [file('Celeste/setup.exe', 2_000_000), file('Celeste/readme.txt', 1_000_000)],
};

beforeEach(() => {
  locale.set('en');
  vi.mocked(fetchMetadata).mockResolvedValue(TORRENT);
  vi.mocked(startDownloadFrom).mockClear();
});

afterEach(() => {
  document.body.replaceChildren();
});

async function files() {
  const view = renderDom(AddDownloadModal, { open: true, initialSource: 'magnet:?xt=urn:btih:abc' });
  await screen.findByRole('checkbox', { name: 'Celeste/setup.exe' });
  return view;
}

describe('AddDownloadModal files', () => {
  it('lists every file as a checkbox named by its path and described by its size, all ticked', async () => {
    await files();
    const setup = screen.getByRole('checkbox', { name: 'Celeste/setup.exe' });
    const readme = screen.getByRole('checkbox', { name: 'Celeste/readme.txt' });
    expect([setup, readme].map((box) => box.getAttribute('aria-checked'))).toEqual(['true', 'true']);
    expect(description(setup)).toMatch(/^2(?:[.,]0)?\s?MB$/);
  });

  it('Space unticks a file, Enter does not, and only the ticked files are started', async () => {
    const { user } = await files();
    const readme = screen.getByRole('checkbox', { name: 'Celeste/readme.txt' });
    readme.focus();

    await user.keyboard(' ');
    await user.keyboard('{Enter}');
    expect(readme.getAttribute('aria-checked')).toBe('false');

    await user.click(screen.getByRole('button', { name: 'Start download' }));

    await waitFor(() => expect(startDownloadFrom).toHaveBeenCalledOnce());
    expect(vi.mocked(startDownloadFrom).mock.calls[0].slice(0, 3)).toEqual(['abc', '/downloads', [0]]);
  });

  it('the install options are checkboxes with their descriptions; rights ahead appears with install for an installer', async () => {
    const { user } = await files();
    const install = screen.getByRole('checkbox', { name: 'Install after the download' });
    expect(install.getAttribute('aria-checked')).toBe('false');
    expect(description(install)).toBe('The default comes from settings; here it applies to this download only');
    expect(screen.queryByRole('checkbox', { name: 'Confirm administrator rights now' })).toBeNull();

    await user.click(install);

    expect(install.getAttribute('aria-checked')).toBe('true');
    const rights = await screen.findByRole('checkbox', { name: 'Confirm administrator rights now' });
    await user.click(rights);
    expect(rights.getAttribute('aria-checked')).toBe('true');
  });

  it('names the folder field by its label, and keeps the focus on "start", busy, while the download starts', async () => {
    let started!: () => void;
    vi.mocked(startDownloadFrom).mockReturnValueOnce(new Promise((resolve) => (started = () => resolve({} as never))));
    const { user } = await files();
    expect(screen.getByRole('textbox', { name: 'Destination folder' })).toHaveProperty('value', '/downloads');
    const start = screen.getByRole('button', { name: 'Start download' });
    start.focus();

    await user.keyboard('{Enter}');

    expect(start.getAttribute('aria-busy')).toBe('true');
    expect(start).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(start);
    started();
  });
});
