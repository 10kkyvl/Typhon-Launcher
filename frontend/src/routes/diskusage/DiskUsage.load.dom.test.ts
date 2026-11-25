import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, cancelScan, scanUsage, type Report } from '../../lib/services/diskusage';
import { renderDom } from '../../lib/testing/dom';
import { clockTime, longDate } from '../../lib/utils/format';
import DiskUsage from './DiskUsage.svelte';
import { game, report } from './diskUsageTesting';

// The first read of the page and what it says about it: a cached report at once, with how old it is and
// that it is being measured again; a scan that fails keeps what was shown and says so with Retry; with
// nothing cached, the loading state, then the report or the failure. Leaving the page stops its scan,
// and a scan stopped that way is not a failure. The services are mocked whole: nothing is measured.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
}));

const CACHED_AT = '2026-11-12T08:15:00Z';
const SCANNED_AT = '2026-11-12T10:40:00Z';
const cached = report({ games: [game('g1', 'From the cache')], scannedAt: CACHED_AT });
const fresh = report({ games: [game('g1', 'Fresh')], scannedAt: SCANNED_AT });

const measured = (iso: string) => msg('diskusage.measuredAt', { time: clockTime(new Date(iso)) });
const exact = (iso: string) => msg('transfers.historyExactTime', { date: longDate(new Date(iso)), time: clockTime(new Date(iso)) });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

async function settle() {
  for (let i = 0; i < 5; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

beforeEach(() => {
  vi.mocked(cancelScan).mockResolvedValue();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('DiskUsage: a cached report', () => {
  it('says how old it is, with the exact time for assistive tech, and that it is measured again', async () => {
    const scan = deferred<Report>();
    vi.mocked(cachedUsage).mockResolvedValue(cached);
    vi.mocked(scanUsage).mockReturnValue(scan.promise);
    renderDom(DiskUsage);

    const age = await screen.findByText(measured(CACHED_AT));
    expect(age.closest('time')?.getAttribute('datetime')).toBe(CACHED_AT);
    expect(age.closest('time')?.textContent).toContain(exact(CACHED_AT));
    expect(screen.getByText(msg('diskusage.remeasuring'))).toBeTruthy();
    expect(screen.getByText('From the cache')).toBeTruthy();

    scan.resolve(fresh);
    expect(await screen.findByText(measured(SCANNED_AT))).toBeTruthy();
    expect(screen.queryByText(msg('diskusage.remeasuring'))).toBeNull();
    expect(screen.queryByText(measured(CACHED_AT))).toBeNull();
  });

  it('stays when the scan fails, the failure said with Retry, which measures again', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(cached);
    vi.mocked(scanUsage).mockRejectedValueOnce(new Error('typhon:diskusage.not_started: текст из Go'));
    const { user } = renderDom(DiskUsage);

    const notice = (await screen.findByText(msg('errInstall.diskusageNotStarted'))).closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');
    expect(screen.getByText('From the cache')).toBeTruthy();
    expect(screen.getByText(measured(CACHED_AT))).toBeTruthy();
    expect(screen.queryByText(msg('diskusage.remeasuring'))).toBeNull();
    expect(document.body.textContent).not.toContain('текст из Go');

    vi.mocked(scanUsage).mockResolvedValueOnce(fresh);
    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));
    expect(await screen.findByText('Fresh')).toBeTruthy();
    await vi.waitFor(() => expect(screen.queryByText(msg('errInstall.diskusageNotStarted'))).toBeNull());
    expect(scanUsage).toHaveBeenCalledTimes(2);
  });

  it('keeps the failure and its Retry when the retry fails again', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(cached);
    vi.mocked(scanUsage).mockRejectedValue(new Error('typhon:diskusage.not_started: x'));
    const { user } = renderDom(DiskUsage);

    const said = await screen.findByText(msg('errInstall.diskusageNotStarted'));
    const notice = said.closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    await user.click(retry);
    await vi.waitFor(() => expect(scanUsage).toHaveBeenCalledTimes(2));
    await settle();
    // A retry the page answers as failed is said again: the notice draws its words anew in its alert.
    // One the page answered as worked would leave the old words where they were, unsaid.
    expect(said.isConnected).toBe(false);
    expect(screen.getByText(msg('errInstall.diskusageNotStarted')).closest('[role="alert"]')).not.toBeNull();
    expect(document.activeElement).toBe(retry);
    expect(screen.getByText('From the cache')).toBeTruthy();
  });
});

describe('DiskUsage: nothing cached', () => {
  it('shows the loading state, then the report', async () => {
    const scan = deferred<Report>();
    vi.mocked(cachedUsage).mockResolvedValue(null);
    vi.mocked(scanUsage).mockReturnValue(scan.promise);
    renderDom(DiskUsage);

    await vi.waitFor(() => expect(document.querySelector('.spinner[role="status"]')).not.toBeNull());
    scan.resolve(fresh);
    expect(await screen.findByText('Fresh')).toBeTruthy();
    expect(document.querySelector('.spinner[role="status"]')).toBeNull();
  });

  it('says the failure with Retry, which brings the report', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(null);
    vi.mocked(scanUsage).mockRejectedValueOnce(new Error('boom'));
    const { user } = renderDom(DiskUsage);

    const notice = (await screen.findByText(msg('diskusage.scanFailed'))).closest('.notice') as HTMLElement;
    expect(document.querySelector('.spinner[role="status"]')).toBeNull();
    vi.mocked(scanUsage).mockResolvedValueOnce(fresh);
    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));
    expect(await screen.findByText('Fresh')).toBeTruthy();
  });
});

describe('DiskUsage: leaving the page', () => {
  it('stops the scan it started, once', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(cached);
    vi.mocked(scanUsage).mockReturnValue(new Promise(() => {}));
    const view = renderDom(DiskUsage);
    await screen.findByText('From the cache');
    await settle();
    expect(cancelScan).not.toHaveBeenCalled();

    view.unmount();
    expect(cancelScan).toHaveBeenCalledTimes(1);
  });

  it('stops nothing when no scan of its own runs', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(null);
    vi.mocked(scanUsage).mockResolvedValue(fresh);
    const view = renderDom(DiskUsage);
    await screen.findByText('Fresh');
    view.unmount();
    expect(cancelScan).not.toHaveBeenCalled();
  });

  it('does not take a cancelled scan for a failure', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(cached);
    vi.mocked(scanUsage).mockRejectedValue(new Error('typhon:diskusage.scan_cancelled: подсчёт места отменён'));
    renderDom(DiskUsage);
    await screen.findByText('From the cache');
    await settle();
    expect(document.querySelector('.notice[data-tone="danger"]')).toBeNull();
    expect(screen.queryByText(msg('diskusage.remeasuring'))).toBeNull();
    expect(screen.getByText(measured(CACHED_AT))).toBeTruthy();
  });
});
