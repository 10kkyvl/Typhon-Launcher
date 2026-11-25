import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../../lib/i18n';
import { cachedUsage, scanUsage } from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { resetHistory, route } from '../../lib/stores/router';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';
import UsageRowSource from './UsageRow.svelte?raw';
import { download, game, group, leftover, report, rowOf, section } from './diskUsageTesting';

// How the page is drawn: each section named by a section heading with its count, its rows a list, a
// path shown whole, a game of the library leading to its page. The services are mocked whole.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
}));

const LONG = 'D:\\Games\\A folder with a long name that a narrow row would cut\\and a second level\\.partial-download-1234567890';

const shown = report({
  games: [game('g-hades', 'Hades'), game('g-celeste', 'Celeste')],
  downloads: [download('d-1', 'Celeste torrent')],
  leftovers: [leftover(LONG, 'install_partial'), leftover('D:\\Games\\.b', 'download_part')],
  screenshots: [group('g-hades', 'Hades', 12), group('g-gone', 'Gone Game', 2, { inLibrary: false })],
  snapshots: [group('g-celeste', 'Celeste', 4)],
});

const SECTIONS: [MessageKey, number][] = [
  ['diskusage.gamesTitle', 2],
  ['diskusage.downloadsTitle', 1],
  ['diskusage.leftoversTitle', 2],
  ['diskusage.screenshotsTitle', 2],
  ['diskusage.snapshotsTitle', 1],
];

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(shown);
  libraryGames.set([{ id: 'g-hades', title: 'Hades' }, { id: 'g-celeste', title: 'Celeste' }] as LibraryGame[]);
  resetHistory();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  resetHistory();
});

async function shownPage() {
  const view = renderDom(DiskUsage);
  await screen.findByText('Celeste torrent');
  return view;
}

describe('DiskUsage: sections and rows', () => {
  it.each(SECTIONS)('%s is a section named by its heading, with the count of its rows, its rows a list', async (title, count) => {
    await shownPage();
    const box = section(title);
    const heading = document.getElementById(box.getAttribute('aria-labelledby') ?? '') as HTMLElement;
    expect(heading.tagName).toBe('H2');
    expect(heading.textContent?.replace(/\s+/g, ' ').trim()).toBe(`${msg(title)} ${count}`);
    expect(heading.getAttribute('tabindex')).toBe('-1');
    const rows = within(box).getAllByRole('listitem');
    expect(rows).toHaveLength(count);
    expect(rows.every((row) => row.classList.contains('row'))).toBe(true);
  });

  it('shows a path whole, with no native hint anywhere on the page', async () => {
    await shownPage();
    expect(within(rowOf('diskusage.leftoversTitle', LONG)).getByText(LONG)).toBeTruthy();
    expect(document.querySelectorAll('[title]')).toHaveLength(0);
    const rule = /\.path\s*\{([^}]*)\}/.exec(UsageRowSource)?.[1] ?? '';
    expect(rule).not.toMatch(/text-overflow|nowrap|overflow:\s*hidden/);
    expect(rule).toMatch(/overflow-wrap:\s*anywhere/);
  });
});

describe('DiskUsage: a game of the library', () => {
  it('leads to its page from a game row and from a group of its files', async () => {
    const { user } = await shownPage();
    await user.click(within(rowOf('diskusage.gamesTitle', 'Celeste')).getByRole('button', { name: 'Celeste' }));
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-celeste' } });
    await user.click(within(rowOf('diskusage.screenshotsTitle', 'Hades')).getByRole('button', { name: 'Hades' }));
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-hades' } });
  });

  it('is plain text, marked as gone, once the library no longer holds it', async () => {
    await shownPage();
    const row = rowOf('diskusage.screenshotsTitle', 'Gone Game');
    expect(within(row).queryByRole('button', { name: 'Gone Game' })).toBeNull();
    expect(within(row).getByText(msg('diskusage.leftLibrary'))).toBeTruthy();
  });
});
