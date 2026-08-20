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
      <button class="row" onclick={() => navigate('game', { id: game.id })}>
        <div class="thumb">
          <Artwork src={game.cover} alt={game.title} radius="var(--radius-sm)" />
        </div>
        <div class="text">
          <span class="title">{game.title}</span>
          <div class="progress">
            <ProgressBar value={row.data.earned} max={row.data.total} height={5} />
            <span class="nums">{row.data.earned} / {row.data.total}</span>
          </div>
          {#if row.data.recent[0]}
            <span class="recent">
              <Trophy size={13} strokeWidth={1.8} />
              {row.data.recent[0].name} · {row.data.recent[0].date}
            </span>
          {/if}
        </div>
        <span class="pct" class:done={row.data.earned === row.data.total}>
          {Math.round((row.data.earned / row.data.total) * 100)}%
        </span>
      </button>
    {/if}
  {/each}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: 860px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    text-align: left;
    transition:
      border-color var(--dur) var(--ease),
      transform var(--dur) var(--ease);
  }

  .row:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }

  .thumb {
    width: 56px;
    height: 74px;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 7px;
  }

  .title {
    font-size: 15px;
    font-weight: 600;
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    max-width: 420px;
  }

  .nums {
    font-size: 12.5px;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .recent {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .recent :global(svg) {
    color: #e8c35a;
    flex-shrink: 0;
  }

  .pct {
    font-size: 17px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--text-2);
  }

  .pct.done {
    color: var(--success);
  }
</style>
