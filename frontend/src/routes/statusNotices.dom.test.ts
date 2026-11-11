import { render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../lib/i18n';
import { getProfileSnapshot } from '../lib/services/profile';
import { listTickets, ticketLimits } from '../lib/services/support';
import { profileFailed } from '../lib/stores/profile';
import { authState } from '../lib/stores/user';
import { user } from '../lib/testing/dom';
import Activity from './activity/Activity.svelte';
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
    expect(alert.textContent).toContain('Failed to load profile statistics');
    expect(alert.classList.contains('notice')).toBe(true);
    expect(alert.getAttribute('data-tone')).toBe('danger');
  });

  it('keeps the retry of a failed list inside its notice, and the notice goes once the list loads', async () => {
    vi.mocked(listTickets).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ tickets: [], next: '' });
    render(TicketList);

    const alert = await screen.findByRole('alert');
    expect(alert.getAttribute('data-tone')).toBe('danger');
    expect(alert.textContent).toContain('Could not load your requests.');

    await user().click(within(alert).getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(listTickets).toHaveBeenCalledTimes(2);
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
