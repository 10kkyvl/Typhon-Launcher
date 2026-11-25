import { cleanup, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { createTicket, getTicket, ticketLimits } from '../../lib/services/support';
import { navigate, resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import SupportRouteHarness from '../../lib/testing/SupportRouteHarness.svelte';
import { held, LIMITS, thread, ticket } from '../../lib/testing/supportTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  authState.set('authenticated');
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  navigate('support', { view: 'new', topic: 'account' });
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, getTicket, createTicket]) vi.mocked(mock).mockReset();
  toasts.set([]);
  authState.set('bootstrapping');
  resetHistory();
  vi.unstubAllGlobals();
});

describe('Support: after Create', () => {
  it.each([
    ['the send shortcut in the text', true],
    ['Send', false],
  ])('the new request opens with focus on its heading, sent with %s', async (_how, shortcut) => {
    const opened = held<ReturnType<typeof thread>>();
    vi.mocked(createTicket).mockResolvedValue(ticket(21, 'Cannot sign in', { topic: 'account' }));
    vi.mocked(getTicket).mockReturnValue(opened.promise);
    const { user } = renderDom(SupportRouteHarness, {});
    await screen.findByRole('heading', { level: 1, name: msg('tickets.formTitle') });
    await user.type(screen.getByRole('textbox', { name: msg('tickets.subjectLabel') }), 'Cannot sign in');
    await user.type(screen.getByRole('textbox', { name: msg('tickets.bodyLabel') }), 'The code never arrives.');

    if (shortcut) await user.keyboard('{Control>}{Enter}{/Control}');
    else await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));

    await vi.waitFor(() => expect(getTicket).toHaveBeenCalledWith(21));
    opened.resolve(thread(ticket(21, 'Cannot sign in', { topic: 'account' })));
    const heading = await screen.findByRole('heading', { level: 1, name: 'Cannot sign in' });
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));
  });
});
