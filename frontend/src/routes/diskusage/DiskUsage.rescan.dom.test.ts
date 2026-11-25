import { cleanup, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, cancelScan, scanUsage, type Report } from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';
import { game, report } from './diskUsageTesting';

// A measure asked while a scan is in flight: Go joins the running walk, whose game list was read when
// it began, so its answer may predate the change that asked. The page remembers the request and
// measures once more after the scan answers: one follow-up for any number of requests, none once the
// page is left, and a follow-up that fails is said as any scan failure. Scans are held promises.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
}));

interface Held {
  resolve: (value: Report) => void;
  reject: (err: unknown) => void;
}

let held: Held[] = [];

async function settle() {
  for (let i = 0; i < 6; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

const one = (title: string) => report({ games: [game('g1', title)] });

// The installed games change: each change asks the page to measure again (DiskUsage.svelte).
let changes = 0;
function libraryChanges() {
  changes += 1;
  libraryGames.set([{ id: 'g1', title: 'One', installDir: `C:\\Games\\One-${changes}` }] as LibraryGame[]);
}

beforeEach(() => {
  held = [];
  changes = 0;
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(cancelScan).mockResolvedValue();
  vi.mocked(scanUsage).mockImplementation(
    () =>
      new Promise<Report>((resolve, reject) => {
        held.push({ resolve, reject });
      }),
  );
  libraryGames.set([{ id: 'g1', title: 'One', installDir: 'C:\\Games\\One' }] as LibraryGame[]);
  libraryLoaded.set(true);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  libraryLoaded.set(false);
});

describe('DiskUsage: a measure asked during a scan', () => {
  it('runs once more after the scan answers, however many times it was asked', async () => {
    renderDom(DiskUsage);
    await settle();
    expect(held).toHaveLength(1);

    for (let i = 0; i < 10; i++) {
      libraryChanges();
      await settle();
    }
    expect(held).toHaveLength(1);

    held[0].resolve(one('Before the change'));
    await settle();
    expect(held).toHaveLength(2);
    expect(screen.getByText('Before the change')).toBeTruthy();

    held[1].resolve(one('After the change'));
    expect(await screen.findByText('After the change')).toBeTruthy();
    await settle();
    expect(held).toHaveLength(2);
    expect(screen.queryByText(msg('diskusage.remeasuring'))).toBeNull();
  });

  it('measures right away when asked with no scan in flight', async () => {
    renderDom(DiskUsage);
    await settle();
    held[0].resolve(one('First'));
    await screen.findByText('First');

    libraryChanges();
    await settle();
    expect(held).toHaveLength(2);
  });

  it('drops the follow-up when the page is left', async () => {
    const view = renderDom(DiskUsage);
    await settle();
    libraryChanges();
    await settle();

    view.unmount();
    held[0].resolve(one('Late'));
    await settle();
    expect(held).toHaveLength(1);
    expect(cancelScan).toHaveBeenCalledTimes(1);
  });

  it('says a follow-up that fails as any scan failure, keeping the report', async () => {
    renderDom(DiskUsage);
    await settle();
    libraryChanges();
    await settle();
    held[0].resolve(one('Before the change'));
    await settle();
    expect(held).toHaveLength(2);

    held[1].reject(new Error('typhon:diskusage.not_started: текст из Go'));
    expect(await screen.findByText(msg('errInstall.diskusageNotStarted'))).toBeTruthy();
    expect(screen.getByText('Before the change')).toBeTruthy();
    expect(document.body.textContent).not.toContain('текст из Go');
  });
});
