<script lang="ts">
  import { MessageCircle } from '@lucide/svelte';
  import IconButton from '../lib/components/IconButton.svelte';
  import { msg } from '../lib/i18n';
  import type { ChatPeer } from '../lib/services/messaging';
  import type { FriendView } from '../lib/services/social';
  import { presenceDot, presenceLine, sortFriends } from '../lib/social/presence';
  import { friendsPage } from '../lib/stores/social';
  import FriendRow from '../routes/friends/FriendRow.svelte';
  import { overlayFriendsFailed, retryFriends } from './data';

  let { onchat }: { onchat: (peer: ChatPeer) => void } = $props();

  const list = $derived(sortFriends($friendsPage.friends));

  function peerOf(friend: FriendView): ChatPeer {
    return {
      id: friend.id,
      username: friend.username,
      displayName: friend.displayName,
      avatarUrl: friend.avatarUrl,
    };
  }
</script>

<div class="friends">
  {#if $overlayFriendsFailed}
    <div class="notice error">
      {msg('overlay.friendsError')}
      <button type="button" onclick={() => retryFriends()}>{msg('common.retry')}</button>
    </div>
  {/if}
  {#if list.length === 0 && !$overlayFriendsFailed}
    <div class="notice">{msg('social.friendsEmptyNobodyTitle')}</div>
  {:else}
    {#each list as friend (friend.id)}
      <FriendRow
        user={friend}
        status={presenceDot(friend.presence)}
        meta={presenceLine(friend.presence)}
        compact
        onopen={() => onchat(peerOf(friend))}
      >
        {#snippet actions()}
          <IconButton label={msg('social.chatWrite')} size="sm" onclick={() => onchat(peerOf(friend))}>
            <MessageCircle size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {/snippet}
      </FriendRow>
    {/each}
  {/if}
</div>

<style>
  .friends {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-2);
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

  .notice.error {
    min-height: 0;
    padding: var(--space-2) var(--space-3);
    color: var(--danger);
    font-size: var(--font-xs);
  }

  .notice button {
    border: 0;
    background: transparent;
    color: var(--accent-text);
    font: inherit;
    text-decoration: underline;
    cursor: pointer;
  }
</style>
