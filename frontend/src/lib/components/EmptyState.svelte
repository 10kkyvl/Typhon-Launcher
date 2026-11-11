<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * "Nothing here yet". On a page (`size="page"`) the title is a heading of the given `level`; inline,
   * inside a dialog or a card that already has its heading, it is a quiet note and adds none.
   */
  let {
    title,
    description,
    icon,
    actions,
    level = 3,
    size = 'page',
    align = 'start',
  }: {
    title: string;
    description?: string;
    icon?: Snippet;
    actions?: Snippet;
    /** Heading level of the title on a page: one below the heading it sits under. */
    level?: 2 | 3 | 4;
    size?: 'page' | 'inline';
    align?: 'start' | 'center';
  } = $props();
</script>

<div class="empty" class:inline={size === 'inline'} class:center={align === 'center'}>
  {#if icon}
    <div class="icon">
      {@render icon()}
    </div>
  {/if}
  {#if size === 'inline'}
    <p class="title">{title}</p>
  {:else}
    <svelte:element this={`h${level}`} class="title">{title}</svelte:element>
  {/if}
  {#if description}
    <p class="description">{description}</p>
  {/if}
  {#if actions}
    <div class="actions">
      {@render actions()}
    </div>
  {/if}
</div>

<style>
  .empty {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    text-align: left;
    padding: var(--space-8) 0;
    max-width: 44rem;
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .empty.center {
    align-items: center;
    text-align: center;
    margin-inline: auto;
  }

  .empty.inline {
    padding: var(--space-4) 0;
    max-width: none;
    animation: none;
  }

  .icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.4rem;
    height: 4.4rem;
    margin-bottom: var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-2);
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .inline .title {
    font-size: var(--font-sm);
    font-weight: 400;
    color: var(--text-3);
  }

  .description {
    margin-top: 0.6rem;
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text-3);
  }

  .inline .description {
    margin-top: 0.4rem;
    font-size: var(--font-xs);
  }

  .actions {
    margin-top: var(--space-5);
  }
</style>
