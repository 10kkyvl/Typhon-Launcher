<script lang="ts">
  import { tick } from 'svelte';
  import { Plus } from '@lucide/svelte';
  import Chip from '../Chip.svelte';
  import Tooltip from '../Tooltip.svelte';
  import ReactionPicker from './ReactionPicker.svelte';
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

  let popped = $state('');
  let bar = $state<HTMLElement>();

  const used = $derived(REACTIONS.filter((id) => reactionCount(event, id) > 0));
  const idle = $derived(REACTIONS.filter((id) => reactionCount(event, id) === 0));

  // While a reaction is being sent the bar takes no other (`disabled`): its chips are busy, so they
  // keep focus (a disabled button would drop it, and a keyboard user would lose their place in the
  // feed) and ignore presses.
  async function toggle(id: ReactionId) {
    if (disabled) return;
    const adding = !hasReacted(event, id);
    popped = adding ? id : '';
    ontoggle(id);
    await tick();
    // Focus follows the reaction onto the chip a pick has just added.
    if (adding) bar?.querySelector<HTMLElement>(`[data-reaction="${id}"]`)?.focus();
  }

  // A chip goes with its last reaction: taken back by the user, rolled back when the request failed,
  // or withdrawn by its only other author. Focus that was on it moves to the "+", not off the feed.
  $effect.pre(() => {
    void used;
    const active = document.activeElement;
    const held = active instanceof HTMLElement && active.matches('[data-reaction]') && bar?.contains(active) ? active : null;
    if (!held) return;
    void tick().then(() => {
      const now = document.activeElement;
      if (held.isConnected || (now !== null && now !== document.body)) return;
      bar?.querySelector<HTMLElement>('[aria-haspopup]')?.focus();
    });
  });
</script>

{#snippet reaction(id: ReactionId)}
  {@const Icon = reactionIcons[id]}
  {@const count = reactionCount(event, id)}
  {@const mine = hasReacted(event, id)}
  <Tooltip text={reactionLabel(id)} describe={false}>
    <Chip
      selected={mine}
      data-reaction={id}
      busy={disabled}
      aria-label={msg('social.reactionWithCount', { reaction: reactionLabel(id), count })}
      onclick={() => toggle(id)}
    >
      <span class="glyph" class:pop={mine && popped === id}><Icon /></span>
      <span class="count">{count}</span>
    </Chip>
  </Tooltip>
{/snippet}

<div class="bar" bind:this={bar}>
  {#each used as id (id)}
    {@render reaction(id)}
  {/each}
  {#if idle.length > 0}
    <ReactionPicker options={idle} label={msg('social.reactionAdd')} onpick={toggle}>
      {#snippet trigger({ open, props })}
        <Tooltip text={msg('social.reactionAdd')} describe={false}>
          <Chip
            variant="quiet"
            aria-label={msg('social.reactionAdd')}
            {...props}
            busy={disabled}
          >
            <span class="plus" class:open><Plus size="1.4rem" strokeWidth={2} /></span>
          </Chip>
        </Tooltip>
      {/snippet}
    </ReactionPicker>
  {/if}
</div>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .plus {
    display: inline-flex;
    transition: transform var(--dur) var(--ease);
  }

  .plus.open {
    transform: rotate(45deg);
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
