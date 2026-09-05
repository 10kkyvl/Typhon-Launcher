<script lang="ts">
  import type { FeedEvent } from '../services/social';
  import { eventLine, kindLabel } from '../social/feed';
  import { openGameByIGDB } from '../social/openGame';
  import { feedPending } from '../stores/feed';
  import { navigate } from '../stores/router';
  import { relativeDate } from '../utils/format';
  import Artwork from './Artwork.svelte';
  import Avatar from './Avatar.svelte';
  import FeedNote from './FeedNote.svelte';
  import ReactionBar from './reactions/ReactionBar.svelte';

  let {
    event,
    compact = false,
    own = false,
    onreact,
    onnote,
  }: {
    event: FeedEvent;
    compact?: boolean;
    own?: boolean;
    onreact?: (emoji: string) => void;
    onnote?: (note: string) => void;
  } = $props();

  const name = $derived(event.user.displayName || event.user.username);
  const when = $derived(relativeDate(event.createdAt));
  const kind = $derived(kindLabel(event.kind));
  const line = $derived(eventLine(event));

  function openUser() {
    navigate('user', { username: event.user.username });
  }

  function openGame() {
    void openGameByIGDB(event.game.igdbId, event.game.title);
  }
</script>

<div class="row" class:compact>
  <button class="who" type="button" aria-label={name} title={name} onclick={openUser}>
    <Avatar src={event.user.avatarUrl} {name} size="sm" />
    {#if compact}
      <span class="name">{name}</span>
    {/if}
  </button>

  <div class="body">
    {#if !compact}
      <div class="head">
        <button class="name" type="button" onclick={openUser}>{name}</button>
        <span class="when">{when}</span>
      </div>
      <span class="kind">{kind}</span>
    {/if}

    <button class="game" type="button" onclick={openGame}>
      {#if compact}
        <span class="thumb">
          <Artwork src={event.game.coverUrl} alt={event.game.title} ratio="3 / 4" />
        </span>
        <span class="line">{line}</span>
      {:else}
        <span class="cover">
          <Artwork src={event.game.coverUrl} alt={event.game.title} ratio="16 / 9" radius="var(--radius-md)" />
          <span class="fade"></span>
        </span>
        <span class="caption">{event.game.title}</span>
      {/if}
    </button>

    {#if !compact}
      <FeedNote note={event.note} own={own && onnote != null} saving={$feedPending.has(event.id)} onsave={onnote} />
    {/if}

    {#if !compact && onreact}
      <ReactionBar {event} disabled={$feedPending.has(event.id)} ontoggle={onreact} />
    {/if}
  </div>

  {#if compact}
    <span class="when">{when}</span>
  {/if}
</div>

<style>
  .row {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
  }

  .row.compact {
    align-items: center;
    padding: 0.6rem 0.8rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row.compact:hover {
    background: var(--hover);
  }

  .who {
    display: inline-flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    border-radius: var(--radius-md);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    flex: 1;
    min-width: 0;
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .name {
    font-size: var(--font-sm);
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color var(--dur) var(--ease);
  }

  .who:hover .name,
  .name:hover {
    color: var(--accent-text);
  }

  .when {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .kind {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .game {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.8rem;
    width: 100%;
    padding: 0.6rem;
    margin: -0.6rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .game:hover {
    background: var(--hover);
  }

  .compact .game {
    flex-direction: row;
    align-items: center;
    gap: var(--space-3);
    flex: 1;
    min-width: 0;
  }

  .cover {
    position: relative;
    display: block;
    width: 100%;
    max-width: 40rem;
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--surface-3);
  }

  .fade {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, transparent 55%, rgba(5, 8, 12, 0.55));
    pointer-events: none;
  }

  .caption {
    font-size: var(--font-md);
    font-weight: 600;
    color: var(--text);
  }

  .thumb {
    display: block;
    width: 2.6rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .line {
    min-width: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
