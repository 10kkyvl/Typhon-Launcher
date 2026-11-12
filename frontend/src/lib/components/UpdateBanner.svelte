<script lang="ts">
  import { Download, RefreshCw, X } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import UpdateRestartModal from './UpdateRestartModal.svelte';
  import {
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
  let retrying = $state(false);

  const status = $derived($selfUpdateStatus);
  const view = $derived($selfUpdateView);
  const downloading = $derived($selfUpdateDownloading);
  const progress = $derived($selfUpdateProgress);

  const totalBytes = $derived(progress?.totalBytes ?? status.totalBytes ?? 0);
  const downloadedBytes = $derived(progress?.downloadedBytes ?? status.downloadedBytes ?? 0);
  const pct = $derived(totalBytes > 0 ? Math.min(100, Math.max(0, (downloadedBytes / totalBytes) * 100)) : 0);

  // The failure stays on screen until the retry's answer changes the status: the notice and its
  // focused Retry stay, the Retry busy.
  async function retry() {
    if (retrying) return;
    retrying = true;
    try {
      await retryFailed();
    } finally {
      retrying = false;
    }
  }
</script>

{#if view === 'failed'}
  <div class="message">
    <Notice tone="danger">
      {statusReason(status) || msg('ui.updateCheckFailed')}
      {#snippet action()}
        <Button size="sm" busy={retrying} onclick={retry}>
          {#if !retrying}<RefreshCw size="1.5rem" strokeWidth={1.8} />{/if}
          {msg('common.retry')}
        </Button>
      {/snippet}
    </Notice>
  </div>
{:else if downloading || view === 'downloading'}
  <div class="banner">
    <div class="banner-text">
      <span>{msg(status.delta ? 'ui.downloadingUpdatePatch' : 'ui.downloadingUpdate', { version: status.availableVersion ?? '' })}</span>
    </div>
    <div class="progress">
      <ProgressBar
        value={Math.round(pct)}
        stretch
        label={msg(status.delta ? 'ui.downloadingUpdatePatch' : 'ui.downloadingUpdate', { version: status.availableVersion ?? '' })}
      />
      <span class="muted">{msg('ui.bytesOfBytes', { done: bytesSize(downloadedBytes), total: bytesSize(totalBytes) })} · {Math.round(pct)}%</span>
      <Button size="sm" onclick={requestCancelDownload}>
        <X size="1.5rem" strokeWidth={1.8} />
        {msg('ui.cancelVerb')}
      </Button>
    </div>
  </div>
{:else if view === 'applying'}
  <div class="message">
    <Notice title={msg('ui.installingUpdate', { version: status.availableVersion ?? '' })}>
      {msg('ui.launcherRestartShort')}
    </Notice>
  </div>
{:else if view === 'ready'}
  <div class="message">
    <Notice tone="success" title={msg('ui.updateReadyMessage', { version: status.availableVersion ?? '' })}>
      {msg('ui.launcherRestartNewVersion')}
      {#snippet action()}
        <Button variant="primary" size="sm" onclick={() => (confirmOpen = true)}>{msg('ui.restartAndUpdate')}</Button>
        <Button size="sm" onclick={requestDismiss}>{msg('ui.notNow')}</Button>
      {/snippet}
    </Notice>
  </div>
{:else if view === 'available'}
  <!-- Found by a check that ran on its own: a standing fact of the settings page, read with it. -->
  <div class="message">
    <Notice announce={false} title={status.notes ? msg('ui.versionAvailable', { version: status.availableVersion ?? '' }) : undefined}>
      {status.notes || msg('ui.versionAvailable', { version: status.availableVersion ?? '' })}
      {#snippet action()}
        <Button variant="primary" size="sm" onclick={requestDownload}>
          <Download size="1.5rem" strokeWidth={1.8} />
          {msg('ui.download')}
        </Button>
      {/snippet}
    </Notice>
  </div>
{/if}

<UpdateRestartModal bind:open={confirmOpen} />

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

  .message {
    margin-top: var(--space-4);
  }

  .banner-text {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .muted {
    color: var(--text-3);
    font-size: var(--font-xs);
    margin: 0;
  }
</style>
