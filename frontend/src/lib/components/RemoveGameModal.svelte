<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import OptionList from './OptionList.svelte';
  import Spinner from './Spinner.svelte';
  import Toggle from './Toggle.svelte';
  import type { OptionItem } from './optionList';
  import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
  import { REASONS, installErrorText } from '../install/installErrors';
  import { currentRouteKey } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { focusHandOff } from '../utils/focusHandOff';
  import { errorCode, msg } from '../i18n';

  type Mode = 'disk' | 'library';

  /**
   * Where the game goes: off the disk with its card kept, or out of the library. Both modes delete, so
   * the arrows only walk them and a press, Space or Enter chooses; the dialog opens on Cancel. What a
   * mode sends is exactly what the dialog shows (install.Service.RemoveGame). While the removal runs
   * nothing else can be pressed and the dialog stays.
   */
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
  const failureId = `${uid}-failure`;

  let info = $state<RemovalInfo | null>(null);
  let loading = $state(false);
  let readFailure = $state('');
  let readRetrying = $state(false);
  let working = $state(false);
  let failure = $state('');
  let deleteFiles = $state(false);
  let deleteDownload = $state(false);

  const blocked = $derived(!!info && (info.running || info.busy));
  const manualImage = $derived(emulated && !info?.owned);
  const canDeleteFiles = $derived(!external && !manualImage && !!info && info.owned && !info.dirMissing);
  const canUninstall = $derived(!external && !manualImage && !!info && (info.method === 'installer' || canDeleteFiles));

  // One per opening and game. The game page unmounts the dialog when the card it was opened for leaves
  // the library, which Go does before the removal answers: a destroyed dialog, or one turned to
  // another game, closes nothing and says the answer once, naming its game. It calls back only when
  // destroyed while the player is still on the page the removal was pressed on.
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });
  $effect(() => {
    const isOpen = open;
    const id = gameId;
    untrack(() => {
      opening += 1;
      info = null;
      loading = false;
      readFailure = '';
      working = false;
      failure = '';
      deleteFiles = false;
      deleteDownload = false;
      if (isOpen && id) void load();
    });
  });

  $effect(() => {
    if (info && !canUninstall) mode = 'library';
  });

  async function load(): Promise<boolean | undefined> {
    const at = opening;
    // A failed read stays on screen through its Retry (Notice's `retrying`) until the answer replaces it.
    loading = true;
    try {
      const loaded = await inspectRemoval(gameId);
      if (at !== opening) return undefined;
      readFailure = '';
      info = loaded;
      deleteFiles = loaded.owned && !loaded.dirMissing;
      deleteDownload = false;
      return true;
    } catch (err) {
      if (at !== opening) return undefined;
      readFailure = installErrorText(err, msg('modals.removeGameInspectFailed'));
      return false;
    } finally {
      if (at === opening) loading = false;
    }
  }

  function refusedFor(name: string, err: unknown): string {
    const key = REASONS[errorCode(err)];
    return key ? msg('modals.removeGameFailedForReason', { title: name, reason: msg(key) }) : msg('modals.removeGameFailedFor', { title: name });
  }

  async function confirm() {
    if (!info || working || blocked) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const id = gameId;
    const name = title;
    const chosen = mode;
    const keepInLibrary = chosen === 'disk';
    const options = {
      deleteFiles: keepInLibrary ? canDeleteFiles : deleteFiles && canDeleteFiles,
      deleteDownload,
      keepInLibrary,
    };
    const page = currentRouteKey();
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    working = true;
    failure = '';
    try {
      await removeGame(id, options);
      toast(keepInLibrary ? msg('modals.removeGameRemovedFromDisk', { title: name }) : msg('modals.removeGameRemovedFromLibrary', { title: name }));
      if (!heard()) {
        // The game page unmounts the dialog when the card leaves first: while the player is still on
        // the page the removal was pressed on, the page's own flow (where it leaves to) finishes.
        if (!alive && page === currentRouteKey()) onremoved?.(chosen);
        return;
      }
      working = false;
      open = false;
      onremoved?.(chosen);
    } catch (err) {
      if (!heard()) {
        toast(refusedFor(name, err), 'danger');
        return;
      }
      working = false;
      failure = installErrorText(err, msg('modals.removeGameDeleteFailed'));
      await hand(() => document.getElementById(failureId));
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

  const modes = $derived.by<OptionItem[]>(() => {
    const diskSub = canUninstall
      ? msg('modals.removeGameDiskSubCanUninstall')
      : info?.dirMissing
        ? msg('modals.removeGameDiskSubDirMissing')
        : msg('modals.removeGameDiskSubNotOwned');
    return [
      canUninstall
        ? { id: 'disk', title: msg('modals.removeGameModeDiskLabel'), note: diskSub }
        : { id: 'disk', title: msg('modals.removeGameModeDiskLabel'), disabledReason: diskSub },
      { id: 'library', title: msg('modals.removeGameModeLibraryLabel'), note: msg('modals.removeGameLibrarySub') },
    ];
  });

  const freedText = $derived.by(() => {
    if (!info) return '';
    if (info.sizeUnknown) return msg('modals.removeGameSizeUnknown');
    if (info.sizeBytes > 0) return msg('modals.removeGameFreedSize', { size: bytesSize(info.sizeBytes) });
    return msg('modals.removeGameFreedByUninstaller');
  });

  // Why the removal cannot be pressed now: the same words the dialog shows for it.
  const confirmReason = $derived.by(() => {
    if (working) return '';
    if (readFailure) return msg('modals.removeGameInspectFailed');
    if (loading || !info) return msg('modals.removeGameLoading', { title });
    if (info.running) return msg('modals.removeGameRunningWarning');
    if (info.busy) return msg('modals.removeGameBusyWarning');
    return '';
  });
</script>

<Modal dismissible={!working} bind:open title={msg('modals.removeGameTitle')} {returnFocus}>
  {#if readFailure}
    <div class="notice">
      <Notice tone="danger" compact bind:retrying={readRetrying} retry={{ label: msg('common.retry'), run: load }}>{readFailure}</Notice>
    </div>
  {:else if loading}
    <div class="loading"><Spinner size="sm" label={msg('modals.removeGameLoading', { title })} /></div>
  {:else if info}
    <OptionList
      options={modes}
      value={mode}
      onchange={(id) => (mode = id as Mode)}
      label={msg('modals.removeGameTitle')}
      activation="manual"
      disabled={working}
    />

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
          <Toggle
            bind:checked={deleteFiles}
            labelledby="{uid}-delete-files-label"
            describedby="{uid}-delete-files-sub"
            disabled={!canDeleteFiles || working}
          />
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
          <Toggle
            bind:checked={deleteDownload}
            labelledby="{uid}-remove-game-delete-download-label"
            describedby="{uid}-remove-game-delete-download-sub"
            disabled={working}
          />
        </div>
      {/if}
    </div>
  {/if}

  {#if failure}
    <div class="notice"><Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice></div>
  {/if}

  {#snippet footer()}
    <Button data-autofocus disabled={working} reason={working ? msg('modals.actionRunning') : ''} onclick={() => (open = false)}>
      {msg('common.cancel')}
    </Button>
    <Button variant="danger" onclick={confirm} disabled={!info || blocked} reason={confirmReason} busy={working}>
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
