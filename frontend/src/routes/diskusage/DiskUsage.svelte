<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { onMount, untrack } from 'svelte';
  import { HardDrive, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import DiskMeter from '../../lib/components/DiskMeter.svelte';
  import { countUnknown, measuredWhen, sizeSorted, sortGames, summarySegments, type GameSort, type SummaryKey } from '../../lib/diskusage/usage';
  import { focusQuietly } from '../../lib/layers/focus';
  import { refreshStorage, storageInfo } from '../../lib/stores/storage';
  import { usageSectionIds, type UsageSection as UsageSectionKey } from './sections';
  import { isEmulated } from '../../lib/library/emulation';
  import { libraryGames, libraryLoaded } from '../../lib/stores/library';
  import { msg } from '../../lib/i18n';
  import { deleteErrorText } from '../../lib/diskusage/errors';
  import { activeMove } from '../../lib/stores/relocate';
  import { createDiskUsagePage } from './diskUsagePage.svelte';
  import { catchLeaving, placeOf, returnTo, type RowPlace } from './rowFocus';
  import DownloadsSection from './DownloadsSection.svelte';
  import GamesSection from './GamesSection.svelte';
  import GroupsSection from './GroupsSection.svelte';
  import LeftoversSection from './LeftoversSection.svelte';
  import UsageWarnings from './UsageWarnings.svelte';
  import { total } from './usageSizes';
  import { bytesSize } from '../../lib/utils/format';

  const page = createDiskUsagePage();
  let sort = $state<GameSort>('size');
  let removeOpen = $state(false);
  let removeTarget = $state<{ id: string; title: string } | null>(null);
  let removeMode = $state<'disk' | 'library'>('disk');

  const sortLabels: Record<GameSort, string> = {
    size: msg('diskusage.sortSize'),
    played: msg('diskusage.sortPlayed'),
  };

  const now = $derived(page.report ? Date.now() : 0);
  const games = $derived(page.report ? sortGames(page.report.games, sort) : []);
  const downloads = $derived(page.report ? sizeSorted(page.report.downloads) : []);
  const leftovers = $derived(page.report ? sizeSorted(page.report.leftovers) : []);
  const screenshots = $derived(page.report ? sizeSorted(page.report.screenshots) : []);
  const snapshots = $derived(page.report ? sizeSorted(page.report.snapshots) : []);
  const unknownCount = $derived(page.report ? countUnknown(page.report) : 0);
  const isEmpty = $derived(
    !!page.report &&
      games.length + downloads.length + leftovers.length + screenshots.length + snapshots.length === 0,
  );

  onMount(() => {
    void page.load();
    return () => page.leave();
  });

  const when = $derived(page.report ? measuredWhen(page.report.scannedAt) : null);
  let retrying = $state(false);

  let hadMove = false;
  $effect(() => {
    const active = $activeMove !== null;
    if (hadMove && !active) void page.refresh();
    hadMove = active;
  });

  // The installed games and their folders, as the library last said (library:updated): a removal or a
  // move that ended anywhere changes them, and the report's rows and flags with them. The library's
  // first load is not a change: the page's own scan reads it.
  const installedFolders = $derived(
    $libraryLoaded
      ? $libraryGames
          .filter((game) => !game.uninstalled)
          .map((game) => `${game.id}\u0000${game.installDir}`)
          .sort()
          .join('\u0001')
      : null,
  );
  let seenFolders: string | null = null;
  $effect(() => {
    const folders = installedFolders;
    if (folders === null) return;
    if (seenFolders !== null && folders !== seenFolders) void page.refresh();
    seenFolders = folders;
  });

  // Any new report can drop the row that holds focus, or the control in it, whoever asked for the scan.
  $effect.pre(() => {
    void page.report;
    untrack(catchLeaving);
  });

  // The library disk as last measured; a disk with no size counts as not measured.
  const disk = $derived($storageInfo && $storageInfo.totalBytes > 0 ? $storageInfo : null);

  // The disk is measured with each report, so its free space and the report's sizes are of one moment.
  $effect(() => {
    if (page.report) void refreshStorage();
  });

  const SEGMENT_SECTIONS: Record<SummaryKey, UsageSectionKey> = {
    games: 'games',
    downloads: 'downloads',
    'leftovers-reclaim': 'leftovers',
    'leftovers-kept': 'leftovers',
    screenshots: 'screenshots',
    snapshots: 'snapshots',
  };

  // A picked segment leads to its section: its heading takes focus and is brought into view (focus
  // moved by script does not scroll).
  function pickSection(key: string) {
    const section = SEGMENT_SECTIONS[key as SummaryKey];
    const heading = section ? document.getElementById(usageSectionIds[section]) : null;
    if (!heading) return;
    focusQuietly(heading);
    if (typeof heading.scrollIntoView === 'function') heading.scrollIntoView({ block: 'start' });
  }

  // Where the removed game's row stood, read at the press: focus follows the row rule once it has gone.
  let removePlace: RowPlace | null = null;

  function openRemove(game: { id: string; title: string }) {
    removePlace = placeOf(`games:${game.id}`, 'remove');
    removeTarget = { id: game.id, title: game.title };
    removeMode = 'disk';
    removeOpen = true;
  }
</script>

<Page>
  <PageHeader title={msg('diskusage.title')} subtitle={msg('diskusage.subtitle')}>
    {#snippet actions()}
      <Button onclick={page.refresh} busy={page.scanning}>
        {#if !page.scanning}<RefreshCw size="1.5rem" strokeWidth={2} />{/if}
        {page.scanning ? msg('diskusage.scanning') : msg('diskusage.rescan')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if when}
    <p class="measured">
      <!-- The exact time goes to assistive tech as text and to the pointer as a hint (the History
           convention); the time element takes focus only from the pointer. -->
      <Tooltip text={when.exact} describe={false}>
        <time datetime={page.report?.scannedAt} tabindex="-1"><span aria-hidden="true">{when.words}</span><span class="sr-only">{when.exact}</span></time>
      </Tooltip>
      {#if page.scanning}<span class="remeasuring">{msg('diskusage.remeasuring')}</span>{/if}
    </p>
  {/if}

  {#if page.scanError || retrying}
    <div class="error">
      <Notice tone="danger" retry={{ label: msg('common.retry'), run: page.refresh }} bind:retrying>{page.scanError}</Notice>
    </div>
  {/if}

  {#if !page.report}
    {#if page.scanning && !page.scanError && !retrying}
      <Spinner label={msg('diskusage.scanning')} />
    {/if}
  {:else}
    <Card>
      <div class="summary">
        <!-- The launcher's files lie on every drive, so they are drawn against their own total, with no
             free space and no other files beside them; the library disk is said on a line of its own. -->
        <DiskMeter
          segments={summarySegments(page.report)}
          sum
          label={msg('diskusage.meterLauncher')}
          onpick={pickSection}
        />
        <!-- Go's totals in words: the same numbers the meter draws, summed by Go, not by the page. -->
        <div class="figures">
          <div class="figure">
            <span class="figure-label">{msg('diskusage.used')}</span>
            <span class="figure-value">{total(page.report.totals.totalBytes, page.report.totals.unknownSizes)}</span>
          </div>
          <div class="figure">
            <span class="figure-label">{msg('diskusage.reclaimable')}</span>
            <span class="figure-value">{total(page.report.totals.reclaimableBytes, page.report.totals.unknownSizes)}</span>
          </div>
        </div>
        {#if page.report.totals.unknownSizes}
          <p class="note">{msg('diskusage.unknownNote', { count: unknownCount })}</p>
        {/if}
        {#if disk}
          <p class="note">{msg('diskusage.libraryDisk', { volume: disk.volume || disk.path, free: bytesSize(disk.freeBytes), total: bytesSize(disk.totalBytes) })}</p>
        {:else}
          <p class="note">{msg('diskusage.freeUnknown')}</p>
        {/if}
      </div>
    </Card>

    <UsageWarnings warnings={page.report.warnings} />

    {#if isEmpty}
      <EmptyState level={2} title={msg('diskusage.emptyTitle')} description={msg('diskusage.emptyText')}>
        {#snippet icon()}
          <HardDrive size="2rem" strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
    {:else}
      {#if games.length > 0}
        <GamesSection {games} totalBytes={page.report.totals.gamesBytes} {now} {sort} {sortLabels} onsort={(next) => (sort = next)} onremove={openRemove} />
      {/if}

      <DownloadsSection {downloads} totalBytes={page.report.totals.downloadsBytes} ondelete={page.askDownload} />

      <LeftoversSection {leftovers} totalBytes={page.report.totals.leftoversBytes} ondelete={page.askLeftover} />

      <GroupsSection kind="screenshots" groups={screenshots} totalBytes={page.report.totals.screenshotsBytes} ondelete={page.askScreenshots} />

      <GroupsSection kind="snapshots" groups={snapshots} totalBytes={page.report.totals.snapshotsBytes} ondelete={page.askSnapshots} />
    {/if}
  {/if}

  {#if removeTarget}
    <RemoveGameModal
      bind:open={removeOpen}
      bind:mode={removeMode}
      gameId={removeTarget.id}
      title={removeTarget.title}
      emulated={isEmulated($libraryGames.find((game) => game.id === removeTarget?.id))}
      onremoved={() => void page.refresh()}
      returnFocus={() => returnTo(removePlace)}
    />
  {/if}

  {#if page.pending}
    {@const current = page.pending}
    <ConfirmModal
      prompt={current.prompt}
      onconfirm={current.run}
      onclose={() => page.closePending()}
      errorText={deleteErrorText}
      returnFocus={current.returnFocus}
      live={current.follows}
    />
  {/if}
</Page>

<style>
  .error {
    margin-bottom: var(--space-4);
  }

  .measured {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .remeasuring {
    color: var(--text-3);
  }

  .summary {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .figures {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-8);
  }

  .figure {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .figure-label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .figure-value {
    font-size: var(--font-lg);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .note {
    margin-top: var(--space-3);
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
