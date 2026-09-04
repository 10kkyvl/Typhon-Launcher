<script lang="ts">
  import {
    ChevronDown,
    EllipsisVertical,
    FolderOpen,
    Gamepad2,
    HardDrive,
    LayoutGrid,
    List,
    Plus,
    RefreshCw,
    Search,
    Square,
  } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { inWails } from '../../lib/services/backend';
  import {
    addGame,
    playGame,
    selectExecutable,
    setExecutable,
    stopGame,
    type LibraryGame,
  } from '../../lib/services/library';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import { openFolder } from '../../lib/services/settings';
  import { rescan, scanProgress, scanSummary, scanning } from '../../lib/stores/discovery';
  import { installedGames, runningGames } from '../../lib/stores/library';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { openGameMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { storageInfo } from '../../lib/stores/storage';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { updatesByGame } from '../../lib/stores/updates';
  import { bytesSize, relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  type Sort = 'recent' | 'alpha' | 'size';

  const sortLabels: Record<Sort, string> = {
    recent: msg('games.recentLabel'),
    alpha: msg('games.sortAlpha'),
    size: msg('games.sortSize'),
  };

  let search = $state('');
  let sort = $state<Sort>('recent');

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

  function sizeLabel(game: LibraryGame) {
    return game.sizeBytes > 0 ? bytesSize(game.sizeBytes) : '';
  }

  const gamesBytes = $derived($installedGames.reduce((sum, game) => sum + game.sizeBytes, 0));
  const hasUnknownSize = $derived($installedGames.some((game) => game.sizeUnknown));
  const usedPct = $derived($storageInfo ? ($storageInfo.usedBytes / $storageInfo.totalBytes) * 100 : 0);
  const otherBytes = $derived.by(() => {
    if (!$storageInfo || hasUnknownSize) return null;
    const value = $storageInfo.usedBytes - gamesBytes;
    return value >= 0 ? value : null;
  });

  function coverFor(game: LibraryGame) {
    return (game.canonicalGameId && $gameArt[game.canonicalGameId]?.cover) || game.cover;
  }

  function heroFor(game: LibraryGame) {
    return (game.canonicalGameId && $gameArt[game.canonicalGameId]?.hero) || coverFor(game);
  }

  $effect(() => {
    loadArt($installedGames.map((game) => game.canonicalGameId).filter((cid): cid is string => Boolean(cid)));
  });

  let addOpen = $state(false);
  let newExecutable = $state('');
  let newTitle = $state('');
  let adding = $state(false);

  function titleFromPath(path: string) {
    const base = path.split(/[\\/]/).pop() ?? '';
    return base.replace(/\.exe$/i, '').replace(/[_.-]+/g, ' ').trim();
  }

  function openAddDialog() {
    if (!inWails) {
      toast(msg('games.installedAddDesktopOnly'));
      return;
    }
    newExecutable = '';
    newTitle = '';
    addOpen = true;
  }

  async function browseExecutable() {
    try {
      const path = await selectExecutable(msg('games.installedSelectExeDialog'));
      if (path) {
        newExecutable = path;
        if (!newTitle.trim()) newTitle = titleFromPath(path);
      }
    } catch {
      toast(msg('games.installedOpenDialogError'), 'danger');
    }
  }

  async function submitAdd() {
    if (!newExecutable.trim()) return;
    adding = true;
    try {
      const game = await addGame(newExecutable.trim(), newTitle.trim());
      toast(msg('games.installedGameAddedToast', { title: game.title }), 'success');
      addOpen = false;
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : msg('games.installedAddGameError'), 'danger');
    } finally {
      adding = false;
    }
  }

  async function play(game: LibraryGame) {
    try {
      await playGame(game.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : msg('games.errorPlayFailed'), 'danger');
    }
  }

  async function stop(game: LibraryGame) {
    try {
      await stopGame(game.id);
    } catch {
      toast(msg('games.errorStopFailed'), 'danger');
    }
  }

  async function openInstallDir(game: LibraryGame) {
    try {
      await openFolder(game.installDir);
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
      toast(err instanceof Error && err.message ? err.message : msg('games.installedScanError'), 'danger');
    }
  }

  async function chooseExecutable(game: LibraryGame) {
    try {
      const path = await selectExecutable(msg('games.installedChooseExeDialog', { title: game.title }));
      if (!path) return;
      await setExecutable(game.id, path);
      toast(msg('games.installedExeSavedToast', { title: game.title }), 'success');
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : msg('games.installedChooseExeError'), 'danger');
    }
  }

  function statusKind(game: LibraryGame, running: boolean, updateKind?: string): 'success' | 'warning' | 'accent' | 'neutral' {
    if (running) return 'accent';
    if (!game.executable) return 'warning';
    if (updateKind === 'update') return 'warning';
    if (updateKind === 'new_release') return 'neutral';
    return 'success';
  }

  function statusLabel(game: LibraryGame, running: boolean) {
    if (running) return msg('games.runningLabel');
    if (!game.executable) return msg('games.installedNeedsExeLabel');
    return relativeDate(game.lastPlayed);
  }

  let librarySetupOpen = $state(false);
</script>

<PageHeader
  title={msg('games.installedStatusWord')}
  subtitle={$installedGames.length > 0
    ? msg('installed.youHave', { count: $installedGames.length })
    : msg('games.installedEmptySubtitle')}
>
  {#snippet actions()}
    <div class="search-wrap">
      <SearchInput bind:value={search} placeholder={msg('games.installedSearchPlaceholder')} />
    </div>
    <DropdownMenu
      items={[
        { id: 'recent', label: sortLabels.recent },
        { id: 'alpha', label: sortLabels.alpha },
        { id: 'size', label: sortLabels.size },
      ]}
      onselect={(id) => (sort = id as Sort)}
    >
      {#snippet trigger({ open, toggle })}
        <Chip selected={open} onclick={toggle}>
          {msg('games.installedSortByLabel', { sort: sortLabels[sort] })}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </Chip>
      {/snippet}
    </DropdownMenu>
    <SegmentedControl
      bind:value={$installedView}
      options={[
        { id: 'list', label: msg('games.viewList') },
        { id: 'grid', label: msg('games.viewGrid') },
      ]}
    >
      {#snippet item(option)}
        {#if option.id === 'list'}
          <List size="1.6rem" strokeWidth={1.8} />
        {:else}
          <LayoutGrid size="1.6rem" strokeWidth={1.8} />
        {/if}
      {/snippet}
    </SegmentedControl>
    <Button onclick={findGames} disabled={$scanning}>
      <RefreshCw size="1.5rem" strokeWidth={2} class={$scanning ? 'spin' : ''} />
      {$scanning ? scanLabel : msg('games.installedFindGames')}
    </Button>
    <Button variant="primary" onclick={openAddDialog}>
      <Plus size="1.5rem" strokeWidth={2} />
      {msg('games.installedAddGameButton')}
    </Button>
  {/snippet}
</PageHeader>

{#if !$settings?.libraryPath}
  <div class="storage-block">
    <Card>
      <div class="storage-empty">
        <div class="disk">
          <HardDrive size="1.8rem" strokeWidth={1.8} />
          <div class="disk-text">
            <span class="disk-name">{msg('games.installedLibraryNotSetTitle')}</span>
            <span class="disk-meta">{msg('games.installedLibraryNotSetHint')}</span>
          </div>
        </div>
        <Button size="sm" variant="primary" onclick={() => (librarySetupOpen = true)}>
          <FolderOpen size="1.5rem" strokeWidth={1.8} />
          {msg('games.installedChooseFolderButton')}
        </Button>
      </div>
    </Card>
  </div>
{:else if $storageInfo}
  <div class="storage-block">
    <Card>
      <div class="storage">
        <div class="storage-primary">
          <div class="storage-head">
            <span class="disk-icon">
              <HardDrive size="1.8rem" strokeWidth={1.8} />
            </span>
            <div class="disk-text">
              <span class="disk-name">{msg('games.installedStorageTitle')}</span>
              <span class="disk-meta">
                {msg('games.installedStorageUsage', {
                  used: bytesSize($storageInfo.usedBytes),
                  total: bytesSize($storageInfo.totalBytes),
                })}
              </span>
            </div>
          </div>
          <div class="storage-bar">
            <div class="storage-bar-track">
              <ProgressBar value={usedPct} height={6} />
            </div>
            <span class="storage-pct">{Math.round(usedPct)}%</span>
          </div>
        </div>
        <ul class="storage-legend">
          <li>
            <span class="dot games"></span>
            <span class="legend-label">{msg('games.installedLegendGames')}</span>
            <span class="legend-value">{bytesSize(gamesBytes)}</span>
          </li>
          {#if otherBytes !== null}
            <li>
              <span class="dot other"></span>
              <span class="legend-label">{msg('games.installedLegendOther')}</span>
              <span class="legend-value">{bytesSize(otherBytes)}</span>
            </li>
          {/if}
          <li>
            <span class="dot free"></span>
            <span class="legend-label">{msg('games.installedLegendFree')}</span>
            <span class="legend-value">{bytesSize($storageInfo.freeBytes)}</span>
          </li>
        </ul>
        <Button onclick={() => navigate('settings', { tab: 'general' })}>{msg('games.installedManageStorage')}</Button>
      </div>
    </Card>
  </div>
{/if}

{#if $installedGames.length === 0}
  <EmptyState
    title={msg('games.installedNoGamesTitle')}
    description={msg('games.installedNoGamesDescription')}
  >
    {#snippet icon()}
      <Gamepad2 size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={findGames} disabled={$scanning}>
        <RefreshCw size="1.5rem" strokeWidth={2} class={$scanning ? 'spin' : ''} />
        {$scanning ? scanLabel : msg('games.installedFindGames')}
      </Button>
      <Button onclick={openAddDialog}>
        <Plus size="1.5rem" strokeWidth={2} />
        {msg('games.installedAddGameButton')}
      </Button>
    {/snippet}
  </EmptyState>
{:else if filteredGames.length === 0}
  <EmptyState title={msg('games.nothingFoundTitle')} description={msg('games.installedNothingFoundDescription')}>
    {#snippet icon()}
      <Search size="2rem" strokeWidth={1.8} />
    {/snippet}
  </EmptyState>
{:else if $installedView === 'list'}
  <div class="list">
    {#each filteredGames as game (game.id)}
      {@const running = $runningGames.has(game.id)}
      {@const update = $updatesByGame.get(game.id)?.availability}
      <div class="row" role="presentation" oncontextmenu={(event) => openGameMenu(event, game.id)}>
        <button class="game" onclick={() => navigate('game', { id: game.id })}>
          <div class="thumb">
            <Artwork src={heroFor(game)} alt={game.title} radius="var(--radius-sm)" />
          </div>
          <div class="titles">
            <span class="title">{game.title}</span>
            <span class="path">{game.installDir}</span>
            {#if sizeLabel(game)}<span class="size">{sizeLabel(game)}</span>{/if}
          </div>
        </button>
        <div class="status">
          <span class="status-label">{msg('games.lastPlayedLabel')}</span>
          <StatusBadge kind={statusKind(game, running, update?.kind)} label={statusLabel(game, running)} plain />
        </div>
        <div class="actions">
          {#if running}
            <Button size="sm" onclick={() => stop(game)}>
              <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
              {msg('games.installedStopButton')}
            </Button>
          {:else if !game.executable}
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
          <DropdownMenu items={menuItems} onselect={(id) => onMenu(game, id)}>
            {#snippet trigger({ toggle })}
              <IconButton label={msg('games.installedMenuLabel')} size="sm" onclick={toggle}>
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
      {@const update = $updatesByGame.get(game.id)?.availability}
      <div class="card" role="presentation" oncontextmenu={(event) => openGameMenu(event, game.id)}>
        <button class="card-cover" onclick={() => navigate('game', { id: game.id })} aria-label={game.title}>
          <Artwork src={coverFor(game)} alt={game.title} ratio="3 / 4" radius="var(--radius-md)" />
        </button>
        <div class="card-info">
          <span class="card-title">{game.title}</span>
          <span class="card-meta">
            {sizeLabel(game)}
            {#if update?.available}
              <span class="card-update">
                {update.kind === 'update' ? msg('games.updateKindUpdate') : msg('games.updateKindNewRelease')}
              </span>
            {/if}
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

{#if removeTarget}
  <RemoveGameModal
    bind:open={removeOpen}
    bind:mode={removeMode}
    gameId={removeTarget.id}
    title={removeTarget.title}
  />
{/if}

<Modal bind:open={addOpen} title={msg('games.installedAddGameModalTitle')}>
  <div class="form">
    <label class="field">
      <span class="field-label">{msg('games.executableLabel')}</span>
      <div class="field-row">
        <input class="input" type="text" placeholder="C:\Games\Game\game.exe" bind:value={newExecutable} />
        <Button onclick={browseExecutable}>{msg('games.installedBrowseButton')}</Button>
      </div>
    </label>
    <label class="field">
      <span class="field-label">{msg('games.installedTitleFieldLabel')}</span>
      <input class="input" type="text" placeholder={msg('games.installedTitlePlaceholder')} bind:value={newTitle} />
    </label>
    <p class="form-hint">{msg('games.installedFolderAutoHint')}</p>
  </div>
  {#snippet footer()}
    <Button onclick={() => (addOpen = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!newExecutable.trim() || adding} onclick={submitAdd}>
      {adding ? msg('games.installedAddingLabel') : msg('common.add')}
    </Button>
  {/snippet}
</Modal>

<style>
  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .search-wrap {
    width: 26rem;
  }

  .storage-block {
    margin-bottom: var(--space-6);
  }

  .storage-empty {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .storage {
    display: flex;
    align-items: center;
    gap: var(--space-6);
  }

  .storage-primary {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    flex: 1;
    min-width: 0;
  }

  .storage-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .disk {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    color: var(--text-2);
  }

  .disk-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.4rem;
    height: 4.4rem;
    flex-shrink: 0;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .disk-text {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .disk-name {
    font-size: var(--font-lg);
    font-weight: 600;
    color: var(--text);
  }

  .disk-meta {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .storage-bar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .storage-bar-track {
    flex: 1;
    min-width: 0;
  }

  .storage-pct {
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .storage-legend {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  .storage-legend li {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    font-size: var(--font-sm);
  }

  .dot {
    width: 0.8rem;
    height: 0.8rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot.games {
    background: var(--accent);
  }

  .dot.other {
    background: var(--text-2);
  }

  .dot.free {
    background: var(--text-3);
  }

  .legend-label {
    min-width: 7rem;
    color: var(--text-2);
  }

  .legend-value {
    margin-left: auto;
    padding-left: var(--space-5);
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

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

  .card-cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: transform var(--dur) var(--ease);
  }

  .card-cover:hover {
    transform: scale(1.01);
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
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .card-update {
    margin-left: 0.8rem;
    color: var(--warning);
  }

  .count {
    margin-top: var(--space-5);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .field-row {
    display: flex;
    gap: 0.8rem;
  }

  .form-hint {
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
