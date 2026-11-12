<script lang="ts">
  import { tick } from 'svelte';
  import { get } from 'svelte/store';
  import { Download, X } from '@lucide/svelte';
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
  import { focusableIn, focusQuietly } from '../layers/focus';
  import { msg } from '../i18n';

  let confirmOpen = $state(false);
  let retrying = $state(false);
  let root = $state<HTMLElement>();

  const status = $derived($selfUpdateStatus);
  const view = $derived($selfUpdateView);
  const downloading = $derived($selfUpdateDownloading);
  const progress = $derived($selfUpdateProgress);

  const totalBytes = $derived(progress?.totalBytes ?? status.totalBytes ?? 0);
  const downloadedBytes = $derived(progress?.downloadedBytes ?? status.downloadedBytes ?? 0);
  const pct = $derived(totalBytes > 0 ? Math.min(100, Math.max(0, (downloadedBytes / totalBytes) * 100)) : 0);

  // What was on screen when the page opened is read with the page; what came after it (the answer to
  // a check, a download that finished) is announced.
  const openedWith = get(selfUpdateView);
  let moved = $state(false);
  $effect.pre(() => {
    if (view !== openedWith) moved = true;
  });

  // The answer to a retry is the status leaving "failed" (a download that starts shows its progress at
  // once), else the end of the request.
  function retry(): Promise<boolean> {
    return new Promise((resolve, reject) => {
      let answered = false;
      let off: (() => void) | undefined;
      const answer = (ok: boolean) => {
        if (answered) return;
        answered = true;
        off?.();
        resolve(ok);
      };
      off = selfUpdateView.subscribe((next) => {
        if (next !== 'failed') answer(true);
      });
      if (answered) off();
      retryFailed().then(
        () => answer(get(selfUpdateView) !== 'failed'),
        (err: unknown) => {
          off?.();
          reject(err);
        },
      );
    });
  }

  // A pressed button the next state replaces (Download by the progress, "Not now" by nothing) would
  // drop focus to the page: it goes to the first control of what came, else on past the banner.
  $effect.pre(() => {
    void view;
    void downloading;
    const box = root;
    const active = document.activeElement;
    if (!box || !(active instanceof HTMLElement) || !box.contains(active)) return;
    const all = focusableIn(document.body);
    const mine = all.filter((el) => box.contains(el));
    const after = all[all.indexOf(mine[mine.length - 1]) + 1];
    void tick().then(() => {
      const now = document.activeElement;
      if (now && now !== document.body && now.isConnected) return;
      const target = focusableIn(box)[0] ?? after;
      if (target?.isConnected) focusQuietly(target);
    });
  });
</script>

<div bind:this={root}>
{#if view === 'failed' || retrying}
  <div class="message">
    <Notice tone="danger" announce={moved} bind:retrying retry={{ label: msg('common.retry'), run: retry, within: () => root }}>
      {statusReason(status) || msg('ui.updateCheckFailed')}
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
    <Notice announce={moved} title={msg('ui.installingUpdate', { version: status.availableVersion ?? '' })}>
      {msg('ui.launcherRestartShort')}
    </Notice>
  </div>
{:else if view === 'ready'}
  <div class="message">
    <Notice tone="success" announce={moved} title={msg('ui.updateReadyMessage', { version: status.availableVersion ?? '' })}>
      {msg('ui.launcherRestartNewVersion')}
      {#snippet action()}
        <Button variant="primary" size="sm" onclick={() => (confirmOpen = true)}>{msg('ui.restartAndUpdate')}</Button>
        <Button size="sm" onclick={requestDismiss}>{msg('ui.notNow')}</Button>
      {/snippet}
    </Notice>
  </div>
{:else if view === 'available'}
  <div class="message">
    <Notice announce={moved} title={status.notes ? msg('ui.versionAvailable', { version: status.availableVersion ?? '' }) : undefined}>
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
</div>

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
