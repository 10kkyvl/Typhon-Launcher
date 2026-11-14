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
  import { untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import type UpdateCard from '../../lib/components/UpdateCard.svelte';
  import LeadAction from './LeadAction.svelte';
  import { libraryErrorText, type GamePageData } from './gamePageData.svelte';
  import { playLabel } from '../../lib/game/profiles';
  import { statusLabel } from '../../lib/game/status';
  import type { BusyState, PrimaryAction, PrimaryKind, TerminalDownloadStatus } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import { isInstalledImage } from '../../lib/library/emulation';
  import type { Download as DownloadItem } from '../../lib/services/downloads';
  import { launchesInFlight, playGame, playGameProfile, stopGame, type LibraryGame } from '../../lib/services/library';
  import { openGameProperties } from '../../lib/stores/gameProperties';
  import { metadataAvailable } from '../../lib/stores/metadata';
  import { saveBackupsEnabled } from '../../lib/stores/savebackup';
  import { toast } from '../../lib/stores/toasts';
  import { toggleWatch, watchedIds } from '../../lib/stores/watchlist';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { progressPercent } from '../../lib/utils/format';

  /**
   * The game page's action row: the main action, the library, watch, favourite, properties, status and
   * tags controls and the ⋯ menu, then the note on a failed download or an uninstalled game.
   */
  let {
    data,
    primary,
    busy,
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

  // A launch or a stop this page sent and Go has not answered yet. A launch another screen sent counts
  // too (the service's set): the button is busy either way and a second press sends nothing.
  let pending = $state<'launch' | 'stop' | null>(null);
  const launching = $derived(pending === 'launch' || Boolean(localGame && $launchesInFlight.has(localGame.id)));
  const launchKinds: readonly string[] = ['play', 'update', 'stop'];

  interface Lead {
    /** The action a press runs; with Update offered beside it, the main button is Play. */
    kind: PrimaryKind;
    label: string;
    /** What the live region says of it: the label without a percentage that keeps moving. */
    said: string;
    busy: boolean;
    progress: { value: number; indeterminate?: boolean } | null;
  }

  /** What the main button shows: the page's action, or the launch or stop in flight over it. */
  const lead = $derived.by((): Lead => {
    const kind = primary.kind === 'update' ? 'play' : primary.kind;
    const plain = (label: string, busy = false): Lead => ({ kind, label, said: label, busy, progress: null });
    if (launchKinds.includes(primary.kind) && launching) return plain(msg('games.mainActionLaunching'), true);
    if (primary.kind === 'stop' && pending === 'stop') return plain(msg('games.mainActionStopping'), true);
    if (primary.kind === 'progress') {
      const label = busy?.indeterminate
        ? primary.label
        : msg('games.mainActionProgress', { label: primary.label, percent: busyPercent });
      return { kind, label, said: primary.label, busy: true, progress: { value: busyPercent, indeterminate: busy?.indeterminate } };
    }
    if (primary.kind === 'resolving') return plain(primary.label, true);
    if (kind === 'play') return plain(localGame ? playLabel(localGame) : msg('games.play'));
    return plain(primary.label);
  });

  // Said once in the live region when the main button changes what it says; a percentage moving is
  // not a change. Nothing is said for the state the page opens in.
  const said = $derived(lead.said);
  let announcement = $state('');
  let opened = false;
  $effect(() => {
    const text = said;
    untrack(() => {
      if (opened) announcement = text;
      opened = true;
    });
  });

  let leadButton = $state<HTMLButtonElement>();
  let row = $state<HTMLDivElement>();
  const showAdd = $derived(!localGame && Boolean(canonicalId));
  const showUpdate = $derived(primary.kind === 'update');

  // A control of the row that leaves the page while it holds focus (Add to library once the game is in
  // the library, Update once the update runs) hands focus to the main button. Runs before it leaves.
  $effect.pre(() => {
    void showAdd;
    void showUpdate;
    untrack(() => {
      const active = document.activeElement;
      if (!row || !leadButton || !active || active === leadButton || !row.contains(active)) return;
      // A main button with nothing to offer is disabled and takes no focus: the next control does.
      void focusHandOff()(() => (leadButton?.disabled ? row?.querySelector<HTMLElement>('button:not(:disabled)') : leadButton));
    });
  });

  async function play() {
    if (!localGame || launching) return;
    pending = 'launch';
    try {
      await playGame(localGame.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    } finally {
      pending = null;
    }
  }

  async function playProfile(profileID: string) {
    if (!localGame || launching) return;
    pending = 'launch';
    try {
      await playGameProfile(localGame.id, profileID);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    } finally {
      pending = null;
    }
  }

  async function stop() {
    if (!localGame || pending) return;
    pending = 'stop';
    try {
      await stopGame(localGame.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorStopFailed')), 'danger');
    } finally {
      pending = null;
    }
  }

  // The second click of a double click is never an action of its own: the click that started a launch
  // can land again after the game is already running, and must not be read as Stop.
  function pressLead(event: MouseEvent) {
    if (event.detail > 1 || lead.busy) return;
    if (lead.kind === 'play') play();
    else if (lead.kind === 'stop') stop();
    else if (lead.kind === 'download') startDownload();
    else if (lead.kind === 'other-versions') otherVersionsEl?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    else if (lead.kind === 'retry-download') retryTerminalDownload();
    else if (lead.kind === 'install-download') installFromTerminalDownload();
  }

  function startUpdate() {
    updateCard?.start();
  }
</script>

<div class="cta" data-part="game-actions" bind:this={row}>
  <LeadAction
    kind={lead.kind}
    game={localGame}
    disabled={primary.disabled && !lead.busy}
    busy={lead.busy}
    progress={lead.progress}
    bind:element={leadButton}
    onclick={pressLead}
    onpick={playProfile}
  >
    {#if !lead.busy}
      {#if lead.kind === 'play'}
        <Play size="1.8rem" strokeWidth={2} fill="currentColor" />
      {:else if lead.kind === 'stop'}
        <Square size="1.4rem" strokeWidth={2} fill="currentColor" />
      {:else if lead.kind === 'download'}
        <Download size="1.6rem" strokeWidth={1.8} />
      {:else if lead.kind === 'install-download'}
        <HardDriveDownload size="1.6rem" strokeWidth={1.8} />
      {/if}
    {/if}
    {lead.label}
  </LeadAction>
  <span class="sr-only" role="status">{announcement}</span>

  {#if showUpdate}
    <Button size="lg" onclick={startUpdate}>
      {#if updateIsRelease}
        <Sparkles size="1.5rem" strokeWidth={1.8} />
      {:else}
        <ArrowUp size="1.5rem" strokeWidth={2} />
      {/if}
      {primary.label}
    </Button>
  {/if}

  {#if showAdd && canonicalId}
    <Button size="lg" busy={data.addingToLibrary} onclick={data.addToLibrary}>
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

  .note-plate {
    margin-top: var(--space-4);
    max-width: 56rem;
  }
</style>
