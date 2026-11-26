<script lang="ts" generics="T extends UserCard">
  import type { Snippet } from 'svelte';
  import { fly } from 'svelte/transition';
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
    /** Whether a card's action is in flight: that card dims while it runs. */
    busy: (id: string) => boolean;
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
</script>

<!-- A card that leaves goes at once, with no fade: a fading card would still hold the focus of its
     pressed button, and the row rule would find it still there (rowFocus.ts). -->
<div class="cards">
  {#each users as user (user.id)}
    <div
      class="request"
      data-friends-row={user.id}
      class:settling={busy(user.id)}
      in:fly={{ y: '0.8rem', duration: motion('--dur-panel') }}
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
