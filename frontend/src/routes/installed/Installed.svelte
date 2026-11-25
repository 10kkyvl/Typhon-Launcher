<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Page from '../../lib/components/Page.svelte';
  import { Gamepad2, Plus, RefreshCw, Search } from '@lucide/svelte';
  import AddImageModal from '../../lib/components/AddImageModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { focusableIn, focusQuietly } from '../../lib/layers/focus';
  import { inWails } from '../../lib/services/backend';
  import type { LibraryGame } from '../../lib/services/library';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import { rescan, scanProgress, scanSummary, scanning } from '../../lib/stores/discovery';
  import { installedGames, libraryLoaded, libraryLoadError, loadLibrary, runningGames } from '../../lib/stores/library';
  import { isEmulated } from '../../lib/library/emulation';
  import { refreshMissingImages } from '../../lib/stores/missingImages';
  import { followStorage } from '../../lib/stores/storage';
  import { externalLabel } from '../../lib/library/external';
  import { loadArt } from '../../lib/stores/metadata';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { compatStatuses, type CompatStatus } from '../../lib/services/compat';
  import { msg } from '../../lib/i18n';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { libraryErrorText, type Sort } from './installedGame';
  import InstalledAddGame from './InstalledAddGame.svelte';
  import InstalledGrid from './InstalledGrid.svelte';
  import InstalledList from './InstalledList.svelte';
  import InstalledStorage from './InstalledStorage.svelte';
  import InstalledToolbar from './InstalledToolbar.svelte';

  let search = $state('');
  let sort = $state<Sort>('recent');

  onMount(followStorage);

  // Журнал совместимости набирается сам из исходов запусков. Перечитываем его
  // при каждой смене состава запущенных игр: сессия только что закончилась —
  // значит вывод про игру мог измениться.
  let compat = $state<Map<string, CompatStatus>>(new Map());
  let compatRead = 0;
  $effect(() => {
    void $runningGames;
    void $installedGames;
    const mine = ++compatRead;
    compatStatuses().then(
      (next) => {
        if (mine === compatRead) compat = next;
      },
      // The log only adds the broken badge: a failed read keeps what the last one said, unsaid.
      () => {},
    );
  });

  // Until the first list arrives an empty list is not an empty library.
  const loadFailed = $derived($libraryLoadError != null && !$libraryLoaded);
  let libraryRetrying = $state(false);
  let results = $state<HTMLElement>();

  // A row is found by its game's id as data, never as a selector.
  const rowOf = (id: string) => [...(results?.querySelectorAll<HTMLElement>('[data-game-row]') ?? [])].find((row) => row.dataset.gameRow === id);

  // A game picked on the storage meter: its row brought into view and focused, after clearing a search
  // that hid it.
  async function revealGame(id: string) {
    if (!rowOf(id)) {
      search = '';
      await tick();
    }
    const row = rowOf(id);
    const target = row ? focusableIn(row)[0] : undefined;
    if (!row || !target) return;
    focusQuietly(target);
    if (typeof row.scrollIntoView === 'function') row.scrollIntoView({ block: 'nearest', inline: 'nearest' });
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

  let imageOpen = $state(false);

  function openImageDialog() {
    if (!inWails) {
      toast(msg('games.installedAddDesktopOnly'));
      return;
    }
    imageOpen = true;
  }

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('disk');
  let removeTarget = $state<LibraryGame | null>(null);

  function askRemove(game: LibraryGame, mode: 'disk' | 'library') {
    removeMode = mode;
    removeTarget = game;
    removeOpen = true;
  }

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

  let librarySetupOpen = $state(false);

  let addGameDialog = $state<InstalledAddGame | undefined>(undefined);

  const uid = $props.id();
</script>

<Page>
  <PageHeader
    title={msg('games.installedStatusWord')}
    subtitle={!$libraryLoaded
      ? undefined
      : $installedGames.length > 0
        ? msg('installed.youHave', { count: $installedGames.length })
        : msg('games.installedEmptySubtitle')}
  >
    {#snippet actions()}
      <InstalledToolbar bind:search bind:sort {scanLabel} onfind={findGames} onaddimage={openImageDialog} onaddgame={openAddDialog} />
    {/snippet}
  </PageHeader>

  <InstalledStorage onchoosefolder={() => (librarySetupOpen = true)} onpick={revealGame} />

  <div bind:this={results}>
    {#if loadFailed || libraryRetrying}
      <Notice
        tone="danger"
        title={msg('library.loadFailedTitle')}
        retry={{ label: msg('common.retry'), run: () => loadLibrary(), within: () => results }}
        bind:retrying={libraryRetrying}
      >
        {libraryErrorText($libraryLoadError, msg('library.loadFailedText'))}
      </Notice>
    {:else if !$libraryLoaded}
      <EmptyState level={2} title={msg('library.loadingTitle')} />
    {:else if $installedGames.length === 0}
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
      <InstalledList games={filteredGames} {compat} onremove={askRemove} />
    {:else}
      <InstalledGrid games={filteredGames} {uid} />
    {/if}
  </div>

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
  .count {
    margin-top: var(--space-5);
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
