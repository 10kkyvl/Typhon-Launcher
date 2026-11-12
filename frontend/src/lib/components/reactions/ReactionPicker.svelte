<script lang="ts">
  import type { Snippet } from 'svelte';
  import Popover, { type PopoverTriggerProps } from '../Popover.svelte';
  import IconButton from '../IconButton.svelte';
  import { reactionLabel, type ReactionId } from '../../social/feed';
  import type { Placement } from '../../utils/position';
  import { reactionIcons } from './icons';

  /**
   * The reactions to choose from, in a popover next to the control that opens it (the chat's
   * "Reaction", the feed's "+"). Each reaction is an icon button, its name shown as its tooltip. A
   * pick closes the picker and puts focus back on the control before `onpick` runs; what a pick does
   * (toggle, add) is the caller's.
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
        <IconButton label={reactionLabel(id)} pressed={chosen.includes(id)} onclick={() => pick(id, close)}>
          <span class="glyph"><Icon /></span>
        </IconButton>
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

  .glyph {
    display: inline-flex;
    font-size: 2rem;
  }
</style>
