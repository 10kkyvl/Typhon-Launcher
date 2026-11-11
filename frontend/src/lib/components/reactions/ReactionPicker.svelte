<script lang="ts">
  import type { Snippet } from 'svelte';
  import Popover, { type PopoverTriggerProps } from '../Popover.svelte';
  import Tooltip from '../Tooltip.svelte';
  import { reactionLabel, type ReactionId } from '../../social/feed';
  import type { Placement } from '../../utils/position';
  import { reactionIcons } from './icons';

  /**
   * The reactions to choose from, in a popover next to the control that opens it (the chat's
   * "Reaction", the feed's "+"). Each reaction is a button named for a screen reader and shown by a
   * tooltip. A pick closes the picker and puts focus back on the control before `onpick` runs; what
   * a pick does (toggle, add) is the caller's.
   */
  let {
    options,
    label,
    chosen = [],
    placement = 'bottom-start',
    onpick,
    trigger: opener,
  }: {
    options: readonly ReactionId[];
    /** The picker's name: the name of the control that opens it. */
    label: string;
    /** The user's own reactions among `options`: shown pressed. */
    chosen?: readonly string[];
    placement?: Placement;
    onpick: (id: ReactionId) => void;
    trigger: Snippet<[{ open: boolean; props: PopoverTriggerProps }]>;
  } = $props();

  function pick(id: ReactionId, close: () => void) {
    close();
    onpick(id);
  }
</script>

<Popover {label} {placement}>
  {#snippet trigger({ open, props })}
    {@render opener({ open, props })}
  {/snippet}
  {#snippet children({ close })}
    <div class="choices">
      {#each options as id (id)}
        {@const Icon = reactionIcons[id]}
        <Tooltip text={reactionLabel(id)} describe={false}>
          <button
            type="button"
            class="choice"
            aria-label={reactionLabel(id)}
            aria-pressed={chosen.includes(id)}
            onclick={() => pick(id, close)}
          >
            <Icon />
          </button>
        </Tooltip>
      {/each}
    </div>
  {/snippet}
</Popover>

<style>
  .choices {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }

  .choice {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 3.2rem;
    height: 3.2rem;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    font-size: 1.4rem;
    transition:
      background var(--dur-fast) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .choice :global(svg) {
    width: 1.4em;
    height: 1.4em;
  }

  .choice:hover,
  .choice:focus-visible {
    background: var(--hover-strong);
  }

  .choice:active {
    transform: scale(0.94);
  }

  .choice[aria-pressed='true'] {
    border-color: var(--accent-ring);
    background: var(--accent-subtle);
  }
</style>
