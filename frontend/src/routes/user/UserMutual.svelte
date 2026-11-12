<script lang="ts">
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import { cardFrame } from '../../lib/profile/frameLook';
  import Card from '../../lib/components/Card.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import type { UserCard } from '../../lib/services/social';
  import { mutualMore } from '../../lib/social/view';
  import { navigate } from '../../lib/stores/router';
  import { msg } from '../../lib/i18n';

  let { friends, count }: { friends: UserCard[]; count: number } = $props();

  const shown = $derived(friends.slice(0, 6));
  const more = $derived(mutualMore(shown.length, count));
</script>

<Card title={msg('social.userMutualTitle', { count })}>
  {#snippet action()}
    <span class="all"><TextLink onclick={() => navigate('friends')}>{msg('social.viewAllButton')}</TextLink></span>
  {/snippet}
  <div class="row">
    {#each shown as friend (friend.id)}
      <Tooltip text={friend.displayName || friend.username} describe={false}>
        <button
          class="person"
          type="button"
          aria-label={friend.displayName || friend.username}
          onclick={() => navigate('user', { username: friend.username })}
        >
          <Avatar size="sm" name={friend.displayName || friend.username} src={friend.avatarUrl} {...cardFrame(friend.card)} />
        </button>
      </Tooltip>
    {/each}
    {#if more}<span class="more">{more}</span>{/if}
  </div>
</Card>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-wrap: wrap;
  }

  .person {
    display: inline-flex;
    padding: 0;
    border: 0;
    background: none;
    border-radius: 50%;
    cursor: pointer;
    transition: transform var(--dur-fast) var(--ease);
  }

  .person:hover {
    transform: translateY(-1px);
  }

  .more {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 3.2rem;
    height: 3.2rem;
    padding: 0 0.6rem;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .all {
    font-size: var(--font-sm);
    font-weight: 500;
  }
</style>
