import { Service as MessagingService } from '../../../bindings/typhon/internal/messaging';
import { inWails } from './backend';

export interface ChatPeer {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string;
}

export interface Reaction {
  emoji: string;
  userIds: string[];
}

export interface Message {
  id: string;
  clientId: string;
  senderId: string;
  recipientId: string;
  text: string;
  createdAt: string;
  editedAt?: string | null;
  expiresAt: string;
  reactions: Reaction[];
}

export interface Conversation {
  peer: ChatPeer;
  lastMessage?: Message | null;
  unread: number;
  canSend: boolean;
}

export interface MessagePage {
  messages: Message[];
  next: string;
  canSend?: boolean;
}

export type ChatEventKind = 'sync' | 'message' | 'updated' | 'read' | 'typing' | 'connection';

export interface ChatEvent {
  ownerId: string;
  kind: ChatEventKind;
  peerId: string;
  message?: Message;
  typing?: boolean;
  connected?: boolean;
}

export const MAX_MESSAGE_LENGTH = 2000;
export const HISTORY_DAYS = 7;
export const REACTION_KEYS = ['fire', 'salute', 'heart', 'clap', 'skull', 'party', 'eyes', 'joy'] as const;
export type ReactionKey = (typeof REACTION_KEYS)[number];

export const REACTION_GLYPHS: Record<ReactionKey, string> = {
  fire: '🔥',
  salute: '🫡',
  heart: '❤️',
  clap: '👏',
  skull: '💀',
  party: '🎉',
  eyes: '👀',
  joy: '😂',
};

export function isMessageAlive(message: Message, now = Date.now()): boolean {
  const created = Date.parse(message.createdAt);
  const expires = Date.parse(message.expiresAt);
  const cutoff = now - HISTORY_DAYS * 24 * 60 * 60 * 1000;
  return (Number.isNaN(expires) ? (Number.isNaN(created) || created >= cutoff) : expires > now) &&
    (Number.isNaN(created) || created >= cutoff);
}

export function normalizeMessage(value: Message): Message {
  return {
    ...value,
    reactions: (value.reactions ?? []).map((reaction) => ({
      emoji: reaction.emoji,
      userIds: reaction.userIds ?? [],
    })),
  };
}

function unavailable(): never {
  throw new Error('messaging_unavailable');
}

export async function conversations(): Promise<Conversation[]> {
  if (!inWails) return [];
  return (((await MessagingService.Conversations()) as unknown as Conversation[] | null) ?? []).map((conversation) => ({
    ...conversation,
    lastMessage: conversation.lastMessage ? normalizeMessage(conversation.lastMessage) : null,
  }));
}

export async function messages(peerId: string, before = ''): Promise<MessagePage> {
  if (!inWails) return { messages: [], next: '', canSend: false };
  const page = (await MessagingService.Messages(peerId, before)) as unknown as Partial<MessagePage> | null;
  return {
    messages: (page?.messages ?? []).map(normalizeMessage),
    next: page?.next ?? '',
    canSend: page?.canSend,
  };
}

export async function send(peerId: string, clientId: string, text: string): Promise<Message> {
  if (!inWails) return unavailable();
  return (await MessagingService.Send(peerId, clientId, text)) as unknown as Message;
}

export async function edit(peerId: string, messageId: string, text: string): Promise<Message> {
  if (!inWails) return unavailable();
  return (await MessagingService.Edit(peerId, messageId, text)) as unknown as Message;
}

export async function react(peerId: string, messageId: string, emoji: ReactionKey): Promise<void> {
  if (!inWails) return unavailable();
  await MessagingService.React(peerId, messageId, emoji);
}

export async function unreact(peerId: string, messageId: string, emoji: ReactionKey): Promise<void> {
  if (!inWails) return unavailable();
  await MessagingService.Unreact(peerId, messageId, emoji);
}

export async function read(peerId: string, messageId: string): Promise<void> {
  if (!inWails) return;
  await MessagingService.Read(peerId, messageId);
}

export async function typing(peerId: string, value: boolean): Promise<void> {
  if (!inWails) return;
  await MessagingService.Typing(peerId, value);
}

export async function start(): Promise<void> {
  if (!inWails) return;
  await MessagingService.Start();
}

export async function stop(): Promise<void> {
  if (!inWails) return;
  await MessagingService.Stop();
}

export async function notify(peerId: string, title: string, body: string): Promise<boolean> {
  if (!inWails) return false;
  return Boolean(await MessagingService.Notify(peerId, title, body));
}
