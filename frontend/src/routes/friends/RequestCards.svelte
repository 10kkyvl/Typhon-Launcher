<script lang="ts" generics="T extends UserCard">
  import type { Snippet } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import type { FrameReserve } from '../../lib/profile/frameLook';
  import type { UserCard } from '../../lib/services/social';
  import FriendRow from './FriendRow.svelte';

  let {
    users,
    busy,
    reserve,
    stats,
    onopen,
    actions: cardActions,
  }: {
    users: T[];
    /** The page's action in flight, `<action>:<user id>`: its card dims while it runs. */
    busy: string;
    reserve: FrameReserve;
    stats?: (user: T) => string[];
    onopen?: (user: T) => void;
    actions: Snippet<[T]>;
  } = $props();

  function motion(token: string): number {
    const root = document.documentElement;
    if (root.classList.contains('no-anim') || matchMedia('(prefers-reduced-motion: reduce)').matches) return 0;
    return parseFloat(getComputedStyle(root).getPropertyValue(token)) || 0;
  }

  function settling(id: string): boolean {
    return busy.endsWith(`:${id}`);
  }
</script>

<div class="cards">
  {#each users as user (user.id)}
    <div
      class="request"
      class:settling={settling(user.id)}
      in:fly={{ y: '0.8rem', duration: motion('--dur-panel') }}
      out:fade={{ duration: motion('--dur') }}
    >
      <FriendRow {user} variant="card" {reserve} stats={stats?.(user)} onopen={onopen && (() => onopen(user))}>
        {#snippet actions()}
          {@render cardActions(user)}
        {/snippet}
      </FriendRow>
    </div>
  {/each}
</div>

<style>
  .request {
    transition: opacity var(--dur) var(--ease);
  }

  .request.settling {
    opacity: 0.55;
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
</style>
