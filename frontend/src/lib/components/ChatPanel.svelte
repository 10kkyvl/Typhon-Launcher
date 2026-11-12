<script lang="ts">
  import { ArrowLeft, ChevronUp, MessageCircle, X } from '@lucide/svelte';
  import { onMount, tick } from 'svelte';
  import { fade } from 'svelte/transition';
  import Avatar from './Avatar.svelte';
  import { fieldsFrame } from '../profile/frameLook';
  import { themeLayout } from '../stores/layout';
  import Button from './Button.svelte';
  import ChatComposer from './ChatComposer.svelte';
  import ChatConversations from './ChatConversations.svelte';
  import ChatMessage from './ChatMessage.svelte';
  import EmptyState from './EmptyState.svelte';
  import IconButton from './IconButton.svelte';
  import Lightbox, { type LightboxItem } from './Lightbox.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import {
    activePeer,
    attachmentDrafts,
    chatConnected,
    chatLoading,
    chatLoadingMore,
    chatHistoryError,
    chatConversationsError,
    canSendByPeer,
    chatOpen,
    chatToast,
    chatTyping,
    closeChat,
    conversations,
    getDraft,
    loadMessages,
    loadMore,
    messagesByPeer,
    openChat,
    retryMessaging,
    sendMessage,
    setAttachmentDrafts,
    setPanelVisible,
    nextByPeer,
  } from '../stores/messaging';
  import { messagePreview, type ChatAttachmentDraft } from '../social/chatImage';
  import { chatErrorText } from '../social/chatErrors';
  import { toast } from '../stores/toasts';
  import { focusableIn } from '../layers/focus';
  import { uploadChatImage, type Attachment, type ChatPeer, type Message } from '../services/messaging';
  import { msg } from '../i18n';

  let draft = $state('');
  let scrollBox = $state<HTMLElement | undefined>(undefined);
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let lightboxItems = $state<LightboxItem[]>([]);
  let error = $state('');
  let historyRetrying = $state(false);
  let conversationsRetrying = $state(false);
  let reconnecting = $state(false);
  // The history failure is about an older page the user asked for, not the newest messages.
  let olderFailed = false;
  let root = $state<HTMLElement>();
  let panel = $state<HTMLElement>();
  let list = $state<HTMLElement>();
  let toastPlate = $state<HTMLElement>();
  let composerPart = $state<{ field: () => HTMLElement | undefined }>();
  let now = $state(new Date());
  let lastRenderedLastMessage = '';
  // A send in flight belongs to its conversation, not to the composer drawn for it: after Back and
  // reopening the conversation, Send stays refused and the upload's progress shows. `total` counts the
  // images still to upload when it started.
  let sends = $state<Record<string, { current: number; total: number }>>({});
  const idle = { current: 0, total: 0 };

  const peer = $derived($activePeer);
  const activeMessages = $derived(peer ? ($messagesByPeer[peer.id] ?? []) : []);
  const conversation = $derived(peer ? $conversations.find((item) => item.peer.id === peer.id) : undefined);
  const canSend = $derived(conversation?.canSend ?? $canSendByPeer[peer?.id ?? ''] ?? true);
  const unreadTotal = $derived($conversations.reduce((sum, item) => sum + item.unread, 0));
  const sending = $derived(peer ? peer.id in sends : false);

  function displayName(target: ChatPeer): string {
    return target.displayName || target.username;
  }

  // A part of the panel swapped for another takes the pressed control with it: focus that fell to the
  // page goes to what took its place.
  async function land(target: () => HTMLElement | null | undefined): Promise<void> {
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    target()?.focus();
  }

  async function uploadDrafts(targetPeerId: string, pending: ChatAttachmentDraft[]): Promise<Attachment[] | null> {
    let list = pending;
    const total = list.filter((item) => !item.uploaded).length;
    let done = 0;
    sends[targetPeerId] = { current: 0, total };
    for (const item of pending) {
      if (item.uploaded) continue;
      sends[targetPeerId] = { current: done + 1, total };
      try {
        const uploaded = await uploadChatImage(item.encoded);
        list = list.map((entry) => (entry.key === item.key ? { ...entry, uploaded } : entry));
        setAttachmentDrafts(targetPeerId, list);
        done += 1;
      } catch (err) {
        toast(chatErrorText(err, 'social.chatSendError'), 'danger');
        return null;
      }
    }
    return list.map((item) => ({ ...(item.uploaded as Attachment), url: item.previewUrl }));
  }

  async function submit(): Promise<void> {
    if (!peer || !canSend || sending) return;
    const targetPeerId = peer.id;
    const pending = $attachmentDrafts[targetPeerId] ?? [];
    const submitted = draft.trim();
    if (!submitted && pending.length === 0) return;
    sends[targetPeerId] = { ...idle };
    error = '';
    try {
      let attachments: Attachment[] = [];
      if (pending.length > 0) {
        const uploaded = await uploadDrafts(targetPeerId, pending);
        if (!uploaded) return;
        attachments = uploaded;
      }
      await sendMessage(targetPeerId, submitted, undefined, attachments);
      if (attachments.length > 0) setAttachmentDrafts(targetPeerId, []);
      if ($activePeer?.id === targetPeerId && draft.trim() === submitted) draft = '';
      if ($activePeer?.id === targetPeerId) {
        composerPart?.field()?.focus();
        scrollToEnd();
      }
    } catch (err) {
      error = chatErrorText(err, 'social.chatSendError');
    } finally {
      delete sends[targetPeerId];
    }
  }

  // A failed message that went through on retry: the composer may still hold its text, and the row
  // that held Retry is the sent message's now.
  function resent(peerId: string, text: string, clientId: string): void {
    if ($activePeer?.id === peerId && draft.trim() === text.trim()) draft = '';
    void land(() => {
      const row = [...(scrollBox?.querySelectorAll<HTMLElement>('[data-client]') ?? [])].find((item) => item.dataset.client === clientId);
      return (row ? focusableIn(row)[0] : undefined) ?? composerPart?.field();
    });
  }

  // The conversation's way in: where a message is written, else its way back.
  const conversationStart = () => composerPart?.field() ?? (panel ? focusableIn(panel)[0] : undefined);

  function openImages(messageId: string, urls: string[], position: number): void {
    lightboxItems = urls.map((url, index) => ({ id: `${messageId}:${index}`, url }));
    lightboxIndex = position;
    lightboxOpen = true;
  }

  function imageLoaded(message: Message): void {
    if (activeMessages[activeMessages.length - 1]?.id === message.id) scrollToEnd();
  }

  function scrollToEnd(): void {
    requestAnimationFrame(() => {
      if (scrollBox) scrollBox.scrollTop = scrollBox.scrollHeight;
    });
  }

  // The toast fades out with the pressed button still in it: focus is moved on now, not when it falls.
  async function leaveToast(target: () => HTMLElement | null | undefined): Promise<void> {
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected && !toastPlate?.contains(active)) return;
    target()?.focus();
  }

  function openToast(): void {
    const item = $chatToast;
    if (!item) return;
    chatToast.set(null);
    openChat(item.peer);
    void leaveToast(conversationStart);
  }

  function dismissToast(): void {
    chatToast.set(null);
    void leaveToast(() => (root ? focusableIn(root).find((el) => !toastPlate?.contains(el)) : undefined));
  }

  function openConversation(target: ChatPeer): void {
    openChat(target);
    draft = getDraft(target.id);
    error = '';
    chatToast.set(null);
    void land(conversationStart);
  }

  function showList(): void {
    const from = peer?.id;
    closeChat();
    chatOpen.set(true);
    draft = '';
    void land(
      () =>
        [...(panel?.querySelectorAll<HTMLElement>('[data-peer]') ?? [])].find((row) => row.dataset.peer === from) ??
        panel?.querySelector<HTMLElement>('h2'),
    );
  }

  // Opening the panel takes the opener away, closing it the panel: focus goes with the user.
  function openPanel(): void {
    chatOpen.set(true);
    void land(() => (peer ? conversationStart() : (list ? focusableIn(list)[0] : undefined)) ?? panel?.querySelector<HTMLElement>('h2'));
  }

  function closePanel(): void {
    closeChat();
    void land(() => root?.querySelector<HTMLElement>('.chat-toggle button'));
  }

  async function earlier(target: string): Promise<void> {
    try {
      await loadMore(target);
      olderFailed = false;
      // The last older page takes "Earlier" away: focus goes to the first message it brought.
      void land(() => {
        const first = scrollBox?.querySelector<HTMLElement>('[data-client]');
        return first ? focusableIn(first)[0] : undefined;
      });
    } catch {
      // The store shows this failure in the history's notice; its Retry asks for this page again.
      olderFailed = true;
    }
  }

  // Retry asks again for what failed: an older page the user asked for, else the newest messages.
  async function retryHistory(): Promise<boolean> {
    if (!peer) return false;
    try {
      if (olderFailed) await loadMore(peer.id);
      else await loadMessages(peer.id);
    } catch {
      // The store has logged the failure and keeps it in the notice.
    }
    const ok = $chatHistoryError === '';
    if (ok) olderFailed = false;
    return ok;
  }

  async function retryConversations(): Promise<boolean> {
    await retryMessaging();
    return $chatConversationsError === '';
  }

  async function reconnect(): Promise<void> {
    if (reconnecting) return;
    reconnecting = true;
    try {
      await retryMessaging();
    } finally {
      reconnecting = false;
    }
  }

  $effect(() => {
    const target = peer;
    draft = target ? getDraft(target.id) : '';
    error = '';
    olderFailed = false;
  });

  $effect(() => {
    const last = activeMessages[activeMessages.length - 1]?.id ?? '';
    if (last && last !== lastRenderedLastMessage && !$chatLoadingMore) scrollToEnd();
    lastRenderedLastMessage = last;
  });

  $effect(() => {
    setPanelVisible($chatOpen && !!peer);
  });

  onMount(() => {
    const updateFocus = () => {
      now = new Date();
      setPanelVisible($chatOpen && !!peer);
    };
    const clockInterval = window.setInterval(() => { now = new Date(); }, 60_000);
    window.addEventListener('focus', updateFocus);
    window.addEventListener('blur', updateFocus);
    document.addEventListener('visibilitychange', updateFocus);
    return () => {
      window.clearInterval(clockInterval);
      window.removeEventListener('focus', updateFocus);
      window.removeEventListener('blur', updateFocus);
      document.removeEventListener('visibilitychange', updateFocus);
    };
  });
</script>

<div class="chat-root" class:sidebar-right={$themeLayout.side === 'right'} bind:this={root}>
  <!-- On the page from the start, so a message that comes in is announced as it is written into it. -->
  <div class="sr-only toast-announce" role="status">
    {#if $chatToast}{msg('social.chatToastAnnounce', { name: displayName($chatToast.peer), text: messagePreview($chatToast.message) })}{/if}
  </div>
  {#if $chatToast}
    <div class="chat-toast" out:fade={{ duration: 240 }} bind:this={toastPlate}>
      <button class="toast-open" type="button" onclick={openToast}>
        <Avatar size="sm" name={displayName($chatToast.peer)} src={$chatToast.peer.avatarUrl} {...fieldsFrame($chatToast.peer)} />
        <span class="toast-copy">
          <strong>{$chatToast.peer.displayName || $chatToast.peer.username}</strong>
          <span>{messagePreview($chatToast.message)}</span>
        </span>
      </button>
      <IconButton size="sm" label={msg('social.chatToastDismiss')} onclick={dismissToast}><X size="1.5rem" strokeWidth={1.8} /></IconButton>
    </div>
  {/if}

  {#if !$chatOpen}
    <span class="chat-toggle">
      <IconButton
        variant="primary"
        size="lg"
        label={msg('social.chatOpen')}
        count={unreadTotal}
        countLabel={msg('social.chatOpenUnread', { count: unreadTotal })}
        onclick={openPanel}
      >
        <MessageCircle size="2rem" strokeWidth={1.8} />
      </IconButton>
    </span>
  {:else}
    <section class="chat-panel" aria-label={msg('social.chatPanelLabel')} bind:this={panel}>
      <header class="chat-header">
        {#if peer}
          <IconButton size="sm" label={msg('social.chatBack')} onclick={showList}><ArrowLeft size="1.7rem" /></IconButton>
          <Avatar size="sm" name={displayName(peer)} src={peer.avatarUrl} {...fieldsFrame(peer)} />
          <div class="header-copy">
            <strong>{displayName(peer)}</strong>
            <span>{peer.username ? `@${peer.username}` : ''}{#if !$chatConnected} · {msg('social.chatOffline')}{/if}</span>
          </div>
        {:else}
          <MessageCircle size="1.8rem" />
          <div class="header-copy"><h2 class="list-title" tabindex="-1">{msg('social.chatMessages')}</h2><span>{msg('social.chatFriends')}</span></div>
        {/if}
        <IconButton label={msg('social.chatClose')} size="sm" onclick={closePanel}><X size="1.7rem" /></IconButton>
      </header>

      {#if peer}
        <div class="history-note">{msg('social.chatSevenDayNote')}</div>
        <div class="messages" bind:this={scrollBox}>
          {#if $nextByPeer[peer.id]}
            <div class="load-more">
              <Button variant="ghost" size="sm" busy={$chatLoadingMore} onclick={() => earlier(peer.id)}>
                {#if !$chatLoadingMore}<ChevronUp size="1.4rem" />{/if}
                {$chatLoadingMore ? msg('social.chatLoadingHistory') : msg('social.chatEarlier')}
              </Button>
            </div>
          {/if}
          {#if $chatHistoryError || historyRetrying}
            <div class="history-error">
              <Notice tone="danger" compact bind:retrying={historyRetrying} retry={{ label: msg('social.chatRetry'), run: retryHistory, within: () => scrollBox }}>
                {$chatHistoryError}
              </Notice>
            </div>
          {/if}
          {#if $chatLoading && activeMessages.length === 0 && !$chatHistoryError}
            <div class="state"><Spinner label={msg('social.chatLoadingHistory')} /></div>
          {:else if activeMessages.length === 0 && !$chatHistoryError}
            <div class="state"><EmptyState size="inline" align="center" title={msg('social.chatNoHistory')} /></div>
          {:else}
            {#each activeMessages as message (message.id)}
              <ChatMessage
                {message}
                {peer}
                {now}
                canReact={conversation?.canSend}
                onerror={(text) => (error = text)}
                onresent={resent}
                onopenimages={(urls, position) => openImages(message.id, urls, position)}
                onimageload={() => imageLoaded(message)}
              />
            {/each}
          {/if}
          {#if $chatTyping[peer.id]}<div class="typing">{msg('social.chatTyping', { name: displayName(peer) })}</div>{/if}
        </div>

        {#if error}<div class="panel-notice"><Notice tone="danger" compact>{error}</Notice></div>{/if}
        {#if !$chatConnected}
          <div class="panel-notice">
            <Notice tone="warning" compact>
              {msg('social.chatReconnecting')}
              {#snippet action()}
                <Button size="sm" busy={reconnecting} onclick={reconnect}>{msg('social.chatReconnect')}</Button>
              {/snippet}
            </Notice>
          </div>
        {/if}
        {#if !canSend}
          <div class="panel-notice"><Notice compact announce={false}>{msg('social.chatDisabled')}</Notice></div>
        {:else}
          <ChatComposer bind:this={composerPart} {peer} bind:draft {sending} progress={sends[peer.id] ?? idle} onsubmit={() => void submit()} />
        {/if}
      {:else}
        <div class="conversation-list" bind:this={list}>
          {#if $chatConversationsError || conversationsRetrying}
            <div class="history-error">
              <Notice tone="danger" compact bind:retrying={conversationsRetrying} retry={{ label: msg('social.chatRetry'), run: retryConversations, within: () => list }}>
                {$chatConversationsError}
              </Notice>
            </div>
          {/if}
          <ChatConversations onopen={openConversation} />
        </div>
      {/if}
    </section>
  {/if}
  <Lightbox bind:open={lightboxOpen} bind:index={lightboxIndex} items={lightboxItems} />
</div>

<style>
  .chat-root { position: fixed; z-index: var(--z-chat); right: 2.4rem; bottom: 2.4rem; display: flex; flex-direction: column; align-items: flex-end; pointer-events: none; }
  .chat-root.sidebar-right { right: calc(2.4rem + var(--sidebar-now)); }
  .chat-root > * { pointer-events: auto; }
  /* The opener floats over the page: its shadow lifts it off whatever is under it. */
  .chat-toggle { display: flex; border-radius: var(--radius-control); box-shadow: var(--shadow-pop); }
  .chat-panel { display: flex; flex-direction: column; width: 38rem; height: min(64rem, calc(100vh - var(--titlebar-h) - 8rem)); overflow: hidden; background: var(--surface-2); border: 1px solid var(--border-strong); border-radius: var(--radius-lg); box-shadow: var(--shadow-pop); }
  .chat-header { display: flex; align-items: center; gap: 0.9rem; min-height: 5.6rem; padding: 0 1.2rem; border-bottom: 1px solid var(--border); background: var(--surface-3); }
  .header-copy, .toast-copy { min-width: 0; display: flex; flex-direction: column; gap: 0.15rem; text-align: left; }
  .header-copy { flex: 1; }
  .header-copy strong, .list-title { font-size: var(--font-sm); }
  .list-title { letter-spacing: normal; }
  .header-copy span, .toast-copy span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-3); font-size: var(--font-xs); }
  .history-note { padding: 0.7rem 1rem 0; color: var(--text-3); font-size: 1rem; text-align: center; }
  .messages { flex: 1; overflow-y: auto; padding: 1rem; }
  .load-more { display: flex; justify-content: center; margin-bottom: 1rem; }
  .state { display: grid; place-items: center; min-height: 13rem; padding: 2rem; }
  .history-error { margin-bottom: 0.7rem; }
  .typing { margin: 0.6rem 0; color: var(--text-3); font-size: var(--font-xs); font-style: italic; }
  .panel-notice { padding: 0.7rem 1rem 0; }
  .conversation-list { flex: 1; overflow-y: auto; padding: 0.8rem; }
  .chat-toast { display: flex; align-items: center; gap: 0.4rem; width: 32rem; margin-bottom: 1rem; padding: 0.6rem; border: 1px solid var(--border-strong); border-radius: var(--radius-md); background: var(--surface-4); color: var(--text); box-shadow: var(--shadow-pop); }
  .toast-open { display: flex; flex: 1; align-items: center; gap: 0.8rem; min-width: 0; padding: 0.4rem; border: 0; border-radius: var(--radius-sm); background: none; color: inherit; font: inherit; cursor: pointer; text-align: left; }
  .toast-copy { flex: 1; }
  .toast-copy strong { font-size: var(--font-sm); }
  @media (max-width: 640px) { .chat-panel { width: calc(100vw - 2.4rem); height: calc(100vh - var(--titlebar-h) - 6rem); } .chat-toast { width: calc(100vw - 2.4rem); } }
</style>
