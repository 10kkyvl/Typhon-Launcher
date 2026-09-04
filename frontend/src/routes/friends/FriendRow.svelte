<script lang="ts">
  import type { Snippet } from 'svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import type { UserCard } from '../../lib/services/social';

  let {
    user,
    meta,
    actions,
    onopen,
  }: {
    user: UserCard;
    meta?: string;
    actions?: Snippet;
    onopen?: () => void;
  } = $props();
</script>

{#snippet who()}
  <Avatar size="sm" name={user.displayName || user.username} src={user.avatarUrl} />
  <span class="names">
    <span class="name">{user.displayName || user.username}</span>
    <span class="handle">@{user.username}</span>
  </span>
{/snippet}

<div class="row">
  {#if onopen}
    <button class="who" type="button" onclick={onopen}>
      {@render who()}
    </button>
  {:else}
    <div class="who">
      {@render who()}
    </div>
  {/if}
  <span class="meta">{meta || '—'}</span>
  {#if actions}
    <div class="actions">
      {@render actions()}
    </div>
  {/if}
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  button.who {
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex: 1;
    min-width: 0;
    text-align: left;
  }

  .names {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }

  .name {
    font-size: var(--font-md);
    font-weight: 500;
    line-height: 1.3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .handle {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: right;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }
</style>
