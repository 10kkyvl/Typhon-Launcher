<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
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
  import { launchesInFlight, type LibraryGame } from '../../lib/services/library';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import { rescan, scanProgress, scanSummary, scanning } from '../../lib/stores/discovery';
  import { installedGames, libraryLoaded, libraryLoadError, loadLibrary, runningGames } from '../../lib/stores/library';
  import { isEmulated } from '../../lib/library/emulation';
  import { refreshMissingImages } from '../../lib/stores/missingImages';
  import { followStorage, refreshStorage } from '../../lib/stores/storage';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
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

  // A row's action (Play, Stop, choosing its image or its file) waits on Go: its control is busy and
  // a second press sends nothing. A launch another screen started counts too.
  const pressed = new SvelteSet<string>();
  const busy = (id: string) => pressed.has(id) || $launchesInFlight.has(id);

  async function act(id: string, run: () => Promise<void>) {
    if (busy(id)) return;
    pressed.add(id);
    try {
      await run();
    } finally {
      pressed.delete(id);
    }
  }

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

  let toolbar = $state<HTMLElement>();
  let storageBox = $state<HTMLElement>();

  // A row is found by its game's id as data, never as a selector.
  const rowOf = (id: string) => [...(results?.querySelectorAll<HTMLElement>('[data-game-row]') ?? [])].find((row) => row.dataset.gameRow === id);
  const primaryOf = (box: HTMLElement | null | undefined) => (box ? focusableIn(box)[0] : undefined);
  const pageHeading = () => document.querySelector<HTMLElement>('h1[tabindex="-1"]') ?? undefined;

  // Focus moved by script does not scroll: the row it lands in is brought into view too.
  function focusInView(target: HTMLElement) {
    focusQuietly(target);
    const box = target.closest<HTMLElement>('[data-game-row]') ?? target;
    if (typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }

  // A game picked on the storage meter: its row brought into view and focused, after clearing a search
  // that hid it.
  async function revealGame(id: string) {
    if (!rowOf(id)) {
      search = '';
      await tick();
    }
    const target = primaryOf(rowOf(id));
    if (target) focusInView(target);
  }

  // The screen's one rule for a row that leaves (removed here or from the game menu, gone by Go's
  // doing): the row now at its place, else the row above, else the header's first control, else the
  // page heading. Kept by the game's id: a re-sort moves indexes.
  function leaveTarget(index: number, ids: readonly string[]): HTMLElement | undefined {
    const near = ids.length > 0 ? primaryOf(rowOf(ids[Math.min(Math.max(index, 0), ids.length - 1)])) : undefined;
    return near ?? primaryOf(toolbar) ?? pageHeading();
  }

  // Read before the list changes on screen, acted on right after, when the focused row left and focus
  // fell with it.
  let shownIds: string[] = [];
  let held: { id: string; index: number } | null = null;

  $effect.pre(() => {
    void filteredGames;
    untrack(() => {
      const active = document.activeElement;
      const row = active instanceof HTMLElement && results?.contains(active) ? active.closest<HTMLElement>('[data-game-row]') : null;
      const id = row?.dataset.gameRow;
      held = id ? { id, index: shownIds.indexOf(id) } : null;
    });
  });

  $effect(() => {
    const ids = filteredGames.map((game) => game.id);
    untrack(() => {
      const was = held;
      held = null;
      shownIds = ids;
      if (!was || ids.includes(was.id)) return;
      const active = document.activeElement;
      if (active && active !== document.body && active.isConnected) return;
      const target = leaveTarget(was.index, ids);
      if (target) focusInView(target);
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
  let removePlace = 0;

  function askRemove(game: LibraryGame, mode: 'disk' | 'library') {
    removeMode = mode;
    removeTarget = game;
    removePlace = shownIds.indexOf(game.id);
    removeOpen = true;
  }

  // The dialog returns focus to the row's menu button while the row stays; a row already gone by
  // then hands it on by the screen's rule.
  function removeReturn(): HTMLElement | undefined {
    const row = removeTarget ? rowOf(removeTarget.id) : undefined;
    return primaryOf(row) ?? leaveTarget(removePlace, shownIds);
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

  // The dialog's opener ("Choose folder") goes with the empty card it stood on: once the disk of the
  // new library is read, focus goes to the storage card, else to the header's first control.
  async function libraryReady() {
    const handOff = focusHandOff();
    await refreshStorage();
    await handOff(() => primaryOf(storageBox) ?? primaryOf(toolbar));
  }

  // The game the add dialog added: focus goes to its row when its opener (on the empty screen) is gone.
  let addedId = '';
  function addReturn(): HTMLElement | undefined {
    return primaryOf(rowOf(addedId)) ?? primaryOf(toolbar);
  }

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
      <div class="toolbar" bind:this={toolbar}>
        <InstalledToolbar bind:search bind:sort {scanLabel} onfind={findGames} onaddimage={openImageDialog} onaddgame={openAddDialog} />
      </div>
    {/snippet}
  </PageHeader>

  <div bind:this={storageBox}>
    <InstalledStorage onchoosefolder={() => (librarySetupOpen = true)} onpick={revealGame} />
  </div>

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
      <InstalledList games={filteredGames} {compat} {busy} {act} onremove={askRemove} />
    {:else}
      <InstalledGrid games={filteredGames} {uid} {busy} {act} />
    {/if}
  </div>

  {#if $installedGames.length > 0}
    <p class="count">
      {msg('installed.shownOf', { shown: filteredGames.length, count: $installedGames.length })}
    </p>
  {/if}

  <LibrarySetupModal bind:open={librarySetupOpen} onready={() => void libraryReady()} />

  <AddImageModal bind:open={imageOpen} />

  {#if removeTarget}
    <RemoveGameModal
      bind:open={removeOpen}
      bind:mode={removeMode}
      gameId={removeTarget.id}
      title={removeTarget.title}
      external={externalLabel(removeTarget)}
      emulated={isEmulated(removeTarget)}
      returnFocus={removeReturn}
      onremoved={() => void refreshStorage()}
    />
  {/if}

  <InstalledAddGame bind:this={addGameDialog} {uid} returnFocus={addReturn} onadded={(game) => (addedId = game.id)} />
</Page>

<style>
  .toolbar {
    display: contents;
  }

  .count {
    margin-top: var(--space-5);
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
