<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { onMount } from 'svelte';
  import { HardDrive, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import {
    countUnknown,
    sizeSorted,
    sortGames,
    type GameSort,
  } from '../../lib/diskusage/usage';
  import { isEmulated } from '../../lib/library/emulation';
  import { libraryGames } from '../../lib/stores/library';
  import { msg } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import { activeMove } from '../../lib/stores/relocate';
  import { createDiskUsagePage } from './diskUsagePage.svelte';
  import DownloadsSection from './DownloadsSection.svelte';
  import GamesSection from './GamesSection.svelte';
  import LeftoversSection from './LeftoversSection.svelte';
  import UsageRow from './UsageRow.svelte';
  import UsageSection from './UsageSection.svelte';
  import { size, total } from './usageSizes';

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
  });

  let hadMove = false;
  $effect(() => {
    const active = $activeMove !== null;
    if (hadMove && !active) void page.refresh();
    hadMove = active;
  });

  function openRemove(game: { id: string; title: string }) {
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

  {#if page.scanError}
    <div class="error"><Notice tone="danger">{page.scanError}</Notice></div>
  {/if}

  {#if !page.report}
    {#if page.scanning}
      <Spinner label={msg('diskusage.scanning')} />
    {/if}
  {:else}
    <Card>
      <div class="summary">
        <span class="summary-icon"><HardDrive size="1.8rem" strokeWidth={1.8} /></span>
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
    </Card>

    {#if page.report.warnings.length > 0}
      <div class="warnings">
        <Notice tone="danger" title={msg('diskusage.warningsTitle')} level={2}>
          <ul>
            {#each page.report.warnings as warning, index (index)}
              <li>{msg('diskusage.warningLine', { area: warning.area, message: warning.message })}</li>
            {/each}
          </ul>
        </Notice>
      </div>
    {/if}

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

      <UsageSection
        title={msg('diskusage.screenshotsTitle')}
        hint={msg('diskusage.screenshotsHint')}
        total={total(page.report.totals.screenshotsBytes, screenshots.some((s) => s.sizeUnknown))}
        empty={screenshots.length === 0}
        emptyText={msg('diskusage.screenshotsEmpty')}
      >
        {#each screenshots as group (group.gameId)}
          <UsageRow title={group.title} path={msg('diskusage.screenshotsCount', { count: group.count })} size={size(group)}>
            {#snippet badges()}
              {#if !group.inLibrary}
                <StatusBadge kind="warning" label={msg('diskusage.leftLibrary')} plain />
              {/if}
            {/snippet}
            {#snippet actions()}
              <Button size="sm" variant="danger" onclick={() => page.askScreenshots(group)}>
                {msg('diskusage.deleteAll')}
              </Button>
            {/snippet}
          </UsageRow>
        {/each}
      </UsageSection>

      <UsageSection
        title={msg('diskusage.snapshotsTitle')}
        hint={msg('diskusage.snapshotsHint')}
        total={total(page.report.totals.snapshotsBytes, snapshots.some((s) => s.sizeUnknown))}
        empty={snapshots.length === 0}
        emptyText={msg('diskusage.snapshotsEmpty')}
      >
        {#each snapshots as group (group.gameId)}
          <UsageRow title={group.title} path={msg('diskusage.snapshotsCount', { count: group.count })} size={size(group)}>
            {#snippet badges()}
              {#if !group.inLibrary}
                <StatusBadge kind="warning" label={msg('diskusage.leftLibrary')} plain />
              {/if}
            {/snippet}
            {#snippet actions()}
              <Button size="sm" variant="danger" onclick={() => page.askSnapshots(group)}>
                {msg('diskusage.deleteAll')}
              </Button>
            {/snippet}
          </UsageRow>
        {/each}
      </UsageSection>
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
    />
  {/if}

  {#if page.pending}
    {@const current = page.pending}
    <ConfirmModal prompt={current.prompt} onconfirm={current.run} onclose={() => page.closePending()} errorText={(err) => installErrorText(err)} />
  {/if}
</Page>

<style>
  .error {
    margin-bottom: var(--space-4);
  }

  .summary {
    display: flex;
    align-items: center;
    gap: var(--space-8);
  }

  .summary-icon {
    display: inline-flex;
    color: var(--text-2);
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

  .warnings {
    margin-top: var(--space-4);
  }

  .warnings ul {
    padding-left: var(--space-5);
  }
</style>
