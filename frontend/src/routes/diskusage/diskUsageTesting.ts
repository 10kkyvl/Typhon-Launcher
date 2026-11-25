import { screen, within } from '@testing-library/svelte';
import { msg, type MessageKey } from '../../lib/i18n';
import type { DownloadUsage, GameUsage, GroupUsage, Leftover, Report } from '../../lib/services/diskusage';

// Fixtures and queries of the disk usage page's dom tests. Each test file mocks the services itself
// (vi.mock is hoisted per file): nothing here reaches Go, and nothing is measured or deleted.

export const GB = 1_000_000_000;

export function game(id: string, title: string, patch: Partial<GameUsage> = {}): GameUsage {
  return {
    id,
    title,
    installDir: `C:\\Games\\${title}`,
    sizeBytes: GB,
    sizeUnknown: false,
    missing: false,
    external: false,
    linked: false,
    lastPlayed: null,
    installedAt: '2026-01-01T00:00:00Z',
    running: false,
    busy: false,
    ...patch,
  };
}

export function download(id: string, name: string, patch: Partial<DownloadUsage> = {}): DownloadUsage {
  return {
    id,
    name,
    root: `D:\\Downloads\\${name}`,
    sizeBytes: GB,
    sizeUnknown: false,
    missing: false,
    seeding: false,
    installed: false,
    gameId: '',
    referenced: false,
    reason: '',
    reasonCode: '',
    ...patch,
  };
}

export function leftover(path: string, kind: string, patch: Partial<Leftover> = {}): Leftover {
  return { path, kind, sizeBytes: GB, sizeUnknown: false, deletable: true, reason: '', reasonCode: '', ...patch };
}

export function group(gameId: string, title: string, count: number, patch: Partial<GroupUsage> = {}): GroupUsage {
  return { gameId, title, titleUnknown: false, inLibrary: true, count, sizeBytes: GB, sizeUnknown: false, ...patch };
}

export function report(patch: Partial<Report> = {}): Report {
  return {
    games: [],
    downloads: [],
    leftovers: [],
    screenshots: [],
    snapshots: [],
    warnings: [],
    totals: { gamesBytes: 0, downloadsBytes: 0, leftoversBytes: 0, screenshotsBytes: 0, snapshotsBytes: 0, totalBytes: 0, reclaimableBytes: 0, unknownSizes: false },
    scannedAt: '2026-11-12T10:05:00Z',
    ...patch,
  };
}

/** The section of the page whose heading starts with the words of `title` (a count may follow them). */
export function section(title: MessageKey): HTMLElement {
  const words = msg(title);
  const heading = screen.getAllByRole('heading', { level: 2 }).find((h) => (h.textContent ?? '').trim().startsWith(words));
  if (!heading) throw new Error(`no section headed ${words}`);
  return heading.closest('section') as HTMLElement;
}

/** The row of `title`'s section that shows `text` (its title, its path, its kind). */
export function rowOf(title: MessageKey, text: string): HTMLElement {
  const row = within(section(title))
    .getAllByText(text, { exact: false })
    .map((el) => el.closest<HTMLElement>('.row'))
    .find((el): el is HTMLElement => el !== null);
  if (!row) throw new Error(`no row of ${title} shows ${text}`);
  return row;
}

/** The size a row says. */
export const sizeOf = (row: HTMLElement) => row.querySelector('.size')?.textContent?.trim();
