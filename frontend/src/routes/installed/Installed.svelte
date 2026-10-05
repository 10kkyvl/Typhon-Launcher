<script lang="ts">
  import {
    ArrowUp,
    ChevronDown,
    EllipsisVertical,
    FolderOpen,
    Gamepad2,
    HardDrive,
    LayoutGrid,
    List,
    Play,
    Plus,
    RefreshCw,
    Search,
    Sparkles,
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
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { inWails } from '../../lib/services/backend';
  import {
    addGame,
    playGame,
    selectExecutable,
    selectGameExecutable,
    setExecutable,
    stopGame,
    type LibraryGame,
  } from '../../lib/services/library';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import { openGameFolder, openFolder } from '../../lib/services/settings';
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
  import { compatStatuses, type CompatStatus } from '../../lib/services/compat';
  import { bytesSize, relativeDate } from '../../lib/utils/format';
  import { errorCode, hasMessage, msg } from '../../lib/i18n';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';

  function libraryErrorText(err: unknown, fallback: string): string {
    const code = errorCode(err);
    return hasMessage(code) ? msg(code) : fallback;
  }

  type Sort = 'recent' | 'alpha' | 'size';

  const sortLabels: Record<Sort, string> = {
    recent: msg('games.recentLabel'),
    alpha: msg('games.sortAlpha'),
    size: msg('games.sortSize'),
  };

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
    const status = compat.get(game.id);
    if (!status || status.state !== 'broken') return '';
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

  function sizeLabel(game: LibraryGame) {
    return game.sizeBytes > 0 ? bytesSize(game.sizeBytes) : '';
  }

  function updateLabel(update?: { available: boolean; kind: string }) {
    if (!update?.available) return '';
    return update.kind === 'update' ? msg('games.updateKindUpdate') : msg('games.updateKindNewRelease');
  }

  const gamesBytes = $derived($installedGames.reduce((sum, game) => sum + game.sizeBytes, 0));
  const hasUnknownSize = $derived($installedGames.some((game) => game.sizeUnknown));
  const usedPct = $derived($storageInfo ? ($storageInfo.usedBytes / $storageInfo.totalBytes) * 100 : 0);
  const otherBytes = $derived.by(() => {
    if (!$storageInfo || hasUnknownSize) return null;
    const value = $storageInfo.usedBytes - gamesBytes;
    return value >= 0 ? value : null;
  });
  const segments = $derived.by(() => {
    if (!$storageInfo || $storageInfo.totalBytes <= 0) return [];
    if (otherBytes === null) return [{ id: 'used', pct: Math.min(100, usedPct) }];
    const total = $storageInfo.totalBytes;
    return [
      { id: 'games', pct: (gamesBytes / total) * 100 },
      { id: 'other', pct: (otherBytes / total) * 100 },
    ].filter((segment) => segment.pct > 0);
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
      toast(libraryErrorText(err, msg('games.installedAddGameError')), 'danger');
    } finally {
      adding = false;
    }
  }

  async function play(game: LibraryGame) {
    try {
      await playGame(game.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
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
        { id: 'grid', label: msg('games.viewGrid') },
        { id: 'list', label: msg('games.viewList') },
      ]}
    >
      {#snippet item(option)}
        {#if option.id === 'grid'}
          <LayoutGrid size="1.6rem" strokeWidth={1.8} />
        {:else}
          <List size="1.6rem" strokeWidth={1.8} />
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
          <Button onclick={() => navigate('settings', { tab: 'general' })}>{msg('games.installedManageStorage')}</Button>
        </div>
        <div class="storage-bar">
          <div class="segments" aria-hidden="true">
            {#each segments as segment (segment.id)}
              <span class="segment {segment.id}" style:width="{segment.pct}%"></span>
            {/each}
            <span class="segment free"></span>
          </div>
          <span class="storage-pct">{Math.round(usedPct)}%</span>
        </div>
        <ul class="storage-legend">
          {#if otherBytes !== null}
            <li>
              <span class="dot games"></span>
              <span class="legend-label">{msg('games.installedLegendGames')}</span>
              <span class="legend-value">{bytesSize(gamesBytes)}</span>
            </li>
            <li>
              <span class="dot other"></span>
              <span class="legend-label">{msg('games.installedLegendOther')}</span>
              <span class="legend-value">{bytesSize(otherBytes)}</span>
            </li>
          {:else}
            <li>
              <span class="dot used"></span>
              <span class="legend-label">{msg('games.installedLegendUsed')}</span>
              <span class="legend-value">{bytesSize($storageInfo.usedBytes)}</span>
            </li>
          {/if}
          <li>
            <span class="dot free"></span>
            <span class="legend-label">{msg('games.installedLegendFree')}</span>
            <span class="legend-value">{bytesSize($storageInfo.freeBytes)}</span>
          </li>
        </ul>
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
      {@const updateText = updateLabel(update)}
      <div class="row" role="presentation" oncontextmenu={(event) => openGameMenu(event, game.id)}>
        <button class="game" onclick={() => navigate('game', { id: game.id })}>
          <div class="thumb">
            <Artwork src={heroFor(game)} alt={game.title} radius="var(--radius-sm)" />
          </div>
          <div class="titles">
            <span class="title">{game.title}</span>
            <span class="path">{game.installDir}</span>
            {#if sizeLabel(game) || updateText}
              <span class="size">
                {sizeLabel(game)}
                {#if updateText}
                  <span class="row-update" class:release={update?.kind !== 'update'}>
                    {#if update?.kind === 'update'}
                      <ArrowUp size="1.2rem" strokeWidth={2.2} />
                    {:else}
                      <Sparkles size="1.2rem" strokeWidth={2} />
                    {/if}
                    {updateText}
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
            <span class="compat" title={brokenNote(game)}>
              <StatusBadge kind="danger" label={msg('games.compatBrokenBadge')} plain />
            </span>
          {/if}
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
      {@const running = $runningGames.has(game.id)}
      {@const update = $updatesByGame.get(game.id)?.availability}
      {@const updateText = updateLabel(update)}
      <div class="card" role="presentation" oncontextmenu={(event) => openGameMenu(event, game.id)}>
        <div class="card-media">
          <button
            class="card-cover"
            onclick={() => navigate('game', { id: game.id })}
            aria-label={updateText ? `${game.title} — ${updateText}` : game.title}
          >
            <Artwork src={coverFor(game)} alt={game.title} ratio="3 / 4" radius="var(--radius-md)" />
            {#if updateText}
              <span class="card-badge" class:release={update?.kind !== 'update'}>
                {#if update?.kind === 'update'}
                  <ArrowUp size="1.3rem" strokeWidth={2.4} />
                {:else}
                  <Sparkles size="1.3rem" strokeWidth={2} />
                {/if}
                <span class="card-badge-text">{updateText}</span>
              </span>
            {/if}
          </button>
          {#if running}
            <button class="card-play running" aria-label={msg('games.installedStopButton')} onclick={() => stop(game)}>
              <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
            </button>
          {:else if game.executable}
            <button class="card-play" aria-label={msg('games.play')} onclick={() => play(game)}>
              <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
            </button>
          {/if}
        </div>
        <div class="card-info">
          <span class="card-title">{game.title}</span>
          <span class="card-meta">{sizeLabel(game)}</span>
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
    --seg-games: var(--accent);
    --seg-other: var(--text-3);
    --seg-free: color-mix(in srgb, var(--text-3) 28%, transparent);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .storage-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .storage-head .disk-text {
    flex: 1;
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

  .segments {
    display: flex;
    gap: 0.2rem;
    flex: 1;
    min-width: 0;
    height: 0.8rem;
    border-radius: 99rem;
    overflow: hidden;
  }

  .segment {
    min-width: 0.4rem;
    transform-origin: left center;
    animation: segment-grow var(--dur-slow) var(--ease) backwards;
  }

  .segment.games {
    background: var(--seg-games);
  }

  .segment.other {
    background: var(--seg-other);
    animation-delay: calc(var(--dur-fast) / 2);
  }

  .segment.used {
    background: var(--seg-other);
  }

  .segment.free {
    flex: 1;
    min-width: 0;
    background: var(--seg-free);
    animation-name: segment-fade;
  }

  @keyframes segment-grow {
    from {
      transform: scaleX(0);
    }
  }

  @keyframes segment-fade {
    from {
      opacity: 0;
    }
  }

  .storage-pct {
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .storage-legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-8);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .storage-legend li {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    font-size: var(--font-sm);
  }

  .dot {
    width: 1rem;
    height: 1rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot.games {
    background: var(--seg-games);
  }

  .dot.other,
  .dot.used {
    background: var(--seg-other);
  }

  .dot.free {
    background: var(--seg-free);
  }

  .legend-label {
    color: var(--text-2);
  }

  .legend-value {
    padding-left: var(--space-2);
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
    align-items: center;
    gap: 0.4rem;
    max-width: calc(100% - 1.2rem);
    height: 2.2rem;
    padding: 0 0.7rem 0 0.5rem;
    border-radius: var(--radius-sm);
    background: rgba(5, 8, 12, 0.78);
    color: white;
    font-size: 1.1rem;
    font-weight: 500;
    line-height: 1.2;
    transform-origin: left top;
    animation: badge-pop var(--dur-slow) var(--ease-spring) backwards;
  }

  .card-badge :global(svg) {
    flex-shrink: 0;
    color: color-mix(in srgb, var(--warning) 75%, white);
  }

  .card-badge.release :global(svg) {
    color: white;
  }

  .card-badge-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    align-items: center;
    justify-content: center;
    width: 3.2rem;
    height: 3.2rem;
    border-radius: var(--radius-md);
    background: var(--accent);
    color: var(--accent-on, #fff);
    opacity: 0;
    transform: translateY(0.4rem) scale(0.9);
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur-panel) var(--ease-spring),
      background var(--dur) var(--ease);
  }

  .card-play:hover {
    background: var(--accent-hover);
  }

  .card-media:hover .card-play,
  .card-play:focus-visible,
  .card-play.running {
    opacity: 1;
    transform: translateY(0) scale(1);
  }

  .card-play:active {
    transform: scale(0.92);
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

  .row-update {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    margin-left: 0.8rem;
    color: var(--warning);
    vertical-align: bottom;
  }

  .row-update.release {
    color: var(--text-2);
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
