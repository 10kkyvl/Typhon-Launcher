import { cleanup, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError, openStatusPage, openSupportSite } from '../../lib/services/account';
import { getTicket, listTickets, ticketLimits } from '../../lib/services/support';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, authView } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { LIMITS, page, thread, ticket } from './supportTesting';
import Support from './Support.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('./supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('./supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('./supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('./supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('./supportTesting')).accountOpeners(await original()));

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  authState.set('authenticated');
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(listTickets).mockResolvedValue(page([ticket(4, 'Game will not start')]));
  vi.mocked(getTicket).mockResolvedValue(thread(ticket(4, 'Game will not start')));
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, listTickets, getTicket, openStatusPage, openSupportSite]) vi.mocked(mock).mockClear();
  toasts.set([]);
  authState.set('bootstrapping');
  resetHistory();
});

const said = () => get(toasts).map((item) => [item.kind, item.message]);

describe('Support: which screen the route params open', () => {
  it('the list with no params', async () => {
    renderDom(Support, {});
    expect(await screen.findByRole('button', { name: /Game will not start/ })).toBeTruthy();
    expect(screen.getByRole('heading', { level: 1, name: msg('tickets.title') })).toBeTruthy();
    expect(getTicket).not.toHaveBeenCalled();
  });

  it('a request by its number', async () => {
    renderDom(Support, { ticket: '4' });
    expect(await screen.findByRole('heading', { level: 1, name: 'Game will not start' })).toBeTruthy();
    expect(getTicket).toHaveBeenCalledWith(4);
    expect(listTickets).not.toHaveBeenCalled();
  });

  it.each([['0'], ['-3'], ['4x'], ['']])('the list for a number that is none (%s)', async (value) => {
    renderDom(Support, { ticket: value });
    expect(await screen.findByRole('button', { name: /Game will not start/ })).toBeTruthy();
    expect(getTicket).not.toHaveBeenCalled();
  });

  it('the new request form on a topic, as the About tab opens it', async () => {
    renderDom(Support, { view: 'new', topic: 'launcher' });
    expect(await screen.findByRole('heading', { level: 1, name: msg('tickets.formTitle') })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: msg('tickets.topicLabel') }).textContent?.trim()).toBe(msg('tickets.topicLauncher'));
    expect(screen.getByRole('switch', { name: msg('tickets.attachLogs') }).getAttribute('aria-checked')).toBe('true');
  });

  it('the new request form on the install topic when the topic is none', async () => {
    renderDom(Support, { view: 'new', topic: 'nonsense', subject: 'Beta 2.0: crash' });
    await screen.findByRole('heading', { level: 1, name: msg('tickets.formTitle') });
    expect(screen.getByRole('combobox', { name: msg('tickets.topicLabel') }).textContent?.trim()).toBe(msg('tickets.topicInstall'));
    expect((screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement).value).toBe('Beta 2.0: crash');
  });
});

describe('Support: a guest', () => {
  it('is told support needs an account and is offered the way to it, reading nothing', async () => {
    authState.set('guest');
    renderDom(Support, { ticket: '4' });

    expect(screen.getByRole('heading', { level: 2, name: msg('tickets.guestTitle') })).toBeTruthy();
    expect(screen.getByText(msg('tickets.guestText'))).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('tickets.guestSignIn') })).toBeTruthy();
    expect(listTickets).not.toHaveBeenCalled();
    expect(getTicket).not.toHaveBeenCalled();
    expect(ticketLimits).not.toHaveBeenCalled();
  });

  it('signs out of the guest session to the sign-in form', async () => {
    authState.set('guest');
    const { user } = renderDom(Support, {});

    await user.click(screen.getByRole('button', { name: msg('tickets.guestSignIn') }));

    await vi.waitFor(() => expect(get(authState)).toBe('unauthenticated'));
    expect(get(authView)).toBe('login');
    expect(said()).toEqual([]);
  });

  it('can write from the website instead, and is told when it does not open', async () => {
    authState.set('guest');
    vi.mocked(openSupportSite).mockRejectedValueOnce(new AccountError('network_error'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { user } = renderDom(Support, {});

    await user.click(screen.getByRole('button', { name: msg('tickets.cantSignIn') }));

    expect(openSupportSite).toHaveBeenCalledTimes(1);
    expect(said()).toEqual([['danger', msg('tickets.siteOpenFailed')]]);
    error.mockRestore();
  });

  it('can open the service status page', async () => {
    authState.set('guest');
    const { user } = renderDom(Support, {});
    await user.click(screen.getByRole('button', { name: msg('tickets.statusPage') }));
    expect(openStatusPage).toHaveBeenCalledTimes(1);
  });
});
