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
    padding: 64px 32px;
  }

  .icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 52px;
    height: 52px;
    margin-bottom: 18px;
    border-radius: 14px;
    background: var(--surface-3);
    border: 1px solid var(--border);
    color: var(--text-3);
  }

  h3 {
    font-size: 16px;
    font-weight: 600;
  }

  p {
    margin-top: 8px;
    max-width: 360px;
    font-size: 13.5px;
    color: var(--text-3);
  }

  .actions {
    margin-top: 20px;
  }
</style>
