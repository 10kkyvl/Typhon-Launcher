<script lang="ts">
  import { ChevronRight, Pause, Play, X } from '@lucide/svelte';
  import type { Download } from '../services/downloads';
  import { cancel, pause, resume, statusLabels } from '../stores/downloads';
  import { gameArt, requestArt } from '../stores/metadata';
  import { sources } from '../stores/sources';
  import { bytesSize, etaLabel, speedBytes } from '../utils/format';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let {
    download,
    onopen,
  }: {
    download: Download;
    onopen?: (download: Download) => void;
  } = $props();

  const downloading = $derived(download.status === 'downloading');
  const pct = $derived(download.progress * 100);
  const barColor = $derived(download.status === 'paused' ? 'var(--text-3)' : 'var(--accent)');

  const typeTag = $derived.by(() => {
    if (download.origin.purpose === 'update') return 'Обновление';
    if (download.origin.purpose === 'repair') return 'Восстановление';
    if (download.origin.gameId || download.origin.releaseId) return 'Игра';
    return '';
  });

  const sourceTag = $derived($sources.find((s) => s.id === download.origin.sourceId)?.name ?? '');
  const cover = $derived((download.origin.gameId && $gameArt[download.origin.gameId]?.cover) || '');

  $effect(() => {
    if (download.origin.gameId) requestArt([download.origin.gameId]);
  });

  let confirmOpen = $state(false);

  function stop(e: MouseEvent) {
    e.stopPropagation();
  }
</script>

<div
  class="item"
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
    <Artwork src={cover} alt={download.name} ratio="3 / 4" radius="var(--radius-sm)" />
  </div>
  <div class="main">
    <span class="title">{download.name}</span>
    {#if typeTag || sourceTag}
      <div class="tags">
        {#if typeTag}<StatusBadge kind="neutral" label={typeTag} dot={false} />{/if}
        {#if sourceTag}<StatusBadge kind="neutral" label={sourceTag} dot={false} />{/if}
      </div>
    {/if}
    <div class="progress-row">
      <div class="bar">
        <ProgressBar value={pct} color={barColor} height={6} />
      </div>
      <span class="pct">{Math.floor(pct)}%</span>
    </div>
    <div class="meta">
      {#if downloading && download.stalled}
        <span class="stalled">Ожидание источников</span>
        <span class="sep">·</span>
        <span class="dim">{download.seeders} сид / {download.peers} пир</span>
      {:else if downloading}
        <span>{bytesSize(download.downloaded)} из {bytesSize(download.total)}</span>
        {#if download.etaSeconds >= 0}
          <span class="sep">·</span>
          <span>Осталось {etaLabel(download.etaSeconds)}</span>
        {/if}
      {:else}
        <span>{statusLabels[download.status]}</span>
      {/if}
    </div>
  </div>
  {#if downloading}
    <div class="speeds">
      <span>↓ {speedBytes(download.downloadSpeed)}</span>
      <span>↑ {speedBytes(download.uploadSpeed)}</span>
    </div>
  {/if}
  <div class="controls">
    {#if downloading}
      <Button
        size="sm"
        onclick={(e) => {
          stop(e);
          pause(download.id);
        }}
      >
        <Pause size="1.5rem" strokeWidth={1.8} />
        Пауза
      </Button>
    {:else if download.status === 'paused'}
      <Button
        size="sm"
        onclick={(e) => {
          stop(e);
          resume(download.id);
        }}
      >
        <Play size="1.5rem" strokeWidth={1.8} />
        Продолжить
      </Button>
    {/if}
    <Button
      size="sm"
      onclick={(e) => {
        stop(e);
        confirmOpen = true;
      }}
    >
      <X size="1.5rem" strokeWidth={1.8} />
      Отменить
    </Button>
  </div>
  <IconButton
    label="Подробнее о загрузке"
    onclick={(e) => {
      stop(e);
      onopen?.(download);
    }}
  >
    <ChevronRight size="1.8rem" strokeWidth={1.8} />
  </IconButton>
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
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    cursor: pointer;
    transition: border-color var(--dur) var(--ease);
  }

  .item:hover {
    border-color: var(--border-strong);
  }

  .thumb {
    width: 6.4rem;
    flex-shrink: 0;
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tags {
    display: flex;
    gap: 0.6rem;
  }

  .progress-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .bar {
    flex: 1;
    min-width: 0;
  }

  .pct {
    flex-shrink: 0;
    min-width: 3.8rem;
    text-align: right;
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .meta {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sep {
    margin: 0 0.5rem;
    opacity: 0.6;
  }

  .dim {
    color: var(--text-3);
  }

  .stalled {
    color: var(--warning, var(--text-2));
  }

  .speeds {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.4rem;
    flex-shrink: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .controls {
    display: flex;
    gap: 0.6rem;
    flex-shrink: 0;
  }

  .modal-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
  }
</style>
