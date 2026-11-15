<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import {
    EllipsisVertical,
    FolderOpen,
    Gamepad2,
    Play,
    Plus,
    RefreshCw,
    Search,
    Square,
  } from '@lucide/svelte';
  import AddImageModal from '../../lib/components/AddImageModal.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { inWails } from '../../lib/services/backend';
  import {
    selectGameExecutable,
    setExecutable,
    type LibraryGame,
  } from '../../lib/services/library';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import { openGameFolder, openFolder } from '../../lib/services/settings';
  import { rescan, scanProgress, scanSummary, scanning } from '../../lib/stores/discovery';
  import { installedGames, runningGames } from '../../lib/stores/library';
  import { emulationPlatform, isEmulated, isInstalledImage } from '../../lib/library/emulation';
  import { selectImageFile } from '../../lib/services/emulators';
  import { missingImages, refreshMissingImages } from '../../lib/stores/missingImages';
  import { externalLabel, isExternal, isLaunchable } from '../../lib/library/external';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { updatesByGame } from '../../lib/stores/updates';
  import { brokenStatus } from '../../lib/game/compat';
  import { compatStatuses, type CompatStatus } from '../../lib/services/compat';
  import { relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { coverFor, heroFor, libraryErrorText, play, sizeLabel, stop, updateLabel, type Sort } from './installedGame';
  import InstalledAddGame from './InstalledAddGame.svelte';
  import InstalledStorage from './InstalledStorage.svelte';
  import InstalledToolbar from './InstalledToolbar.svelte';

  let search = $state('');
  let sort = $state<Sort>('recent');

  // Журнал совместимости набирается сам из исходов запусков. Перечитываем его
  // при каждой смене состава запущенных игр: сессия только что закончилась —
  // значит вывод про игру мог измениться.
  let compat = $state<Map<string, CompatStatus>>(new Map());
  $effect(() => {
    void $runningGames;
    void $installedGames;
    compatStatuses().then((next) => {
      compat = next;
    });
  });

  function brokenNote(game: LibraryGame): string {
    const status = brokenStatus(game, compat.get(game.id));
    if (!status) return '';
    const reason = libraryErrorText(status.lastError, '');
    return reason
      ? msg('games.compatBrokenWithReason', { reason })
      : msg('games.compatBroken');
  }

  function timeOf(value: string | null) {
    if (!value) return 0;
    const parsed = Date.parse(value);
    return Number.isNaN(parsed) ? 0 : parsed;
  }

  const filteredGames = $derived.by(() => {
    const query = search.trim().toLowerCase();
    const base = query ? $installedGames.filter((game) => game.title.toLowerCase().includes(query)) : $installedGames;
    return base.toSorted((a, b) => {
      switch (sort) {
        case 'alpha':
          return a.title.localeCompare(b.title, 'ru');
        case 'size':
          return b.sizeBytes - a.sizeBytes || a.title.localeCompare(b.title, 'ru');
        default:
          return timeOf(b.lastPlayed) - timeOf(a.lastPlayed) || a.title.localeCompare(b.title, 'ru');
      }
    });
  });

  $effect(() => {
    loadArt($installedGames.map((game) => game.canonicalGameId).filter((cid): cid is string => Boolean(cid)));
  });

  function openAddDialog() {
    if (!inWails) {
      toast(msg('games.installedAddDesktopOnly'));
      return;
    }
    addGameDialog?.show();
  }

  $effect(() => {
    void $installedGames;
    void refreshMissingImages();
  });

  async function chooseImage(game: LibraryGame) {
    try {
      const path = await selectImageFile(msg('emul.addImageDialog'), [emulationPlatform(game)]);
      if (!path) return;
      await setExecutable(game.id, path);
      await refreshMissingImages();
      toast(msg('emul.imageSaved'), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.imageSaveFailed')), 'danger');
    }
  }

  let imageOpen = $state(false);

  function openImageDialog() {
    if (!inWails) {
      toast(msg('games.installedAddDesktopOnly'));
      return;
    }
    imageOpen = true;
  }

  async function openInstallDir(game: LibraryGame) {
    try {
      await openGameFolder(game.installDir, game.executable);
    } catch {
      toast(msg('games.errorFolderUnavailable'), 'danger');
    }
  }

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('disk');
  let removeTarget = $state<LibraryGame | null>(null);

  async function onMenu(game: LibraryGame, action: string) {
    if (action === 'folder') {
      await openInstallDir(game);
    } else if (action === 'executable') {
      await chooseExecutable(game);
    } else if (action === 'uninstall' || action === 'remove') {
      removeMode = action === 'uninstall' ? 'disk' : 'library';
      removeTarget = game;
      removeOpen = true;
    }
  }

  const menuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'executable', label: msg('games.installedChooseExeLabel') },
    { id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true },
  ];

  const emulatedMenuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: true },
  ];

  const imageMenuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true },
  ];

  const externalMenuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: true },
  ];

  const scanLabel = $derived(
    $scanProgress.total > 0
      ? msg('games.installedScanProgress', { processed: $scanProgress.processed, total: $scanProgress.total })
      : msg('games.installedScanning'),
  );

  async function findGames() {
    if (!inWails) {
      toast(msg('games.installedScanDesktopOnly'));
      return;
    }
    if ($scanning) return;
    try {
      const result = await rescan();
      if (result.cancelled) {
        toast(msg('games.installedScanStopped'));
        return;
      }
      toast(scanSummary(result), result.errors > 0 ? 'danger' : 'success');
    } catch (err) {
      toast(sourceErrorText(err, msg('games.installedScanError')), 'danger');
    }
  }

  async function chooseExecutable(game: LibraryGame) {
    try {
      const path = await selectGameExecutable(
        msg('games.installedChooseExeDialog', { title: game.title }),
        game.installDir,
        game.executable,
      );
      if (!path) return;
      await setExecutable(game.id, path);
      toast(msg('games.installedExeSavedToast', { title: game.title }), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('games.installedChooseExeError')), 'danger');
    }
  }

  function statusKind(game: LibraryGame, running: boolean, updateKind?: string): 'success' | 'warning' | 'accent' | 'neutral' {
    if (running) return 'accent';
    if ($missingImages.has(game.id)) return 'warning';
    if (!isLaunchable(game)) return 'warning';
    if (updateKind === 'update') return 'warning';
    if (updateKind === 'new_release') return 'neutral';
    return 'success';
  }

  function statusLabel(game: LibraryGame, running: boolean) {
    if (running) return msg('games.runningLabel');
    if ($missingImages.has(game.id)) return msg('emul.imageMissing');
    if (!isLaunchable(game)) return msg('games.installedNeedsExeLabel');
    return relativeDate(game.lastPlayed);
  }

  let librarySetupOpen = $state(false);

  let addGameDialog = $state<InstalledAddGame | undefined>(undefined);

  const uid = $props.id();
</script>

<Page>
  <PageHeader
    title={msg('games.installedStatusWord')}
    subtitle={$installedGames.length > 0
      ? msg('installed.youHave', { count: $installedGames.length })
      : msg('games.installedEmptySubtitle')}
  >
    {#snippet actions()}
      <InstalledToolbar bind:search bind:sort {scanLabel} onfind={findGames} onaddimage={openImageDialog} onaddgame={openAddDialog} />
    {/snippet}
  </PageHeader>

  <InstalledStorage onchoosefolder={() => (librarySetupOpen = true)} />

  {#if $installedGames.length === 0}
    <EmptyState level={2}
      title={msg('games.installedNoGamesTitle')}
      description={msg('games.installedNoGamesDescription')}
    >
      {#snippet icon()}
        <Gamepad2 size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={findGames} busy={$scanning}>
          {#if !$scanning}<RefreshCw size="1.5rem" strokeWidth={2} />{/if}
          {$scanning ? scanLabel : msg('games.installedFindGames')}
        </Button>
        <Button onclick={openAddDialog}>
          <Plus size="1.5rem" strokeWidth={2} />
          {msg('games.installedAddGameButton')}
        </Button>
        <Button onclick={openImageDialog}>
          <Plus size="1.5rem" strokeWidth={2} />
          {msg('emul.addButton')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else if filteredGames.length === 0}
    <EmptyState level={2} title={msg('games.nothingFoundTitle')} description={msg('games.installedNothingFoundDescription')}>
      {#snippet icon()}
        <Search size="2rem" strokeWidth={1.8} />
      {/snippet}
    </EmptyState>
  {:else if $installedView === 'list'}
    <div class="list">
      {#each filteredGames as game (game.id)}
        {@const running = $runningGames.has(game.id)}
        {@const update = $updatesByGame.get(game.id)?.availability}
        {@const updateText = updateLabel(update)}
        <div class="row" role="presentation" {@attach gameContextMenu(game.id)}>
          <button class="game" onclick={() => navigate('game', { id: game.id })}>
            <div class="thumb" aria-hidden="true">
              <Artwork src={heroFor(game, $gameArt)} alt="" label={game.title} radius="var(--radius-sm)" />
            </div>
            <div class="titles">
              <span class="title-line">
                <span class="title">{game.title}</span>
                {#if isEmulated(game)}<ConsoleTag platform={emulationPlatform(game)} />{/if}
              </span>
              <span class="path">{game.installDir}</span>
              {#if sizeLabel(game) || updateText}
                <span class="size">
                  {sizeLabel(game)}
                  {#if updateText}
                    <span class="row-update">
                      <StatusBadge kind={update?.kind === 'update' ? 'warning' : 'neutral'} label={updateText} plain />
                    </span>
                  {/if}
                </span>
              {/if}
            </div>
          </button>
          <div class="status">
            <span class="status-label">{msg('games.lastPlayedLabel')}</span>
            <StatusBadge kind={statusKind(game, running, update?.kind)} label={statusLabel(game, running)} plain />
            {#if brokenNote(game)}
              <span class="compat">
                <StatusBadge kind="danger" label={msg('games.compatBrokenBadge')} hint={brokenNote(game)} plain />
              </span>
            {/if}
          </div>
          <div class="actions">
            {#if running}
              <Button size="sm" onclick={() => stop(game)}>
                <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
                {msg('games.installedStopButton')}
              </Button>
            {:else if $missingImages.has(game.id)}
              <Button size="sm" onclick={() => chooseImage(game)}>
                <FolderOpen size="1.3rem" strokeWidth={1.8} />
                {msg('emul.pickImage')}
              </Button>
            {:else if !isLaunchable(game)}
              <Button size="sm" onclick={() => chooseExecutable(game)}>
                <FolderOpen size="1.3rem" strokeWidth={1.8} />
                {msg('games.installedSetExeButton')}
              </Button>
            {:else}
              <Button variant="primary" size="sm" onclick={() => play(game)}>
                <Gamepad2 size="1.3rem" strokeWidth={2} />
                {msg('games.play')}
              </Button>
            {/if}
            <Button size="sm" onclick={() => openInstallDir(game)}>
              <FolderOpen size="1.3rem" strokeWidth={1.8} />
              {msg('games.openFolder')}
            </Button>
            <DropdownMenu items={isExternal(game) ? externalMenuItems : isInstalledImage(game) ? imageMenuItems : isEmulated(game) ? emulatedMenuItems : menuItems} onselect={(id) => onMenu(game, id)}>
              {#snippet trigger({ props })}
                <IconButton {...props} label={msg('games.installedMenuFor', { title: game.title })} size="sm">
                  <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <div class="grid">
      {#each filteredGames as game (game.id)}
        {@const running = $runningGames.has(game.id)}
        {@const update = $updatesByGame.get(game.id)?.availability}
        {@const updateText = updateLabel(update)}
        <div class="card" role="presentation" {@attach gameContextMenu(game.id)}>
          <div class="card-media">
            <button
              class="card-cover"
              onclick={() => navigate('game', { id: game.id })}
              aria-label={updateText ? `${game.title} — ${updateText}` : game.title}
            >
              <Artwork src={coverFor(game, $gameArt)} alt="" label={game.title} ratio="3 / 4" radius="var(--radius-md)" />
              {#if updateText}
                <span class="card-badge">
                  <StatusBadge kind={update?.kind === 'update' ? 'warning' : 'neutral'} label={updateText} />
                </span>
              {/if}
            </button>
            {#if running}
              <span class="card-play running">
                <IconButton variant="primary" size="sm" label={msg('games.installedStopButton')} aria-describedby="{uid}-{game.id}-title" onclick={() => stop(game)}>
                  <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
                </IconButton>
              </span>
            {:else if isLaunchable(game) && !$missingImages.has(game.id)}
              <span class="card-play">
                <IconButton variant="primary" size="sm" label={msg('games.play')} aria-describedby="{uid}-{game.id}-title" onclick={() => play(game)}>
                  <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
                </IconButton>
              </span>
            {/if}
          </div>
          <div class="card-info">
            <span class="card-title" id="{uid}-{game.id}-title">{game.title}</span>
            <span class="card-meta">
              {#if isEmulated(game)}<ConsoleTag platform={emulationPlatform(game)} />{/if}
              {sizeLabel(game)}
              {#if $missingImages.has(game.id)}<StatusBadge kind="danger" label={msg('emul.imageMissing')} plain />{/if}
            </span>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  {#if $installedGames.length > 0}
    <p class="count">
      {msg('installed.shownOf', { shown: filteredGames.length, count: $installedGames.length })}
    </p>
  {/if}

  <LibrarySetupModal bind:open={librarySetupOpen} />

  <AddImageModal bind:open={imageOpen} />

  {#if removeTarget}
    <RemoveGameModal
      bind:open={removeOpen}
      bind:mode={removeMode}
      gameId={removeTarget.id}
      title={removeTarget.title}
      external={externalLabel(removeTarget)}
      emulated={isEmulated(removeTarget)}
    />
  {/if}

  <InstalledAddGame bind:this={addGameDialog} {uid} />
</Page>

<style>

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .row {
    display: grid;
    grid-template-columns: minmax(30rem, 1fr) 17rem auto;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .game {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-width: 0;
    text-align: left;
  }

  .thumb {
    width: 10.4rem;
    height: 5.8rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .titles {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .title-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .path {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .size {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    min-width: 0;
  }

  .status-label {
    font-size: 1.2rem;
    color: var(--text-3);
    white-space: nowrap;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    justify-self: end;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    min-width: 0;
  }

  .card-media {
    position: relative;
    border-radius: var(--radius-md);
    transition: transform var(--dur-panel) var(--ease);
  }

  .card-media::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    top: 100%;
    height: 0.4rem;
  }

  .card-media:hover,
  .card-media:focus-within {
    transform: translateY(-0.4rem);
  }

  .card-cover {
    position: relative;
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: box-shadow var(--dur-panel) var(--ease);
  }

  .card-cover :global(img) {
    transition: transform var(--dur-slow) var(--ease);
  }

  .card-media:hover .card-cover {
    box-shadow: var(--shadow-lift);
  }

  .card-media:hover .card-cover :global(img) {
    transform: scale(1.04);
  }

  .card-badge {
    position: absolute;
    top: 0.6rem;
    left: 0.6rem;
    display: flex;
    max-width: calc(100% - 1.2rem);
    overflow: hidden;
    transform-origin: left top;
    animation: badge-pop var(--dur-slow) var(--ease-spring) backwards;
  }

  @keyframes badge-pop {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }

  .card-play {
    position: absolute;
    left: 1rem;
    bottom: 1rem;
    display: flex;
    opacity: 0;
    transform: translateY(0.4rem) scale(0.9);
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur-panel) var(--ease-spring);
  }

  .card-media:hover .card-play,
  .card-play:focus-within,
  .card-play.running {
    opacity: 1;
    transform: translateY(0) scale(1);
  }

  .card-info {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .card-title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .row-update {
    display: inline-flex;
    margin-left: 0.8rem;
    vertical-align: bottom;
  }

  .count {
    margin-top: var(--space-5);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  @media (min-width: 2200px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    }
  }

  @media (max-width: 1400px) {
    .row {
      grid-template-columns: minmax(22rem, 1fr) auto;
    }

    .status {
      display: none;
    }

    .grid {
      grid-template-columns: repeat(auto-fill, minmax(14.5rem, 1fr));
    }
  }
</style>
