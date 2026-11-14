<script lang="ts">
  import {
    ArrowUp,
    BookmarkPlus,
    Bell,
    BellRing,
    Download,
    EllipsisVertical,
    HardDriveDownload,
    Heart,
    Play,
    SlidersHorizontal,
    Sparkles,
    Square,
    Tag,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import type UpdateCard from '../../lib/components/UpdateCard.svelte';
  import LeadAction from './LeadAction.svelte';
  import { libraryErrorText, type GamePageData } from './gamePageData.svelte';
  import { playLabel } from '../../lib/game/profiles';
  import { statusLabel } from '../../lib/game/status';
  import type { BusyState, PrimaryAction, TerminalDownloadStatus } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import { isInstalledImage } from '../../lib/library/emulation';
  import type { Download as DownloadItem } from '../../lib/services/downloads';
  import { playGame, playGameProfile, stopGame, type LibraryGame } from '../../lib/services/library';
  import { openGameProperties } from '../../lib/stores/gameProperties';
  import { metadataAvailable } from '../../lib/stores/metadata';
  import { saveBackupsEnabled } from '../../lib/stores/savebackup';
  import { toast } from '../../lib/stores/toasts';
  import { toggleWatch, watchedIds } from '../../lib/stores/watchlist';
  import { progressPercent } from '../../lib/utils/format';

  /**
   * The game page's action row: the main action, the library, watch, favourite, properties, status and
   * tags controls and the ⋯ menu, then the note on a failed download or an uninstalled game.
   */
  let {
    data,
    primary,
    busy,
    title,
    localGame,
    canonicalId,
    installed,
    wasInstalled,
    external,
    managed,
    emulated,
    updateIsRelease,
    terminalDownload,
    updateCard,
    otherVersionsEl,
    statusOpen = $bindable(),
    tagsOpen = $bindable(),
    onMenu,
    startDownload,
    retryTerminalDownload,
    installFromTerminalDownload,
  }: {
    data: GamePageData;
    primary: PrimaryAction;
    busy: BusyState | null;
    title: string;
    localGame: LibraryGame | undefined;
    canonicalId: string | undefined;
    installed: boolean;
    wasInstalled: boolean;
    external: boolean;
    managed: boolean;
    emulated: boolean;
    updateIsRelease: boolean;
    terminalDownload: (DownloadItem & { status: TerminalDownloadStatus }) | undefined;
    updateCard: UpdateCard | undefined;
    otherVersionsEl: HTMLElement | null;
    statusOpen: boolean;
    tagsOpen: boolean;
    onMenu: (actionId: string) => void;
    startDownload: () => void;
    retryTerminalDownload: () => void;
    installFromTerminalDownload: () => void;
  } = $props();

  const busyPercent = $derived(progressPercent(busy?.progress ?? 0));

  const menuItems = $derived([
    ...($saveBackupsEnabled && localGame && !emulated ? [{ id: 'saves-backups', label: msg('games.detailSavesRollback') }] : []),
    ...($metadataAvailable && canonicalId
      ? data.metaView?.resolved
        ? [
            {
              id: 'meta-refresh',
              label: data.metaRefreshing ? msg('games.detailMetaRefreshingLabel') : msg('games.detailMetaRefreshLabel'),
            },
            { id: 'meta-change', label: msg('games.detailMetaChangeLabel') },
          ]
        : [{ id: 'meta-find', label: msg('games.detailMetaFindLabel') }]
      : []),
    ...(installed && (localGame?.shortcutPath || !external || localGame?.executable)
      ? [
          localGame?.shortcutPath
            ? { id: 'shortcut-remove', label: msg('games.actionShortcutRemove') }
            : { id: 'shortcut-create', label: msg('games.actionShortcutCreate') },
        ]
      : []),
    ...(installed && (managed || isInstalledImage(localGame))
      ? [{ id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true }]
      : []),
    ...(localGame
      ? [{ id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: !installed || !(managed || isInstalledImage(localGame)) }]
      : []),
    ...(terminalDownload
      ? [
          { id: 'remove-download', label: msg('games.detailRemoveDownloadLabel'), danger: true, separator: true },
          { id: 'discard-download', label: msg('games.detailDiscardDownloadLabel'), danger: true },
        ]
      : []),
  ]);

  async function play() {
    if (!localGame) return;
    try {
      await playGame(localGame.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    }
  }

  async function playProfile(profileID: string) {
    if (!localGame) return;
    try {
      await playGameProfile(localGame.id, profileID);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    }
  }

  async function stop() {
    if (!localGame) return;
    try {
      await stopGame(localGame.id);
    } catch {
      toast(msg('games.errorStopFailed'), 'danger');
    }
  }

  function runPrimary() {
    if (primary.kind === 'play') play();
    else if (primary.kind === 'stop') stop();
    else if (primary.kind === 'download') startDownload();
    else if (primary.kind === 'other-versions') otherVersionsEl?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    else if (primary.kind === 'update') updateCard?.start();
    else if (primary.kind === 'retry-download') retryTerminalDownload();
    else if (primary.kind === 'install-download') installFromTerminalDownload();
  }
</script>

<div class="cta" data-part="game-actions">
  {#if primary.kind === 'progress'}
    <div class="progress">
      <span class="progress-label">{primary.label}</span>
      <ProgressBar
        value={busyPercent}
        indeterminate={busy?.indeterminate}
        stretch
        label={title}
        valueText={`${primary.label}, ${busyPercent}%`}
      />
      {#if !busy?.indeterminate}<span class="progress-pct">{busyPercent}%</span>{/if}
    </div>
  {:else if primary.kind === 'update'}
    <LeadAction kind="play" game={localGame} onclick={play} onpick={playProfile}>
      <Play size="1.8rem" strokeWidth={2} fill="currentColor" />
      {localGame ? playLabel(localGame) : msg('games.play')}
    </LeadAction>
    <Button size="lg" disabled={primary.disabled} onclick={runPrimary}>
      {#if updateIsRelease}
        <Sparkles size="1.5rem" strokeWidth={1.8} />
      {:else}
        <ArrowUp size="1.5rem" strokeWidth={2} />
      {/if}
      {primary.label}
    </Button>
  {:else}
    <LeadAction
      kind={primary.kind}
      game={localGame}
      disabled={primary.disabled}
      onclick={runPrimary}
      onpick={playProfile}
    >
      {#if primary.kind === 'play'}
        <Play size="1.8rem" strokeWidth={2} fill="currentColor" />
      {:else if primary.kind === 'stop'}
        <Square size="1.4rem" strokeWidth={2} fill="currentColor" />
      {:else if primary.kind === 'download'}
        <Download size="1.6rem" strokeWidth={1.8} />
      {:else if primary.kind === 'install-download'}
        <HardDriveDownload size="1.6rem" strokeWidth={1.8} />
      {/if}
      {primary.kind === 'play' && localGame ? playLabel(localGame) : primary.label}
    </LeadAction>
  {/if}

  {#if !localGame && canonicalId}
    <Button size="lg" disabled={data.addingToLibrary} onclick={data.addToLibrary}>
      <BookmarkPlus size="1.5rem" strokeWidth={1.8} />
      {msg('games.detailAddToLibraryButton')}
    </Button>
  {/if}

  {#if !installed && canonicalId}
    {@const watching = $watchedIds.has(canonicalId)}
    <Button size="lg" onclick={() => toggleWatch(canonicalId)}>
      {#if watching}
        <BellRing size="1.5rem" strokeWidth={1.8} />
        {msg('games.watchingButton')}
      {:else}
        <Bell size="1.5rem" strokeWidth={1.8} />
        {msg('games.watchButton')}
      {/if}
    </Button>
  {/if}

  {#if localGame}
    <IconButton
      size="lg"
      variant="secondary"
      label={localGame.favorite ? msg('games.actionFavoriteRemove') : msg('games.actionFavoriteAdd')}
      lit={Boolean(localGame.favorite)}
      onclick={data.toggleFavorite}
    >
      <span class="heart" class:pulse={data.favoritePulse} onanimationend={() => (data.favoritePulse = false)}>
        <Heart size="1.8rem" strokeWidth={1.8} fill={localGame.favorite ? 'currentColor' : 'none'} />
      </span>
    </IconButton>

    {#if installed}
      <IconButton size="lg" variant="secondary" label={msg('games.actionProperties')} onclick={() => openGameProperties(localGame.id)}>
        <SlidersHorizontal size="1.8rem" strokeWidth={1.8} />
      </IconButton>
    {/if}

    <Button size="lg" onclick={() => (statusOpen = true)}>
      {msg('games.actionStatus', { status: statusLabel(localGame.status) })}
    </Button>

    <IconButton
      size="lg"
      variant="secondary"
      label={localGame.tags?.length ? msg('games.tagsEditLabel') : msg('games.tagsAddLabel')}
      onclick={() => (tagsOpen = true)}
    >
      <Tag size="1.8rem" strokeWidth={1.8} />
    </IconButton>
  {/if}

  {#if menuItems.length > 0}
    <DropdownMenu items={menuItems} onselect={onMenu}>
      {#snippet trigger({ props })}
        <IconButton {...props} size="lg" variant="secondary" label={msg('games.moreLabel')}>
          <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
        </IconButton>
      {/snippet}
    </DropdownMenu>
  {/if}
</div>

{#if primary.kind === 'retry-download' && terminalDownload?.error}
  <div class="note-plate"><Notice tone="danger" announce={false}>{installErrorText(terminalDownload.error)}</Notice></div>
{:else if localGame && !installed && wasInstalled}
  <div class="note-plate">
    <Notice tone="info" announce={false}>
      {primary.kind === 'install-download'
        ? msg('games.detailUninstalledDownloadedNote')
        : msg('games.detailUninstalledNote')}
    </Notice>
  </div>
{/if}

<style>
  .cta {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
    margin-top: var(--space-5);
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .heart {
    display: inline-flex;
  }

  .heart.pulse {
    animation: pop-in var(--dur-slow) var(--ease-spring);
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: min(42rem, 100%);
    height: var(--control-lg);
    padding: 0 1.8rem;
    border-radius: var(--radius-md);
    background: var(--surface-3);
  }

  .progress-label {
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
  }

  .progress-pct {
    font-size: var(--font-xs);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .note-plate {
    margin-top: var(--space-4);
    max-width: 56rem;
  }
</style>
