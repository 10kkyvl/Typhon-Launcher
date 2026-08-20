<script lang="ts">
  import { Pause, Play, X } from '@lucide/svelte';
  import type { ActiveDownload, DownloadStage } from '../mock/types';
  import { gameById } from '../mock/games';
  import { cancelDownload, stageLabels, togglePause } from '../stores/downloads';
  import { navigate } from '../stores/router';
  import { eta, gb, speed } from '../utils/format';
  import Artwork from './Artwork.svelte';
  import IconButton from './IconButton.svelte';
  import ProgressBar from './ProgressBar.svelte';

  let {
    download,
    compact = false,
  }: {
    download: ActiveDownload;
    compact?: boolean;
  } = $props();

  const game = $derived(gameById(download.gameId));
  const downloading = $derived(download.stage === 'downloading');
  const pct = $derived(
    downloading ? (download.doneGb / download.totalGb) * 100 : download.stagePct * 100,
  );
  const stages: DownloadStage[] = ['downloading', 'unpacking', 'verifying', 'installing'];
</script>

<div class="item" class:compact>
  <button class="thumb" onclick={() => game && navigate('game', { id: game.id })} aria-label={game?.title}>
    <Artwork src={game?.hero ?? ''} alt={game?.title ?? ''} radius="var(--radius-sm)" />
  </button>
  <div class="main">
    <div class="head">
      <div class="titles">
        <span class="title">{game?.title ?? download.label}</span>
        <span class="sub">{download.label}</span>
      </div>
      {#if !compact}
        <div class="stats">
          {#if downloading}
            <div class="stat">
              <span class="stat-label">Осталось</span>
              <span class="stat-value">
                {download.paused ? 'Пауза' : eta(download.doneGb, download.totalGb, download.speedMbs)}
              </span>
            </div>
            <div class="stat">
              <span class="stat-label">Скорость</span>
              <span class="stat-value">{download.paused ? '—' : speed(download.speedMbs)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Пиров</span>
              <span class="stat-value">{download.peers[0]} из {download.peers[1]}</span>
            </div>
          {:else}
            <div class="stat">
              <span class="stat-label">Этап</span>
              <span class="stat-value">{stageLabels[download.stage]}</span>
            </div>
          {/if}
        </div>
      {/if}
      <div class="controls">
        {#if downloading}
          <IconButton label={download.paused ? 'Продолжить' : 'Пауза'} onclick={() => togglePause(download.id)}>
            {#if download.paused}
              <Play size="1.7rem" strokeWidth={1.8} />
            {:else}
              <Pause size="1.7rem" strokeWidth={1.8} />
            {/if}
          </IconButton>
        {/if}
        <IconButton label="Отменить" onclick={() => cancelDownload(download.id)}>
          <X size="1.7rem" strokeWidth={1.8} />
        </IconButton>
      </div>
    </div>
    <div class="progress-row">
      <ProgressBar value={pct} color={download.paused ? 'var(--text-3)' : 'var(--accent)'} />
      <span class="pct">{Math.floor(pct)}%</span>
    </div>
    <div class="foot">
      {#if downloading}
        <span class="size">{gb(download.doneGb)} / {gb(download.totalGb)}</span>
      {:else}
        <span class="size">{stageLabels[download.stage]}…</span>
      {/if}
      {#if !compact}
        <div class="steps">
          {#each stages as s, i (s)}
            {#if i > 0}<span class="step-line" class:done={stages.indexOf(download.stage) >= i}></span>{/if}
            <span
              class="step-dot"
              class:done={stages.indexOf(download.stage) > i}
              class:current={download.stage === s}
              title={stageLabels[s]}
            ></span>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .item {
    display: flex;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    transition: border-color var(--dur) var(--ease);
  }

  .item:hover {
    border-color: var(--border-strong);
  }

  .item.compact {
    background: transparent;
    border: none;
    padding: 0;
  }

  .thumb {
    width: 17.8rem;
    aspect-ratio: 16 / 9;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
    align-self: center;
  }

  .compact .thumb {
    width: 12rem;
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 0.9rem;
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--space-5);
  }

  .titles {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .title {
    font-size: 1.6rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sub {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .stats {
    display: flex;
    gap: var(--space-8);
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 7.6rem;
  }

  .stat-label {
    font-size: 1.2rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-3);
  }

  .stat-value {
    font-size: 1.4rem;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }

  .controls {
    display: flex;
    gap: 0.4rem;
  }

  .progress-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .pct {
    font-size: 1.4rem;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
    color: var(--text-2);
    min-width: 3.8rem;
    text-align: right;
  }

  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .size {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .steps {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .step-dot {
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.12);
    transition: background var(--dur) var(--ease);
  }

  .step-dot.done {
    background: var(--accent);
  }

  .step-dot.current {
    background: var(--accent);
    box-shadow: 0 0 0 0.3rem var(--accent-subtle);
  }

  .step-line {
    width: 1.4rem;
    height: 1.5px;
    background: rgba(255, 255, 255, 0.1);
  }

  .step-line.done {
    background: var(--accent);
  }

  @media (max-width: 1240px) {
    .stats {
      gap: var(--space-5);
    }

    .thumb {
      width: 14rem;
    }
  }
</style>
