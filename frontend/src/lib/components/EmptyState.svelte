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
    align-items: center;
    text-align: center;
    padding: 6.4rem 3.2rem;
  }

  .icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 5.2rem;
    height: 5.2rem;
    margin-bottom: 1.8rem;
    border-radius: 1.4rem;
    background: var(--surface-3);
    border: 1px solid var(--border);
    color: var(--text-3);
  }

  h3 {
    font-size: 1.7rem;
    font-weight: 600;
  }

  p {
    margin-top: 0.8rem;
    max-width: 36rem;
    font-size: 1.4rem;
    color: var(--text-3);
  }

  .actions {
    margin-top: 2rem;
  }
</style>
