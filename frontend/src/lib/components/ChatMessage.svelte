<script lang="ts">
  import { Check, MoreHorizontal, Pencil, RefreshCw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import IconButton from './IconButton.svelte';
  import ChatAttachments from './ChatAttachments.svelte';
  import Spinner from './Spinner.svelte';
  import ReactionPicker from './reactions/ReactionPicker.svelte';
  import { editMessage, editingMessageId, failedMessages, pendingMessages, retryMessage, sessionWatch, toggleReaction } from '../stores/messaging';
  import { currentUser } from '../stores/user';
  import { clockTime, shortDate } from '../utils/format';
  import { chatTextParts } from '../social/chatText';
  import { chatErrorText } from '../social/chatErrors';
  import { attachmentsOf } from '../social/chatImage';
  import { REACTION_KEYS, type ChatPeer, type Message, type ReactionKey } from '../services/messaging';
  import { reactionIcons } from './reactions/icons';
  import { reactionLabel } from '../social/feed';
  import { focusHandOff } from '../utils/focusHandOff';
  import { msg, t } from '../i18n';

  /**
   * One message of the open conversation: its text and images, its state (being sent, failed), and what
   * can be done with it (edit, retry, react). A failure of one of those is the panel's to show
   * (`onerror`, for the conversation it happened in; '' clears it).
   */
  let {
    message,
    peer,
    now,
    canReact,
    onerror,
    onresent,
    onopenimages,
    onimageload,
  }: {
    message: Message;
    peer: ChatPeer;
    now: Date;
    /** Whether the conversation takes messages: a reaction to the other side's message needs it. */
    canReact: boolean | undefined;
    onerror: (peerId: string, text: string) => void;
    /** A failed message went through on retry: the composer may still hold its text. */
    onresent: (peerId: string, text: string, clientId: string, hand: ReturnType<typeof focusHandOff>) => void;
    onopenimages: (urls: string[], position: number) => void;
    onimageload: () => void;
  } = $props();

  let editDraft = $state('');
  let row = $state<HTMLElement>();
  let editor = $state<HTMLTextAreaElement>();
  let resending = $state(false);

  const own = $derived(message.senderId === $currentUser?.id);
  const myReactions = $derived(
    message.reactions.filter((reaction) => reaction.userIds.includes($currentUser?.id ?? '')).map((reaction) => reaction.emoji),
  );

  function displayName(target: ChatPeer): string {
    return target.displayName || target.username;
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

  const editButton = () => row?.querySelector<HTMLElement>('[data-edit]');

  // The control that was pressed goes when the editor opens or closes: focus it held moves to what
  // took its place (focusHandOff).
  function startEdit(): void {
    const hand = focusHandOff();
    editingMessageId.set(message.id);
    editDraft = message.text;
    void hand(() => editor);
  }

  function cancelEdit(): void {
    const hand = focusHandOff();
    editingMessageId.set(null);
    void hand(editButton);
  }

  // What an edit, a retry or a reaction awaits may answer after the session has ended, into another
  // account's: each reports back only while its own session is still the one running (sessionWatch).
  async function saveEdit(): Promise<void> {
    if (!editDraft.trim() && attachmentsOf(message).length === 0) return;
    const targetPeerId = peer.id;
    const hand = focusHandOff();
    const live = sessionWatch();
    try {
      await editMessage(targetPeerId, message.id, editDraft.trim());
      if (!live()) return;
      onerror(targetPeerId, '');
      await hand(editButton);
    } catch (err) {
      if (live()) onerror(targetPeerId, chatErrorText(err, 'social.chatEditError'));
    }
  }

  // The store takes the failed mark off at once; Retry stays, busy, until the answer.
  async function retry(): Promise<void> {
    if (resending) return;
    resending = true;
    const targetPeerId = peer.id;
    const hand = focusHandOff();
    const live = sessionWatch();
    onerror(targetPeerId, '');
    const { text, clientId } = message;
    try {
      await retryMessage(targetPeerId, clientId);
      if (live()) onresent(targetPeerId, text, clientId, hand);
    } catch (err) {
      if (live()) onerror(targetPeerId, chatErrorText(err, 'social.chatSendError'));
    } finally {
      resending = false;
    }
  }

  async function chooseReaction(key: ReactionKey): Promise<void> {
    if (!own && !canReact) return;
    const targetPeerId = peer.id;
    const live = sessionWatch();
    onerror(targetPeerId, '');
    try {
      await toggleReaction(targetPeerId, message, key);
    } catch {
      if (live()) onerror(targetPeerId, msg('social.chatReactionError'));
    }
  }
</script>

<div class="message-row" class:own data-client={message.clientId} bind:this={row}>
  <article class="message" class:failed={$failedMessages.has(message.clientId)} aria-busy={$pendingMessages.has(message.clientId)}>
    <div class="message-meta">
      <span>{own ? msg('social.chatYou') : displayName(peer)}</span>
      <time datetime={message.createdAt}>{time(message.createdAt, now, $t('format.yesterday'))}</time>
      {#if message.editedAt}<span class="edited">{msg('social.chatEdited')}</span>{/if}
    </div>
    {#if $editingMessageId === message.id}
      <textarea class="input edit-input" aria-label={msg('social.chatEditField')} bind:this={editor} bind:value={editDraft} maxlength="2000" rows="3"></textarea>
      <div class="message-actions">
        <Button size="sm" onclick={cancelEdit}>{msg('social.chatCancel')}</Button>
        <Button size="sm" variant="primary" onclick={saveEdit}><Check size="1.3rem" /> {msg('social.chatSave')}</Button>
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
      <ChatAttachments {attachments} onopen={onopenimages} onload={onimageload} />
      <div class="message-footer">
        {#if $pendingMessages.has(message.clientId)}
          <span class="sending-status"><Spinner size="sm" label={msg('social.chatSending')} /></span>
        {/if}
        {#if own && !message.id.startsWith('local:')}
          <IconButton size="sm" label={msg('social.chatEdit')} data-edit onclick={startEdit}><Pencil size="1.4rem" /></IconButton>
        {/if}
        {#if $failedMessages.has(message.clientId) || resending}
          <Button variant="ghost" size="sm" busy={resending} onclick={retry}>{#if !resending}<RefreshCw size="1.4rem" />{/if} {msg('social.chatRetry')}</Button>
        {/if}
        {#if !message.id.startsWith('local:')}
          <ReactionPicker options={REACTION_KEYS} label={msg('social.chatReaction')} chosen={myReactions} placement="bottom-end" onpick={(key) => void chooseReaction(key)}>
            {#snippet trigger({ props })}
              <IconButton size="sm" label={msg('social.chatReaction')} {...props}><MoreHorizontal size="1.4rem" /></IconButton>
            {/snippet}
          </ReactionPicker>
        {/if}
      </div>
      {#if message.reactions.length > 0}
        <div class="reactions">
          {#each message.reactions as reaction (reaction.emoji)}
            {@const ReactionIcon = Object.hasOwn(reactionIcons, reaction.emoji) ? reactionIcons[reaction.emoji as ReactionKey] : undefined}
            {@const mine = reaction.userIds.includes($currentUser?.id ?? '')}
            <Chip
              selected={mine}
              aria-label={msg('social.reactionWithCount', { reaction: reactionLabel(reaction.emoji), count: reaction.userIds.length })}
              onclick={() => chooseReaction(reaction.emoji as ReactionKey)}
            >
              <span class="glyph">{#if ReactionIcon}<ReactionIcon />{:else}{reaction.emoji}{/if}</span>
              <span class="count">{reaction.userIds.length}</span>
            </Chip>
          {/each}
        </div>
      {/if}
    {/if}
  </article>
</div>

<style>
  .sending-status { display: inline-flex; align-items: center; }
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
  .reactions { display: flex; flex-wrap: wrap; gap: 0.3rem; margin-top: 0.45rem; }
  .glyph { display: inline-flex; }
  .count { font-size: var(--font-xs); font-weight: 600; font-variant-numeric: tabular-nums; }
  .edit-input { height: auto; resize: none; padding: 0.7rem; font: inherit; font-size: var(--font-sm); }
  .message-actions { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.6rem; }
</style>
