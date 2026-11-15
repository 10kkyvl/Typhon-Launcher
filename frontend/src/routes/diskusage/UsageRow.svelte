<script lang="ts">
  import type { Snippet } from 'svelte';

  // One row of a section: what it is and where, why it cannot go, its marks, its size and its actions.
  let {
    title,
    path,
    pathTitle,
    reason,
    badges,
    played,
    size,
    actions,
  }: {
    title: string;
    path: string;
    /** The path again as the hint of its line; none when the line is not a path. */
    pathTitle?: string;
    /**
     * The line under the path: rows that can have one pass it, or null while there is none; rows that
     * never have one leave it out.
     */
    reason?: string | null;
    badges?: Snippet;
    /** When the game was last played; only rows of games have it. */
    played?: string;
    size: string;
    actions: Snippet;
  } = $props();
</script>

<!-- Spaces between the parts are written out: a part that is absent leaves no space behind. -->
<div class="row"
  ><div class="main"
    ><span class="title">{title}</span>{' '}<span class="path" title={pathTitle}>{path}</span
    >{#if reason !== undefined}{' '}{#if reason !== null}<span class="reason">{reason}</span>{/if}{/if}</div
  >{#if badges}{' '}<div class="badges">{@render badges()}</div>{/if}{#if played !== undefined}{' '}<span class="played"
      >{played}</span
    >{/if}{' '}<span class="size">{size}</span>{' '}<div class="actions">{@render actions()}</div></div
>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .main {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .path {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .reason {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .badges {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .played {
    width: 15rem;
    flex-shrink: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .size {
    width: 14rem;
    flex-shrink: 0;
    text-align: right;
    font-size: var(--font-sm);
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    min-width: 12rem;
    flex-shrink: 0;
  }
</style>
