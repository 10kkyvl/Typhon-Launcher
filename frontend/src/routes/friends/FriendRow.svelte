<script lang="ts">
  import type { Snippet } from 'svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { UserCard } from '../../lib/services/social';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { msg } from '../../lib/i18n';

  let {
    user,
    meta,
    status,
    game,
    stats,
    variant = 'list',
    actions,
    onopen,
  }: {
    user: UserCard;
    meta?: string;
    status?: 'online' | 'away' | 'busy' | 'offline';
    game?: { igdbId: number; title: string } | null;
    stats?: string[];
    variant?: 'list' | 'grid' | 'card';
    actions?: Snippet;
    onopen?: () => void;
  } = $props();

  const avatarSize = $derived(variant === 'list' ? 'sm' : 'md');

  function openGame(event: MouseEvent) {
    event.stopPropagation();
    if (game) openGameByIGDB(game.igdbId, game.title);
  }
</script>

{#snippet who()}
  <Avatar size={avatarSize} name={user.displayName || user.username} src={user.avatarUrl} {status} />
  <span class="names">
    <span class="name">{user.displayName || user.username}</span>
    <span class="handle">@{user.username}</span>
  </span>
{/snippet}

{#snippet identity()}
  {#if onopen}
    <button class="who" type="button" onclick={onopen}>
      {@render who()}
    </button>
  {:else}
    <div class="who">
      {@render who()}
    </div>
  {/if}
{/snippet}

{#snippet metaLine()}
  {#if game}
    <span class="playing">{msg('social.playingLabel')} <button type="button" class="game-link" onclick={openGame}>{game.title}</button></span>
  {:else}
    <span class="meta">{meta || '—'}</span>
  {/if}
{/snippet}

{#if variant === 'grid'}
  <Card padding="var(--space-4)">
    <div class="grid-head">
      {@render identity()}
      {#if actions}
        <div class="actions">{@render actions()}</div>
      {/if}
    </div>
    {@render metaLine()}
  </Card>
{:else if variant === 'card'}
  <Card padding="var(--space-4)">
    <div class="card-row">
      {@render identity()}
      {#if stats && stats.length > 0}
        <div class="stats">
          {#each stats as stat}<span class="stat">{stat}</span>{/each}
        </div>
      {/if}
      {#if actions}
        <div class="actions">{@render actions()}</div>
      {/if}
    </div>
  </Card>
{:else}
  <div class="row">
    {@render identity()}
    {@render metaLine()}
    {#if actions}
      <div class="actions">{@render actions()}</div>
    {/if}
  </div>
{/if}

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

  .grid-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }

  .grid-head .who {
    flex: 1;
  }

  .card-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-5);
  }

  .card-row .who {
    flex: 0 1 20rem;
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    text-align: left;
  }

  .row .who {
    flex: 1;
  }

  button.who {
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
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

  .meta,
  .playing {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .row .meta,
  .row .playing {
    text-align: right;
  }

  .game-link {
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    color: var(--accent-text);
    cursor: pointer;
  }

  .game-link:hover {
    text-decoration: underline;
  }

  .stats {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    gap: var(--space-6);
    min-width: 0;
  }

  .stat {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }
</style>
