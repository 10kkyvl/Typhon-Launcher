<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { PlayedGame } from '../../lib/services/social';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { playtime, relativeDate } from '../../lib/utils/format';

  let { games }: { games: PlayedGame[] } = $props();

  function meta(game: PlayedGame): string {
    const played = relativeDate(game.lastPlayedAt);
    const seconds = game.playtimeSeconds ?? 0;
    return seconds > 0 ? `${playtime(seconds)} · ${played}` : played;
  }
</script>

<Card title="Недавно играл">
  <div class="row">
    {#each games as game (game.igdbId)}
      <button class="capsule" type="button" onclick={() => openGameByIGDB(game.igdbId, game.title)}>
        <span class="cover">
          <Artwork src={game.coverUrl} alt={game.title} ratio="16 / 9" radius="var(--radius-md)" />
        </span>
        <span class="title">{game.title}</span>
        <span class="meta">
          <span class="dot"></span>
          {meta(game)}
        </span>
      </button>
    {/each}
  </div>
</Card>

<style>
  .row {
    display: flex;
    gap: var(--space-4);
    overflow-x: auto;
    padding-bottom: var(--space-2);
  }

  .capsule {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
    flex: 0 0 auto;
    width: 22rem;
    padding: 0;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: transform var(--dur) var(--ease);
  }

  .capsule:hover .cover {
    transform: scale(1.01);
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.3;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .meta {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .meta .dot {
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 50%;
    background: var(--success);
    flex-shrink: 0;
  }
</style>
