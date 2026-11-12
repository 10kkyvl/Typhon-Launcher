import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen } from '@testing-library/svelte';
import { msg } from '../../lib/i18n';
import { getTicket, ticketLimits, type TicketThread } from '../../lib/services/support';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import NewTicket from './NewTicket.svelte';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/support', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/support')>()),
  ticketLimits: vi.fn(),
  getTicket: vi.fn(),
}));

const THREAD: TicketThread = {
  ticket: {
    number: 7,
    ref: 'T-7',
    topic: 'launcher',
    subject: 'Crash on start',
    status: 'open',
    gameIgdbId: null,
    client: null,
    unread: false,
    createdAt: '2026-11-01T10:00:00Z',
    updatedAt: '2026-11-01T10:00:00Z',
    resolvedAt: null,
  },
  messages: [],
};

beforeEach(() => {
  vi.mocked(ticketLimits).mockResolvedValue({ maxSubjectRunes: 120, maxBodyRunes: 5000 });
  vi.mocked(getTicket).mockResolvedValue(THREAD);
});

afterEach(() => {
  vi.mocked(ticketLimits).mockReset();
  vi.mocked(getTicket).mockReset();
});

// An invalid field says so (aria-invalid, which the global ring paints in the danger colour) and is
// described by its message, and stops saying so once it is edited.
function expectInvalid(field: HTMLElement, message: string) {
  expect(field.getAttribute('aria-invalid')).toBe('true');
  expect(description(field)).toBe(message);
}

describe('NewTicket: an empty draft', () => {
  it('marks the subject and the body invalid, each described by its message', async () => {
    const { container, user } = renderDom(NewTicket, { initialTopic: 'launcher' });
    const submit = await screen.findByRole('button', { name: msg('tickets.submit') });
    const subject = container.querySelector<HTMLInputElement>('input.input');
    const body = container.querySelector<HTMLTextAreaElement>('textarea.input');
    expect(subject).toBeInstanceOf(HTMLInputElement);
    expect(body).toBeInstanceOf(HTMLTextAreaElement);
    expect((subject as HTMLInputElement).hasAttribute('aria-invalid')).toBe(false);

    await user.click(submit);

    expectInvalid(subject as HTMLInputElement, msg('tickets.errSubjectRequired'));
    expectInvalid(body as HTMLTextAreaElement, msg('tickets.errBodyRequired'));

    await user.type(subject as HTMLInputElement, 'Crash');
    expect((subject as HTMLInputElement).hasAttribute('aria-invalid')).toBe(false);
    expect((subject as HTMLInputElement).hasAttribute('aria-describedby')).toBe(false);
  });
});

describe('TicketView: an empty reply', () => {
  it('marks the reply invalid, described by its message', async () => {
    const { container, user } = renderDom(TicketView, { number: 7 });
    const send = await screen.findByRole('button', { name: msg('tickets.reply') });
    const reply = container.querySelector<HTMLTextAreaElement>('textarea.input');
    expect(reply).toBeInstanceOf(HTMLTextAreaElement);
    expect((reply as HTMLTextAreaElement).hasAttribute('aria-invalid')).toBe(false);

    await user.click(send);

    expectInvalid(reply as HTMLTextAreaElement, msg('tickets.errBodyRequired'));

    await user.type(reply as HTMLTextAreaElement, 'Still broken');
    expect((reply as HTMLTextAreaElement).hasAttribute('aria-invalid')).toBe(false);
  });
});
