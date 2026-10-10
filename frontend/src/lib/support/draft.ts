import { msg } from '../i18n';
import { runeCount } from '../profile/layout';
import { createTicket, replyTicket } from '../services/support';
import type { Limits, Message, NewTicket, Ticket } from '../services/support';

export interface DraftErrors {
  subject?: string;
  body?: string;
}

export type SendOutcome<T> = { sent: true; value: T } | { sent: false };

export type ConfirmLogs = () => Promise<boolean>;

const LINE_INVISIBLE = /[\p{Cc}\p{Zl}\p{Zp}\p{Cf}]/u;
const CONTROL = /\p{Cc}/u;

function foldBreaks(text: string): string {
  return text.replace(/\r\n?/g, '\n').trim();
}

function hasInvisibleInBody(text: string): boolean {
  for (const char of text) {
    const code = char.codePointAt(0) ?? 0;
    if (code === 0x0a || code === 0x09) continue;
    if (CONTROL.test(char)) return true;
    if ((code >= 0x202a && code <= 0x202e) || (code >= 0x2066 && code <= 0x2069)) return true;
    if (code === 0x2028 || code === 0x2029 || code === 0xfeff) return true;
  }
  return false;
}

export function bodyLength(body: string): number {
  return runeCount(foldBreaks(body));
}

export function validateBody(body: string, limits: Limits): string {
  const folded = foldBreaks(body);
  const length = runeCount(folded);
  if (length < 1) return msg('tickets.errBodyRequired');
  if (hasInvisibleInBody(folded)) return msg('tickets.errBodyChars');
  if (length > limits.maxBodyRunes) return msg('tickets.errBodyTooLong', { max: limits.maxBodyRunes });
  return '';
}

export function validateDraft(draft: { subject: string; body: string }, limits: Limits): DraftErrors {
  const errors: DraftErrors = {};
  const trimmed = draft.subject.trim();
  const subject = runeCount(trimmed);
  if (subject < 1) errors.subject = msg('tickets.errSubjectRequired');
  else if (LINE_INVISIBLE.test(trimmed)) errors.subject = msg('tickets.errSubjectChars');
  else if (subject > limits.maxSubjectRunes) {
    errors.subject = msg('tickets.errSubjectTooLong', { max: limits.maxSubjectRunes });
  }
  const body = validateBody(draft.body, limits);
  if (body) errors.body = body;
  return errors;
}

export function hasErrors(errors: DraftErrors): boolean {
  return Boolean(errors.subject || errors.body);
}

export async function sendNewTicket(input: NewTicket, confirmLogs: ConfirmLogs): Promise<SendOutcome<Ticket>> {
  if (input.attachLogs && !(await confirmLogs())) return { sent: false };
  const value = await createTicket({
    ...input,
    subject: input.subject.trim(),
    body: input.body.trim(),
  });
  return { sent: true, value };
}

export function announceCreated(
  ticket: Pick<Ticket, 'number' | 'ref'>,
  formOnScreen: boolean,
  deps: { notify: (text: string) => void; open: (number: number) => void },
): void {
  deps.notify(msg('tickets.created', { ref: ticket.ref }));
  if (formOnScreen) deps.open(ticket.number);
}

export async function sendReply(
  number: number,
  body: string,
  attachLogs: boolean,
  confirmLogs: ConfirmLogs,
): Promise<SendOutcome<Message>> {
  if (attachLogs && !(await confirmLogs())) return { sent: false };
  const value = await replyTicket(number, body.trim(), attachLogs);
  return { sent: true, value };
}
