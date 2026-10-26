<script lang="ts">
  import { ArrowLeft, Check, ChevronUp, MessageCircle, MoreHorizontal, Paperclip, Pencil, RefreshCw, Send, X } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import Avatar from './Avatar.svelte';
  import { fieldsFrame } from '../profile/frameLook';
  import { themeLayout } from '../stores/layout';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import ChatAttachments from './ChatAttachments.svelte';
  import Lightbox, { type LightboxItem } from './Lightbox.svelte';
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
    editMessage,
    editingMessageId,
    failedMessages,
    getDraft,
    loadMessages,
    loadMore,
    messagesByPeer,
    openChat,
    pendingMessages,
    retryMessage,
    retryMessaging,
    sendMessage,
    setAttachmentDrafts,
    setDraft,
    setPanelVisible,
    setTyping,
    toggleReaction,
    nextByPeer,
  } from '../stores/messaging';
  import { currentUser } from '../stores/user';
  import { toast } from '../stores/toasts';
  import { bytesSize, clockTime, shortDate } from '../utils/format';
  import { chatTextParts } from '../social/chatText';
  import { chatErrorText } from '../social/chatErrors';
  import { appendDrafts, attachmentsOf, checkChatImage, clipboardImages, draftFromFile, draftFromPicked, messagePreview, type ChatAttachmentDraft } from '../social/chatImage';
  import { pickChatImages, uploadChatImage, REACTION_KEYS, type Attachment, type ChatPeer, type Message, type ReactionKey } from '../services/messaging';
  import { reactionIcons } from './reactions/icons';
  import { msg, t } from '../i18n';

  let draft = $state('');
  let editDraft = $state('');
  let composer = $state<HTMLTextAreaElement | undefined>(undefined);
  let scrollBox = $state<HTMLElement | undefined>(undefined);
  let sending = $state(false);
  let uploadProgress = $state({ current: 0, total: 0 });
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let lightboxItems = $state<LightboxItem[]>([]);
  let error = $state('');
  let reactionMenu = $state<string | null>(null);
  let now = $state(new Date());
  let lastRenderedLastMessage = '';

  const peer = $derived($activePeer);
  const activeMessages = $derived(peer ? ($messagesByPeer[peer.id] ?? []) : []);
  const conversation = $derived(peer ? $conversations.find((item) => item.peer.id === peer.id) : undefined);
  const canSend = $derived(conversation?.canSend ?? $canSendByPeer[peer?.id ?? ''] ?? true);
  const draftList = $derived(peer ? ($attachmentDrafts[peer.id] ?? []) : []);
  const hasUnread = $derived($conversations.some((item) => item.unread > 0));

  function displayName(target: ChatPeer): string {
    return target.displayName || target.username;
  }

  const reactionLabels: Record<ReactionKey, Parameters<typeof msg>[0]> = {
    fire: 'social.reactionFire',
    salute: 'social.reactionSalute',
    heart: 'social.reactionHeart',
    clap: 'social.reactionClap',
    skull: 'social.reactionSkull',
    party: 'social.reactionParty',
    eyes: 'social.reactionEyes',
    joy: 'social.reactionJoy',
  };

  function isOwn(message: Message): boolean {
    return message.senderId === $currentUser?.id;
  }

  function time(iso: string, current: Date, yesterdayLabel: string): string {
    const value = new Date(iso);
    if (Number.isNaN(value.getTime())) return '';
    const clock = clockTime(value);
    if (value.toDateString() === current.toDateString()) return clock;
    const yesterday = new Date(current);
    yesterday.setDate(yesterday.getDate() - 1);
    const day = value.toDateString() === yesterday.toDateString() ? yesterdayLabel : shortDate(value);
    return `${day}, ${clock}`;
  }

  function setComposer(value: string): void {
    draft = value;
    if (peer) {
      setDraft(peer.id, draft);
      void setTyping(peer.id, draft.trim().length > 0);
    }
  }

  function addDrafts(targetPeerId: string, incoming: ChatAttachmentDraft[]): void {
    const { drafts, overflow } = appendDrafts($attachmentDrafts[targetPeerId] ?? [], incoming);
    setAttachmentDrafts(targetPeerId, drafts);
    if (overflow) toast(msg('chat.too_many_attachments'), 'danger');
  }

  async function attach(): Promise<void> {
    if (!peer || !canSend || sending) return;
    const targetPeerId = peer.id;
    try {
      const images = await pickChatImages();
      const accepted: ChatAttachmentDraft[] = [];
      for (const image of images) {
        const checked = checkChatImage({ type: image.mime, size: image.bytes });
        if (checked.ok) accepted.push(draftFromPicked(image));
        else toast(msg(checked.code), 'danger');
      }
      if (accepted.length > 0) addDrafts(targetPeerId, accepted);
    } catch (err) {
      toast(chatErrorText(err, 'chat.image_unreadable'), 'danger');
    }
  }

  async function onPaste(event: ClipboardEvent): Promise<void> {
    const files = clipboardImages(event.clipboardData?.items);
    if (files.length === 0) return;
    event.preventDefault();
    if (!peer || !canSend || sending) return;
    const targetPeerId = peer.id;
    const accepted: ChatAttachmentDraft[] = [];
    for (const file of files) {
      const checked = checkChatImage(file);
      if (!checked.ok) {
        toast(msg(checked.code), 'danger');
        continue;
      }
      try {
        accepted.push(await draftFromFile(file));
      } catch (err) {
        toast(chatErrorText(err, 'chat.image_unreadable'), 'danger');
      }
    }
    if (accepted.length > 0) addDrafts(targetPeerId, accepted);
  }

  function removeAttachment(key: string): void {
    if (peer && !sending) setAttachmentDrafts(peer.id, draftList.filter((item) => item.key !== key));
  }

  async function uploadDrafts(targetPeerId: string, pending: ChatAttachmentDraft[]): Promise<Attachment[] | null> {
    let list = pending;
    const total = list.filter((item) => !item.uploaded).length;
    let done = 0;
    uploadProgress = { current: 0, total };
    for (const item of pending) {
      if (item.uploaded) continue;
      uploadProgress = { current: done + 1, total };
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
    const pending = draftList;
    if (!peer || !canSend || (!draft.trim() && pending.length === 0) || sending) return;
    const targetPeerId = peer.id;
    const submitted = draft.trim();
    sending = true;
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
        composer?.focus();
        scrollToEnd();
      }
    } catch (err) {
      error = chatErrorText(err, 'social.chatSendError');
    } finally {
      uploadProgress = { current: 0, total: 0 };
      sending = false;
    }
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter' || event.shiftKey) return;
    event.preventDefault();
    void submit();
  }

  function startEdit(message: Message): void {
    editingMessageId.set(message.id);
    editDraft = message.text;
    reactionMenu = null;
  }

  async function saveEdit(message: Message): Promise<void> {
    if (!peer || (!editDraft.trim() && attachmentsOf(message).length === 0)) return;
    try {
      await editMessage(peer.id, message.id, editDraft.trim());
    } catch (err) {
      error = chatErrorText(err, 'social.chatEditError');
    }
  }

  async function retry(message: Message): Promise<void> {
    if (!peer) return;
    error = '';
    const targetPeerId = peer.id;
    try {
      await retryMessage(targetPeerId, message.clientId);
      if ($activePeer?.id === targetPeerId && draft.trim() === message.text.trim()) draft = '';
    } catch (err) {
      error = chatErrorText(err, 'social.chatSendError');
    }
  }

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

  async function chooseReaction(message: Message, key: ReactionKey): Promise<void> {
    if (!peer || !isOwn(message) && !conversation?.canSend) return;
    const targetPeerId = peer.id;
    reactionMenu = null;
    error = '';
    try {
      await toggleReaction(targetPeerId, message, key);
    } catch {
      if ($activePeer?.id === targetPeerId) error = msg('social.chatReactionError');
    }
  }

  function openToast(): void {
    const item = $chatToast;
    if (!item) return;
    chatToast.set(null);
    openChat(item.peer);
  }

  function openConversation(target: ChatPeer): void {
    openChat(target);
    draft = getDraft(target.id);
    error = '';
    chatToast.set(null);
  }

  function showList(): void {
    closeChat();
    chatOpen.set(true);
    draft = '';
  }

  function retryHistory(): void {
    if (!peer) return;
    if ($nextByPeer[peer.id]) void loadMore(peer.id).catch(() => undefined);
    else void loadMessages(peer.id).catch(() => undefined);
  }

  $effect(() => {
    const target = peer;
    draft = target ? getDraft(target.id) : '';
    error = '';
    reactionMenu = null;
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

<div class="chat-root" class:sidebar-right={$themeLayout.side === 'right'}>
  {#if $chatToast}
    <button class="chat-toast" type="button" onclick={openToast} out:fade={{ duration: 240 }}>
      <Avatar size="sm" name={displayName($chatToast.peer)} src={$chatToast.peer.avatarUrl} {...fieldsFrame($chatToast.peer)} />
      <span class="toast-copy">
        <strong>{$chatToast.peer.displayName || $chatToast.peer.username}</strong>
        <span>{messagePreview($chatToast.message)}</span>
      </span>
      <X size="1.5rem" strokeWidth={1.8} />
    </button>
  {/if}

  {#if !$chatOpen}
    <button class="chat-toggle" type="button" aria-label={msg('social.chatOpen')} onclick={() => (chatOpen.set(true))}>
      <MessageCircle size="2rem" strokeWidth={1.8} />
      {#if hasUnread}<span class="unread-dot">{$conversations.reduce((sum, item) => sum + item.unread, 0)}</span>{/if}
    </button>
  {:else}
    <section class="chat-panel" aria-label={msg('social.chatPanelLabel')}>
      <header class="chat-header">
        {#if peer}
          <button class="back" type="button" aria-label={msg('social.chatBack')} onclick={showList}><ArrowLeft size="1.7rem" /></button>
          <Avatar size="sm" name={displayName(peer)} src={peer.avatarUrl} {...fieldsFrame(peer)} />
          <div class="header-copy">
            <strong>{displayName(peer)}</strong>
            <span>{peer.username ? `@${peer.username}` : ''}{#if !$chatConnected} · {msg('social.chatOffline')}{/if}</span>
          </div>
        {:else}
          <MessageCircle size="1.8rem" />
          <div class="header-copy"><strong>{msg('social.chatMessages')}</strong><span>{msg('social.chatFriends')}</span></div>
        {/if}
        <IconButton label={msg('social.chatClose')} size="sm" onclick={closeChat}><X size="1.7rem" /></IconButton>
      </header>

      {#if peer}
        <div class="history-note">{msg('social.chatSevenDayNote')}</div>
        <div class="messages" bind:this={scrollBox}>
          {#if $nextByPeer[peer.id]}
            <button class="load-more" type="button" disabled={$chatLoadingMore} onclick={() => loadMore(peer.id).catch(() => undefined)}>
              <ChevronUp size="1.4rem" /> { $chatLoadingMore ? msg('social.chatLoadingHistory') : msg('social.chatEarlier') }
            </button>
          {/if}
          {#if $chatHistoryError}
            <div class="history-error">{$chatHistoryError}<button type="button" onclick={retryHistory}>{msg('social.chatRetry')}</button></div>
          {/if}
          {#if $chatLoading && activeMessages.length === 0 && !$chatHistoryError}
            <div class="state">{msg('social.chatLoadingHistory')}</div>
          {:else if activeMessages.length === 0 && !$chatHistoryError}
            <div class="state">{msg('social.chatNoHistory')}</div>
          {:else}
            {#each activeMessages as message (message.id)}
              <div class="message-row" class:own={isOwn(message)}>
                <article class="message" class:failed={$failedMessages.has(message.clientId)} aria-busy={$pendingMessages.has(message.clientId)}>
                  <div class="message-meta">
                    <span>{isOwn(message) ? msg('social.chatYou') : displayName(peer)}</span>
                    <time datetime={message.createdAt}>{time(message.createdAt, now, $t('format.yesterday'))}</time>
                    {#if message.editedAt}<span class="edited">{msg('social.chatEdited')}</span>{/if}
                  </div>
                  {#if $editingMessageId === message.id}
                    <textarea class="edit-input" bind:value={editDraft} maxlength="2000" rows="3"></textarea>
                    <div class="message-actions">
                      <Button size="sm" onclick={() => editingMessageId.set(null)}>{msg('social.chatCancel')}</Button>
                      <Button size="sm" variant="primary" onclick={() => saveEdit(message)}><Check size="1.3rem" /> {msg('social.chatSave')}</Button>
                    </div>
                  {:else}
                    {@const attachments = attachmentsOf(message)}
                    {#if message.text.trim() || attachments.length === 0}
                      <p class="message-text">
                        {#each chatTextParts(message.text) as part}
                          {#if part.href}<a href={part.href} target="_blank" rel="noopener noreferrer">{part.text}</a>{:else}{part.text}{/if}
                        {/each}
                      </p>
                    {/if}
                    <ChatAttachments {attachments} onopen={(urls, position) => openImages(message.id, urls, position)} onload={() => imageLoaded(message)} />
                    <div class="message-footer">
                      {#if $pendingMessages.has(message.clientId)}
                        <span class="sending-status" role="status">{msg('social.chatSending')}</span>
                      {/if}
                      {#if isOwn(message) && !message.id.startsWith('local:')}
                        <button class="tiny-action" type="button" title={msg('social.chatEdit')} onclick={() => startEdit(message)}><Pencil size="1.25rem" /></button>
                      {/if}
                      {#if $failedMessages.has(message.clientId)}
                        <button class="retry" type="button" onclick={() => retry(message)}><RefreshCw size="1.25rem" /> {msg('social.chatRetry')}</button>
                      {/if}
                      {#if !message.id.startsWith('local:')}
                      <button class="tiny-action" type="button" title={msg('social.chatReaction')} onclick={() => (reactionMenu = reactionMenu === message.id ? null : message.id)}><MoreHorizontal size="1.3rem" /></button>
                      {/if}
                    </div>
                    {#if message.reactions.length > 0}
                      <div class="reactions">
                        {#each message.reactions as reaction (reaction.emoji)}
                          {@const ReactionIcon = Object.hasOwn(reactionIcons, reaction.emoji) ? reactionIcons[reaction.emoji as ReactionKey] : undefined}
                          <button class:reacted={reaction.userIds.includes($currentUser?.id ?? '')} type="button" onclick={() => chooseReaction(message, reaction.emoji as ReactionKey)}>
                            {#if ReactionIcon}<ReactionIcon />{:else}{reaction.emoji}{/if} {reaction.userIds.length}
                          </button>
                        {/each}
                      </div>
                    {/if}
                    {#if reactionMenu === message.id}
                      <div class="reaction-menu">
                        {#each REACTION_KEYS as key}
                          {@const MenuIcon = reactionIcons[key]}
                          <button type="button" title={msg(reactionLabels[key])} onclick={() => chooseReaction(message, key)}><MenuIcon /></button>
                        {/each}
                      </div>
                    {/if}
                  {/if}
                </article>
              </div>
            {/each}
          {/if}
          {#if $chatTyping[peer.id]}<div class="typing">{msg('social.chatTyping', { name: displayName(peer) })}</div>{/if}
        </div>

        {#if error}<div class="chat-error">{error}</div>{/if}
        {#if !$chatConnected}
          <div class="disabled-copy" role="status">{msg('social.chatReconnecting')} <button class="connection-retry" type="button" onclick={() => retryMessaging()}>{msg('social.chatReconnect')}</button></div>
        {/if}
        {#if !canSend}
          <div class="disabled-copy">{msg('social.chatDisabled')}</div>
        {:else}
          {#if draftList.length > 0}
            <div class="draft-list">
              {#each draftList as item (item.key)}
                <div class="draft-chip">
                  <img src={item.previewUrl} alt={msg('social.chatImageLabel')} />
                  <span class="draft-copy">
                    <strong>{item.name}</strong>
                    <span>{bytesSize(item.bytes)}</span>
                  </span>
                  <button class="tiny-action" type="button" title={msg('social.chatAttachRemove')} aria-label={msg('social.chatAttachRemove')} disabled={sending} onclick={() => removeAttachment(item.key)}><X size="1.4rem" /></button>
                </div>
              {/each}
            </div>
            {#if uploadProgress.total > 0}
              <div class="upload-status" role="status">{uploadProgress.total === 1 ? msg('social.chatUploading') : msg('social.chatUploadProgress', { current: uploadProgress.current, total: uploadProgress.total })}</div>
            {/if}
          {/if}
          <div class="composer">
            <button class="attach" type="button" title={msg('social.chatAttachHint')} aria-label={msg('social.chatAttach')} disabled={sending} onclick={() => attach()}><Paperclip size="1.7rem" /></button>
            <textarea
              bind:this={composer}
              value={draft}
              maxlength="2000"
              rows="2"
              placeholder={msg('social.chatPlaceholder')}
              disabled={sending}
              oninput={(event) => setComposer((event.currentTarget as HTMLTextAreaElement).value)}
              onkeydown={onKeydown}
              onpaste={onPaste}
            ></textarea>
            <button class="send" type="button" aria-label={msg('social.chatSend')} disabled={(!draft.trim() && draftList.length === 0) || sending} onclick={() => submit()}><Send size="1.7rem" /></button>
          </div>
          <div class="composer-hint">{msg('social.chatHint')}</div>
        {/if}
      {:else}
        <div class="conversation-list">
          {#if $chatConversationsError}
            <div class="history-error">{$chatConversationsError}<button type="button" onclick={() => retryMessaging()}>{msg('social.chatRetry')}</button></div>
          {/if}
          {#if $conversations.length === 0 && !$chatConversationsError}
            <div class="state">{msg('social.chatNoConversations')}</div>
          {:else}
            {#each $conversations as item (item.peer.id)}
              <button class="conversation" type="button" onclick={() => openConversation(item.peer)}>
                <Avatar size="md" name={displayName(item.peer)} src={item.peer.avatarUrl} {...fieldsFrame(item.peer)} />
                <span class="conversation-copy">
                  <strong>{displayName(item.peer)}</strong>
                  <span>{item.lastMessage ? messagePreview(item.lastMessage) : msg('social.chatNewConversation')}</span>
                </span>
                {#if item.unread > 0}<span class="conversation-unread">{item.unread}</span>{/if}
              </button>
            {/each}
          {/if}
        </div>
      {/if}
    </section>
  {/if}
  <Lightbox bind:open={lightboxOpen} bind:index={lightboxIndex} items={lightboxItems} />
</div>

<style>
  .chat-root { position: fixed; z-index: 140; right: 2.4rem; bottom: 2.4rem; display: flex; flex-direction: column; align-items: flex-end; pointer-events: none; }
  .chat-root.sidebar-right { right: calc(2.4rem + var(--sidebar-now)); }
  .chat-root > * { pointer-events: auto; }
  .chat-toggle { position: relative; display: flex; align-items: center; justify-content: center; width: 4.8rem; height: 4.8rem; border: 1px solid var(--border-strong); border-radius: 50%; background: var(--accent); color: var(--accent-on, #fff); box-shadow: var(--shadow-pop); cursor: pointer; }
  .chat-toggle:hover { background: var(--accent-hover); }
  .unread-dot { position: absolute; top: -0.5rem; right: -0.5rem; min-width: 2rem; height: 2rem; padding: 0 0.45rem; border-radius: 2rem; background: var(--danger); color: #fff; font-size: var(--font-xs); display: flex; align-items: center; justify-content: center; }
  .chat-panel { display: flex; flex-direction: column; width: 38rem; height: min(64rem, calc(100vh - 8rem)); overflow: hidden; background: var(--surface-2); border: 1px solid var(--border-strong); border-radius: var(--radius-lg); box-shadow: var(--shadow-pop); }
  .chat-header { display: flex; align-items: center; gap: 0.9rem; min-height: 5.6rem; padding: 0 1.2rem; border-bottom: 1px solid var(--border); background: var(--surface-3); }
  .back { display: inline-flex; border: 0; background: transparent; color: var(--text-2); padding: 0.5rem; cursor: pointer; }
  .header-copy, .conversation-copy, .toast-copy { min-width: 0; display: flex; flex-direction: column; gap: 0.15rem; text-align: left; }
  .header-copy { flex: 1; }
  .header-copy strong { font-size: var(--font-sm); }
  .header-copy span, .conversation-copy span, .toast-copy span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-3); font-size: var(--font-xs); }
  .history-note { padding: 0.7rem 1rem 0; color: var(--text-3); font-size: 1rem; text-align: center; }
  .messages { flex: 1; overflow-y: auto; padding: 1rem; }
  .load-more { display: flex; align-items: center; gap: 0.4rem; margin: 0 auto 1rem; border: 0; background: transparent; color: var(--accent-text); font-size: var(--font-xs); cursor: pointer; }
  .state { display: grid; place-items: center; min-height: 13rem; padding: 2rem; color: var(--text-3); text-align: center; font-size: var(--font-sm); }
  .sending-status { color: var(--text-3); font-size: 1rem; }
  .connection-retry { border: 0; background: transparent; color: var(--accent-text); cursor: pointer; font: inherit; text-decoration: underline; }
  .history-error { display: flex; align-items: center; justify-content: center; gap: 0.7rem; margin-bottom: 0.7rem; color: var(--danger); font-size: var(--font-xs); text-align: center; }
  .history-error button { border: 0; background: transparent; color: var(--accent-text); cursor: pointer; font: inherit; text-decoration: underline; }
  .message-row { display: flex; justify-content: flex-start; margin: 0.8rem 0; }
  .message-row.own { justify-content: flex-end; }
  .message { position: relative; max-width: 86%; padding: 0.85rem 1rem; border: 1px solid var(--border); border-radius: 1.2rem 1.2rem 1.2rem 0.35rem; background: var(--surface-3); }
  .own .message { border-radius: 1.2rem 1.2rem 0.35rem 1.2rem; background: color-mix(in srgb, var(--accent) 18%, var(--surface-3)); }
  .message.failed { border-color: var(--danger); }
  .message-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 0.6rem; margin-bottom: 0.35rem; color: var(--text-3); font-size: 1rem; }
  .message-meta time { opacity: 0.75; white-space: nowrap; }
  .edited { font-style: italic; }
  .message-text { margin: 0; color: var(--text); font-size: var(--font-sm); line-height: 1.45; white-space: pre-wrap; overflow-wrap: anywhere; }
  .message-text a { color: var(--accent-text); text-decoration: underline; }
  .message-footer { display: flex; justify-content: flex-end; gap: 0.3rem; margin-top: 0.35rem; }
  .tiny-action, .retry { display: inline-flex; align-items: center; gap: 0.3rem; border: 0; padding: 0.25rem; background: transparent; color: var(--text-3); font-size: 1rem; cursor: pointer; }
  .retry { color: var(--danger); }
  .reactions, .reaction-menu { display: flex; flex-wrap: wrap; gap: 0.3rem; margin-top: 0.45rem; }
  .reactions button, .reaction-menu button { display: inline-flex; align-items: center; gap: 0.3rem; border: 1px solid var(--border); border-radius: 1rem; padding: 0.2rem 0.45rem; background: var(--surface-2); cursor: pointer; font-size: 1.2rem; }
  .reactions button :global(svg), .reaction-menu button :global(svg) { width: 1.4em; height: 1.4em; }
  .reactions button.reacted { border-color: var(--accent); background: var(--accent-subtle); }
  .reaction-menu { padding: 0.4rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface-4); }
  .typing { margin: 0.6rem 0; color: var(--text-3); font-size: var(--font-xs); font-style: italic; }
  .edit-input, .composer textarea { width: 100%; resize: none; border: 1px solid var(--border-strong); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); padding: 0.7rem; font: inherit; font-size: var(--font-sm); outline: none; }
  .edit-input:focus, .composer textarea:focus { border-color: var(--accent); }
  .message-actions { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.6rem; }
  .chat-error, .disabled-copy { padding: 0.7rem 1rem; color: var(--danger); font-size: var(--font-xs); }
  .disabled-copy { color: var(--text-3); }
  .composer { display: flex; align-items: flex-end; gap: 0.7rem; padding: 0.8rem 1rem 0; border-top: 1px solid var(--border); }
  .composer textarea { flex: 1; min-height: 4.2rem; }
  .send { display: flex; align-items: center; justify-content: center; width: 3.8rem; height: 3.8rem; flex-shrink: 0; border: 0; border-radius: 50%; background: var(--accent); color: var(--accent-on, #fff); cursor: pointer; }
  .send:disabled { opacity: 0.45; cursor: default; }
  .attach { display: flex; align-items: center; justify-content: center; width: 3.8rem; height: 3.8rem; flex-shrink: 0; border: 0; border-radius: 50%; background: transparent; color: var(--text-2); cursor: pointer; }
  .attach:hover:not(:disabled) { background: var(--hover); color: var(--text); }
  .attach:disabled { opacity: 0.45; cursor: default; }
  .draft-list { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; margin: 0.8rem 1rem 0; }
  .draft-chip { display: flex; align-items: center; gap: 0.6rem; min-width: 0; padding: 0.4rem 0.5rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface-3); }
  .draft-chip img { width: 3.2rem; height: 3.2rem; flex-shrink: 0; border-radius: var(--radius-sm); object-fit: cover; }
  .draft-copy { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.15rem; }
  .draft-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--font-xs); }
  .draft-copy span { color: var(--text-3); font-size: 1rem; }
  .upload-status { margin: 0.5rem 1rem 0; color: var(--text-3); font-size: 1rem; }
  .composer-hint { padding: 0.45rem 1rem 0.8rem; color: var(--text-3); font-size: 1rem; }
  .conversation-list { flex: 1; overflow-y: auto; padding: 0.8rem; }
  .conversation { display: flex; align-items: center; gap: 1rem; width: 100%; padding: 1rem; border: 0; border-radius: var(--radius-md); background: transparent; color: inherit; cursor: pointer; text-align: left; }
  .conversation:hover { background: var(--hover); }
  .conversation-copy { flex: 1; }
  .conversation-copy strong { font-size: var(--font-sm); }
  .conversation-unread { display: grid; place-items: center; min-width: 2rem; height: 2rem; padding: 0 0.4rem; border-radius: 2rem; background: var(--accent); color: var(--accent-on, #fff); font-size: var(--font-xs); }
  .chat-toast { display: flex; align-items: center; gap: 0.8rem; width: 32rem; margin-bottom: 1rem; padding: 1rem; border: 1px solid var(--border-strong); border-radius: var(--radius-md); background: var(--surface-4); color: var(--text); box-shadow: var(--shadow-pop); cursor: pointer; text-align: left; }
  .toast-copy { flex: 1; }
  .toast-copy strong { font-size: var(--font-sm); }
  @media (max-width: 640px) { .chat-panel { width: calc(100vw - 2.4rem); height: calc(100vh - 6rem); } .chat-toast { width: calc(100vw - 2.4rem); } }
</style>
