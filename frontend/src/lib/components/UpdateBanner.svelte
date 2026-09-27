<script lang="ts">
  import { AlertTriangle, Download, RefreshCw, RotateCcw, X } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import {
    requestApply,
    requestCancelDownload,
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
  import { msg } from '../i18n';

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
      <span>{statusReason(status) || msg('ui.updateCheckFailed')}</span>
    </div>
    <Button size="sm" onclick={retryFailed}>
      <RefreshCw size="1.5rem" strokeWidth={1.8} />
      {msg('common.retry')}
    </Button>
  </div>
{:else if downloading || view === 'downloading'}
  <div class="banner">
    <div class="banner-text">
      <span>{msg('ui.downloadingUpdate', { version: status.availableVersion ?? '' })}</span>
    </div>
    <div class="progress">
      <ProgressBar value={pct} />
      <span class="muted">{msg('ui.bytesOfBytes', { done: bytesSize(downloadedBytes), total: bytesSize(totalBytes) })} · {Math.round(pct)}%</span>
      <Button size="sm" onclick={requestCancelDownload}>
        <X size="1.5rem" strokeWidth={1.8} />
        {msg('ui.cancelVerb')}
      </Button>
    </div>
  </div>
{:else if view === 'applying'}
  <div class="banner">
    <div class="banner-text">
      <span>{msg('ui.installingUpdate', { version: status.availableVersion ?? '' })}</span>
    </div>
    <p class="muted">{msg('ui.launcherRestartShort')}</p>
  </div>
{:else if view === 'ready'}
  <div class="banner">
    <div class="banner-text">
      <span>{msg('ui.updateReadyMessage', { version: status.availableVersion ?? '' })}</span>
    </div>
    <p class="muted">{msg('ui.launcherRestartNewVersion')}</p>
    <div class="actions">
      <Button variant="primary" size="sm" onclick={() => (confirmOpen = true)}>{msg('ui.restartAndUpdate')}</Button>
      <Button size="sm" onclick={requestDismiss}>{msg('ui.notNow')}</Button>
    </div>
  </div>
{:else if view === 'available'}
  <div class="banner">
    <div class="banner-text">
      <span>{msg('ui.versionAvailable', { version: status.availableVersion ?? '' })}</span>
    </div>
    {#if status.notes}
      <p class="muted notes">{status.notes}</p>
    {/if}
    <div class="actions">
      <Button variant="primary" size="sm" onclick={requestDownload}>
        <Download size="1.5rem" strokeWidth={1.8} />
        {msg('ui.download')}
      </Button>
    </div>
  </div>
{/if}

<Modal bind:open={confirmOpen} title={msg('ui.restartLauncherTitle')}>
  <p class="modal-text">
    {msg('ui.restartLauncherWarning', { version: status.availableVersion ?? '' })}
  </p>
  {#snippet footer()}
    <Button onclick={() => (confirmOpen = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" onclick={confirmApply}>
      <RotateCcw size="1.5rem" strokeWidth={1.8} />
      {msg('ui.restartAndUpdate')}
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
