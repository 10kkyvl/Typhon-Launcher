<script lang="ts">
  import { AlertTriangle, Download, RefreshCw, RotateCcw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import {
    requestApply,
    requestDismiss,
    requestDownload,
    retryFailed,
    selfUpdateDownloading,
    selfUpdateProgress,
    selfUpdateStatus,
    selfUpdateView,
  } from '../stores/selfupdate';
  import { statusReason } from '../services/selfupdateMessages';
  import { bytesSize } from '../utils/format';

  let confirmOpen = $state(false);

  const status = $derived($selfUpdateStatus);
  const view = $derived($selfUpdateView);
  const downloading = $derived($selfUpdateDownloading);
  const progress = $derived($selfUpdateProgress);

  const totalBytes = $derived(progress?.totalBytes ?? status.totalBytes ?? 0);
  const downloadedBytes = $derived(progress?.downloadedBytes ?? status.downloadedBytes ?? 0);
  const pct = $derived(totalBytes > 0 ? Math.min(100, Math.max(0, (downloadedBytes / totalBytes) * 100)) : 0);

  function confirmApply() {
    confirmOpen = false;
    requestApply();
  }
</script>

{#if view === 'failed'}
  <div class="banner banner-danger">
    <div class="banner-text">
      <AlertTriangle size="1.8rem" strokeWidth={1.8} />
      <span>{statusReason(status) || 'Не удалось проверить обновления'}</span>
    </div>
    <Button size="sm" onclick={retryFailed}>
      <RefreshCw size="1.5rem" strokeWidth={1.8} />
      Повторить
    </Button>
  </div>
{:else if downloading || view === 'downloading'}
  <div class="banner">
    <div class="banner-text">
      <span>Загрузка обновления {status.availableVersion}</span>
    </div>
    <div class="progress">
      <ProgressBar value={pct} />
      <span class="muted">{bytesSize(downloadedBytes)} из {bytesSize(totalBytes)} · {Math.round(pct)}%</span>
    </div>
  </div>
{:else if view === 'applying'}
  <div class="banner">
    <div class="banner-text">
      <span>Устанавливаем обновление {status.availableVersion}</span>
    </div>
    <p class="muted">Лаунчер закроется и запустится заново.</p>
  </div>
{:else if view === 'ready'}
  <div class="banner">
    <div class="banner-text">
      <span>Обновление {status.availableVersion} загружено и готово к установке</span>
    </div>
    <p class="muted">Лаунчер закроется и перезапустится с новой версией.</p>
    <div class="actions">
      <Button variant="primary" size="sm" onclick={() => (confirmOpen = true)}>Перезапустить и обновить</Button>
      <Button size="sm" onclick={requestDismiss}>Не сейчас</Button>
    </div>
  </div>
{:else if view === 'available'}
  <div class="banner">
    <div class="banner-text">
      <span>Доступна версия {status.availableVersion}</span>
    </div>
    {#if status.notes}
      <p class="muted notes">{status.notes}</p>
    {/if}
    <div class="actions">
      <Button variant="primary" size="sm" onclick={requestDownload}>
        <Download size="1.5rem" strokeWidth={1.8} />
        Скачать
      </Button>
    </div>
  </div>
{/if}

<Modal bind:open={confirmOpen} title="Перезапустить лаунчер?">
  <p class="modal-text">
    Typhon Launcher закроется прямо сейчас, установит обновление {status.availableVersion} и запустится заново.
    Незавершённые загрузки и установки продолжатся после перезапуска.
  </p>
  {#snippet footer()}
    <Button onclick={() => (confirmOpen = false)}>Отмена</Button>
    <Button variant="primary" onclick={confirmApply}>
      <RotateCcw size="1.5rem" strokeWidth={1.8} />
      Перезапустить и обновить
    </Button>
  {/snippet}
</Modal>

<style>
  .banner {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding: var(--space-4) var(--space-5);
    border-radius: var(--radius-md);
    background: var(--surface-3);
  }

  .banner-danger {
    background: var(--danger-subtle);
  }

  .banner-text {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .banner-danger .banner-text {
    color: var(--danger);
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .progress :global(.track) {
    flex: 1;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .muted {
    color: var(--text-3);
    font-size: var(--font-xs);
    margin: 0;
  }

  .modal-text {
    color: var(--text-2);
    line-height: 1.5;
  }
</style>
