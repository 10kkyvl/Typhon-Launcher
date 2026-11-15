<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * The heading of a section of a screen: its title, a count beside it, and actions on the right (a
   * section's "Clear", "Show all"). The count is part of the heading's name ("Queue 3"), as the
   * downloads sections say it; the actions stand outside the heading. Give the section
   * `aria-labelledby` with `id` (or the heading's own id, read from the DOM) to name it. The heading
   * takes focus from script (`tabindex="-1"`): a section's hand-off target when its last focused row goes.
   */
  let {
    title,
    count,
    level = 2,
    id,
    actions,
    testid,
  }: {
    title: string;
    /** Shown when given, 0 included. */
    count?: number | string;
    level?: 2 | 3;
    /** The heading's id; one of its own when not given. */
    id?: string;
    actions?: Snippet;
    testid?: string;
  } = $props();

  const uid = $props.id();
</script>

{#snippet heading()}
  {title}
  {#if count !== undefined}<span class="count">{count}</span>{/if}
{/snippet}

<!-- Two fixed tags rather than a computed one: a section heading is never the page's h1. Focusable by
     script only: focus lands here when the row it was on leaves the section and no row takes it. -->
<div class="section-header" data-testid={testid}>
  {#if level === 3}
    <h3 id={id ?? `${uid}-heading`} class="title" tabindex="-1">{@render heading()}</h3>
  {:else}
    <h2 id={id ?? `${uid}-heading`} class="title" tabindex="-1">{@render heading()}</h2>
  {/if}
  {#if actions}
    <div class="actions">{@render actions()}</div>
  {/if}
</div>

<style>
  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    min-width: 0;
    margin-bottom: var(--space-3);
  }

  .title {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    font-size: var(--font-lg);
  }

  .count {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
</style>
