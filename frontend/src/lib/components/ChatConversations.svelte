<script lang="ts">
  import Avatar from './Avatar.svelte';
  import { fieldsFrame, frameReserve } from '../profile/frameLook';
  import CountBadge from './CountBadge.svelte';
  import EmptyState from './EmptyState.svelte';
  import { chatConversationsError, conversations } from '../stores/messaging';
  import { messagePreview } from '../social/chatImage';
  import type { ChatPeer } from '../services/messaging';
  import { msg } from '../i18n';

  let { onopen }: { onopen: (peer: ChatPeer) => void } = $props();

  const conversationReserve = $derived(frameReserve($conversations.map((item) => fieldsFrame(item.peer))));

  function displayName(target: ChatPeer): string {
    return target.displayName || target.username;
  }
</script>

{#if $conversations.length === 0 && !$chatConversationsError}
  <div class="state"><EmptyState size="inline" align="center" title={msg('social.chatNoConversations')} /></div>
{:else}
  {#each $conversations as item (item.peer.id)}
    <button class="conversation" type="button" data-peer={item.peer.id} onclick={() => onopen(item.peer)}>
      <Avatar size="md" name={displayName(item.peer)} src={item.peer.avatarUrl} reserve={conversationReserve} {...fieldsFrame(item.peer)} />
      <span class="conversation-copy">
        <strong>{displayName(item.peer)}</strong>
        <span>{item.lastMessage ? messagePreview(item.lastMessage) : msg('social.chatNewConversation')}</span>
      </span>
      <CountBadge count={item.unread} label={msg('social.chatUnreadCount', { count: item.unread })} />
    </button>
  {/each}
{/if}

<style>
  .state { display: grid; place-items: center; min-height: 13rem; padding: 2rem; }
  .conversation { display: flex; align-items: center; gap: 1rem; width: 100%; padding: 1rem; border: 0; border-radius: var(--radius-md); background: transparent; color: inherit; cursor: pointer; text-align: left; }
  .conversation:hover { background: var(--hover); }
  .conversation-copy { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.15rem; text-align: left; }
  .conversation-copy strong { font-size: var(--font-sm); }
  .conversation-copy span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-3); font-size: var(--font-xs); }
</style>
