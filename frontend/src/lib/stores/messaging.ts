import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { needsSocialConsent } from './social';
import { authState, currentUser } from './user';
import { presenceStatus } from './presence';
import { msg } from '../i18n';
import {
  HISTORY_DAYS,
  isMessageAlive,
  messages as fetchMessages,
  conversations as fetchConversations,
  notify,
  read,
  send as sendMessageCall,
  edit as editMessageCall,
  react as reactMessageCall,
  unreact as unreactMessageCall,
  start,
  stop,
  typing as typingCall,
  type ChatEvent,
  type ChatPeer,
  type Conversation,
  type Message,
  type MessagePage,
  type ReactionKey,
  normalizeMessage,
} from '../services/messaging';

export const conversations = writable<Conversation[]>([]);
export const messagesByPeer = writable<Record<string, Message[]>>({});
export const nextByPeer = writable<Record<string, string>>({});
export const activePeer = writable<ChatPeer | null>(null);
export const chatOpen = writable(false);
export const chatLoading = writable(false);
export const chatLoadingMore = writable(false);
export const chatConnected = writable(true);
export const chatTyping = writable<Record<string, boolean>>({});
export const chatHistoryError = writable('');
export const canSendByPeer = writable<Record<string, boolean>>({});
export const chatToast = writable<{ id: number; peer: ChatPeer; message: Message } | null>(null);
export const chatPanelVisible = writable(false);
export const pendingMessages = writable<Set<string>>(new Set());
export const failedMessages = writable<Set<string>>(new Set());
export const editingMessageId = writable<string | null>(null);

export const unreadCount = derived(conversations, ($conversations) =>
  $conversations.reduce((sum, item) => sum + Math.max(0, item.unread), 0),
);

const DRAFT_PREFIX = 'typhon.chat.draft';
const TYPING_EXPIRY_MS = 6000;
const TYPING_THROTTLE_MS = 3000;
let started = false;
let generation = 0;
let authKey = '';
let toastId = 0;
let panelVisible = false;
let typingTimers = new Map<string, ReturnType<typeof setTimeout>>();
let typingSentAt = new Map<string, number>();
let notifyIds = new Set<string>();
let messageLoad = new Map<string, number>();
let expiryTimer: ReturnType<typeof setInterval> | null = null;
let conversationRefreshTimer: ReturnType<typeof setInterval> | null = null;
let startRetryTimer: ReturnType<typeof setTimeout> | null = null;
let unreadMessageIds = new Map<string, Set<string>>();
let messageRevisions = new Map<string, number>();
let sessionQueue = Promise.resolve();
let sessionSerial = 0;

export function draftKey(peerId: string, accountId = get(currentUser)?.id ?? ''): string {
  return `${DRAFT_PREFIX}:${accountId}:${peerId}`;
}

export function getDraft(peerId: string): string {
  if (typeof localStorage === 'undefined') return '';
  try {
    return localStorage.getItem(draftKey(peerId)) ?? '';
  } catch {
    return '';
  }
}

export function setDraft(peerId: string, value: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    if (value) localStorage.setItem(draftKey(peerId), value);
    else localStorage.removeItem(draftKey(peerId));
  } catch {
    // Drafts are a convenience and may be unavailable in private storage.
  }
}

function sortMessages(list: Message[]): Message[] {
  return [...list]
    .map(normalizeMessage)
    .filter((message) => isMessageAlive(message))
    .sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt));
}

function pruneExpired(): void {
  const currentMessages = get(messagesByPeer);
  let messagesChanged = false;
  const nextMessages: Record<string, Message[]> = {};
  for (const [peerId, list] of Object.entries(currentMessages)) {
    const hasExpired = list.some((message) => !isMessageAlive(message));
    const alive = hasExpired ? sortMessages(list) : list;
    nextMessages[peerId] = alive;
    if (alive.length !== list.length) messagesChanged = true;
  }
  if (messagesChanged) messagesByPeer.set(nextMessages);
  const currentConversations = get(conversations);
  let conversationsChanged = false;
  const updatedConversations = currentConversations.map((conversation) => {
    const messages = (messagesChanged ? nextMessages : currentMessages)[conversation.peer.id] ?? [];
    const aliveLast = conversation.lastMessage && isMessageAlive(conversation.lastMessage) ? conversation.lastMessage : null;
    const unreadIds = unreadMessageIds.get(conversation.peer.id);
    let knownExpired = 0;
    if (unreadIds) {
      for (const id of [...unreadIds]) {
        if (!messages.some((item) => item.id === id)) {
          unreadIds.delete(id);
          knownExpired += 1;
        }
      }
    }
    const unread = aliveLast ? Math.max(0, conversation.unread - knownExpired) : 0;
    if (aliveLast?.id !== conversation.lastMessage?.id || unread !== conversation.unread) conversationsChanged = true;
    return { ...conversation, lastMessage: aliveLast, unread };
  });
  if (conversationsChanged) conversations.set(updatedConversations);
}

function messageRevisionKey(peerId: string, messageId: string): string {
  return `${peerId}:${messageId}`;
}

function mergeMessage(
  list: Message[],
  incoming: Message,
  preserveNewer = false,
  protectedIds?: Set<string>,
): Message[] {
  incoming = normalizeMessage(incoming);
  const index = list.findIndex((message) => message.id === incoming.id || message.clientId === incoming.clientId);
  if (index < 0) return sortMessages([...list, incoming]);
  const current = list[index];
  if (preserveNewer && !current.id.startsWith('local:')) {
    if (protectedIds?.has(current.id)) return sortMessages(list);
    const currentAt = Date.parse(current.editedAt ?? current.createdAt);
    const incomingAt = Date.parse(incoming.editedAt ?? incoming.createdAt);
    if ((Number.isNaN(currentAt) ? 0 : currentAt) > (Number.isNaN(incomingAt) ? 0 : incomingAt)) {
      return sortMessages(list);
    }
  }
  const next = [...list];
  next[index] = incoming;
  return sortMessages(next);
}

function setMessage(peerId: string, message: Message): void {
  const key = messageRevisionKey(peerId, message.id);
  messageRevisions.set(key, (messageRevisions.get(key) ?? 0) + 1);
  messagesByPeer.update((all) => ({ ...all, [peerId]: mergeMessage(all[peerId] ?? [], message) }));
}

function peerFor(peerId: string): ChatPeer | null {
  const fromConversation = get(conversations).find((conversation) => conversation.peer.id === peerId)?.peer;
  if (fromConversation) return fromConversation;
  return get(activePeer)?.id === peerId ? get(activePeer) : null;
}

function visibleOpenPeer(peerId: string): boolean {
  return panelVisible && get(chatOpen) && get(activePeer)?.id === peerId &&
    typeof document !== 'undefined' && document.visibilityState === 'visible' && document.hasFocus();
}

function updateConversation(peerId: string, apply: (conversation: Conversation) => Conversation): void {
  conversations.update((list) => list.map((conversation) =>
    conversation.peer.id === peerId ? apply(conversation) : conversation,
  ));
}

function clearTyping(peerId: string): void {
  const timer = typingTimers.get(peerId);
  if (timer) clearTimeout(timer);
  typingTimers.delete(peerId);
  chatTyping.update((value) => {
    if (!value[peerId]) return value;
    const next = { ...value };
    delete next[peerId];
    return next;
  });
}

function stopTyping(peerId: string): void {
  clearTyping(peerId);
  void typingCall(peerId, false).catch(() => undefined);
}

function scheduleStartRetry(expectedGeneration: number): void {
  if (startRetryTimer || !authKey) return;
  startRetryTimer = setTimeout(() => {
    startRetryTimer = null;
    if (expectedGeneration !== generation || !authKey) return;
    void retryMessaging();
  }, 5000);
}

function handleTyping(peerId: string, value: boolean): void {
  if (!value) {
    clearTyping(peerId);
    return;
  }
  chatTyping.update((current) => ({ ...current, [peerId]: true }));
  const timer = typingTimers.get(peerId);
  if (timer) clearTimeout(timer);
  typingTimers.set(peerId, setTimeout(() => clearTyping(peerId), TYPING_EXPIRY_MS));
}

function replaceLocalMessage(clientId: string, message: Message): void {
  messagesByPeer.update((all) => {
    const next = { ...all };
    for (const [peerId, list] of Object.entries(all)) {
      if (list.some((item) => item.clientId === clientId)) next[peerId] = mergeMessage(list, message);
    }
    return next;
  });
}

function markPending(clientId: string, value: boolean): void {
  pendingMessages.update((set) => {
    const next = new Set(set);
    if (value) next.add(clientId);
    else next.delete(clientId);
    return next;
  });
}

function markFailed(clientId: string, value: boolean): void {
  failedMessages.update((set) => {
    const next = new Set(set);
    if (value) next.add(clientId);
    else next.delete(clientId);
    return next;
  });
}

function localMessage(peerId: string, clientId: string, text: string): Message {
  const now = new Date().toISOString();
  const ownId = get(currentUser)?.id ?? 'me';
  return {
    id: `local:${clientId}`,
    clientId,
    senderId: ownId,
    recipientId: peerId,
    text,
    createdAt: now,
    expiresAt: new Date(Date.now() + HISTORY_DAYS * 24 * 60 * 60 * 1000).toISOString(),
    reactions: [],
  };
}

async function reloadConversations(expected = generation): Promise<void> {
  if (expected !== generation) return;
  try {
    const loaded = (await fetchConversations()).filter((conversation) => !conversation.lastMessage || isMessageAlive(conversation.lastMessage));
    if (expected === generation) {
      conversations.set(loaded);
      chatHistoryError.set('');
    }
  } catch (err) {
    console.warn('messaging conversations failed', err);
    if (expected === generation) chatHistoryError.set(msg('social.chatConversationsError'));
  }
}

export async function loadMessages(peerId: string, before = '', append = false, markReadAfter = false): Promise<void> {
  const expectedGeneration = generation;
  const request = (messageLoad.get(peerId) ?? 0) + 1;
  messageLoad.set(peerId, request);
  const requestRevisions = new Map(
    (get(messagesByPeer)[peerId] ?? []).map((item) => [item.id, messageRevisions.get(messageRevisionKey(peerId, item.id)) ?? 0]),
  );
  if (append) chatLoadingMore.set(true);
  else chatLoading.set(true);
  try {
    const page: MessagePage = await fetchMessages(peerId, before);
    if (expectedGeneration !== generation || messageLoad.get(peerId) !== request) return;
    const filtered = sortMessages(page.messages);
    const hadMessages = (get(messagesByPeer)[peerId] ?? []).length > 0;
    messagesByPeer.update((all) => {
      const current = all[peerId] ?? [];
      const keepLocal = current.filter((message) => message.id.startsWith('local:'));
      const base = append ? current : current.filter((message) => !message.id.startsWith('local:'));
      const protectedIds = new Set(
        current
          .filter((message) => (messageRevisions.get(messageRevisionKey(peerId, message.id)) ?? 0) > (requestRevisions.get(message.id) ?? 0))
          .map((message) => message.id),
      );
      const merged = [...base, ...filtered, ...keepLocal].reduce<Message[]>(
        (list, message) => mergeMessage(list, message, true, protectedIds),
        [],
      );
      return { ...all, [peerId]: merged };
    });
    nextByPeer.update((all) => {
      const current = get(messagesByPeer)[peerId] ?? [];
      const next = append || !hadMessages ? page.next ?? '' : all[peerId] ?? page.next ?? '';
      return { ...all, [peerId]: next };
    });
    if (page.canSend !== undefined) {
      canSendByPeer.update((all) => ({ ...all, [peerId]: page.canSend === true }));
      updateConversation(peerId, (conversation) => ({ ...conversation, canSend: page.canSend ?? conversation.canSend }));
    }
    if (!append && markReadAfter) await markPeerRead(peerId);
    if (expectedGeneration === generation) chatHistoryError.set('');
  } catch (err) {
    console.warn('messaging history failed', err);
    if (expectedGeneration === generation) chatHistoryError.set(msg('social.chatHistoryError'));
    throw err;
  } finally {
    if (expectedGeneration === generation) {
      if (append) chatLoadingMore.set(false);
      else chatLoading.set(false);
    }
  }
}

export async function loadMore(peerId: string): Promise<void> {
  const cursor = get(nextByPeer)[peerId];
  if (!cursor || get(chatLoadingMore)) return;
  await loadMessages(peerId, cursor, true);
}

export function openChat(peer: ChatPeer): void {
  activePeer.set(peer);
  chatOpen.set(true);
  panelVisible = true;
  chatPanelVisible.set(true);
  clearTyping(peer.id);
  void loadMessages(peer.id, '', false, true).catch(() => undefined);
}

export function openChatById(peerId: string): void {
  const peer = peerFor(peerId);
  if (peer) openChat(peer);
}

export function closeChat(): void {
  const peer = get(activePeer);
  if (peer) stopTyping(peer.id);
  activePeer.set(null);
  chatOpen.set(false);
  panelVisible = false;
  chatPanelVisible.set(false);
  editingMessageId.set(null);
}

export function setPanelVisible(value: boolean): void {
  panelVisible = value;
  chatPanelVisible.set(value);
  if (value) {
    const peer = get(activePeer);
    if (peer) void markPeerRead(peer.id);
  }
}

export async function markPeerRead(peerId: string): Promise<void> {
  if (!visibleOpenPeer(peerId)) return;
  const list = get(messagesByPeer)[peerId] ?? [];
  const ownId = get(currentUser)?.id;
  const last = [...list].reverse().find((message) => message.senderId !== ownId && !message.id.startsWith('local:'));
  if (!last) return;
  const expectedGeneration = generation;
  try {
    await read(peerId, last.id);
    if (expectedGeneration !== generation || !visibleOpenPeer(peerId)) return;
    unreadMessageIds.delete(peerId);
    updateConversation(peerId, (conversation) => ({ ...conversation, unread: 0 }));
  } catch (err) {
    console.warn('messaging read failed', err);
  }
}

export async function sendMessage(peerId: string, text: string, clientId: string = crypto.randomUUID()): Promise<Message> {
  const value = text.trim();
  if (!value) throw new Error('message_empty');
  if (Array.from(value).length > 2000) throw new Error('message_too_long');
  const expectedGeneration = generation;
  const optimistic = localMessage(peerId, clientId, value);
  setMessage(peerId, optimistic);
  markPending(clientId, true);
  markFailed(clientId, false);
  stopTyping(peerId);
  setDraft(peerId, '');
  try {
    const sent = await sendMessageCall(peerId, clientId, value);
    if (expectedGeneration !== generation) throw new Error('session_changed');
    replaceLocalMessage(clientId, sent);
    updateConversation(peerId, (conversation) => ({ ...conversation, lastMessage: sent }));
    return sent;
  } catch (err) {
    if (expectedGeneration === generation) markFailed(clientId, true);
    throw err;
  } finally {
    if (expectedGeneration === generation) markPending(clientId, false);
  }
}

export async function retryMessage(peerId: string, clientId: string): Promise<Message> {
  const local = (get(messagesByPeer)[peerId] ?? []).find((message) => message.clientId === clientId);
  if (!local) throw new Error('message_not_found');
  markFailed(clientId, false);
  markPending(clientId, true);
  const expectedGeneration = generation;
  try {
    const sent = await sendMessageCall(peerId, clientId, local.text);
    if (expectedGeneration !== generation) throw new Error('session_changed');
    replaceLocalMessage(clientId, sent);
    return sent;
  } catch (err) {
    if (expectedGeneration === generation) markFailed(clientId, true);
    throw err;
  } finally {
    if (expectedGeneration === generation) markPending(clientId, false);
  }
}

export async function editMessage(peerId: string, messageId: string, text: string): Promise<void> {
  const value = text.trim();
  if (!value) throw new Error('message_empty');
  if (Array.from(value).length > 2000) throw new Error('message_too_long');
  const expectedGeneration = generation;
  const sent = await editMessageCall(peerId, messageId, value);
  if (expectedGeneration !== generation) return;
  setMessage(peerId, sent);
  editingMessageId.set(null);
}

export async function toggleReaction(peerId: string, message: Message, emoji: ReactionKey): Promise<void> {
  const ownId = get(currentUser)?.id;
  if (!ownId) return;
  const current = message.reactions.find((reaction) => reaction.emoji === emoji);
  const mine = current?.userIds.includes(ownId) ?? false;
  try {
    if (mine) await unreactMessageCall(peerId, message.id, emoji);
    else await reactMessageCall(peerId, message.id, emoji);
    await loadMessages(peerId);
  } catch (err) {
    console.warn('messaging reaction failed', err);
  }
}

export async function setTyping(peerId: string, value: boolean): Promise<void> {
  if (!value) {
    clearTyping(peerId);
    await typingCall(peerId, false).catch(() => undefined);
    return;
  }
  const now = Date.now();
  if (now - (typingSentAt.get(peerId) ?? 0) < TYPING_THROTTLE_MS) return;
  typingSentAt.set(peerId, now);
  await typingCall(peerId, true).catch(() => undefined);
}

function showChatToast(peer: ChatPeer, message: Message): void {
  const id = ++toastId;
  chatToast.set({ id, peer, message });
  setTimeout(() => chatToast.update((current) => (current?.id === id ? null : current)), 5000);
}

async function incomingMessage(peerId: string, message: Message, expectedGeneration: number): Promise<void> {
  const ownId = get(currentUser)?.id;
  if (!ownId || !isMessageAlive(message)) return;
  if (message.senderId === ownId) {
    if (expectedGeneration !== generation) return;
    setMessage(peerId, message);
    updateConversation(peerId, (conversation) => ({ ...conversation, lastMessage: message }));
    return;
  }
  const knownBeforeReload = !!peerFor(peerId);
  const peer = peerFor(peerId);
  if (!peer) {
    await reloadConversations();
  }
  const resolved = peerFor(peerId);
  if (!resolved || expectedGeneration !== generation) return;
  const existingMessages = get(messagesByPeer)[peerId] ?? [];
  if (existingMessages.some((item) => item.id === message.id || item.clientId === message.clientId)) return;
  setMessage(peerId, message);
  const visible = visibleOpenPeer(peerId);
  if (visible) {
    void markPeerRead(peerId);
    return;
  }
  const unreadIds = unreadMessageIds.get(peerId) ?? new Set<string>();
  unreadIds.add(message.id);
  unreadMessageIds.set(peerId, unreadIds);
  updateConversation(peerId, (conversation) => ({
    ...conversation,
    lastMessage: message,
    unread: knownBeforeReload ? conversation.unread + 1 : conversation.unread,
  }));
  if (get(presenceStatus) === 'busy') return;
  const notifyKey = message.id || message.clientId;
  if (notifyIds.has(notifyKey)) return;
  notifyIds.add(notifyKey);
  const popupShown = await notify(peerId, resolved.displayName || resolved.username, message.text).catch(() => false);
  if (expectedGeneration !== generation || get(currentUser)?.id !== ownId) return;
  if (!popupShown) showChatToast(resolved, message);
}

async function handleEvent(event: ChatEvent): Promise<void> {
  if (!event || event.ownerId !== get(currentUser)?.id) return;
  const expectedGeneration = generation;
  if (event.kind === 'connection') {
    chatConnected.set(event.connected !== false);
    return;
  }
  if (event.kind === 'message' && event.message) {
    await incomingMessage(event.peerId, event.message, expectedGeneration);
    return;
  }
  if (event.kind === 'typing') {
    handleTyping(event.peerId, event.typing === true);
    return;
  }
  if (event.kind === 'sync') {
    await reloadConversations();
    const peer = get(activePeer);
    if (peer) await loadMessages(peer.id).catch(() => undefined);
    return;
  }
  if (event.kind === 'updated' && event.message) setMessage(event.peerId, event.message);
  if (event.kind === 'updated' || event.kind === 'read') {
    await reloadConversations();
    const peer = get(activePeer);
    if (peer && peer.id === event.peerId) await loadMessages(peer.id).catch(() => undefined);
  }
}

function setSession(nextKey: string): Promise<void> {
  const serial = ++sessionSerial;
  sessionQueue = sessionQueue.catch(() => undefined).then(() => runSession(nextKey, serial));
  return sessionQueue;
}

async function runSession(nextKey: string, serial: number): Promise<void> {
  if (serial !== sessionSerial) return;
  if (nextKey === authKey) return;
  authKey = nextKey;
  generation += 1;
  if (startRetryTimer) {
    clearTimeout(startRetryTimer);
    startRetryTimer = null;
  }
  notifyIds = new Set();
  unreadMessageIds = new Map();
  for (const peerId of typingTimers.keys()) clearTyping(peerId);
  messagesByPeer.set({});
  messageRevisions = new Map();
  nextByPeer.set({});
  conversations.set([]);
  canSendByPeer.set({});
  chatHistoryError.set('');
  chatToast.set(null);
  pendingMessages.set(new Set());
  failedMessages.set(new Set());
  chatTyping.set({});
  editingMessageId.set(null);
  closeChat();
  if (!nextKey) {
    chatConnected.set(false);
    await stop().catch(() => undefined);
    return;
  }
  const expectedGeneration = generation;
  try {
    await start();
    if (serial !== sessionSerial || expectedGeneration !== generation || authKey !== nextKey) return;
    chatConnected.set(true);
  } catch (err) {
    if (serial !== sessionSerial || expectedGeneration !== generation || authKey !== nextKey) return;
    chatConnected.set(false);
    console.warn('messaging start failed', err);
    scheduleStartRetry(expectedGeneration);
    return;
  }
  await reloadConversations(expectedGeneration);
}

export async function retryMessaging(): Promise<void> {
  const expectedGeneration = generation;
  if (!authKey || get(authState) !== 'authenticated' || get(needsSocialConsent)) return;
  try {
    await start();
    if (expectedGeneration !== generation) return;
    chatConnected.set(true);
    await reloadConversations(expectedGeneration);
    const peer = get(activePeer);
    if (peer) await loadMessages(peer.id).catch(() => undefined);
  } catch (err) {
    if (expectedGeneration !== generation || !authKey) return;
    chatConnected.set(false);
    console.warn('messaging retry failed', err);
    scheduleStartRetry(expectedGeneration);
  }
}

export function initMessaging(): void {
  if (started) return;
  started = true;
  if (!expiryTimer) expiryTimer = setInterval(pruneExpired, 1000);
  if (!conversationRefreshTimer) conversationRefreshTimer = setInterval(() => {
    if (authKey) void reloadConversations(generation);
  }, 60000);
  if (inWails) {
    Events.On('chat:event', (event) => {
      void handleEvent(event.data as ChatEvent);
    });
    Events.On('chat:open', (event) => {
      const data = event.data as { ownerId?: string; peerId?: string } | null;
      if (!data || data.ownerId !== get(currentUser)?.id || !data.peerId) return;
      openChatById(data.peerId);
    });
  }
  let authStateValue = '';
  let consent = false;
  const kick = () => {
    const user = get(currentUser);
    const state = get(authState);
    const needs = get(needsSocialConsent);
    const next = state === 'authenticated' && user?.id && !needs ? user.id : '';
    if (next !== authStateValue || needs !== consent) {
      consent = needs;
      void setSession(next as string);
      authStateValue = next as string;
    }
  };
  authState.subscribe(kick);
  currentUser.subscribe(kick);
  needsSocialConsent.subscribe(kick);
}
