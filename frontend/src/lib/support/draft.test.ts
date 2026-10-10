import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/support', () => ({
  createTicket: vi.fn(),
  replyTicket: vi.fn(),
}));

import { ru } from '../i18n/catalog/ru';
import { createTicket, replyTicket } from '../services/support';
import type { NewTicket, Ticket } from '../services/support';
import { announceCreated, hasErrors, sendNewTicket, sendReply, validateBody, validateDraft } from './draft';

const create = vi.mocked(createTicket);
const reply = vi.mocked(replyTicket);

const limits = { maxSubjectRunes: 10, maxBodyRunes: 20 };

const draft: NewTicket = {
  topic: 'install',
  subject: '  Не ставится  ',
  body: '  Ошибка на 90%  ',
  gameIgdbId: 7,
  attachLogs: false,
};

beforeEach(() => {
  create.mockReset();
  reply.mockReset();
  create.mockResolvedValue({ number: 1, ref: 'T-1' } as Ticket);
  reply.mockResolvedValue({ id: 1 } as never);
});

describe('validateDraft', () => {
  it('accepts a filled form', () => {
    expect(validateDraft({ subject: 'Сбой', body: 'Ошибка' }, limits)).toEqual({});
  });

  it('asks for a subject and a body, whitespace does not count', () => {
    const errors = validateDraft({ subject: '   ', body: '\n\t ' }, limits);

    expect(errors.subject).toBe(ru['tickets.errSubjectRequired']);
    expect(errors.body).toBe(ru['tickets.errBodyRequired']);
    expect(hasErrors(errors)).toBe(true);
  });

  it('applies the limits Go reported and names them', () => {
    const errors = validateDraft({ subject: 'x'.repeat(11), body: 'y'.repeat(21) }, limits);

    expect(errors.subject).toBe('Тема не длиннее 10 символов.');
    expect(errors.body).toBe('Сообщение не длиннее 20 символов.');
  });

  it('allows exactly the limit, measured after trimming', () => {
    expect(validateDraft({ subject: ` ${'x'.repeat(10)} `, body: 'y'.repeat(20) }, limits)).toEqual({});
  });

  it('counts characters, not UTF-16 units, the way Go counts runes', () => {
    expect(validateDraft({ subject: '😀'.repeat(10), body: 'ok' }, limits)).toEqual({});
    expect(validateDraft({ subject: '😀'.repeat(11), body: 'ok' }, limits).subject).toBeTruthy();
  });

  it('reports the error of one field without the other', () => {
    const errors = validateDraft({ subject: 'ok', body: '' }, limits);

    expect(errors.subject).toBeUndefined();
    expect(hasErrors(errors)).toBe(true);
    expect(hasErrors({})).toBe(false);
  });
});

const around = (code: number) => `one${String.fromCodePoint(code)}two`;

describe('characters the server refuses', () => {
  it.each([
    ['tab', 0x09],
    ['nul', 0x00],
    ['escape', 0x1b],
    ['delete', 0x7f],
    ['c1 control', 0x85],
    ['soft hyphen', 0xad],
    ['zero width space', 0x200b],
    ['zero width non-joiner', 0x200c],
    ['zero width joiner', 0x200d],
    ['left-to-right mark', 0x200e],
    ['line separator', 0x2028],
    ['paragraph separator', 0x2029],
    ['right-to-left override', 0x202e],
    ['word joiner', 0x2060],
    ['bidi isolate', 0x2066],
    ['byte order mark', 0xfeff],
    ['tag character', 0xe0041],
  ])('a subject with a %s is refused', (_name, code) => {
    const subject = validateDraft({ subject: around(code), body: 'ok' }, limits).subject;
    expect(subject).toBeTruthy();
    expect(subject).toBe(ru['tickets.errSubjectChars']);
  });

  it.each([
    ['nul', 0x00],
    ['escape', 0x1b],
    ['form feed', 0x0c],
    ['delete', 0x7f],
    ['c1 control', 0x85],
    ['line separator', 0x2028],
    ['paragraph separator', 0x2029],
    ['left-to-right embedding', 0x202a],
    ['right-to-left override', 0x202e],
    ['left-to-right isolate', 0x2066],
    ['pop directional isolate', 0x2069],
    ['byte order mark', 0xfeff],
  ])('a body with a %s is refused', (_name, code) => {
    const body = validateDraft({ subject: 'ok', body: around(code) }, limits).body;
    expect(body).toBeTruthy();
    expect(body).toBe(ru['tickets.errBodyChars']);
    expect(validateBody(around(code), limits)).toBe(ru['tickets.errBodyChars']);
  });

  it.each([
    ['tab', 0x09],
    ['line feed', 0x0a],
    ['zero width joiner', 0x200d],
    ['zero width non-joiner', 0x200c],
    ['variation selector', 0xfe0f],
    ['no-break space', 0xa0],
    ['soft hyphen', 0xad],
  ])('a body with a %s is fine', (_name, code) => {
    expect(validateBody(around(code), limits)).toBe('');
  });

  it.each([
    ['no-break space', 0xa0],
    ['em space', 0x2003],
  ])('a subject with a %s is fine', (_name, code) => {
    expect(validateDraft({ subject: around(code), body: 'ok' }, limits).subject).toBeUndefined();
  });

  it('counts a Windows line break as one character, the way the server does after folding it', () => {
    const body = 'a\r\n'.repeat(8) + 'b';
    expect(validateBody(body, limits)).toBe('');
    expect(validateBody(`${'a\r\n'.repeat(10)}bb`, limits)).toBe(ru['tickets.errBodyTooLong'].replace('{max}', '20'));
  });

  it('reports a bad character before the length', () => {
    const body = `${'y'.repeat(30)}${String.fromCodePoint(0x202e)}`;
    expect(validateBody(body, limits)).toBe(ru['tickets.errBodyChars']);
  });
});

describe('validateBody', () => {
  it('is empty for a good reply and a message for a bad one', () => {
    expect(validateBody('ok', limits)).toBe('');
    expect(validateBody('  ', limits)).toBe(ru['tickets.errBodyRequired']);
    expect(validateBody('z'.repeat(21), limits)).toBe(ru['tickets.errBodyTooLong'].replace('{max}', '20'));
  });
});

describe('announceCreated', () => {
  it('tells the user and opens the request when the form is still on screen', () => {
    const notify = vi.fn();
    const open = vi.fn();

    announceCreated({ number: 42, ref: 'T-42' }, true, { notify, open });

    expect(notify).toHaveBeenCalledWith(ru['tickets.created'].replace('{ref}', 'T-42'));
    expect(open).toHaveBeenCalledWith(42);
  });

  it('only tells the user when they have already left the form', () => {
    const notify = vi.fn();
    const open = vi.fn();

    announceCreated({ number: 42, ref: 'T-42' }, false, { notify, open });

    expect(notify).toHaveBeenCalledTimes(1);
    expect(open).not.toHaveBeenCalled();
  });
});

describe('sendNewTicket', () => {
  it('sends without asking anything when no logs are attached', async () => {
    const confirm = vi.fn(async () => true);

    const outcome = await sendNewTicket(draft, confirm);

    expect(confirm).not.toHaveBeenCalled();
    expect(create).toHaveBeenCalledWith({ ...draft, subject: 'Не ставится', body: 'Ошибка на 90%', attachLogs: false });
    expect(outcome).toEqual({ sent: true, value: { number: 1, ref: 'T-1' } });
  });

  it('shows the confirmation before the logs leave and sends with them after a yes', async () => {
    const confirm = vi.fn(async () => {
      expect(create).not.toHaveBeenCalled();
      return true;
    });

    const outcome = await sendNewTicket({ ...draft, attachLogs: true }, confirm);

    expect(confirm).toHaveBeenCalledTimes(1);
    expect(create).toHaveBeenCalledTimes(1);
    expect(create.mock.calls[0][0].attachLogs).toBe(true);
    expect(outcome.sent).toBe(true);
  });

  it('sends nothing at all when the user cancels the confirmation', async () => {
    const outcome = await sendNewTicket({ ...draft, attachLogs: true }, async () => false);

    expect(create).not.toHaveBeenCalled();
    expect(outcome).toEqual({ sent: false });
  });

  it('lets the user send without logs only by unchecking the box', async () => {
    const confirm = vi.fn(async () => false);

    await sendNewTicket({ ...draft, attachLogs: false }, confirm);

    expect(confirm).not.toHaveBeenCalled();
    expect(create.mock.calls[0][0].attachLogs).toBe(false);
  });

  it('hands a failure to the caller instead of reporting a send', async () => {
    const failure = new Error('ticket_limit');
    create.mockRejectedValue(failure);

    await expect(sendNewTicket(draft, async () => true)).rejects.toBe(failure);
  });

  it('does not send when the confirmation itself fails', async () => {
    const failure = new Error('modal broke');

    await expect(sendNewTicket({ ...draft, attachLogs: true }, async () => Promise.reject(failure))).rejects.toBe(failure);
    expect(create).not.toHaveBeenCalled();
  });
});

describe('sendReply', () => {
  it('sends a plain reply without a prompt', async () => {
    const confirm = vi.fn(async () => true);

    const outcome = await sendReply(5, '  ещё раз  ', false, confirm);

    expect(confirm).not.toHaveBeenCalled();
    expect(reply).toHaveBeenCalledWith(5, 'ещё раз', false);
    expect(outcome.sent).toBe(true);
  });

  it('attaches the logs only after the confirmation', async () => {
    const outcome = await sendReply(5, 'ещё раз', true, async () => true);

    expect(reply).toHaveBeenCalledWith(5, 'ещё раз', true);
    expect(outcome.sent).toBe(true);
  });

  it('sends nothing when the confirmation is cancelled', async () => {
    const outcome = await sendReply(5, 'ещё раз', true, async () => false);

    expect(reply).not.toHaveBeenCalled();
    expect(outcome).toEqual({ sent: false });
  });

  it('hands a failure to the caller', async () => {
    const failure = new Error('ticket_not_found');
    reply.mockRejectedValue(failure);

    await expect(sendReply(5, 'x', false, async () => true)).rejects.toBe(failure);
  });
});
