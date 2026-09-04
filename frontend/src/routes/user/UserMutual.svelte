<script lang="ts">
  import Avatar from '../../lib/components/Avatar.svelte';
  import type { UserCard } from '../../lib/services/social';
  import { mutualMore } from '../../lib/social/view';
  import { navigate } from '../../lib/stores/router';

  let { friends, count }: { friends: UserCard[]; count: number } = $props();

  const shown = $derived(friends.slice(0, 6));
  const more = $derived(mutualMore(shown.length, count));
</script>

<section class="group">
  <h3>Общие друзья</h3>
  <div class="row">
    {#each shown as friend (friend.id)}
      <button
        class="person"
        title={friend.displayName}
        onclick={() => navigate('user', { username: friend.username })}
      >
        <Avatar size="sm" name={friend.displayName || friend.username} src={friend.avatarUrl} />
      </button>
    {/each}
    {#if more}<span class="more">{more}</span>{/if}
  </div>
</section>

<style>
  .group {
    margin-bottom: var(--space-10);
  }

  h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-3);
  }

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
</style>
