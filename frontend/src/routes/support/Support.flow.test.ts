import { describe, expect, it } from 'vitest';
import { en } from '../../lib/i18n/catalog/en';
import { ru } from '../../lib/i18n/catalog/ru';
import { frontendSources, sourcesUnder } from '../../lib/testing/sources';
import newTicket from './NewTicket.svelte?raw';
import ticketView from './TicketView.svelte?raw';
import ticketList from './TicketList.svelte?raw';
import support from './Support.svelte?raw';
import logsConfirm from './LogsConfirm.svelte?raw';
import app from '../../App.svelte?raw';
import sidebar from '../../lib/components/Sidebar.svelte?raw';
import auth from '../auth/AuthScreen.svelte?raw';
import settings from '../settings/Settings.svelte?raw';
import moveModal from '../../lib/components/MoveGameModal.svelte?raw';
import notifications from '../../lib/stores/notifications.ts?raw';

const screens = { newTicket, ticketView, ticketList, support, logsConfirm };

describe('support screens render user text as text', () => {
  it.each(Object.entries(screens))('%s never injects markup', (_name, source) => {
    expect(source).not.toContain('{@html');
    expect(source).not.toContain('innerHTML');
  });

  it('shows message bodies with preserved line breaks and automatic direction', () => {
    expect(ticketView).toMatch(/<p class="text" dir="auto">\{message\.body\}<\/p>/);
    expect(ticketView).toMatch(/\.text \{[^}]*white-space: pre-wrap;/);
  });

  it('gives the subject the same direction handling in the list and the header', () => {
    expect(ticketList).toMatch(/<span class="subject" dir="auto">\{item\.subject\}<\/span>/);
    expect(ticketView).toMatch(/<h1 dir="auto">\{ticket\.subject\}<\/h1>/);
  });

  it('marks a message that carries a log bundle', () => {
    expect(ticketView).toContain('message.logBundleId');
    expect(ticketView).toContain("msg('tickets.logsAttached')");
  });

  it('tells the user from the support staff by the author field', () => {
    expect(ticketView).toContain("message.author === 'user'");
  });
});

describe('attaching logs goes through the existing confirmation', () => {
  it('asks with its own prompt, whose retention period is the one of a ticket', () => {
    expect(logsConfirm).toContain('ticketLogsPrompt()');
    expect(logsConfirm).not.toContain('sendLogsPrompt');
    expect(logsConfirm).toContain('<ConfirmModal');
  });

  it('routes new tickets and replies through the guarded send helpers', () => {
    expect(newTicket).toContain('sendNewTicket(');
    expect(newTicket).toContain('confirm.ask()');
    expect(ticketView).toContain('sendReply(');
    expect(ticketView).toContain('confirm.ask()');
  });

  it('never calls the create or reply binding directly from a screen', () => {
    for (const source of [newTicket, ticketView]) {
      expect(source).not.toMatch(/createTicket\(|replyTicket\(/);
    }
  });

  it('checks the logs box by default for a new ticket and leaves it off for a reply', () => {
    expect(newTicket).toContain('attachLogsByDefault(');
    expect(ticketView).toContain('let attachLogs = $state(false)');
  });

  it('does not pull the user back to a request when they have left the form', () => {
    expect(newTicket).toContain('announceCreated(outcome.value, onScreen');
    expect(newTicket).toMatch(/return \(\) => \{\s*onScreen = false;/);
    expect(newTicket).not.toMatch(/navigate\('support', \{ ticket: String\(outcome/);
  });

  it('rereads the open request when a new answer arrives', () => {
    expect(ticketView).toContain('unreadGrew(lastUnread, count)');
    expect(ticketView).toContain('$unreadReplies');
  });

  it('spaces the support button of the move dialog from the failure text', () => {
    expect(moveModal).toMatch(/\.support \{\s*margin-top: var\(--space-2\);/);
  });

  it('keeps the form from being sent twice', () => {
    expect(newTicket).toMatch(/async function submit\(\) \{\s*if \(sending/);
    expect(ticketView).toMatch(/async function submitReply\(\) \{\s*if \(sending/);
  });
});

describe('errors reach the user', () => {
  it.each([
    ['newTicket', newTicket],
    ['ticketView', ticketView],
    ['ticketList', ticketList],
  ])('%s maps failures through supportErrorText and has no empty catch', (_name, source) => {
    expect(source).toContain('supportErrorText(');
    expect(source).not.toMatch(/catch\s*(\([^)]*\))?\s*\{\s*\}/);
  });

  it('keeps the form fields tied to the field the backend named', () => {
    expect(newTicket).toContain('accountErrorField(err)');
  });
});

describe('where the section is reachable from', () => {
  it('renders in the app shell and starts the unread polling', () => {
    expect(app).toContain("$route.name === 'support'");
    expect(app).toMatch(/onDestroy\(initSupport\(\)\)/);
  });

  it('has a sidebar entry with the unread badge', () => {
    expect(sidebar).toContain("name: 'support'");
    expect(sidebar).toContain('$unreadReplies');
  });

  it('puts the unread replies into the notification list, pointing to the section', () => {
    expect(notifications).toContain("route: 'support'");
  });

  it('offers a forgot-password link on the sign-in form only', () => {
    expect(auth).toContain('openForgotPassword');
    expect(auth).toMatch(/\{#if !isRegister\}\s*<button class="link forgot"/);
  });

  it('offers the site support page to a guest and on the sign-in form, and tells the user when it cannot open', () => {
    expect(support).toContain('openSupportSite');
    expect(support).toMatch(/<Button onclick=\{writeFromSite\}>\{msg\('tickets\.cantSignIn'\)\}<\/Button>/);
    expect(support).toMatch(/catch \(err\) \{[^}]*toast\(msg\('tickets\.siteOpenFailed'\), 'danger'\)/);
    expect(auth).toContain('openSupportSite');
    expect(auth).toMatch(/onclick=\{onCantSignIn\}>\{msg\('tickets\.cantSignIn'\)\}/);
    expect(auth).toMatch(/catch \(err\) \{[^}]*toast\(msg\('tickets\.siteOpenFailed'\), 'danger'\)/);
    expect(auth).toMatch(/catch \(err\) \{[^}]*toast\(msg\('tickets\.forgotOpenFailed'\), 'danger'\)/);
  });

  it('offers a support button next to the logs in the settings, with the launcher topic', () => {
    expect(settings).toContain("navigate('support', { view: 'new', topic: 'launcher' })");
  });

  it('offers the section from a move that needs manual review', () => {
    expect(moveModal).toContain("'relocate.ambiguous_recovery'");
    expect(moveModal).toContain("navigate('support', { view: 'new', topic: 'other' })");
  });

  it('only navigates to the support route with params the screen reads', () => {
    const used = new Set<string>();
    for (const source of Object.values(frontendSources)) {
      for (const match of source.matchAll(/navigate\('support',\s*\{([^}]*)\}/g)) {
        for (const key of match[1].matchAll(/(\w+):/g)) used.add(key[1]);
      }
    }
    expect([...used].sort()).toEqual(['ticket', 'topic', 'view']);
    expect(app).toContain('ticket={$route.params.ticket}');
    expect(app).toContain('view={$route.params.view}');
    expect(app).toContain('topic={$route.params.topic}');
  });

  it('keeps every support screen in one folder', () => {
    expect(Object.keys(sourcesUnder('routes/support/')).sort()).toEqual([
      'routes/support/AttachLogs.svelte',
      'routes/support/LogsConfirm.svelte',
      'routes/support/NewTicket.svelte',
      'routes/support/Support.svelte',
      'routes/support/TicketList.svelte',
      'routes/support/TicketView.svelte',
    ]);
  });
});

describe('old dead-end wording points to the new section', () => {
  const keys = [
    'state.accountErrorRequestBlocked',
    'transfers.moveErrAmbiguousRecovery',
    'settings.aboutLogsSendFailedHint',
  ] as const;

  it.each(keys)('%s names the Support section in both languages', (key) => {
    expect(ru[key]).toContain('«Поддержка»');
    expect(en[key]).toContain('Support section');
  });
});
