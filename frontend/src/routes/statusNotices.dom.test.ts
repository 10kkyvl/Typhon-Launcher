import { render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import EmulatorHint from '../lib/components/EmulatorHint.svelte';
import { locale, msg } from '../lib/i18n';
import { cachedUsage, scanUsage } from '../lib/services/diskusage';
import { listPlatforms } from '../lib/services/emulators';
import { getProfileSnapshot } from '../lib/services/profile';
import { listTickets, ticketLimits } from '../lib/services/support';
import { historyStatus } from '../lib/stores/history';
import { profileFailed } from '../lib/stores/profile';
import { authState } from '../lib/stores/user';
import { user } from '../lib/testing/dom';
import Activity from './activity/Activity.svelte';
import DiskUsage from './diskusage/DiskUsage.svelte';
import History from './history/History.svelte';
import NewTicket from './support/NewTicket.svelte';
import newTicketSource from './support/NewTicket.svelte?raw';
import TicketList from './support/TicketList.svelte';
import ticketViewSource from './support/TicketView.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../lib/testing/dom')).wailsRuntime());
vi.mock('../lib/services/backend', () => ({ inWails: false }));
vi.mock('../lib/services/profile', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/services/profile')>()),
  getProfileSnapshot: vi.fn(),
}));
vi.mock('../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
}));
vi.mock('../lib/services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/services/emulators')>()),
  listPlatforms: vi.fn(),
}));
vi.mock('../lib/services/support', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/services/support')>()),
  listTickets: vi.fn(),
  ticketLimits: vi.fn(),
}));

// Real pages: what used to be status-coloured text on the page, or a hand-made banner, is one Notice.
describe('a status message of a page', () => {
  beforeEach(() => {
    locale.set('en');
    vi.mocked(getProfileSnapshot).mockReset();
    vi.mocked(listTickets).mockReset();
    vi.mocked(ticketLimits).mockReset();
    profileFailed.set(false);
    authState.set('guest');
  });

  it('shows a failed page load as a danger notice, announced as an alert', async () => {
    vi.mocked(getProfileSnapshot).mockRejectedValue(new Error('offline'));
    render(Activity);

    const alert = await screen.findByRole('alert');
    await waitFor(() => expect(alert.textContent).toContain('Failed to load profile statistics'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
  });

  it('keeps the retry of a failed list inside its notice, and the notice goes once the list loads', async () => {
    vi.mocked(listTickets).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ tickets: [], next: '' });
    render(TicketList);

    const alert = await screen.findByRole('alert');
    await waitFor(() => expect(alert.textContent).toContain('Could not load your requests.'));
    const notice = alert.closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');

    await user().click(within(notice).getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(listTickets).toHaveBeenCalledTimes(2);
  });

  // A standing fact of the page it is rendered with is read in the flow of that page, not announced.
  it('shows degraded history as a danger notice that is not announced', async () => {
    historyStatus.set({ degraded: true, message: 'disk is full' });
    render(History);

    const text = await screen.findByText(/disk is full/);
    expect(text.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
    historyStatus.set({ degraded: false, message: '' });
  });

  it('gives the emulator advice as a warning notice that is not announced when it loads', async () => {
    vi.mocked(listPlatforms).mockResolvedValue([
      { id: 'ps2', name: 'PS2', emulator: 'PCSX2', homepage: '', extensions: [], imageNames: [], path: '/e', status: 'ready', reason: '', warnings: ['discord_exit_hang'] },
    ]);
    render(EmulatorHint, { props: { platform: 'ps2' } });

    const text = await screen.findByText(msg('emul.discordExitHang'));
    expect(text.closest('.notice')?.getAttribute('data-tone')).toBe('warning');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('titles the warnings section of the disk usage page with a heading under the page title', async () => {
    const report = {
      games: [],
      downloads: [],
      leftovers: [],
      screenshots: [],
      snapshots: [],
      warnings: [{ area: 'downloads', message: 'permission denied' }],
      totals: { gamesBytes: 0, downloadsBytes: 0, leftoversBytes: 0, screenshotsBytes: 0, snapshotsBytes: 0, totalBytes: 0, reclaimableBytes: 0, unknownSizes: false },
      scannedAt: '2026-11-12T00:00:00Z',
    };
    vi.mocked(cachedUsage).mockResolvedValue(report);
    vi.mocked(scanUsage).mockResolvedValue(report);
    render(DiskUsage);

    const heading = await screen.findByRole('heading', { level: 2, name: msg('diskusage.warningsTitle') });
    expect(heading.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(heading.closest('[role="alert"]')).not.toBeNull();
  });

  it('turns the counter of a field red next to that field, on a plate of its own', async () => {
    vi.mocked(ticketLimits).mockResolvedValue({ maxSubjectRunes: 5, maxBodyRunes: 20 });
    render(NewTicket, { props: { initialTopic: 'other' } });

    const subject = await screen.findByRole('textbox', { name: /^Summary of the problem/ });
    const field = subject.closest('label') as HTMLElement;
    const counter = within(field).getByText('0 of 5');
    expect(counter.classList.contains('over')).toBe(false);

    await user().type(subject, 'abcdef');
    expect(within(field).getByText('6 of 5').classList.contains('over')).toBe(true);
    expect(screen.queryByRole('alert')).toBeNull();

    for (const source of [newTicketSource, ticketViewSource]) {
      const rule = /\.counter\.over\s*\{([^}]*)\}/.exec(source)?.[1] ?? '';
      expect(rule).toMatch(/background:\s*var\(--surface(-\d)?\)/);
    }
  });
});
