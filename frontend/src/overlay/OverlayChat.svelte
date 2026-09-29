<script lang="ts">
  import { ArrowLeft, ChevronUp, RefreshCw, Send } from '@lucide/svelte';
  import Avatar from '../lib/components/Avatar.svelte';
  import { msg } from '../lib/i18n';
  import type { ChatPeer, Message } from '../lib/services/messaging';
  import { chatTextParts } from '../lib/social/chatText';
  import {
    activePeer,
    canSendByPeer,
    chatConnected,
    chatConversationsError,
    chatHistoryError,
    chatLoading,
    chatLoadingMore,
    chatTyping,
    closeChat,
    conversations,
    failedMessages,
    getDraft,
    loadMessages,
    loadMore,
    markPeerRead,
    messagesByPeer,
    nextByPeer,
    openChat,
    pendingMessages,
    refreshConversations,
    retryMessage,
    sendMessage,
    setDraft,
    setPanelVisible,
    setTyping,
  } from '../lib/stores/messaging';
  import { currentUser } from '../lib/stores/user';
  import { clockTime, shortDate } from '../lib/utils/format';

  let { active, focusTick }: { active: boolean; focusTick: number } = $props();

  let draft = $state('');
  let composer = $state<HTMLTextAreaElement | undefined>(undefined);
  let scrollBox = $state<HTMLElement | undefined>(undefined);
  let sending = $state(false);
  let error = $state('');
  let lastRenderedLastMessage = '';

  const peer = $derived($activePeer);
  const activeMessages = $derived(peer ? ($messagesByPeer[peer.id] ?? []) : []);
  const conversation = $derived(peer ? $conversations.find((item) => item.peer.id === peer.id) : undefined);
  const canSend = $derived(conversation?.canSend ?? $canSendByPeer[peer?.id ?? ''] ?? true);

  function displayName(target: ChatPeer): string {
    return target.displayName || target.username;
  }

  function isOwn(message: Message): boolean {
    return message.senderId === $currentUser?.id;
  }

  function stamp(iso: string): string {
    const value = new Date(iso);
    if (Number.isNaN(value.getTime())) return '';
    const clock = clockTime(value);
    return value.toDateString() === new Date().toDateString() ? clock : `${shortDate(value)}, ${clock}`;
  }

  function scrollToEnd(): void {
    requestAnimationFrame(() => {
      if (scrollBox) scrollBox.scrollTop = scrollBox.scrollHeight;
    });
  }

  function setComposer(value: string): void {
    draft = value;
    if (peer) {
      setDraft(peer.id, draft);
      void setTyping(peer.id, draft.trim().length > 0);
    }
  }

  async function submit(): Promise<void> {
    if (!peer || !canSend || !draft.trim() || sending) return;
    const targetPeerId = peer.id;
    const submitted = draft.trim();
    sending = true;
    error = '';
    try {
      await sendMessage(targetPeerId, submitted);
      if ($activePeer?.id === targetPeerId && draft.trim() === submitted) draft = '';
      if ($activePeer?.id === targetPeerId) {
        composer?.focus();
        scrollToEnd();
      }
    } catch (err) {
      error = err instanceof Error && err.message === 'message_too_long' ? msg('social.chatTooLong') : msg('social.chatSendError');
    } finally {
      sending = false;
    }
  }

  async function retry(message: Message): Promise<void> {
    if (!peer) return;
    error = '';
    try {
      await retryMessage(peer.id, message.clientId);
    } catch {
      error = msg('social.chatSendError');
    }
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter' || event.shiftKey) return;
    event.preventDefault();
    void submit();
  }

  function retryHistory(): void {
    if (!peer) return;
    if ($nextByPeer[peer.id]) void loadMore(peer.id).catch(() => undefined);
    else void loadMessages(peer.id).catch(() => undefined);
  }

  function showList(): void {
    closeChat();
    draft = '';
  }

  $effect(() => {
    const target = peer;
    draft = target ? getDraft(target.id) : '';
    error = '';
  });

  $effect(() => {
    const last = activeMessages[activeMessages.length - 1]?.id ?? '';
    if (last && last !== lastRenderedLastMessage && !$chatLoadingMore) scrollToEnd();
    lastRenderedLastMessage = last;
  });

  $effect(() => {
    setPanelVisible(active && !!peer);
  });

  $effect(() => {
    focusTick;
    if (active && peer && canSend) composer?.focus();
  });

  $effect(() => {
    const target = peer;
    if (!active || !target) return;
    const onFocus = () => {
      void markPeerRead(target.id);
    };
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  });
</script>

<div class="chat">
  {#if peer}
    <header class="dialog-head">
      <button class="back" type="button" aria-label={msg('social.chatBack')} onclick={showList}><ArrowLeft size="1.7rem" /></button>
      <Avatar size="sm" name={displayName(peer)} src={peer.avatarUrl} />
      <div class="head-copy">
        <strong>{displayName(peer)}</strong>
        <span>{peer.username ? `@${peer.username}` : ''}{#if !$chatConnected} · {msg('social.chatOffline')}{/if}</span>
      </div>
    </header>

    <div class="messages" bind:this={scrollBox}>
      {#if $nextByPeer[peer.id]}
        <button class="load-more" type="button" disabled={$chatLoadingMore} onclick={() => loadMore(peer.id).catch(() => undefined)}>
          <ChevronUp size="1.4rem" /> {$chatLoadingMore ? msg('social.chatLoadingHistory') : msg('social.chatEarlier')}
        </button>
      {/if}
      {#if $chatHistoryError}
        <div class="notice error">{$chatHistoryError}<button type="button" onclick={retryHistory}>{msg('social.chatRetry')}</button></div>
      {/if}
      {#if $chatLoading && activeMessages.length === 0 && !$chatHistoryError}
        <div class="notice">{msg('social.chatLoadingHistory')}</div>
      {:else if activeMessages.length === 0 && !$chatHistoryError}
        <div class="notice">{msg('social.chatNoHistory')}</div>
      {:else}
        {#each activeMessages as message (message.id)}
          <div class="message-row" class:own={isOwn(message)}>
            <article class="message" class:failed={$failedMessages.has(message.clientId)} aria-busy={$pendingMessages.has(message.clientId)}>
              <div class="message-meta">
                <span>{isOwn(message) ? msg('social.chatYou') : displayName(peer)}</span>
                <time datetime={message.createdAt}>{stamp(message.createdAt)}</time>
                {#if message.editedAt}<span class="edited">{msg('social.chatEdited')}</span>{/if}
              </div>
              <p class="message-text">
                {#each chatTextParts(message.text) as part}
                  {#if part.href}<a href={part.href} target="_blank" rel="noopener noreferrer">{part.text}</a>{:else}{part.text}{/if}
                {/each}
              </p>
              {#if $pendingMessages.has(message.clientId)}
                <span class="status" role="status">{msg('social.chatSending')}</span>
              {/if}
              {#if $failedMessages.has(message.clientId)}
                <button class="retry" type="button" onclick={() => retry(message)}><RefreshCw size="1.25rem" /> {msg('social.chatRetry')}</button>
              {/if}
            </article>
          </div>
        {/each}
      {/if}
      {#if $chatTyping[peer.id]}<div class="typing">{msg('social.chatTyping', { name: displayName(peer) })}</div>{/if}
    </div>

    {#if error}<div class="notice error">{error}</div>{/if}
    {#if !$chatConnected}
      <div class="notice small" role="status">{msg('social.chatReconnecting')}</div>
    {/if}
    {#if !canSend}
      <div class="notice small">{msg('social.chatDisabled')}</div>
    {:else}
      <div class="composer">
        <textarea
          bind:this={composer}
          value={draft}
          maxlength="2000"
          rows="2"
          placeholder={msg('social.chatPlaceholder')}
          disabled={sending}
          oninput={(event) => setComposer((event.currentTarget as HTMLTextAreaElement).value)}
          onkeydown={onKeydown}
        ></textarea>
        <button class="send" type="button" aria-label={msg('social.chatSend')} disabled={!draft.trim() || sending} onclick={() => submit()}><Send size="1.7rem" /></button>
      </div>
      <div class="hint">{msg('social.chatHint')}</div>
    {/if}
  {:else}
    <div class="list">
      {#if $chatConversationsError}
        <div class="notice error">{$chatConversationsError}<button type="button" onclick={() => refreshConversations()}>{msg('social.chatRetry')}</button></div>
      {/if}
      {#if $conversations.length === 0 && !$chatConversationsError}
        <div class="notice">{msg('social.chatNoConversations')}</div>
      {:else}
        {#each $conversations as item (item.peer.id)}
          <button class="conversation" type="button" onclick={() => openChat(item.peer)}>
            <Avatar size="md" name={displayName(item.peer)} src={item.peer.avatarUrl} />
            <span class="conversation-copy">
              <strong>{displayName(item.peer)}</strong>
              <span>{item.lastMessage?.text ?? msg('social.chatNewConversation')}</span>
            </span>
            {#if item.unread > 0}<span class="unread">{item.unread}</span>{/if}
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .chat {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  .list,
  .messages {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .list {
    padding: var(--space-2);
  }

  .messages {
    padding: var(--space-3);
  }

  .dialog-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 5.2rem;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--border);
  }

  .back {
    display: inline-flex;
    padding: 0.5rem;
    color: var(--text-2);
  }

  .head-copy,
  .conversation-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 0.15rem;
    text-align: left;
  }

  .head-copy {
    flex: 1;
  }

  .head-copy strong,
  .conversation-copy strong {
    font-size: var(--font-sm);
  }

  .head-copy span,
  .conversation-copy span {
    overflow: hidden;
    color: var(--text-3);
    font-size: var(--font-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conversation {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-3);
    border-radius: var(--radius-md);
    color: inherit;
    text-align: left;
  }

  .conversation:hover {
    background: var(--hover);
  }

  .conversation-copy {
    flex: 1;
  }

  .unread {
    display: grid;
    place-items: center;
    min-width: 2rem;
    height: 2rem;
    padding: 0 0.4rem;
    border-radius: 2rem;
    background: var(--accent);
    color: var(--accent-on, #fff);
    font-size: var(--font-xs);
  }

  .notice {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    min-height: 13rem;
    padding: var(--space-5);
    color: var(--text-3);
    font-size: var(--font-sm);
    text-align: center;
  }

  .notice.error,
  .notice.small {
    min-height: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--font-xs);
  }

  .notice.error {
    color: var(--danger);
  }

  .notice button,
  .load-more,
  .retry {
    border: 0;
    background: transparent;
    color: var(--accent-text);
    font: inherit;
    cursor: pointer;
  }

  .notice button {
    text-decoration: underline;
  }

  .load-more {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0 auto var(--space-3);
    font-size: var(--font-xs);
  }

  .message-row {
    display: flex;
    justify-content: flex-start;
    margin: var(--space-2) 0;
  }

  .message-row.own {
    justify-content: flex-end;
  }

  .message {
    max-width: 86%;
    padding: 0.85rem 1rem;
    border: 1px solid var(--border);
    border-radius: 1.2rem 1.2rem 1.2rem 0.35rem;
    background: var(--surface-3);
  }

  .own .message {
    border-radius: 1.2rem 1.2rem 0.35rem 1.2rem;
    background: color-mix(in srgb, var(--accent) 18%, var(--surface-3));
  }

  .message.failed {
    border-color: var(--danger);
  }

  .message-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.35rem;
    color: var(--text-3);
    font-size: 1rem;
  }

  .message-meta time {
    opacity: 0.75;
    white-space: nowrap;
  }

  .edited {
    font-style: italic;
  }

  .message-text {
    color: var(--text);
    font-size: var(--font-sm);
    line-height: 1.45;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    user-select: text;
  }

  .message-text a {
    color: var(--accent-text);
    text-decoration: underline;
  }

  .status {
    color: var(--text-3);
    font-size: 1rem;
  }

  .retry {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    color: var(--danger);
    font-size: 1rem;
  }

  .typing {
    margin: var(--space-2) 0;
    color: var(--text-3);
    font-size: var(--font-xs);
    font-style: italic;
  }

  .composer {
    display: flex;
    align-items: flex-end;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3) 0;
    border-top: 1px solid var(--border);
  }

  .composer textarea {
    flex: 1;
    min-height: 4.2rem;
    padding: 0.7rem;
    resize: none;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: var(--font-sm);
    outline: none;
    user-select: text;
  }

  .composer textarea:focus {
    border-color: var(--accent);
  }

  .send {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.8rem;
    height: 3.8rem;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--accent);
    color: var(--accent-on, #fff);
  }

  .send:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .hint {
    padding: 0.45rem var(--space-3) var(--space-2);
    color: var(--text-3);
    font-size: 1rem;
  }
</style>
