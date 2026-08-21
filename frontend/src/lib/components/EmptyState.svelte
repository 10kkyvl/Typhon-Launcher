<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    description,
    icon,
    actions,
  }: {
    title: string;
    description?: string;
    icon?: Snippet;
    actions?: Snippet;
  } = $props();
</script>

<div class="empty">
  {#if icon}
    <div class="icon">
      {@render icon()}
    </div>
  {/if}
  <h3>{title}</h3>
  {#if description}
    <p>{description}</p>
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
    padding: var(--space-10) 0;
    max-width: 44rem;
  }

  .icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.4rem;
    height: 4.4rem;
    margin-bottom: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-3);
  }

  h3 {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  p {
    margin-top: 0.6rem;
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text-3);
  }

  .actions {
    margin-top: var(--space-5);
  }
</style>
