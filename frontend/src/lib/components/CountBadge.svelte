<script lang="ts">
  import { COUNT_MAX, countShown } from './countBadge';

  /**
   * A count on a control (unread messages, friend requests): a number up to a cap ("99+"), nothing at
   * all when there is nothing to count. The bubble is a picture; what a screen reader hears is `label`,
   * a text the caller words with the real number ("3 new requests"), read as part of the name of the
   * control the badge sits in. So it goes inside its control, after the control's own text: a control
   * named by its text then says both. A control named by `aria-label` hears nothing from it; IconButton
   * takes a count of its own and changes its name with it.
   *
   * `place`: `inline` sits in the row after the text; `corner` is smaller and stands on the top right
   * corner of the nearest positioned box (the control). `on`: what it stands on; on an accent fill it
   * takes the page's colours so it is not lost in the fill.
   */
  let {
    count,
    label,
    max = COUNT_MAX,
    place = 'inline',
    on = 'surface',
  }: {
    count: number;
    /** The count in words, with the real number: said as part of the control's name. */
    label: string;
    max?: number;
    place?: 'inline' | 'corner';
    on?: 'surface' | 'accent';
  } = $props();

  const shown = $derived(countShown(count, max));
</script>

<!-- The space before the badge keeps its words apart from the control's own in the name, whatever
     box the badge is drawn as; in a flex row it draws nothing. -->
{#if shown}{' '}<span class="count-badge" class:corner={place === 'corner'} class:on-accent={on === 'accent'} data-part="count-badge"
    ><span aria-hidden="true">{shown}</span><span class="sr-only">{label}</span></span
  >{/if}

<style>
  .count-badge {
    display: inline-block;
    flex: none;
    min-width: var(--count-h);
    height: var(--count-h);
    padding: 0 var(--count-pad);
    border-radius: var(--radius-count);
    background: var(--accent);
    color: var(--accent-on, var(--accent-on-default));
    font-size: var(--font-xs);
    font-weight: 600;
    line-height: var(--count-h);
    text-align: center;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .count-badge.corner {
    position: absolute;
    top: 0;
    right: 0;
    min-width: var(--count-h-corner);
    height: var(--count-h-corner);
    padding: 0 var(--count-pad-corner);
    font-size: var(--count-font-corner);
    line-height: var(--count-h-corner);
    pointer-events: none;
  }

  /* On an accent fill the badge takes the page's own colours: a pair measured in every preset, and
     one that does not melt into the fill it stands on. */
  .count-badge.on-accent {
    background: var(--bg);
    color: var(--text);
  }
</style>
