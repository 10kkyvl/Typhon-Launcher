import { cleanup, fireEvent, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError, type CurrentUser } from '../../lib/services/account';
import type { LibraryGame } from '../../lib/services/library';
import { igdbIdsOf } from '../../lib/services/profile';
import { createTicket, getTicket, ticketLimits, unreadTickets } from '../../lib/services/support';
import { libraryGames } from '../../lib/stores/library';
import { navigate, resetHistory } from '../../lib/stores/router';
import { unreadReplies } from '../../lib/stores/support';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { held, LIMITS, thread, ticket, words } from '../../lib/testing/supportTesting';
import Support from './Support.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));
// The game picker maps library games to catalog ids through the profile service: answered here.
vi.mock('../../lib/services/profile', async (original) => ({ ...(await original<object>()), igdbIdsOf: vi.fn() }));

// The ways into the new-request form, as the route gives them to the support page: the list's "New
// request" sets nothing; the Beta badge sets the topic and a subject; the Move dialog and Settings set
// a topic. A draft exists only once the player types or chooses content (a subject, a text, a game) or
// restores one; the topic and the logs switch are settings, carried into a draft that exists. A plain
// way in gives the draft back whole; a way in with a prefill opens with the prefill and offers it.

const BETA = { view: 'new', topic: 'launcher', subject: 'Beta 2.0: ' };
const MOVE = { view: 'new', topic: 'other' };
const PLAIN = { view: 'new' };

const player = (id: string) => ({ id, username: id, displayName: id }) as CurrentUser;
let players = 0;

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  navigate('support', { view: 'new' });
  currentUser.set(player(`way-in-${++players}`));
  authState.set('authenticated');
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(unreadTickets).mockResolvedValue(0);
  vi.mocked(getTicket).mockResolvedValue(thread(ticket(7, 'Request 7'), []));
  vi.mocked(igdbIdsOf).mockResolvedValue({ 'canon-1': '42' });
  libraryGames.set([{ id: 'game-1', title: 'A game', canonicalGameId: 'canon-1' } as LibraryGame]);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, getTicket, createTicket, unreadTickets, igdbIdsOf]) vi.mocked(mock).mockReset();
  libraryGames.set([]);
  toasts.set([]);
  currentUser.set(null);
  authState.set('bootstrapping');
  resetHistory();
});

const subjectField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement;
const bodyField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.bodyLabel')) }) as HTMLTextAreaElement;
const logsSwitch = () => screen.getByRole('switch', { name: msg('tickets.attachLogs') });
const topicBox = () => screen.getByRole('combobox', { name: msg('tickets.topicLabel') });
const restoreButton = () => screen.queryByRole('button', { name: msg('tickets.draftRestore') });
const notice = () => restoreButton()?.closest('.notice') ?? null;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));

async function open(params: Record<string, string>) {
  const view = renderDom(Support, params);
  await screen.findByRole('button', { name: msg('tickets.submit') });
  return view;
}

function shown() {
  return {
    topic: (topicBox().textContent ?? '').trim(),
    logs: logsSwitch().getAttribute('aria-checked'),
    subject: subjectField().value,
    body: bodyField().value,
    offered: notice() ? words(notice()) : null,
  };
}

/** A request the player typed through the list's "New request" and left. */
async function oldDraft(subject = 'Old matter') {
  const { user } = await open(PLAIN);
  if (subject) await user.type(subjectField(), subject);
  await user.type(bodyField(), 'Still broken.');
  cleanup();
}

const OLD = { topic: msg('tickets.topicInstall'), logs: 'true', subject: 'Old matter', body: 'Still broken.', offered: null };
const EMPTY = { topic: msg('tickets.topicInstall'), logs: 'true', subject: '', body: '', offered: null };
const BETA_FORM = { topic: msg('tickets.topicLauncher'), logs: 'true', subject: 'Beta 2.0: ', body: '' };
const OFFER = `${msg('tickets.draftKept', { subject: 'Old matter' })} ${msg('tickets.draftRestore')}`;
const SWAP_OFFER = `${msg('tickets.draftKept', { subject: 'Old matter' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`;

describe('1. a draft exists only once the player types or chooses content', () => {
  it('a prefill left untouched is not kept: the next way in shows its own prefill, the plain one nothing', async () => {
    await open(BETA);
    cleanup();
    await open(MOVE);
    expect(shown()).toEqual({ topic: msg('tickets.topicOther'), logs: 'false', subject: '', body: '', offered: null });
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual(EMPTY);
  });

  it('the logs switch and the topic are settings: a prefilled form changed only there keeps nothing', async () => {
    const { user } = await open(BETA);
    await user.click(logsSwitch());
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicOther') }));
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual(EMPTY);
  });

  it('a topic and a logs switch set before the first key are carried into the draft it makes', async () => {
    const { user } = await open(BETA);
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicOther') }));
    await user.click(logsSwitch());
    await user.type(bodyField(), 'Crashes on start.');
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual({ topic: msg('tickets.topicOther'), logs: 'true', subject: 'Beta 2.0: ', body: 'Crashes on start.', offered: null });
  });


  it('a field the player changes is an edit: the prefilled form, as it is then, is kept', async () => {
    const { user } = await open(MOVE);
    await user.type(bodyField(), 'Moved and gone.');
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual({ topic: msg('tickets.topicOther'), logs: 'false', subject: '', body: 'Moved and gone.', offered: null });
  });
});

describe('2. the support page tells a plain way in from a prefilled one', () => {
  it('passes a topic or a subject only when the way in carries one', async () => {
    await oldDraft();
    await open(PLAIN);
    expect(shown()).toEqual(OLD);
    cleanup();
    await open({ view: 'new', topic: 'launcher' });
    expect(shown().offered).toBe(OFFER);
  });
});

describe('3. a plain way in', () => {
  it('gives the draft back whole, with no notice', async () => {
    const { user } = await open(PLAIN);
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
    await user.click(logsSwitch());
    await user.type(subjectField(), 'Cannot sign in');
    await user.type(bodyField(), 'The code never arrives.');
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual({ topic: msg('tickets.topicAccount'), logs: 'true', subject: 'Cannot sign in', body: 'The code never arrives.', offered: null });
  });
});

describe('4. a prefilled way in while a draft is kept', () => {
  it('opens with the prefill and offers the draft by its subject, in one notice that is not an error', async () => {
    await oldDraft();
    await open(BETA);
    expect(shown()).toEqual({ ...BETA_FORM, offered: OFFER });
    expect(screen.queryAllByRole('alert')).toEqual([]);
    expect(notice()?.getAttribute('data-tone')).toBe('info');
    expect(notice()?.querySelector('[role]')).toBeNull();
    expect(screen.getAllByRole('button', { name: msg('tickets.draftRestore') })).toHaveLength(1);
  });

  it('says a draft without a subject is one', async () => {
    await oldDraft('');
    await open(BETA);
    expect(shown().offered).toBe(`${msg('tickets.draftKeptNoSubject')} ${msg('tickets.draftRestore')}`);
  });

  it('leaves the draft as it was when the player leaves without editing', async () => {
    await oldDraft();
    await open(BETA);
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual(OLD);
  });

  it('leaves the draft as it was when the player cancels without editing', async () => {
    await oldDraft();
    const { user } = await open(BETA);
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual(OLD);
  });

  it('"Restore it" swaps the fields for the draft, the notice leaves, focus goes to the subject', async () => {
    await oldDraft();
    const { user } = await open(BETA);
    screen.getByRole('button', { name: msg('tickets.draftRestore') }).focus();
    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(document.activeElement).toBe(subjectField()));
    expect(shown()).toEqual(OLD);
    expect(document.body.textContent).not.toContain('Beta 2.0');
  });

  it('after "Restore it" the form owns the draft: Cancel forgets it', async () => {
    await oldDraft();
    const { user } = await open(BETA);
    await user.click(screen.getByRole('button', { name: msg('tickets.draftRestore') }));
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual(EMPTY);
  });

  it('a logs click in a topic-only prefilled form leaves the older draft kept and offered', async () => {
    await oldDraft();
    const { user } = await open(MOVE);
    await user.click(logsSwitch());
    const offered = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ offered, later: shown() }).toEqual({ offered: OFFER, later: OLD });
  });

  it('a topic picked in the Beta form leaves the older draft kept and offered, never the prefill', async () => {
    await oldDraft();
    const { user } = await open(BETA);
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicOther') }));
    const offered = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ offered, later: shown() }).toEqual({ offered: OFFER, later: OLD });
  });

  it('a field edited after a topic pick makes the form the writer, the topic kept, and the old draft stays offered', async () => {
    await oldDraft();
    const { user } = await open(MOVE);
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
    await user.type(subjectField(), 'New matter');
    const offered = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ offered, later: shown() }).toEqual({
      offered: SWAP_OFFER,
      later: { topic: msg('tickets.topicAccount'), logs: 'false', subject: 'New matter', body: '', offered: SWAP_OFFER },
    });
  });

  it('an edit makes this form the draft and never drops the old one: it stays offered, now as a swap', async () => {
    await oldDraft();
    const { user } = await open(BETA);
    await user.type(bodyField(), 'Crashes on start.');
    const offered = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ offered, later: shown() }).toEqual({ offered: SWAP_OFFER, later: { ...BETA_FORM, body: 'Crashes on start.', offered: SWAP_OFFER } });
  });

  it('a request sent from the prefilled form forgets what it wrote, and the older draft stays kept', async () => {
    vi.mocked(createTicket).mockResolvedValue(ticket(30, 'Beta 2.0: crash'));
    await oldDraft();
    const { user } = await open(BETA);
    await user.type(subjectField(), 'crash');
    await user.type(bodyField(), 'Crashes on start.');
    if (logsSwitch().getAttribute('aria-checked') === 'true') await user.click(logsSwitch());
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    cleanup();
    await open(PLAIN);
    expect(shown()).toEqual(OLD);
  });
});

describe('a failed send and the ways in', () => {
  it('a request that fails after its form was left is offered by a prefilled form open untouched', async () => {
    const first = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(first.promise);
    const { user } = await open(PLAIN);
    await user.type(subjectField(), 'Old matter');
    await user.type(bodyField(), 'Still broken.');
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('tickets.logsConfirmButton') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    cleanup();
    await open(BETA);
    expect(shown().offered).toBeNull();
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(shown()).toEqual({ ...BETA_FORM, offered: OFFER }));
    expect(get(toasts).map((t) => [t.message, t.action?.label ?? null])).toEqual([[msg('tickets.requestNotSentOffered', { subject: 'Old matter' }), msg('tickets.copyText')]]);
    expect({ polite: notice()?.querySelectorAll('[role="status"]').length, alerts: screen.queryAllByRole('alert').length }).toEqual({ polite: 1, alerts: 0 });
  });

  it('"Restore it" is described by the notice\'s sentence', async () => {
    await oldDraft();
    await open(BETA);
    const described = () => {
      const ids = restoreButton()?.getAttribute('aria-describedby') ?? '';
      return ids
        .split(' ')
        .map((id) => document.getElementById(id)?.textContent?.replace(/\s+/g, ' ').trim())
        .join(' ');
    };
    expect(described()).toBe(msg('tickets.draftKept', { subject: 'Old matter' }));
  });

  it('a key on the subject of a prefilled form is an edit even when it changes nothing visible', async () => {
    await oldDraft();
    await open(BETA);
    const before = shown().offered;
    fireEvent.input(subjectField(), { target: { value: 'Beta 2.0: ' } });
    await settled();
    const after = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ before, after, later: shown() }).toEqual({ before: OFFER, after: SWAP_OFFER, later: { ...BETA_FORM, offered: SWAP_OFFER } });
  });

  it('a request that fails while a prefilled form is open untouched stays offered after a key there and after the form is left', async () => {
    const first = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(first.promise);
    const { user } = await open(PLAIN);
    await user.type(subjectField(), 'Old matter');
    await user.type(bodyField(), 'Still broken.');
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('tickets.logsConfirmButton') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const beta = await open(BETA);
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(notice()).not.toBeNull());
    await beta.user.type(subjectField(), 'x');
    const afterKey = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ afterKey, later: shown() }).toEqual({ afterKey: SWAP_OFFER, later: { ...BETA_FORM, subject: 'Beta 2.0: x', offered: SWAP_OFFER } });
  });
});

describe('the writer rule, pinned', () => {
  async function pickGame(user: ReturnType<typeof renderDom>['user']) {
    await user.click(screen.getByRole('button', { name: msg('tickets.gameChoose') }));
    await user.click(await screen.findByRole('button', { name: 'A game' }));
  }
  const chosenGame = () => document.querySelector('.chosen-title')?.textContent ?? null;

  /** A request typed in the plain form, sent and left on its way. */
  async function onItsWay() {
    const first = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(first.promise);
    const { user } = await open(PLAIN);
    await user.type(subjectField(), 'Old matter');
    await user.type(bodyField(), 'Still broken.');
    await user.click(logsSwitch());
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    cleanup();
    return first;
  }

  it('a game picked in a prefilled form makes it the writer: the game is kept, the older draft stays offered', async () => {
    await oldDraft();
    const { user } = await open(MOVE);
    await pickGame(user);
    cleanup();
    await open(PLAIN);
    expect({ game: chosenGame(), offered: shown().offered }).toEqual({ game: 'A game', offered: SWAP_OFFER });
  });

  it('a topic changed once the form is the writer is carried into the draft, the logs switch set by hand', async () => {
    const { user } = await open(PLAIN);
    await user.type(subjectField(), 'Cannot sign in');
    // Set by hand, the switch stays as it is on a topic change: only the topic changes then.
    await user.click(logsSwitch());
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
    cleanup();
    await open(PLAIN);
    expect(shown().topic).toBe(msg('tickets.topicAccount'));
  });

  it('a form that holds only a game is not free: a request that comes back is offered beside it', async () => {
    const first = await onItsWay();
    const { user } = await open(PLAIN);
    await pickGame(user);
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(notice()).not.toBeNull());
    expect({ game: chosenGame(), subject: subjectField().value, said: get(toasts).map((t) => t.message) }).toEqual({
      game: 'A game',
      subject: '',
      said: [msg('tickets.requestNotSentOffered', { subject: 'Old matter' })],
    });
  });

  it('a form that holds nothing and only offers another request takes the one that comes back into its fields', async () => {
    const first = await onItsWay();
    await oldDraft('Offered matter');
    await open(MOVE);
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(subjectField().value).toBe('Old matter'));
    const offered = shown().offered;
    cleanup();
    await open(PLAIN);
    expect({ offered, said: get(toasts).map((t) => t.message), later: [subjectField().value, shown().offered] }).toEqual({
      offered: `${msg('tickets.draftKept', { subject: 'Offered matter' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`,
      said: [msg('tickets.requestNotSentKept', { subject: 'Old matter' })],
      later: ['Old matter', `${msg('tickets.draftKept', { subject: 'Offered matter' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`],
    });
  });

  it('a form that holds nothing takes no text that would push a kept one out: with both places taken it is kept nowhere', async () => {
    const first = await onItsWay();
    await oldDraft('Spare matter');
    const beta = await open(BETA);
    await beta.user.type(bodyField(), 'Beta text.');
    cleanup();
    await open(MOVE);
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(get(toasts)).toHaveLength(1));
    const here = { subject: subjectField().value, said: get(toasts).map((t) => [t.message, t.action?.label ?? null]) };
    cleanup();
    await open(PLAIN);
    expect({ here, later: [subjectField().value, bodyField().value, shown().offered] }).toEqual({
      here: { subject: '', said: [[msg('tickets.requestNotSentNotKept', { subject: 'Old matter' }), msg('tickets.copyText')]] },
      later: ['Beta 2.0: ', 'Beta text.', `${msg('tickets.draftKept', { subject: 'Spare matter' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`],
    });
  });
});
