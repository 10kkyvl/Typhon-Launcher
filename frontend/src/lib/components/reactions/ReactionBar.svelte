<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import type { FeedEvent } from '../../services/social';
  import { REACTIONS, hasReacted, reactionCount, reactionLabel, type ReactionId } from '../../social/feed';
  import { msg } from '../../i18n';
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

  let open = $state(false);
  let popped = $state('');

  const used = $derived(REACTIONS.filter((id) => reactionCount(event, id) > 0));
  const idle = $derived(REACTIONS.filter((id) => reactionCount(event, id) === 0));

  function toggle(id: ReactionId) {
    popped = hasReacted(event, id) ? '' : id;
    open = false;
    ontoggle(id);
  }
</script>

{#snippet reaction(id: ReactionId)}
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
    onclick={() => toggle(id)}
  >
    <span class="glyph" class:pop={mine && popped === id}><Icon /></span>
    {#if count > 0}
      <span class="count">{count}</span>
    {/if}
  </button>
{/snippet}

<div class="bar">
  {#each used as id (id)}
    {@render reaction(id)}
  {/each}
  {#if idle.length > 0}
    <button
      class="reaction add"
      class:open
      type="button"
      {disabled}
      aria-expanded={open}
      aria-label={msg('social.reactionAdd')}
      title={msg('social.reactionAdd')}
      onclick={() => (open = !open)}
    >
      <span class="plus"><Plus size="1.4rem" strokeWidth={2} /></span>
    </button>
    {#if open}
      <span class="tray">
        {#each idle as id (id)}
          {@render reaction(id)}
        {/each}
      </span>
    {/if}
  {/if}
</div>

<style>
  .bar,
  .tray {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .tray {
    animation: media-in var(--dur) var(--ease);
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
      transform var(--dur-fast) var(--ease);
  }

  .reaction.idle,
  .reaction.add {
    background: transparent;
  }

  .reaction.add {
    color: var(--text-3);
  }

  .plus {
    display: inline-flex;
    transition: transform var(--dur) var(--ease);
  }

  .add.open .plus {
    transform: rotate(45deg);
  }

  .reaction:hover:not(:disabled) {
    background: var(--hover-strong);
    color: var(--text);
  }

  .reaction:active:not(:disabled) {
    transform: scale(0.94);
  }

  .reaction.mine {
    background: var(--accent-subtle);
    border-color: var(--accent-ring);
    color: var(--accent-text);
  }

  .reaction:disabled {
    cursor: default;
  }

  .glyph {
    display: inline-flex;
  }

  .glyph.pop {
    animation: pop-in var(--dur-slow) var(--ease-spring);
  }

  .count {
    font-size: var(--font-xs);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
</style>
