<script lang="ts">
  import type { FeedEvent } from '../../services/social';
  import { REACTIONS, hasReacted, reactionCount, reactionLabel } from '../../social/feed';
  import { reactionIcons } from './icons';

  let {
    event,
    disabled = false,
    ontoggle,
  }: {
    event: FeedEvent;
    disabled?: boolean;
    ontoggle: (emoji: string) => void;
  } = $props();
</script>

<div class="bar">
  {#each REACTIONS as id (id)}
    {@const Icon = reactionIcons[id]}
    {@const count = reactionCount(event, id)}
    {@const mine = hasReacted(event, id)}
    <button
      class="reaction"
      class:mine
      class:idle={count === 0}
      type="button"
      {disabled}
      aria-pressed={mine}
      aria-label={reactionLabel(id)}
      title={reactionLabel(id)}
      onclick={() => ontoggle(id)}
    >
      <Icon />
      {#if count > 0}
        <span class="count">{count}</span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .reaction {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    height: 2.6rem;
    padding: 0 0.7rem;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
    font-size: 1.4rem;
    line-height: 1;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }

  .reaction.idle {
    background: transparent;
    opacity: 0.45;
  }

  .reaction:hover:not(:disabled) {
    background: var(--hover-strong);
    opacity: 1;
  }

  .reaction.mine {
    background: var(--accent-subtle);
    border-color: var(--accent-ring);
    color: var(--accent-text);
    border-radius: var(--radius-md);
    opacity: 1;
  }

  .reaction:disabled {
    cursor: default;
  }

  .count {
    font-size: var(--font-xs);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
</style>
