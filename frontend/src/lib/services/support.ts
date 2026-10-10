import { Service as SupportService } from '../../../bindings/typhon/internal/support';
import type { Limits, Message, Ticket } from '../../../bindings/typhon/internal/support';
import { errorCode } from '../i18n';
import { AccountError, toAccountError } from './account';
import { inWails } from './backend';

export type { Limits, Message, Ticket };

export const SUPPORT_TOPICS = ['install', 'launch', 'account', 'launcher', 'other'] as const;
export type SupportTopic = (typeof SUPPORT_TOPICS)[number];

export const SUPPORT_STATUSES = ['open', 'waiting_user', 'resolved'] as const;
export type SupportStatus = (typeof SUPPORT_STATUSES)[number];

export interface TicketPage {
  tickets: Ticket[];
  next: string;
}

export interface TicketThread {
  ticket: Ticket;
  messages: Message[];
}

export interface NewTicket {
  topic: SupportTopic;
  subject: string;
  body: string;
  gameIgdbId: number | null;
  attachLogs: boolean;
}

const unauthenticated = () => new AccountError('unauthenticated');

function failure(err: unknown): unknown {
  return errorCode(err) ? err : toAccountError(err);
}

export function emptyTicketPage(): TicketPage {
  return { tickets: [], next: '' };
}

export async function ticketLimits(): Promise<Limits> {
  if (!inWails) throw unauthenticated();
  try {
    return await SupportService.Limits();
  } catch (err) {
    throw failure(err);
  }
}

export async function listTickets(cursor = ''): Promise<TicketPage> {
  if (!inWails) return emptyTicketPage();
  try {
    const page = await SupportService.List(cursor);
    return { tickets: page?.tickets ?? [], next: page?.next ?? '' };
  } catch (err) {
    throw failure(err);
  }
}

export async function getTicket(number: number): Promise<TicketThread> {
  if (!inWails) throw unauthenticated();
  try {
    const detail = await SupportService.Get(number);
    return { ticket: detail.ticket, messages: detail.messages ?? [] };
  } catch (err) {
    throw failure(err);
  }
}

export async function createTicket(input: NewTicket): Promise<Ticket> {
  if (!inWails) throw unauthenticated();
  try {
    return await SupportService.Create(input);
  } catch (err) {
    throw failure(err);
  }
}

export async function replyTicket(number: number, body: string, attachLogs: boolean): Promise<Message> {
  if (!inWails) throw unauthenticated();
  try {
    return await SupportService.Reply(number, body, attachLogs);
  } catch (err) {
    throw failure(err);
  }
}

export async function resolveTicket(number: number): Promise<Ticket> {
  if (!inWails) throw unauthenticated();
  try {
    return await SupportService.Resolve(number);
  } catch (err) {
    throw failure(err);
  }
}

export async function unreadTickets(): Promise<number> {
  if (!inWails) return 0;
  try {
    return await SupportService.Unread();
  } catch (err) {
    throw failure(err);
  }
}
