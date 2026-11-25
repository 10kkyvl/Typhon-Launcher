import { cleanup, fireEvent, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { createTicket, ticketLimits } from '../../lib/services/support';
import { navigate, resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import { emitUpload, held, LIMITS, ticket, words } from '../../lib/testing/supportTesting';
import NewTicket from './NewTicket.svelte';

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
  navigate('support', { view: 'new' });
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(ticketLimits).mockReset();
  vi.mocked(createTicket).mockReset();
  toasts.set([]);
  resetHistory();
});

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const subjectField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement;
const bodyField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.bodyLabel')) }) as HTMLTextAreaElement;
const logsSwitch = () => screen.getByRole('switch', { name: msg('tickets.attachLogs') });
const submit = () => screen.getByRole('button', { name: msg('tickets.submit') });

async function form(props: { initialTopic: 'install' | 'launch' | 'account' | 'launcher' | 'other'; initialSubject?: string }) {
  const view = renderDom(NewTicket, props);
  await screen.findByRole('button', { name: msg('tickets.submit') });
  return view;
}

async function filled(props: Parameters<typeof form>[0] = { initialTopic: 'account' }) {
  const view = await form(props);
  await view.user.type(subjectField(), '  Cannot sign in  ');
  await view.user.type(bodyField(), '  The code never arrives.  ');
  return view;
}

describe('NewTicket: the form', () => {
  it('starts on the topic it was opened with, and attaches logs by default where they help', async () => {
    await form({ initialTopic: 'launcher' });
    expect(words(screen.getByRole('combobox', { name: msg('tickets.topicLabel') }))).toBe(msg('tickets.topicLauncher'));
    expect(logsSwitch().getAttribute('aria-checked')).toBe('true');
  });

  it.each([
    ['install', 'true'],
    ['launch', 'true'],
    ['launcher', 'true'],
    ['account', 'false'],
    ['other', 'false'],
  ] as const)('the logs switch starts %s at %s', async (topic, on) => {
    await form({ initialTopic: topic });
    expect(logsSwitch().getAttribute('aria-checked')).toBe(on);
  });

  it('follows the topic with the logs default until the player sets the switch', async () => {
    const { user } = await form({ initialTopic: 'account' });
    const topic = screen.getByRole('combobox', { name: msg('tickets.topicLabel') });

    await user.click(topic);
    await user.click(screen.getByRole('option', { name: msg('tickets.topicInstall') }));
    expect(logsSwitch().getAttribute('aria-checked')).toBe('true');

    await user.click(logsSwitch());
    await user.click(topic);
    await user.click(screen.getByRole('option', { name: msg('tickets.topicLaunch') }));
    expect(logsSwitch().getAttribute('aria-checked')).toBe('false');
  });

  it('takes the subject it was opened with', async () => {
    await form({ initialTopic: 'other', initialSubject: 'Beta 2.0: crash' });
    expect(subjectField().value).toBe('Beta 2.0: crash');
  });

  it('shows the lengths against the limits Go gives', async () => {
    vi.mocked(ticketLimits).mockResolvedValue({ maxSubjectRunes: 20, maxBodyRunes: 300 });
    const { user } = await form({ initialTopic: 'account' });
    expect(screen.getByText(msg('tickets.counter', { count: 0, max: 20 }))).toBeTruthy();
    expect(screen.getByText(msg('tickets.counter', { count: 0, max: 300 }))).toBeTruthy();

    await user.type(subjectField(), 'Longer than twenty letters');
    await user.click(submit());

    expect(subjectField().getAttribute('aria-invalid')).toBe('true');
    expect(screen.getByText(msg('tickets.errSubjectTooLong', { max: 20 }))).toBeTruthy();
    expect(createTicket).not.toHaveBeenCalled();
  });

  it('goes back on Cancel', async () => {
    const { user } = await form({ initialTopic: 'account' });
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    expect(get(route).name).toBe('library');
  });
});

describe('NewTicket: what Create sends', () => {
  it('sends the topic, the trimmed subject and text, no game and no logs, then opens the new request', async () => {
    vi.mocked(createTicket).mockResolvedValue(ticket(12, 'Cannot sign in', { topic: 'account' }));
    const { user } = await filled();

    await user.click(submit());

    await vi.waitFor(() => expect(get(route)).toMatchObject({ name: 'support', params: { ticket: '12' } }));
    expect(createTicket).toHaveBeenCalledWith({ topic: 'account', subject: 'Cannot sign in', body: 'The code never arrives.', gameIgdbId: null, attachLogs: false });
    expect(said()).toEqual([['success', msg('tickets.created', { ref: 'T-12' })]]);
  });

  it('asks before attaching logs, and sends nothing when the player declines', async () => {
    const { user } = await filled({ initialTopic: 'launcher' });

    await user.click(submit());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    expect(words(dialog)).toContain(msg('tickets.logsConfirmText'));
    expect(words(dialog)).toContain(msg('tickets.logsConfirmNote'));
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(createTicket).not.toHaveBeenCalled();
    expect(subjectField().value).toBe('  Cannot sign in  ');
    expect(get(route).params).toEqual({ view: 'new' });
  });

  it('sends the logs with the request once the player agrees, and shows the upload while it runs', async () => {
    const sent = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValue(sent.promise);
    const { user } = await filled({ initialTopic: 'launcher' });

    await user.click(submit());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    await user.click(within(dialog).getByRole('button', { name: msg('tickets.logsConfirmButton') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledWith(expect.objectContaining({ topic: 'launcher', attachLogs: true })));

    emitUpload({ state: 'sending', sentBytes: 512, totalBytes: 1024 });
    const bar = await screen.findByRole('progressbar', { name: msg('settings.aboutLogsLabel') });
    expect(bar.getAttribute('aria-valuenow')).toBe('50');
    expect(screen.getByText(msg('settings.aboutLogsSendingProgress', { percent: 50 }))).toBeTruthy();

    sent.resolve(ticket(13, 'Cannot sign in'));
    await vi.waitFor(() => expect(get(route).params).toEqual({ ticket: '13' }));
  });
});

describe('NewTicket: a refused request says why by its code', () => {
  it.each([
    ['ticket_invalid_subject', 'subject', msg('errSupport.invalidSubject')],
    ['ticket_invalid_body', 'body', msg('errSupport.invalidBody')],
  ] as const)('%s at the %s field, keeping what was typed', async (code, field, text) => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError(code));
    const { user } = await filled();

    await user.click(submit());

    const target = field === 'subject' ? subjectField : bodyField;
    await vi.waitFor(() => expect(target().getAttribute('aria-invalid')).toBe('true'));
    expect(screen.getByText(text)).toBeTruthy();
    expect(subjectField().value).toBe('  Cannot sign in  ');
    expect(bodyField().value).toBe('  The code never arrives.  ');
  });

  it.each([
    ['ticket_limit', msg('errSupport.limit')],
    ['network_error', msg('state.accountErrorNetwork')],
    ['ticket_bundle_limit', msg('errSupport.bundleLimit')],
  ])('%s in the form', async (code, text) => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError(code));
    const { user } = await filled();

    await user.click(submit());

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(text));
    expect(subjectField().hasAttribute('aria-invalid')).toBe(false);
    expect(get(route).params).toEqual({ view: 'new' });
  });

  it('a failed log upload says that nothing was sent', async () => {
    vi.mocked(createTicket).mockRejectedValue(new Error('typhon:diagnostics.upload_failed'));
    const { user } = await filled({ initialTopic: 'launcher' });

    await user.click(submit());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    await user.click(within(dialog).getByRole('button', { name: msg('tickets.logsConfirmButton') }));

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('errSupport.logsFailed')));
    expect(subjectField().value).toBe('  Cannot sign in  ');
  });

  it('an unknown failure says the plain failure, never the raw text', async () => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError('something_new'));
    const { user } = await filled();

    await user.click(submit());

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('errSupport.generic')));
    expect(alert.textContent).not.toContain('something_new');
  });
});

describe('NewTicket: attaching logs', () => {
  it('names the upload\'s progress and says its value in words', async () => {
    const sent = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValue(sent.promise);
    const { user } = await filled({ initialTopic: 'launcher' });
    await user.click(submit());
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('tickets.logsConfirmButton') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalled());

    // A stage of unknown length is said beside the bar: ProgressBar claims no value while indeterminate.
    emitUpload({ state: 'preparing' });
    const bar = await screen.findByRole('progressbar', { name: msg('settings.aboutLogsLabel') });
    expect(bar.getAttribute('aria-busy')).toBe('true');
    expect(screen.getByText(msg('settings.aboutLogsPreparingEllipsis'))).toBeTruthy();
    emitUpload({ state: 'sending', sentBytes: 256, totalBytes: 1024 });
    await vi.waitFor(() => expect(bar.getAttribute('aria-valuetext')).toBe(msg('settings.aboutLogsSendingProgress', { percent: 25 })));
    sent.resolve(ticket(16, 'Cannot sign in'));
    await vi.waitFor(() => expect(get(route).params).toEqual({ ticket: '16' }));
  });

  it.each([
    ['diagnostics.log_upload_too_large', 'errLogs.uploadTooLarge'],
    ['diagnostics.log_upload_network', 'errLogs.uploadNetwork'],
    ['support.logs_unavailable', 'errSupport.logsUnavailable'],
  ] as const)('a failed upload (%s) keeps the form and says why', async (code, key) => {
    vi.mocked(createTicket).mockRejectedValue(new Error(`typhon:${code}`));
    const { user } = await filled({ initialTopic: 'launcher' });
    await user.click(submit());
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('tickets.logsConfirmButton') }));

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg(key)));
    expect(bodyField().value).toBe('  The code never arrives.  ');
    expect(logsSwitch().getAttribute('aria-checked')).toBe('true');
    expect(screen.queryByRole('progressbar')).toBeNull();
  });

  it('cancelling the consent returns focus to Send when Send opened it', async () => {
    const { user } = await filled({ initialTopic: 'launcher' });
    submit().focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(submit()));
  });

  it('cancelling the consent returns focus to the text when the send shortcut opened it', async () => {
    const { user } = await filled({ initialTopic: 'launcher' });
    await user.keyboard('{Control>}{Enter}{/Control}');
    await screen.findByRole('dialog');
    await user.keyboard('{Escape}');
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(bodyField()));
    expect(createTicket).not.toHaveBeenCalled();
  });
});

describe('NewTicket: a session gone', () => {
  it('says so in the form, keeps the draft and offers the way to sign in', async () => {
    authState.set('authenticated');
    vi.mocked(createTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = await filled();
    await user.click(submit());

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('state.accountErrorUnauthenticated')));
    expect(subjectField().value).toBe('  Cannot sign in  ');
    await user.click(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('tickets.signInAgain') }));
    expect(get(authState)).toBe('unauthenticated');
    authState.set('bootstrapping');
  });
});

describe('NewTicket: the fields are the base text fields', () => {
  it('names each field by its label alone, with the count on screen beside it', async () => {
    await form({ initialTopic: 'account' });
    const subject = screen.getByRole('textbox', { name: msg('tickets.subjectLabel') });
    const body = screen.getByRole('textbox', { name: msg('tickets.bodyLabel') });
    expect(subject.tagName).toBe('INPUT');
    expect(body.tagName).toBe('TEXTAREA');
    // The number's group separator is a no-break space, which the default matcher would fold.
    const count = screen.getByText(msg('tickets.counter', { count: 0, max: LIMITS.maxBodyRunes }), { normalizer: (text) => text });
    expect(count.getAttribute('aria-hidden')).toBe('true');
  });

  it('says how the text is sent', async () => {
    await form({ initialTopic: 'account' });
    expect(description(bodyField())).toBe(msg('tickets.sendHint', { keys: 'Ctrl Enter' }));
  });

  it('says the body near its limit once, not on every key', async () => {
    vi.mocked(ticketLimits).mockResolvedValue({ maxSubjectRunes: 120, maxBodyRunes: 10 });
    const { user } = await form({ initialTopic: 'account' });
    const live = () => [...document.querySelectorAll('[role="status"]')].map((node) => node.textContent?.trim()).filter(Boolean);
    await user.type(bodyField(), 'abcdefgh');
    expect(live()).toEqual([]);
    await user.type(bodyField(), 'i');
    expect(live()).toEqual([msg('tickets.counterNear', { count: 1 })]);
    await user.type(bodyField(), 'jk');
    expect(live()).toEqual([msg('tickets.counterOver', { count: 1 })]);
  });
});

describe('NewTicket: where a refusal is said', () => {
  it('a draft the form refuses: at each field, with focus on the first', async () => {
    const { user } = await form({ initialTopic: 'account' });
    await user.click(submit());
    expect(subjectField().getAttribute('aria-invalid')).toBe('true');
    expect(bodyField().getAttribute('aria-invalid')).toBe('true');
    expect(document.activeElement).toBe(subjectField());
  });

  it.each([
    ['ticket_invalid_subject', 'subject'],
    ['ticket_invalid_body', 'body'],
  ] as const)('%s from Go: at the %s field, with focus there', async (code, field) => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError(code));
    const { user } = await filled();
    await user.click(submit());
    const target = field === 'subject' ? subjectField : bodyField;
    await vi.waitFor(() => expect(target().getAttribute('aria-invalid')).toBe('true'));
    expect(document.activeElement).toBe(target());
    expect(screen.queryByText(msg('errSupport.generic'))).toBeNull();
  });

  it.each([['ticket_invalid_topic'], ['ticket_invalid_game'], ['ticket_invalid_client']])('%s: once, in the form', async (code) => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError(code));
    const { user } = await filled();
    await user.click(submit());
    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent?.trim()).not.toBe(''));
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    expect(subjectField().hasAttribute('aria-invalid')).toBe(false);
    expect(bodyField().hasAttribute('aria-invalid')).toBe(false);
  });
});

describe('NewTicket: sending', () => {
  it('sends with Ctrl/Cmd+Enter from the text and with Enter from the subject, once while the answer is on its way', async () => {
    const sent = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValue(sent.promise);
    const { user } = await filled();

    await user.keyboard('{Control>}{Enter}{/Control}');
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    expect(bodyField().value).toBe('  The code never arrives.  ');
    await user.keyboard('{Meta>}{Enter}{/Meta}');
    await user.click(subjectField());
    await user.keyboard('{Enter}');

    expect(createTicket).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: msg('tickets.submitting') }).getAttribute('aria-busy')).toBe('true');
    sent.resolve(ticket(14, 'Cannot sign in'));
    await vi.waitFor(() => expect(get(route).params).toEqual({ ticket: '14' }));
  });

  it('sends with Enter from the subject', async () => {
    vi.mocked(createTicket).mockResolvedValue(ticket(15, 'Cannot sign in'));
    const { user } = await filled();
    await user.click(subjectField());
    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
  });

  it('keeps the field it was sent from focused and read-only while the answer is on its way', async () => {
    const sent = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValue(sent.promise);
    const { user } = await filled();

    await user.keyboard('{Control>}{Enter}{/Control}');

    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    expect(document.activeElement).toBe(bodyField());
    expect(bodyField().readOnly).toBe(true);
    expect(bodyField().disabled).toBe(false);
    expect(subjectField().readOnly).toBe(true);
    await user.keyboard('x');
    expect(bodyField().value).toBe('  The code never arrives.  ');
    sent.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(bodyField().readOnly).toBe(false));
    expect(document.activeElement).toBe(bodyField());
  });

  it('keeps a plain Enter in the text a new line', async () => {
    const { user } = await filled();
    await user.keyboard('{Enter}more');
    expect(bodyField().value).toBe('  The code never arrives.  \nmore');
    expect(createTicket).not.toHaveBeenCalled();
  });
});

// An input method (Japanese, Chinese, Korean) takes Enter to commit what is being composed, and a key
// held down repeats: neither is a press that sends. jsdom does not compose or repeat, so the events are
// made by hand, each beside the same press that does send.
describe('NewTicket: keys that do not send', () => {
  async function ready() {
    await filled();
    vi.mocked(createTicket).mockReturnValue(new Promise(() => {}));
  }

  it.each([
    ['Enter that commits a composition in the subject', () => subjectField(), { key: 'Enter', isComposing: true }],
    ['Enter after the composition ended, as WebKit sends it (key code 229), in the subject', () => subjectField(), { key: 'Enter', keyCode: 229 }],
    ['a held Enter in the subject', () => subjectField(), { key: 'Enter', repeat: true }],
    ['a held Ctrl+Enter in the text', () => bodyField(), { key: 'Enter', ctrlKey: true, repeat: true }],
    ['Ctrl+Enter while composing in the text', () => bodyField(), { key: 'Enter', ctrlKey: true, isComposing: true }],
    ['Ctrl+Enter after the composition ended, as WebKit sends it (key code 229), in the text', () => bodyField(), { key: 'Enter', ctrlKey: true, keyCode: 229 }],
  ] as const)('%s', async (_name, field, init) => {
    await ready();
    fireEvent.keyDown(field(), init);
    expect(createTicket).not.toHaveBeenCalled();

    fireEvent.keyDown(field(), { key: 'Enter', ctrlKey: 'ctrlKey' in init, keyCode: 13 });
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
  });
});
