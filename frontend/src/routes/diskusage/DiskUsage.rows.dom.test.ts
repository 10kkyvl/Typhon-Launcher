import { cleanup, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, scanUsage } from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import DiskUsage from './DiskUsage.svelte';
import { GB, download, game, group, leftover, report, rowOf, section, sizeOf } from './diskUsageTesting';

// What each section's rows say: the size exact, as a lower bound or unknown, the section's total, the
// order of the rows, the reason a row cannot go, the gates of a game's Move and Remove. The services
// are mocked whole: nothing is measured or deleted.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
  deleteDownloadData: vi.fn(),
  deleteLeftover: vi.fn(),
  deleteScreenshots: vi.fn(),
  deleteSnapshots: vi.fn(),
}));

const atLeast = (bytes: number) => `≥ ${bytesSize(bytes)}`;

function description(el: HTMLElement): string {
  return (el.getAttribute('aria-describedby') ?? '')
    .split(/\s+/)
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim() ?? '')
    .join(' ');
}

const full = report({
  games: [
    game('g-big', 'Big', { sizeBytes: 9 * GB }),
    game('g-low', 'Lower', { sizeBytes: 3 * GB, sizeUnknown: true }),
    game('g-none', 'Unmeasured', { sizeBytes: 0, sizeUnknown: true }),
    game('g-run', 'Running', { sizeBytes: 2 * GB, running: true }),
    game('g-busy', 'Busy', { sizeBytes: 2 * GB, busy: true }),
    game('g-ext', 'Outside', { sizeBytes: GB, external: true }),
  ],
  downloads: [
    download('d-free', 'Spare', { sizeBytes: 4 * GB }),
    download('d-ref', 'Needed', { sizeBytes: 5 * GB, referenced: true, reason: 'текст из Go', reasonCode: 'diskusage.reason.owned_by_install' }),
  ],
  leftovers: [
    leftover('D:\\Games\\.part', 'install_partial', { sizeBytes: 2 * GB }),
    leftover('D:\\Games\\.kept', 'update_previous', { sizeBytes: GB, deletable: false, reason: 'текст из Go', reasonCode: 'diskusage.reason.update_owned' }),
  ],
  screenshots: [group('g-big', 'Big', 12, { sizeBytes: 0, sizeUnknown: true })],
  snapshots: [group('g-low', 'Lower', 4, { sizeBytes: 2 * GB, sizeUnknown: true })],
  totals: { gamesBytes: 17 * GB, downloadsBytes: 9 * GB, leftoversBytes: 3 * GB, screenshotsBytes: 0, snapshotsBytes: 2 * GB, totalBytes: 31 * GB, reclaimableBytes: 6 * GB, unknownSizes: true },
});

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(full);
  libraryGames.set(full.games.map((g) => ({ id: g.id, title: g.title })) as LibraryGame[]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
});

async function shown() {
  const view = renderDom(DiskUsage);
  await screen.findByText('Unmeasured');
  return view;
}

describe('DiskUsage: sizes', () => {
  it('says a row’s size exact, as a lower bound, or unknown, never 0 for what was not measured', async () => {
    await shown();
    expect(sizeOf(rowOf('diskusage.gamesTitle', 'Big'))).toBe(bytesSize(9 * GB));
    expect(sizeOf(rowOf('diskusage.gamesTitle', 'Lower'))).toBe(atLeast(3 * GB));
    expect(sizeOf(rowOf('diskusage.gamesTitle', 'Unmeasured'))).toBe(msg('diskusage.sizeUnknown'));
    expect(sizeOf(rowOf('diskusage.screenshotsTitle', 'Big'))).toBe(msg('diskusage.sizeUnknown'));
    expect(sizeOf(rowOf('diskusage.snapshotsTitle', 'Lower'))).toBe(atLeast(2 * GB));
  });

  it('says a section’s total as "at least" while a size under it is not measured', async () => {
    await shown();
    expect(within(section('diskusage.gamesTitle')).getByText(msg('diskusage.atLeast', { size: bytesSize(17 * GB) }))).toBeTruthy();
    expect(within(section('diskusage.downloadsTitle')).getByText(bytesSize(9 * GB))).toBeTruthy();
    expect(within(section('diskusage.snapshotsTitle')).getByText(msg('diskusage.atLeast', { size: bytesSize(2 * GB) }))).toBeTruthy();
  });

  it('notes that the totals are incomplete, with how many sizes are not known', async () => {
    await shown();
    expect(screen.getByText(msg('diskusage.unknownNote', { count: 4 }))).toBeTruthy();
  });
});

describe('DiskUsage: order', () => {
  it('lists each section biggest first, unknown sizes last', async () => {
    await shown();
    const titles = (key: Parameters<typeof section>[0]) => [...section(key).querySelectorAll('.row .title')].map((el) => el.textContent?.trim());
    expect(titles('diskusage.gamesTitle')).toEqual(['Big', 'Busy', 'Running', 'Outside', 'Lower', 'Unmeasured']);
    expect(titles('diskusage.downloadsTitle')).toEqual(['Needed', 'Spare']);
  });
});

describe('DiskUsage: why a row cannot go', () => {
  it('says the reason of a download the games still need and of a leftover Go keeps, in catalogued words', async () => {
    await shown();
    expect(within(rowOf('diskusage.downloadsTitle', 'Needed')).getByText(msg('diskusage.reason.owned_by_install'))).toBeTruthy();
    expect(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\.kept')).getByText(msg('diskusage.reason.update_owned'))).toBeTruthy();
    expect(section('diskusage.downloadsTitle').textContent).not.toContain('текст из Go');
  });

  it('offers Delete on a row Go lets go', async () => {
    await shown();
    expect(within(rowOf('diskusage.downloadsTitle', 'Spare')).getByRole('button', { name: msg('diskusage.downloadDelete') }).matches(':disabled')).toBe(false);
    expect(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\.part')).getByRole('button', { name: msg('diskusage.leftoverDelete') }).matches(':disabled')).toBe(false);
  });
});

describe('DiskUsage: Move and Remove of a game', () => {
  it('are offered on a game the launcher manages', async () => {
    await shown();
    const row = rowOf('diskusage.gamesTitle', 'Big');
    for (const name of [msg('diskusage.move'), msg('diskusage.remove')]) {
      const button = within(row).getByRole('button', { name });
      expect(button.matches(':disabled') || button.getAttribute('aria-disabled') === 'true', name).toBe(false);
    }
  });

  it.each([
    ['Running', 'diskusage.blockedRunning'],
    ['Busy', 'diskusage.blockedBusy'],
  ] as const)('are off on %s, saying why', async (title, reason) => {
    await shown();
    const row = rowOf('diskusage.gamesTitle', title);
    for (const name of [msg('diskusage.move'), msg('diskusage.remove')]) {
      const button = within(row).getByRole('button', { name });
      expect(button.getAttribute('aria-disabled'), name).toBe('true');
      expect(description(button), name).toContain(msg(reason));
    }
  });

  it('are not offered on a game another launcher installed', async () => {
    await shown();
    const row = rowOf('diskusage.gamesTitle', 'Outside');
    expect(within(row).queryByRole('button', { name: msg('diskusage.move') })).toBeNull();
    expect(within(row).queryByRole('button', { name: msg('diskusage.remove') })).toBeNull();
    expect(within(row).getByText(msg('diskusage.external'))).toBeTruthy();
  });
});

describe('DiskUsage: Rescan', () => {
  it('measures again and shows the new report', async () => {
    const { user } = await shown();
    vi.mocked(scanUsage).mockResolvedValue(report({ games: [game('g-new', 'Fresh')], totals: { ...full.totals, gamesBytes: GB } }));
    await user.click(screen.getByRole('button', { name: msg('diskusage.rescan') }));
    expect(await screen.findByText('Fresh')).toBeTruthy();
    expect(screen.queryByText('Unmeasured')).toBeNull();
    expect(scanUsage).toHaveBeenCalledTimes(2);
  });
});
