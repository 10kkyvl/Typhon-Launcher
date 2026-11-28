<script lang="ts">
  import type { Download } from '../services/downloads';
  import type { Installation } from '../services/install';
  import { installIndeterminate, installTotalUnknown } from '../install/progress';
  import { platformBadge } from '../library/emulation';
  import { installErrorText } from '../install/installErrors';
  import { installStatusLabels } from '../stores/install';
  import { bytesSize, progressPercent } from '../utils/format';
  import Button from './Button.svelte';
  import ConsoleTag from './ConsoleTag.svelte';
  import EmulatorHint from './EmulatorHint.svelte';
  import Notice from './Notice.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import { msg } from '../i18n';

  /** InstallModal's view of a started install: an image waiting to be installed, progress, done, stopped. */
  let {
    phase,
    installation,
    imageDownload,
    imageTitle,
    imagePlatform,
    externalWait,
    askCleanup,
    cleanupResolved = $bindable(),
    seedingNow,
    own,
    other,
    ondelete,
  }: {
    phase: string;
    installation: Installation | undefined;
    imageDownload: Download | null;
    imageTitle: string;
    imagePlatform: string;
    externalWait: boolean;
    askCleanup: boolean;
    cleanupResolved: boolean;
    seedingNow: boolean;
    own: (by: string) => boolean;
    other: (by: string) => boolean;
    ondelete: () => void;
  } = $props();
</script>

{#if phase === 'image' && imageDownload}
  <div class="block">
    <div class="pkg">
      <span class="pkg-name">{imageTitle}</span>
      <ConsoleTag platform={imagePlatform} />
    </div>
    <div class="rows">
      <div class="row">
        <span class="key">{msg('emul.downloadName')}</span>
        <span class="value path">{imageDownload.name}</span>
      </div>
      <div class="row">
        <span class="key">{msg('emul.downloadSize')}</span>
        <span class="value">{bytesSize(imageDownload.total)}</span>
      </div>
    </div>
    <p class="note">{msg('emul.installLead')}</p>
    <p class="note">{msg('emul.downloadNote')}</p>
    <EmulatorHint platform={imagePlatform} />
  </div>
{:else if phase === 'progress' && installation}
  <div class="block">
    <div class="pkg">
      <span class="pkg-name">{installation.name}</span>
      {#if installation.platform}<ConsoleTag platform={installation.platform} />{/if}
      <span class="pkg-type">{installStatusLabels(installation.status)}</span>
    </div>
    {#if externalWait}
      <p class="note">{msg('modals.installWaitingExternal')}</p>
    {:else}
      <ProgressBar value={installation.progress * 100} indeterminate={installIndeterminate(installation)} label={installation.name} />
      {#if installTotalUnknown(installation)}
      <div class="progress-foot">
        <span class="size">{msg('modals.installWritten', { size: bytesSize(installation.bytesDone) })}</span>
      </div>
      {:else if installation.status !== 'verifying'}
      <div class="progress-foot">
        <span class="size">{bytesSize(installation.bytesDone)} / {bytesSize(installation.bytesTotal)}</span>
        <span class="pct">{progressPercent(installation.progress)}%</span>
      </div>
      {/if}
      {#if installation.currentFile}
        <span class="current-file">{installation.currentFile}</span>
      {/if}
    {/if}
  </div>
{:else if phase === 'done' && installation}
  <div class="block">
    <div class="rows">
      <div class="row">
        <span class="key">{msg('modals.installGameLabel')}</span>
        <span class="value">{installation.name}</span>
      </div>
      <div class="row">
        <span class="key">{msg('modals.installFolderLabel')}</span>
        <span class="value path">{installation.destination}</span>
      </div>
      {#if installation.platform}
        <div class="row">
          <span class="key">{msg('emul.addConsoleLabel')}</span>
          <span class="value">{platformBadge(installation.platform)}</span>
        </div>
      {:else}
        <div class="row">
          <span class="key">{msg('modals.installVersionLabel')}</span>
          <span class="value">{installation.detectedVersion || msg('modals.installUnknownVersion')}</span>
        </div>
      {/if}
    </div>
    {#if installation.platform}
      <p class="note">{msg('emul.installDone')}</p>
      <EmulatorHint platform={installation.platform} />
    {/if}
    <!-- Go's warning is prose with a wrapped error, and Go sets one only when the download's record
         stays after a move (install.releaseMovedDownload): the dialog says that in its own words. -->
    {#if installation.warning}
      <Notice tone="warning" compact announce={false}>{msg('modals.installWarningDownloadLeft')}</Notice>
    {/if}
    {#if askCleanup && !cleanupResolved}
      <div class="field">
        <span class="field-label">{msg('modals.installDownloadedFilesLabel')}</span>
        {#if seedingNow}
          <Notice tone="warning" compact announce={false}>{msg('modals.installSeedingDeleteWarning')}</Notice>
        {/if}
        <div class="cleanup-actions">
          <Button size="sm" variant="danger" disabled={other('delete')} reason={other('delete') ? msg('modals.actionRunning') : ''} busy={own('delete')} onclick={ondelete}>
            {msg('modals.installDeleteDownloadedFiles')}
          </Button>
          <Button size="sm" disabled={other('keep')} reason={other('keep') ? msg('modals.actionRunning') : ''} onclick={() => (cleanupResolved = true)}>{msg('modals.installKeep')}</Button>
        </div>
      </div>
    {/if}
  </div>
{:else if phase === 'problem' && installation}
  <div class="block">
    <div class="pkg">
      <span class="pkg-name">{installation.name}</span>
      {#if installation.platform}<ConsoleTag platform={installation.platform} />{/if}
      <span class="pkg-type">{installStatusLabels(installation.status)}</span>
    </div>
    <Notice tone="danger" compact>{installation.error ? installErrorText(installation.error) : msg('modals.installNotCompleted')}</Notice>
  </div>
{/if}

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .pkg {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .pkg-name {
    font-size: var(--font-lg);
    font-weight: 600;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .pkg-type {
    font-size: var(--font-xs);
    color: var(--text-3);
    flex-shrink: 0;
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.8rem 0;
    font-size: var(--font-sm);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .key {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .value {
    min-width: 0;
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  /* A path or a file name is read whole: it wraps where it must instead of being cut. */
  .value.path {
    overflow: visible;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    align-items: flex-start;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .note {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .progress-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .pct {
    color: var(--text-2);
    font-weight: 500;
  }

  .current-file {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .cleanup-actions {
    display: flex;
    gap: 0.8rem;
  }
</style>
