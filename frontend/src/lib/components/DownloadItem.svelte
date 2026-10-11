<script lang="ts">
  import { ArrowDown, ArrowUp, ChevronRight, Pause, Play, X } from '@lucide/svelte';
  import type { Download } from '../services/downloads';
  import { cancelDownloadPrompt } from '../confirm/prompts';
  import { cancel, pause, resume, statusLabels } from '../stores/downloads';
  import { gameArt, requestArt } from '../stores/metadata';
  import { sources } from '../stores/sources';
  import { bytesSize, etaLabel, speedBytes } from '../utils/format';
  import { msg } from '../i18n';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import IconButton from './IconButton.svelte';
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
    if (download.origin.purpose === 'update') return msg('ui.update');
    if (download.origin.purpose === 'repair') return msg('ui.repair');
    if (download.origin.gameId || download.origin.releaseId) return msg('ui.game');
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
    <div class="head">
      <span class="title" title={download.name}>{download.name}</span>
      {#if typeTag || sourceTag}
        <div class="tags">
          {#if typeTag}<StatusBadge kind="neutral" label={typeTag} dot={false} />{/if}
          {#if sourceTag}<StatusBadge kind="neutral" label={sourceTag} dot={false} />{/if}
        </div>
      {/if}
    </div>
    <div class="progress-row">
      <div class="bar">
        <ProgressBar value={pct} color={barColor} height={6} indeterminate={download.status === 'metadata'} />
      </div>
      <span class="pct">{download.status === 'metadata' ? '' : `${Math.floor(pct)}%`}</span>
    </div>
    <div class="meta">
      {#if downloading && download.stalled}
        <span class="stalled">{msg('ui.awaitingSources')}</span>
        <span class="sep">·</span>
        <span class="dim">{msg('ui.seedersPeers', { seeders: download.seeders, peers: download.peers })}</span>
      {:else if downloading}
        <span>{msg('ui.bytesOfBytes', { done: bytesSize(download.downloaded), total: bytesSize(download.total) })}</span>
        {#if download.etaSeconds >= 0}
          <span class="sep">·</span>
          <span class="eta">{msg('ui.timeLeft', { eta: etaLabel(download.etaSeconds) })}</span>
        {/if}
      {:else}
        <span class="state" class:paused={download.status === 'paused'}>{statusLabels(download.status)}</span>
      {/if}
    </div>
  </div>
  <div class="speeds" aria-hidden={!downloading}>
    {#if downloading}
      <span class="down"><ArrowDown size="1.3rem" strokeWidth={2} />{speedBytes(download.downloadSpeed)}</span>
      <span class="up"><ArrowUp size="1.3rem" strokeWidth={2} />{speedBytes(download.uploadSpeed)}</span>
    {/if}
  </div>
  <div class="controls">
    <span class="toggle">
      {#if downloading}
        <Button
          size="sm"
          onclick={(e) => {
            stop(e);
            pause(download.id);
          }}
        >
          <Pause size="1.5rem" strokeWidth={1.8} />
          {msg('ui.pause')}
        </Button>
      {:else if download.status === 'paused'}
        <Button
          size="sm"
          variant="primary"
          onclick={(e) => {
            stop(e);
            resume(download.id);
          }}
        >
          <Play size="1.5rem" strokeWidth={1.8} />
          {msg('common.continue')}
        </Button>
      {/if}
    </span>
    <IconButton
      label={msg('ui.cancelVerb')}
      size="sm"
      onclick={(e) => {
        stop(e);
        confirmOpen = true;
      }}
    >
      <X size="1.6rem" strokeWidth={1.8} />
    </IconButton>
    <IconButton
      label={msg('ui.moreAboutDownload')}
      size="sm"
      onclick={(e) => {
        stop(e);
        onopen?.(download);
      }}
    >
      <ChevronRight size="1.8rem" strokeWidth={1.8} />
    </IconButton>
  </div>
</div>

{#if confirmOpen}
  <ConfirmModal
    prompt={cancelDownloadPrompt(download.name)}
    onconfirm={() => cancel(download.id)}
    onclose={() => (confirmOpen = false)}
  />
{/if}

<style>
  .item {
    display: grid;
    grid-template-columns: 4.8rem minmax(0, 1fr) 10.5rem 20.5rem;
    align-items: center;
    column-gap: var(--space-5);
    padding: var(--space-3) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    cursor: pointer;
    transition: border-color var(--dur) var(--ease);
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .item:hover {
    border-color: var(--border-strong);
  }

  .main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }

  .title {
    min-width: 0;
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
    flex-shrink: 0;
  }

  .progress-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-height: 2.2rem;
  }

  .bar {
    flex: 1;
    min-width: 0;
  }

  .pct {
    flex-shrink: 0;
    width: 4.2rem;
    text-align: right;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
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

  .eta,
  .state.paused {
    color: var(--text-2);
  }

  .speeds {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.4rem;
    min-width: 0;
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .speeds span {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .down {
    font-weight: 500;
    color: var(--text);
  }

  .up {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .controls {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.4rem;
    min-width: 0;
  }

  .toggle {
    display: flex;
    justify-content: flex-end;
    flex: 1;
    min-width: 0;
    margin-right: 0.4rem;
  }

  .toggle :global(.btn) {
    min-width: 12.4rem;
  }

  @media (max-width: 1300px) {
    .item {
      grid-template-columns: 4.8rem minmax(0, 1fr) 9.5rem 20.5rem;
      column-gap: var(--space-4);
    }

    .tags {
      display: none;
    }
  }
</style>
