<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import Toggle from './Toggle.svelte';
  import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
  import { installErrorText } from '../install/installErrors';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { msg } from '../i18n';
  import { nextIndex } from '../utils/roving';

  type Mode = 'disk' | 'library';

  let {
    open = $bindable(false),
    gameId,
    title,
    mode = $bindable<Mode>('disk'),
    external = '',
    emulated = false,
    onremoved,
    returnFocus,
  }: {
    open?: boolean;
    gameId: string;
    title: string;
    mode?: Mode;
    external?: string;
    emulated?: boolean;
    onremoved?: (mode: Mode) => void;
    /** Where focus goes on close when the element that opened the dialog is gone (a menu item). */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();

  const uid = $props.id();

  let info = $state<RemovalInfo | null>(null);
  let loading = $state(false);
  let working = $state(false);
  let failure = $state('');
  let deleteFiles = $state(false);
  let deleteDownload = $state(false);
  let loadedFor = $state('');
  let modeGroup = $state<HTMLElement>();

  const blocked = $derived(!!info && (info.running || info.busy));
  const manualImage = $derived(emulated && !info?.owned);
  const canDeleteFiles = $derived(!external && !manualImage && !!info && info.owned && !info.dirMissing);
  const canUninstall = $derived(!external && !manualImage && !!info && (info.method === 'installer' || canDeleteFiles));

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
    return installErrorText(err, fallback);
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

  const MODES: Mode[] = ['disk', 'library'];
  const modeOff = (index: number) => MODES[index] === 'disk' && !canUninstall;

  // A radio group: the arrows move the choice and the focus together, skipping a tile that is off.
  function moveMode(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = [...(modeGroup?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), MODES.length, event.key, { orientation: 'both', isDisabled: modeOff });
    if (next === null) return;
    event.preventDefault();
    mode = MODES[next];
    radios[next]?.focus();
  }

  const freedText = $derived.by(() => {
    if (!info) return '';
    if (info.sizeUnknown) return msg('modals.removeGameSizeUnknown');
    if (info.sizeBytes > 0) return msg('modals.removeGameFreedSize', { size: bytesSize(info.sizeBytes) });
    return msg('modals.removeGameFreedByUninstaller');
  });
</script>

<Modal dismissible={!working} bind:open title={msg('modals.removeGameTitle')} {returnFocus}>
  {#if loading}
    <div class="loading"><Spinner size="sm" label={msg('modals.removeGameLoading', { title })} /></div>
  {:else if info}
    <div class="modes" role="radiogroup" aria-label={msg('modals.removeGameTitle')} bind:this={modeGroup}>
      <button
        type="button"
        class="mode"
        role="radio"
        aria-checked={mode === 'disk'}
        aria-disabled={!canUninstall || undefined}
        tabindex={mode === 'disk' && canUninstall ? 0 : -1}
        onclick={() => {
          if (canUninstall) mode = 'disk';
        }}
        onkeydown={moveMode}
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
      <button
        type="button"
        class="mode"
        role="radio"
        aria-checked={mode === 'library'}
        tabindex={mode === 'library' || !canUninstall ? 0 : -1}
        onclick={() => (mode = 'library')}
        onkeydown={moveMode}
      >
        <span class="mode-label">{msg('modals.removeGameModeLibraryLabel')}</span>
        <span class="mode-sub">{msg('modals.removeGameLibrarySub')}</span>
      </button>
    </div>

    {#if manualImage}
      <p class="text">{msg('emul.removeNote')}</p>
    {:else if emulated}
      <p class="text">{msg('emul.removeOwnedNote')}</p>
    {:else if external}
      <p class="text">{msg('games.externalRemoveNote')}</p>
    {:else}
      <p class="text">{methodText}</p>
    {/if}
    {#if info.installDir && !info.dirMissing}
      <p class="path">{info.installDir}</p>
    {/if}

    {#if info.running}
      <div class="notice"><Notice tone="warning" compact announce={false}>{msg('modals.removeGameRunningWarning')}</Notice></div>
    {:else if info.busy}
      <div class="notice"><Notice tone="warning" compact announce={false}>{msg('modals.removeGameBusyWarning')}</Notice></div>
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
            <span class="row-label" id="{uid}-delete-files-label">
              {info.method === 'installer' ? msg('modals.removeGameDeleteLeftoverFolder') : msg('modals.removeGameDeleteGameFiles')}
            </span>
            <span class="row-sub" id="{uid}-delete-files-sub">
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
          <Toggle bind:checked={deleteFiles} labelledby="{uid}-delete-files-label" describedby="{uid}-delete-files-sub" disabled={!canDeleteFiles} />
        </div>
      {/if}

      {#if info.downloadPresent}
        <div class="row">
          <div class="row-text">
            <span class="row-label" id="{uid}-remove-game-delete-download-label">{msg('modals.removeGameDeleteDownloadLabel')}</span>
            <span class="row-sub" id="{uid}-remove-game-delete-download-sub">
              {#if info.downloadSeeding}
                {msg('modals.removeGameSeedingWillStop')}
              {:else}
                {msg('modals.removeGameDownloadInFolder', { size: bytesSize(info.downloadBytes) })}{info.downloadPath ? `: ${info.downloadPath}` : ''}
              {/if}
            </span>
          </div>
          <Toggle bind:checked={deleteDownload} labelledby="{uid}-remove-game-delete-download-label" describedby="{uid}-remove-game-delete-download-sub" />
        </div>
      {/if}
    </div>
  {/if}

  {#if failure}
    <div class="notice"><Notice tone="danger" compact>{failure}</Notice></div>
  {/if}

  {#snippet footer()}
    <Button disabled={working} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="danger" onclick={confirm} disabled={!info || blocked} busy={working}>
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

  .loading {
    display: flex;
    margin: var(--space-5) 0 0;
  }

  .notice {
    margin: var(--space-4) 0 0;
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

  .mode:hover:not([aria-disabled='true']) {
    background: var(--surface-3);
  }

  .mode[aria-checked='true'] {
    border-color: var(--accent-bright);
  }

  .mode[aria-disabled='true'] {
    opacity: 0.5;
    cursor: default;
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
