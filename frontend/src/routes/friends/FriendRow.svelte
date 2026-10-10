<script lang="ts">
  import type { Snippet } from 'svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import Card from '../../lib/components/Card.svelte';
  import PlayerCardPopover from '../../lib/components/PlayerCardPopover.svelte';
  import { playerCardOf } from '../../lib/profile/appearanceCard';
  import type { FriendView, UserCard } from '../../lib/services/social';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { msg } from '../../lib/i18n';

  let {
    user,
    meta,
    status,
    game,
    stats,
    variant = 'list',
    compact = false,
    actions,
    onopen,
  }: {
    user: UserCard;
    meta?: string;
    status?: 'online' | 'away' | 'busy' | 'offline';
    game?: { igdbId: number; title: string } | null;
    stats?: string[];
    variant?: 'list' | 'grid' | 'card';
    compact?: boolean;
    actions?: Snippet;
    onopen?: () => void;
  } = $props();

  const avatarSize = $derived(variant === 'list' ? 'sm' : 'md');
  const look = $derived(playerCardOf(user.card));
  const edge = $derived(user.card ? look.accent : undefined);
  const presence = $derived((user as Partial<FriendView>).presence);
  let whoEl = $state<HTMLElement>();

  function openGame(event: MouseEvent) {
    event.stopPropagation();
    if (game) openGameByIGDB(game.igdbId, game.title);
  }
</script>

{#snippet who()}
  <Avatar size={avatarSize} name={user.displayName || user.username} src={user.avatarUrl} {status} frame={look.appearance.avatarFrame} frameColor={look.accent} />
  <span class="names">
    <span class="name">{user.displayName || user.username}</span>
    {#if !compact}<span class="handle">@{user.username}</span>{/if}
  </span>
{/snippet}

{#snippet identity()}
  {#if onopen}
    <button class="who" type="button" bind:this={whoEl} onclick={onopen}>
      {@render who()}
    </button>
  {:else}
    <div class="who" bind:this={whoEl}>
      {@render who()}
    </div>
  {/if}
  <PlayerCardPopover anchor={whoEl} {user} {presence} />
{/snippet}

{#snippet metaLine()}
  {#if game}
    <span class="playing"><span class="playing-label">{msg('social.playingLabel')}</span> <button type="button" class="game-link" title={game.title} onclick={openGame}>{game.title}</button></span>
  {:else}
    <span class="meta">{meta || '—'}</span>
  {/if}
{/snippet}

{#if variant === 'grid'}
  <Card padding="var(--space-4)">
    <div class="grid-card edged" class:offline={status === 'offline'} style:--edge={edge}>
      {@render identity()}
      <span class="presence">
        {#if status}<span class="presence-dot {status}"></span>{/if}
        {@render metaLine()}
      </span>
      {#if actions}
        <div class="actions grid-actions">{@render actions()}</div>
      {/if}
    </div>
  </Card>
{:else if variant === 'card'}
  <Card padding="var(--space-4)">
    <div class="card-row edged" style:--edge={edge}>
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
  <div class="row" class:compact class:edged={!!edge} class:offline={status === 'offline'} style:--edge={edge}>
    {@render identity()}
    {#if compact}
      {@render metaLine()}
    {:else}
      <span class="presence">
        {#if status}<span class="presence-dot {status}"></span>{/if}
        {@render metaLine()}
      </span>
    {/if}
    {#if actions}
      <div class="actions">{@render actions()}</div>
    {/if}
  </div>
{/if}

<style>
  .row.compact {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 0.35rem;
    padding: 0.8rem 0;
  }

  .compact .playing,
  .compact .meta {
    min-width: 0;
    margin-left: calc(3.2rem + var(--space-3));
    text-align: left;
  }

  .compact .playing {
    display: flex;
    align-items: baseline;
    gap: 0.35rem;
  }

  .playing-label {
    flex-shrink: 0;
  }

  .compact .game-link {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
  }

  .row {
    display: grid;
    grid-template-columns: minmax(0, 30rem) minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row.edged {
    box-shadow: inset 2px 0 0 var(--edge);
  }

  .grid-card.edged,
  .card-row.edged {
    position: relative;
  }

  .grid-card.edged::before,
  .card-row.edged::before {
    content: '';
    position: absolute;
    left: calc(var(--space-4) * -1);
    top: 0;
    bottom: 0;
    width: 2px;
    border-radius: 2px;
    background: var(--edge);
  }

  .row:hover,
  .row:focus-within {
    background: var(--hover);
  }

  .presence {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    min-width: 0;
  }

  .presence .meta,
  .presence .playing {
    min-width: 0;
    flex-shrink: 1;
    font-size: var(--font-sm);
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .presence .playing {
    display: flex;
    align-items: baseline;
    gap: 0.35rem;
  }

  .presence .game-link {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
  }

  .offline .presence .meta {
    color: var(--text-3);
  }

  .offline .name {
    color: var(--text-2);
  }

  .presence-dot {
    width: 0.8rem;
    height: 0.8rem;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--text-3);
  }

  .presence-dot.online {
    background: var(--success);
  }

  .presence-dot.away {
    background: var(--warning);
  }

  .presence-dot.busy {
    background: var(--danger);
  }

  .presence-dot.offline {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--text-3);
  }

  .grid-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .grid-actions {
    justify-content: space-between;
    margin: 0 -0.6rem -0.4rem;
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
