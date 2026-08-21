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

  const purposeLabel = $derived(
    download.origin.purpose === 'update'
      ? 'Обновление'
      : download.origin.purpose === 'repair'
        ? 'Восстановление'
        : '',
  );
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
    <FileDown size="1.8rem" strokeWidth={1.8} />
  </div>
  <div class="main">
    <div class="head">
      <div class="titles">
        <span class="title">{download.name}</span>
        <span class="sub">
          {statusLabels[download.status]}
          {#if purposeLabel}<span class="sep">·</span>{purposeLabel}{/if}
        </span>
      </div>
      <div class="controls">
        {#if downloading}
          <IconButton
            label="Пауза"
            size="sm"
            onclick={(e) => {
              stop(e);
              pause(download.id);
            }}
          >
            <Pause size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {:else if download.status === 'paused' || download.status === 'queued'}
          <IconButton
            label="Продолжить"
            size="sm"
            onclick={(e) => {
              stop(e);
              resume(download.id);
            }}
          >
            <Play size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {/if}
        {#if finished}
          <IconButton
            label="Удалить из списка"
            size="sm"
            onclick={(e) => {
              stop(e);
              remove(download.id);
            }}
          >
            <Trash2 size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {:else}
          <IconButton
            label="Отменить"
            size="sm"
            onclick={(e) => {
              stop(e);
              confirmOpen = true;
            }}
          >
            <X size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {/if}
      </div>
    </div>
    <ProgressBar value={pct} color={barColor} height={compact ? 4 : 5} />
    <div class="foot">
      <span class="size">
        <span class="pct">{Math.floor(pct)}%</span>
        <span class="sep">·</span>
        {bytesSize(download.downloaded)} / {bytesSize(download.total)}
      </span>
      {#if download.status === 'failed' && download.error}
        <span class="error">{download.error}</span>
      {:else if downloading && !compact}
        <span class="stats">
          <span>{speedBytes(download.downloadSpeed)}</span>
          <span class="sep">·</span>
          <span>осталось {etaLabel(download.etaSeconds)}</span>
          <span class="sep">·</span>
          <span class="dim">↑ {speedBytes(download.uploadSpeed)}</span>
          <span class="sep">·</span>
          <span class="dim">{download.seeders} сид / {download.peers} пир</span>
        </span>
      {:else if downloading}
        <span class="stats">
          <span>{speedBytes(download.downloadSpeed)}</span>
          <span class="sep">·</span>
          <span>{etaLabel(download.etaSeconds)}</span>
        </span>
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
    padding: var(--space-4) var(--space-5);
    background: var(--surface);
    border-radius: var(--radius-lg);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .item.clickable {
    cursor: pointer;
  }

  .item.clickable:hover {
    background: var(--surface-2);
  }

  .item.compact {
    background: transparent;
    padding: 0;
  }

  .thumb {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4rem;
    height: 4rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-3);
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .head {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
  }

  .titles {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .sep {
    margin: 0 0.5rem;
    opacity: 0.6;
  }

  .controls {
    display: flex;
    gap: 0.2rem;
    margin-top: -0.4rem;
    margin-right: -0.6rem;
  }

  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .size {
    white-space: nowrap;
  }

  .pct {
    color: var(--text-2);
    font-weight: 500;
  }

  .stats {
    display: inline-flex;
    align-items: center;
    white-space: nowrap;
    color: var(--text-2);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dim {
    color: var(--text-3);
  }

  .error {
    color: var(--danger);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .modal-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
  }
</style>
