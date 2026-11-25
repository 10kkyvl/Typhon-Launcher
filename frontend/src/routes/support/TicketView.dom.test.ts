import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { getTicket, replyTicket, resolveTicket, ticketLimits } from '../../lib/services/support';
import { resetHistory, route } from '../../lib/stores/router';
import { unreadReplies } from '../../lib/stores/support';
import { toasts } from '../../lib/stores/toasts';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { held, LIMITS, message, thread, ticket, words } from './supportTesting';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('./supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('./supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('./supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('./supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('./supportTesting')).accountOpeners(await original()));

const TICKET = ticket(7, 'Game will not start', { topic: 'launch' });
const MESSAGES = [
  message(1, 'user', 'It closes right after the logo.', '2026-11-01T10:00:00Z', { logBundleId: 'b1' }),
  message(2, 'staff', 'Which version of the launcher?', '2026-11-01T12:30:00Z'),
  message(3, 'user', 'The latest one.', '2026-11-01T13:00:00Z'),
];

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(getTicket).mockResolvedValue(thread(TICKET, MESSAGES));
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, getTicket, replyTicket, resolveTicket]) vi.mocked(mock).mockReset();
  unreadReplies.set(0);
  toasts.set([]);
  resetHistory();
});

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const replyBox = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) }) as HTMLTextAreaElement;
const replyButton = () => screen.getByRole('button', { name: msg('tickets.reply') });

async function opened() {
  const view = renderDom(TicketView, { number: 7 });
  await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
  return view;
}

describe('TicketView: the request', () => {
  it('is headed by its subject, with its reference, topic and status', async () => {
    await opened();
    const meta = words(screen.getByRole('heading', { level: 1 }).parentElement);
    expect(meta).toContain('T-7');
    expect(meta).toContain(msg('tickets.topicLaunch'));
    expect(meta).toContain(msg('tickets.statusOpen'));
    expect(getTicket).toHaveBeenCalledWith(7);
  });

  it('shows the messages as a list in the order they were written, each with who wrote it', async () => {
    await opened();
    const items = within(screen.getByRole('list')).getAllByRole('listitem');
    expect(items.map((item) => words(item))).toEqual([
      expect.stringMatching(new RegExp(`^${msg('tickets.you')}.*It closes right after the logo\\.`)),
      expect.stringMatching(new RegExp(`^${msg('tickets.staff')}.*Which version of the launcher\\?`)),
      expect.stringMatching(new RegExp(`^${msg('tickets.you')}.*The latest one\\.`)),
    ]);
    expect(words(items[0])).toContain(msg('tickets.logsAttached'));
    expect(words(items[1])).not.toContain(msg('tickets.logsAttached'));
  });

  it('goes back to the list', async () => {
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.back') }));
    expect(get(route)).toMatchObject({ name: 'support', params: {} });
  });

  it('says a request that is not there by its code', async () => {
    vi.mocked(getTicket).mockRejectedValue(new AccountError('ticket_not_found'));
    renderDom(TicketView, { number: 7 });
    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('errSupport.notFound')));
    expect(screen.queryByRole('heading', { level: 1 })).toBeNull();
  });
});

describe('TicketView: a reply', () => {
  it('sends the trimmed text without logs, then clears the box, says so and reads the request again', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    const { user } = await opened();
    await user.type(replyBox(), '  Still closes.  ');

    await user.click(replyButton());

    await vi.waitFor(() => expect(replyBox().value).toBe(''));
    expect(replyTicket).toHaveBeenCalledWith(7, 'Still closes.', false);
    expect(said()).toEqual([['success', msg('tickets.replySent')]]);
    expect(getTicket).toHaveBeenCalledTimes(2);
  });

  it('counts the text against the limit Go gives', async () => {
    vi.mocked(ticketLimits).mockResolvedValue({ maxSubjectRunes: 10, maxBodyRunes: 12 });
    const { user } = await opened();
    expect(screen.getByText(msg('tickets.counter', { count: 0, max: 12 }))).toBeTruthy();

    await user.type(replyBox(), 'Thirteen char');
    await user.click(replyButton());

    expect(replyBox().getAttribute('aria-invalid')).toBe('true');
    expect(screen.getByText(msg('tickets.errBodyTooLong', { max: 12 }))).toBeTruthy();
    expect(replyTicket).not.toHaveBeenCalled();
  });

  it('keeps the text when the reply is refused', async () => {
    vi.mocked(replyTicket).mockRejectedValue(new AccountError('ticket_invalid_body'));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');

    await user.click(replyButton());

    await vi.waitFor(() => expect(replyBox().getAttribute('aria-invalid')).toBe('true'));
    expect(replyBox().value).toBe('Still closes.');
    expect(screen.getByText(msg('errSupport.invalidBody'))).toBeTruthy();
  });

  it('asks before attaching logs, and sends nothing when the player declines', async () => {
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(screen.getByRole('switch', { name: msg('tickets.attachLogs') }));

    await user.click(replyButton());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    expect(words(dialog)).toContain(msg('tickets.logsConfirmText'));
    expect(words(dialog)).toContain(msg('tickets.logsConfirmNote'));
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(replyTicket).not.toHaveBeenCalled();
    expect(replyBox().value).toBe('Still closes.');
  });

  it('sends the logs with the reply once the player agrees', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z', { logBundleId: 'b2' }));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(screen.getByRole('switch', { name: msg('tickets.attachLogs') }));

    await user.click(replyButton());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    await user.click(within(dialog).getByRole('button', { name: msg('tickets.logsConfirmButton') }));

    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledWith(7, 'Still closes.', true));
    await vi.waitFor(() => expect(screen.getByRole('switch', { name: msg('tickets.attachLogs') }).getAttribute('aria-checked')).toBe('false'));
  });

  it('says a resolved request reopens with a reply', async () => {
    vi.mocked(getTicket).mockResolvedValue(thread({ ...TICKET, status: 'resolved' }, MESSAGES));
    await opened();
    expect(screen.getByText(msg('tickets.reopenNote'))).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('tickets.resolve') })).toBeNull();
    expect(replyButton()).toBeTruthy();
  });
});

describe('TicketView: marking the request resolved', () => {
  it('asks first and sends nothing when the player cancels', async () => {
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.resolve') }));
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.resolveTitle') });
    expect(words(dialog)).toContain(msg('tickets.resolveText', { ref: 'T-7' }));

    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(resolveTicket).not.toHaveBeenCalled();
  });

  it('marks it resolved once confirmed, and says so', async () => {
    const done = held<typeof TICKET>();
    vi.mocked(resolveTicket).mockReturnValue(done.promise);
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.resolve') }));
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.resolveTitle') });

    await user.click(within(dialog).getByRole('button', { name: msg('tickets.resolve') }));
    expect(within(dialog).getByRole('button', { name: msg('tickets.resolveBusy') }).getAttribute('aria-busy')).toBe('true');
    done.resolve({ ...TICKET, status: 'resolved' });

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(resolveTicket).toHaveBeenCalledWith(7);
    expect(words(screen.getByRole('heading', { level: 1 }).parentElement)).toContain(msg('tickets.statusResolved'));
    expect(screen.queryByRole('button', { name: msg('tickets.resolve') })).toBeNull();
    expect(said()).toEqual([['success', msg('tickets.resolved')]]);
  });

  it('says a refusal by its code and keeps the request open', async () => {
    vi.mocked(resolveTicket).mockRejectedValue(new AccountError('network_error'));
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.resolve') }));
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.resolveTitle') });

    await user.click(within(dialog).getByRole('button', { name: msg('tickets.resolve') }));

    await vi.waitFor(() => expect(said()).toEqual([['danger', msg('state.accountErrorNetwork')]]));
    expect(words(screen.getByRole('heading', { level: 1 }).parentElement)).toContain(msg('tickets.statusOpen'));
  });
});
