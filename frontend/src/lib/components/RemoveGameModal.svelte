<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Toggle from './Toggle.svelte';
  import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { msg } from '../i18n';

  type Mode = 'disk' | 'library';

  let {
    open = $bindable(false),
    gameId,
    title,
    mode = $bindable<Mode>('disk'),
    onremoved,
  }: {
    open?: boolean;
    gameId: string;
    title: string;
    mode?: Mode;
    onremoved?: (mode: Mode) => void;
  } = $props();

  let info = $state<RemovalInfo | null>(null);
  let loading = $state(false);
  let working = $state(false);
  let failure = $state('');
  let deleteFiles = $state(false);
  let deleteDownload = $state(false);
  let loadedFor = $state('');

  const blocked = $derived(!!info && (info.running || info.busy));
  const canDeleteFiles = $derived(!!info && info.owned && !info.dirMissing);
  const canUninstall = $derived(!!info && (info.method === 'installer' || canDeleteFiles));

  $effect(() => {
    if (!open) {
      loadedFor = '';
      return;
    }
    if (loadedFor === gameId) return;
    loadedFor = gameId;
    void load();
  });

  $effect(() => {
    if (info && !canUninstall) mode = 'library';
  });

  async function load() {
    loading = true;
    failure = '';
    info = null;
    try {
      const loaded = await inspectRemoval(gameId);
      info = loaded;
      deleteFiles = loaded.owned && !loaded.dirMissing;
      deleteDownload = false;
    } catch (err) {
      failure = message(err, msg('modals.removeGameInspectFailed'));
    } finally {
      loading = false;
    }
  }

  function message(err: unknown, fallback: string) {
    return err instanceof Error && err.message ? err.message : fallback;
  }

  async function confirm() {
    if (!info || working) return;
    const keepInLibrary = mode === 'disk';
    working = true;
    failure = '';
    try {
      await removeGame(gameId, {
        deleteFiles: keepInLibrary ? canDeleteFiles : deleteFiles && canDeleteFiles,
        deleteDownload,
        keepInLibrary,
      });
      open = false;
      toast(
        keepInLibrary
          ? msg('modals.removeGameRemovedFromDisk', { title })
          : msg('modals.removeGameRemovedFromLibrary', { title }),
      );
      onremoved?.(mode);
    } catch (err) {
      failure = message(err, msg('modals.removeGameDeleteFailed'));
    } finally {
      working = false;
    }
  }

  const methodText = $derived.by(() => {
    if (!info) return '';
    if (info.method === 'installer' && info.quietUninstall) {
      return msg('modals.removeGameMethodInstallerQuiet');
    }
    if (info.method === 'installer') {
      return msg('modals.removeGameMethodInstaller');
    }
    if (info.method === 'files') {
      return msg('modals.removeGameMethodFiles');
    }
    if (info.uninstallUnknown) {
      return msg('modals.removeGameMethodUnknown');
    }
    if (info.dirMissing) {
      return msg('modals.removeGameMethodDirMissing');
    }
    return msg('modals.removeGameMethodNotOwned');
  });

  const freedText = $derived.by(() => {
    if (!info) return '';
    if (info.sizeUnknown) return msg('modals.removeGameSizeUnknown');
    if (info.sizeBytes > 0) return msg('modals.removeGameFreedSize', { size: bytesSize(info.sizeBytes) });
    return msg('modals.removeGameFreedByUninstaller');
  });
</script>

<Modal bind:open title={msg('modals.removeGameTitle')}>
  {#if loading}
    <p class="text">{msg('modals.removeGameLoading', { title })}</p>
  {:else if info}
    <div class="modes">
      <button
        class="mode"
        class:selected={mode === 'disk'}
        disabled={!canUninstall}
        onclick={() => (mode = 'disk')}
      >
        <span class="mode-label">{msg('modals.removeGameModeDiskLabel')}</span>
        <span class="mode-sub">
          {#if canUninstall}
            {msg('modals.removeGameDiskSubCanUninstall')}
          {:else if info.dirMissing}
            {msg('modals.removeGameDiskSubDirMissing')}
          {:else}
            {msg('modals.removeGameDiskSubNotOwned')}
          {/if}
        </span>
      </button>
      <button class="mode" class:selected={mode === 'library'} onclick={() => (mode = 'library')}>
        <span class="mode-label">{msg('modals.removeGameModeLibraryLabel')}</span>
        <span class="mode-sub">{msg('modals.removeGameLibrarySub')}</span>
      </button>
    </div>

    <p class="text">{methodText}</p>
    {#if info.installDir && !info.dirMissing}
      <p class="path">{info.installDir}</p>
    {/if}

    {#if info.running}
      <p class="warn">{msg('modals.removeGameRunningWarning')}</p>
    {:else if info.busy}
      <p class="warn">{msg('modals.removeGameBusyWarning')}</p>
    {/if}

    <div class="options">
      {#if mode === 'disk'}
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('modals.removeGameStaysInLibrary')}</span>
            <span class="row-sub">{freedText}</span>
          </div>
        </div>
      {:else}
        <div class="row" class:off={!canDeleteFiles}>
          <div class="row-text">
            <span class="row-label">
              {info.method === 'installer' ? msg('modals.removeGameDeleteLeftoverFolder') : msg('modals.removeGameDeleteGameFiles')}
            </span>
            <span class="row-sub">
              {#if info.dirMissing}
                {msg('modals.removeGameDirAlreadyGone')}
              {:else if !info.owned && info.method === 'installer'}
                {msg('modals.removeGameFolderRemovedByUninstaller')}
              {:else if !info.owned}
                {msg('modals.removeGameFilesNotOwned')}
              {:else if info.sizeUnknown}
                {msg('modals.removeGameSizeUnknown')}
              {:else if info.method === 'installer'}
                {msg('modals.removeGameUninstallerLeftover', { size: bytesSize(info.sizeBytes) })}
              {:else}
                {msg('modals.removeGameFreedSize', { size: bytesSize(info.sizeBytes) })}
              {/if}
            </span>
          </div>
          <Toggle bind:checked={deleteFiles} label={msg('modals.removeGameDeleteGameFiles')} disabled={!canDeleteFiles} />
        </div>
      {/if}

      {#if info.downloadPresent}
        <div class="row" class:off={info.downloadSeeding}>
          <div class="row-text">
            <span class="row-label">{msg('modals.removeGameDeleteDownloadLabel')}</span>
            <span class="row-sub">
              {#if info.downloadSeeding}
                {msg('modals.removeGameSeedingActive')}
              {:else}
                {msg('modals.removeGameDownloadInFolder', { size: bytesSize(info.downloadBytes) })}{info.downloadPath ? `: ${info.downloadPath}` : ''}
              {/if}
            </span>
          </div>
          <Toggle
            bind:checked={deleteDownload}
            label={msg('modals.removeGameDeleteDownloadedFiles')}
            disabled={info.downloadSeeding}
          />
        </div>
      {/if}
    </div>
  {/if}

  {#if failure}
    <p class="error">{failure}</p>
  {/if}

  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="danger" onclick={confirm} disabled={!info || blocked || working}>
      {#if working}
        {msg('modals.removeGameWorking')}
      {:else if mode === 'disk'}
        {msg('modals.removeGameModeDiskLabel')}
      {:else}
        {msg('modals.removeGameModeLibraryLabel')}
      {/if}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: var(--space-5) 0 0;
    color: var(--text-2);
    line-height: 1.5;
  }

  .path {
    margin: var(--space-3) 0 0;
    font-size: 1.3rem;
    color: var(--text-3);
    word-break: break-all;
  }

  .warn {
    margin: var(--space-4) 0 0;
    color: var(--warning);
  }

  .error {
    margin: var(--space-4) 0 0;
    color: var(--danger);
  }

  .modes {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }

  .mode {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
    padding: var(--space-4);
    text-align: left;
    background: var(--surface-2);
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .mode:hover:not(:disabled) {
    background: var(--surface-3);
  }

  .mode.selected {
    border-color: var(--accent);
  }

  .mode:disabled {
    opacity: 0.5;
  }

  .mode-label {
    font-weight: 500;
  }

  .mode-sub {
    font-size: 1.3rem;
    line-height: 1.4;
    color: var(--text-3);
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface-2);
    border-radius: var(--radius-md);
  }

  .row.off {
    opacity: 0.6;
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .row-label {
    font-weight: 500;
  }

  .row-sub {
    font-size: 1.3rem;
    color: var(--text-3);
  }
</style>
