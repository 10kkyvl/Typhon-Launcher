<script lang="ts">
  import { ArrowDownToLine, CloudAlert, CloudCheck, CloudOff, CloudUpload, RotateCcw, TriangleAlert } from '@lucide/svelte';
  import { cloudStatus } from '../stores/cloudsaves';
  import { libraryGames } from '../stores/library';
  import { selfUpdateDownloading, selfUpdateProgress, selfUpdateStatus, selfUpdateView } from '../stores/selfupdate';
  import { authState, isOffline } from '../stores/user';
  import { statusReason } from '../services/selfupdateMessages';
  import { cloudCodeText } from '../cloudsaves/messages';
  import { msg } from '../i18n';
  import UpdateRestartModal from './UpdateRestartModal.svelte';
  import { cloudRow, openCloudRow, runUpdateRow, updateRow } from './sidebarStatus';

  // The quiet line at the foot of the island: cloud saves on the left, the launcher version or the
  // waiting update on the right. Short words fit the column; the full sentence is the tooltip and the
  // accessible name, and stays when the island collapses to icons.

  let restartOpen = $state(false);

  const cloud = $derived(cloudRow($cloudStatus, { offline: $isOffline, guest: $authState === 'guest' }, $libraryGames));
  const update = $derived(updateRow($selfUpdateStatus, $selfUpdateView, $selfUpdateDownloading, $selfUpdateProgress));

  const cloudName = $derived.by(() => {
    switch (cloud.kind) {
      case 'off':
        return msg('cloud.disabled');
      case 'signIn':
        return msg('cloud.not_signed_in');
      case 'account':
        return cloudCodeText(cloud.code, msg('cloud.not_signed_in'));
      case 'error':
        return cloudCodeText(cloud.code, msg('cloud.failed'));
      case 'offline':
        return msg('ui.statusCloudOffline');
      case 'pending':
        return msg('cloudsaves.statePending');
      case 'synced':
        return msg('ui.statusCloudSynced');
      case 'conflict':
        return cloud.count > 1 ? msg('ui.statusCloudConflicts', { count: cloud.count }) : msg('ui.statusCloudConflict', { title: cloud.title });
      case 'failed':
        return msg('ui.statusCloudFailed', { title: cloud.title });
      default:
        return '';
    }
  });

  const retryName = $derived(update.kind === 'failed' ? msg('ui.statusUpdateRetry', { version: update.version }) : '');
</script>

{#snippet words(short: string, full: string)}
  <span class="short" aria-hidden="true">{short}</span><span class="sr-only">{full}</span>
{/snippet}

<div class="status" role="group" aria-label={msg('ui.sidebarStatusLabel')}>
  {#if cloud.kind === 'off' || cloud.kind === 'signIn'}
    <button type="button" class="item" data-cloud={cloud.kind} title={cloudName} onclick={() => void openCloudRow(cloud)}>
      <CloudOff aria-hidden="true" />{@render words(msg(cloud.kind === 'off' ? 'ui.statusCloudOffShort' : 'ui.statusCloudSignInShort'), cloudName)}
    </button>
  {:else if cloud.kind === 'account'}
    <span class="item" data-cloud="account" title={cloudName}>
      <CloudOff aria-hidden="true" />{@render words(msg('ui.statusCloudAccountShort'), cloudName)}
    </span>
  {:else if cloud.kind === 'error'}
    <span class="item danger" data-cloud="error" title={cloudName}>
      <CloudAlert aria-hidden="true" />{@render words(msg('cloudsaves.stateFailed'), cloudName)}
    </span>
  {:else if cloud.kind === 'offline'}
    <span class="item" data-cloud="offline" title={cloudName}>
      <CloudOff aria-hidden="true" />{@render words(msg('ui.statusOfflineShort'), cloudName)}
    </span>
  {:else if cloud.kind === 'pending'}
    <span class="item" data-cloud="pending" title={cloudName}>
      <CloudUpload aria-hidden="true" />{@render words(msg('cloudsaves.syncing'), cloudName)}
    </span>
  {:else if cloud.kind === 'synced'}
    <span class="item" data-cloud="synced" title={cloudName}>
      <CloudCheck aria-hidden="true" />{@render words(msg('cloudsaves.stateSynced'), cloudName)}
    </span>
  {:else if cloud.kind === 'conflict'}
    <button type="button" class="item warn" data-cloud="conflict" title={cloudName} onclick={() => void openCloudRow(cloud)}>
      <CloudAlert aria-hidden="true" />{@render words(
        cloud.count > 1 ? msg('ui.statusCloudConflictsShort', { count: cloud.count }) : msg('cloudsaves.stateConflict'),
        cloudName,
      )}
    </button>
  {:else if cloud.kind === 'failed'}
    <button type="button" class="item danger" data-cloud="failed" title={cloudName} onclick={() => void openCloudRow(cloud)}>
      <CloudAlert aria-hidden="true" />{@render words(msg('cloudsaves.stateFailed'), cloudName)}
    </button>
  {/if}

  {#if update.kind === 'version'}
    {@const full = msg('ui.versionLabel', { version: update.version })}
    <span class="item ver" data-update="version" title={full}>{@render words(update.version, full)}</span>
  {:else if update.kind === 'available'}
    {@const full = msg('ui.statusUpdateDownload', { version: update.version })}
    <button
      type="button"
      class="item ver accent"
      data-update="available"
      title={msg('ui.versionAvailable', { version: update.version })}
      onclick={() => void runUpdateRow(update, () => (restartOpen = true))}
    >
      <ArrowDownToLine aria-hidden="true" />{@render words(update.version, full)}
    </button>
  {:else if update.kind === 'downloading'}
    {@const full = `${msg('ui.downloadingUpdate', { version: update.version })} · ${update.percent}%`}
    <span class="item ver accent" data-update="downloading" title={full}>
      <ArrowDownToLine aria-hidden="true" />{@render words(`${update.percent}%`, full)}
    </span>
  {:else if update.kind === 'ready'}
    <button
      type="button"
      class="item ver accent"
      data-update="ready"
      title={msg('ui.updateReadyMessage', { version: update.version })}
      onclick={() => void runUpdateRow(update, () => (restartOpen = true))}
    >
      <RotateCcw aria-hidden="true" />{@render words(msg('ui.statusUpdateReadyShort'), msg('ui.restartAndUpdate'))}
    </button>
  {:else if update.kind === 'failed'}
    <button
      type="button"
      class="item ver danger"
      data-update="failed"
      title={statusReason($selfUpdateStatus) || retryName}
      onclick={() => void runUpdateRow(update, () => (restartOpen = true))}
    >
      <TriangleAlert aria-hidden="true" />{@render words(msg('ui.statusUpdateFailedShort'), retryName)}
    </button>
  {/if}
</div>

<UpdateRestartModal bind:open={restartOpen} />

<style>
  .status {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-2);
    height: 2.8rem;
    min-width: 0;
    margin-top: var(--space-2);
    padding-inline: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .item {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    white-space: nowrap;
    color: inherit;
  }

  .ver {
    flex: none;
    margin-left: auto;
    font-variant-numeric: tabular-nums;
  }

  button.item {
    min-height: 2.8rem;
    margin-inline: calc(-1 * var(--space-1));
    padding: 0 var(--space-1);
    border-radius: var(--radius-sm);
    transition:
      background var(--motion-fast) var(--ease-out),
      color var(--motion-fast) var(--ease-out);
  }

  button.item:hover {
    background: var(--hover-strong);
    color: var(--text);
  }

  button.item:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: -2px;
  }

  .short {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .accent {
    color: var(--accent-text);
  }

  .warn :global(svg) {
    color: var(--warning);
  }

  .danger :global(svg) {
    color: var(--danger);
  }

  .status :global(svg) {
    flex: none;
    width: 1.5rem;
    height: 1.5rem;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
  }

  @media (max-width: 1140px) {
    .status {
      flex-direction: column;
      height: auto;
      gap: var(--space-1);
      padding-inline: 0;
    }

    .ver {
      margin-left: 0;
    }

    .short {
      display: none;
    }

    button.item {
      justify-content: center;
      width: 100%;
      margin-inline: 0;
    }
  }
</style>
