<script lang="ts">
  import { CheckCircle2, Download, RotateCcw, TriangleAlert, X } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import {
    dismissOutcome,
    requestCancelDownload,
    selfUpdateDownloading,
    selfUpdateOutcome,
    selfUpdateProgress,
    selfUpdateStatus,
  } from '../stores/selfupdate';
  import { route } from '../stores/router';
  import { outcomeReason } from '../services/selfupdateMessages';
  import { bytesSize } from '../utils/format';

  const status = $derived($selfUpdateStatus);
  const progress = $derived($selfUpdateProgress);
  const outcome = $derived($selfUpdateOutcome);

  const downloading = $derived($selfUpdateDownloading || status.state === 'downloading');
  const applying = $derived(status.state === 'applying');

  const version = $derived(progress?.version || status.availableVersion || '');
  const totalBytes = $derived(progress?.totalBytes ?? status.totalBytes ?? 0);
  const downloadedBytes = $derived(progress?.downloadedBytes ?? status.downloadedBytes ?? 0);
  const pct = $derived(totalBytes > 0 ? Math.min(100, Math.max(0, (downloadedBytes / totalBytes) * 100)) : 0);
</script>

{#if applying}
  <div class="overlay" role="dialog" aria-modal="true" aria-label="Установка обновления">
    <div class="panel">
      <RotateCcw class="spin" size="3.2rem" strokeWidth={1.6} />
      <h3>Устанавливаем версию {version}</h3>
      <p>Лаунчер закроется, установит обновление и запустится сам. Это займёт меньше минуты.</p>
      <div class="marquee"><span></span></div>
    </div>
  </div>
{/if}

{#if downloading && !applying && $route.name !== 'settings'}
  <div class="card">
    <div class="row">
      <Download size="1.8rem" strokeWidth={1.8} />
      <span class="title">Загрузка обновления {version}</span>
      <IconButton label="Отменить загрузку" size="sm" onclick={requestCancelDownload}>
        <X size="1.6rem" strokeWidth={1.8} />
      </IconButton>
    </div>
    <ProgressBar value={pct} />
    <span class="meta">{bytesSize(downloadedBytes)} из {bytesSize(totalBytes)} · {Math.round(pct)}%</span>
  </div>
{:else if outcome}
  <div class="card" class:card-danger={!outcome.ok}>
    <div class="row">
      {#if outcome.ok}
        <CheckCircle2 size="1.8rem" strokeWidth={1.8} />
        <span class="title">Лаунчер обновлён до {outcome.version}</span>
      {:else}
        <TriangleAlert size="1.8rem" strokeWidth={1.8} />
        <span class="title">Обновление {outcome.version} не установилось</span>
      {/if}
      <IconButton label="Скрыть" size="sm" onclick={dismissOutcome}>
        <X size="1.6rem" strokeWidth={1.8} />
      </IconButton>
    </div>
    {#if !outcome.ok && outcome.error}
      <span class="meta">{outcomeReason(outcome)}</span>
    {/if}
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(4, 6, 10, 0.82);
  }

  .panel {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-4);
    width: min(42rem, calc(100vw - 4.8rem));
    padding: 3.6rem 3.2rem;
    text-align: center;
    color: var(--text);
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--cut) var(--radius-xl) var(--radius-xl) var(--radius-xl);
    box-shadow: var(--shadow-modal);
  }

  .panel h3 {
    margin: 0;
    font-size: var(--font-lg);
  }

  .panel p {
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-sm);
    line-height: 1.5;
  }

  .panel :global(.spin) {
    color: var(--accent);
    animation: spin 1.6s linear infinite;
  }

  .marquee {
    position: relative;
    width: 100%;
    height: 0.4rem;
    margin-top: var(--space-2);
    background: rgba(255, 255, 255, 0.07);
    border-radius: 99rem;
    overflow: hidden;
  }

  .marquee span {
    position: absolute;
    inset-block: 0;
    width: 34%;
    background: var(--accent);
    border-radius: 99rem;
    animation: slide 1.4s var(--ease) infinite;
  }

  .card {
    position: fixed;
    right: 2.4rem;
    bottom: 2.4rem;
    z-index: 150;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    width: 32rem;
    padding: var(--space-4) var(--space-5);
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--cut) var(--radius-lg) var(--radius-lg) var(--radius-lg);
    box-shadow: var(--shadow-modal);
  }

  .card-danger {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .title {
    flex: 1;
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .meta {
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .card-danger .meta {
    color: var(--danger);
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  @keyframes slide {
    0% {
      left: -34%;
    }
    100% {
      left: 100%;
    }
  }
</style>
