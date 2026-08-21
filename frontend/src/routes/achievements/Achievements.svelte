<script lang="ts">
  import { Trophy } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import { achievements } from '../../lib/mock/achievements';
  import { gameById } from '../../lib/mock/games';
  import { navigate } from '../../lib/stores/router';

  const rows = Object.entries(achievements)
    .map(([gameId, data]) => ({ game: gameById(gameId), data }))
    .filter((r) => r.game)
    .sort((a, b) => b.data.earned / b.data.total - a.data.earned / a.data.total);

  const totalEarned = rows.reduce((sum, r) => sum + r.data.earned, 0);
  const totalAll = rows.reduce((sum, r) => sum + r.data.total, 0);
</script>

<PageHeader title="Достижения" subtitle="{totalEarned} из {totalAll} достижений в {rows.length} играх" />

<div class="list">
  {#each rows as row (row.game?.id)}
    {@const game = row.game}
    {#if game}
      {@const done = row.data.earned === row.data.total}
      <button class="row" class:done onclick={() => navigate('game', { id: game.id })}>
        <div class="thumb">
          <Artwork src={game.cover} alt={game.title} radius="var(--radius-sm)" />
        </div>
        <div class="text">
          <span class="title">{game.title}</span>
          <div class="progress">
            <ProgressBar value={row.data.earned} max={row.data.total} height={4} color={done ? 'var(--success)' : 'var(--accent)'} />
            <span class="nums">{row.data.earned} / {row.data.total}</span>
          </div>
          {#if row.data.recent[0]}
            <span class="recent">
              <Trophy size="1.3rem" strokeWidth={1.8} />
              {row.data.recent[0].name} · {row.data.recent[0].date}
            </span>
          {/if}
        </div>
        <span class="pct">
          {Math.round((row.data.earned / row.data.total) * 100)}%
        </span>
      </button>
    {/if}
  {/each}
</div>

<style>
  .list {
    display: grid;
    grid-template-columns: 1fr;
    max-width: 110rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: 1.4rem 1.2rem;
    margin: 0 -1.2rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row:hover {
    background: var(--hover);
  }

  .thumb {
    width: 5.2rem;
    height: 6.9rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    max-width: 44rem;
  }

  .nums {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .recent {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .recent :global(svg) {
    color: #e3c26b;
    flex-shrink: 0;
  }

  .pct {
    font-size: var(--font-xl);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--text-2);
    min-width: 6rem;
    text-align: right;
  }

  .done .pct {
    color: var(--success);
  }

  .done .title {
    color: var(--text-2);
  }

  @media (min-width: 1800px) {
    .list {
      grid-template-columns: 1fr 1fr;
      column-gap: var(--space-12);
      max-width: none;
    }

    .row + .row {
      border-top: none;
    }

    .row {
      border-top: 1px solid var(--border);
    }

    .row:nth-child(-n + 2) {
      border-top: none;
    }
  }
</style>
