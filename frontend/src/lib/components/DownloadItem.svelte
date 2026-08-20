<script lang="ts">
  import { FileDown, Pause, Play, Trash2, X } from '@lucide/svelte';
  import type { Download } from '../services/downloads';
  import { cancel, pause, remove, resume, statusLabels } from '../stores/downloads';
  import { bytesSize, etaLabel, speedBytes } from '../utils/format';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';

  let {
    download,
    compact = false,
    onopen,
  }: {
    download: Download;
    compact?: boolean;
    onopen?: (download: Download) => void;
  } = $props();

  const downloading = $derived(download.status === 'downloading');
  const finished = $derived(download.status === 'completed' || download.status === 'failed');
  const pct = $derived(download.progress * 100);
  const barColor = $derived(
    download.status === 'failed'
      ? 'var(--danger)'
      : download.status === 'paused'
        ? 'var(--text-3)'
        : 'var(--accent)',
  );

  let confirmOpen = $state(false);

  function stop(e: MouseEvent) {
    e.stopPropagation();
  }
</script>

<div
  class="item"
  class:compact
  class:clickable={!!onopen}
  role="button"
  tabindex="0"
  onclick={() => onopen?.(download)}
  onkeydown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onopen?.(download);
    }
  }}
>
  <div class="thumb">
    <FileDown size="2.6rem" strokeWidth={1.6} />
  </div>
  <div class="main">
    <div class="head">
      <div class="titles">
        <span class="title">{download.name}</span>
        <span class="sub">{statusLabels[download.status]}</span>
      </div>
      {#if !compact}
        <div class="stats">
          {#if downloading}
            <div class="stat">
              <span class="stat-label">Осталось</span>
              <span class="stat-value">{etaLabel(download.etaSeconds)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Скорость</span>
              <span class="stat-value">{speedBytes(download.downloadSpeed)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Отдача</span>
              <span class="stat-value">{speedBytes(download.uploadSpeed)}</span>
            </div>
            <div class="stat">
              <span class="stat-label">Пиры</span>
              <span class="stat-value">{download.seeders} сид / {download.peers} пир</span>
            </div>
          {:else}
            <div class="stat">
              <span class="stat-label">Состояние</span>
              <span class="stat-value">{statusLabels[download.status]}</span>
            </div>
          {/if}
        </div>
      {/if}
      <div class="controls">
        {#if downloading}
          <IconButton
            label="Пауза"
            onclick={(e) => {
              stop(e);
              pause(download.id);
            }}
          >
            <Pause size="1.7rem" strokeWidth={1.8} />
          </IconButton>
        {:else if download.status === 'paused' || download.status === 'queued'}
          <IconButton
            label="Продолжить"
            onclick={(e) => {
              stop(e);
              resume(download.id);
            }}
          >
            <Play size="1.7rem" strokeWidth={1.8} />
          </IconButton>
        {/if}
        {#if finished}
          <IconButton
            label="Удалить из списка"
            onclick={(e) => {
              stop(e);
              remove(download.id);
            }}
          >
            <Trash2 size="1.7rem" strokeWidth={1.8} />
          </IconButton>
        {:else}
          <IconButton
            label="Отменить"
            onclick={(e) => {
              stop(e);
              confirmOpen = true;
            }}
          >
            <X size="1.7rem" strokeWidth={1.8} />
          </IconButton>
        {/if}
      </div>
    </div>
    <div class="progress-row">
      <ProgressBar value={pct} color={barColor} />
      <span class="pct">{Math.floor(pct)}%</span>
    </div>
    <div class="foot">
      <span class="size">{bytesSize(download.downloaded)} / {bytesSize(download.total)}</span>
      {#if download.status === 'failed' && download.error}
        <span class="error">{download.error}</span>
      {/if}
    </div>
  </div>
</div>

<Modal bind:open={confirmOpen} title="Отменить загрузку?">
  <p class="modal-text">
    Загрузка «{download.name}» будет остановлена, а уже скачанные файлы удалены с диска. Это действие нельзя отменить.
  </p>
  {#snippet footer()}
    <Button onclick={() => (confirmOpen = false)}>Не отменять</Button>
    <Button
      variant="danger"
      onclick={() => {
        confirmOpen = false;
        cancel(download.id);
      }}
    >
      Отменить загрузку
    </Button>
  {/snippet}
</Modal>

<style>
  .item {
    display: flex;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    text-align: left;
    transition: border-color var(--dur) var(--ease);
  }

  .item.clickable {
    cursor: pointer;
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
    display: flex;
    align-items: center;
    justify-content: center;
    width: 17.8rem;
    aspect-ratio: 16 / 9;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    border: 1px solid var(--border);
    color: var(--text-3);
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
    white-space: nowrap;
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
    gap: var(--space-4);
  }

  .size {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .error {
    font-size: 1.3rem;
    color: var(--danger);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .modal-text {
    font-size: 1.5rem;
    line-height: 1.55;
    color: var(--text-2);
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
