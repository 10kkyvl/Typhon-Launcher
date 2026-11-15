<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { onMount } from 'svelte';
  import { ChevronDown, HardDrive, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import {
    ageOf,
    ageText,
    canManageGame,
    countUnknown,
    downloadBlocked,
    gameBlockedKey,
    isStale,
    leftoverKindKey,
    reasonLabel,
    sizeSorted,
    sizeText,
    sortGames,
    totalText,
    type GameSort,
  } from '../../lib/diskusage/usage';
  import { openMoveGame } from '../../lib/game/actions/move';
  import { isEmulated } from '../../lib/library/emulation';
  import { libraryGames } from '../../lib/stores/library';
  import { msg } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import { activeMove } from '../../lib/stores/relocate';
  import { bytesSize } from '../../lib/utils/format';
  import { createDiskUsagePage } from './diskUsagePage.svelte';

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

  function format(bytes: number) {
    return bytesSize(bytes);
  }

  function size(item: { sizeBytes: number; sizeUnknown: boolean }) {
    return sizeText(item.sizeBytes, item.sizeUnknown, format, msg('diskusage.sizeUnknown'));
  }

  function total(bytes: number, unknown: boolean) {
    return totalText(bytes, unknown, format, (value) => msg('diskusage.atLeast', { size: value }));
  }

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
        <section>
          <div class="section-head">
            <div>
              <h2>{msg('diskusage.gamesTitle')}</h2>
              <p class="muted">{msg('diskusage.gamesHint')}</p>
            </div>
            <div class="section-tools">
              <span class="section-total">{total(page.report.totals.gamesBytes, games.some((g) => g.sizeUnknown))}</span>
              <DropdownMenu
                items={(['size', 'played'] as const).map((id) => ({ id, label: sortLabels[id], checked: sort === id }))}
                onselect={(id) => (sort = id as GameSort)}
              >
                {#snippet trigger({ props })}
                  <Chip {...props}>
                    {msg('diskusage.sortLabel', { sort: sortLabels[sort] })}
                    <ChevronDown size="1.4rem" strokeWidth={1.8} />
                  </Chip>
                {/snippet}
              </DropdownMenu>
            </div>
          </div>
          <div class="list">
            {#each games as game (game.id)}
              {@const blocked = gameBlockedKey(game)}
              {@const manage = canManageGame(game)}
              <div class="row">
                <div class="main">
                  <span class="title">{game.title}</span>
                  <span class="path" title={game.installDir}>{game.installDir}</span>
                </div>
                <div class="badges">
                  {#if game.external}
                    <StatusBadge kind="neutral" label={msg('diskusage.external')} plain />
                  {/if}
                  {#if game.linked}
                    <StatusBadge kind="neutral" label={msg('diskusage.linked')} hint={msg('diskusage.linkedHint')} plain />
                  {/if}
                  {#if game.missing}
                    <StatusBadge kind="warning" label={msg('diskusage.missing')} plain />
                  {/if}
                  {#if game.running}
                    <StatusBadge kind="accent" label={msg('diskusage.running')} plain />
                  {/if}
                  {#if isStale(game.lastPlayed, now)}
                    <StatusBadge kind="warning" label={msg('diskusage.stale')} plain />
                  {/if}
                </div>
                <span class="played">{ageText(ageOf(game.lastPlayed, now))}</span>
                <span class="size">{size(game)}</span>
                <div class="actions">
                  {#if !game.external}
                    <Button size="sm" disabled={!manage} reason={blocked ? msg(blocked) : ''} onclick={() => openMoveGame(game.id)}>
                      {msg('diskusage.move')}
                    </Button>
                    <Button size="sm" variant="danger" disabled={!manage} reason={blocked ? msg(blocked) : ''} onclick={() => openRemove(game)}>
                      {msg('diskusage.remove')}
                    </Button>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <section>
        <div class="section-head">
          <div>
            <h2>{msg('diskusage.downloadsTitle')}</h2>
            <p class="muted">{msg('diskusage.downloadsHint')}</p>
          </div>
          <span class="section-total">{total(page.report.totals.downloadsBytes, downloads.some((d) => d.sizeUnknown))}</span>
        </div>
        {#if downloads.length === 0}
          <EmptyState size="inline" title={msg('diskusage.downloadsEmpty')} />
        {:else}
          <div class="list">
            {#each downloads as download (download.id)}
              {@const blocked = downloadBlocked(download)}
              <div class="row">
                <div class="main">
                  <span class="title">{download.name}</span>
                  <span class="path" title={download.root}>{download.root}</span>
                  {#if blocked}
                    <span class="reason">{reasonLabel(download.reasonCode, download.reason)}</span>
                  {:else if download.seeding}
                    <span class="reason">{msg('diskusage.reason.seeding')}</span>
                  {/if}
                </div>
                <div class="badges">
                  {#if download.seeding}
                    <StatusBadge kind="accent" label={msg('diskusage.downloadSeeding')} plain />
                  {/if}
                  {#if download.installed}
                    <StatusBadge kind="success" label={msg('diskusage.downloadInstalled')} plain />
                  {/if}
                  {#if download.missing}
                    <StatusBadge kind="warning" label={msg('diskusage.missing')} plain />
                  {/if}
                </div>
                <span class="size">{size(download)}</span>
                <div class="actions">
                  <Button size="sm" variant="danger" disabled={blocked} onclick={() => page.askDownload(download)}>
                    {msg('diskusage.downloadDelete')}
                  </Button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <section>
        <div class="section-head">
          <div>
            <h2>{msg('diskusage.leftoversTitle')}</h2>
            <p class="muted">{msg('diskusage.leftoversHint')}</p>
          </div>
          <span class="section-total">{total(page.report.totals.leftoversBytes, leftovers.some((l) => l.sizeUnknown))}</span>
        </div>
        {#if leftovers.length === 0}
          <EmptyState size="inline" title={msg('diskusage.leftoversEmpty')} />
        {:else}
          <div class="list">
            {#each leftovers as leftover (leftover.path)}
              <div class="row">
                <div class="main">
                  <span class="title">{msg(leftoverKindKey(leftover.kind))}</span>
                  <span class="path" title={leftover.path}>{leftover.path}</span>
                  {#if !leftover.deletable}
                    <span class="reason">{reasonLabel(leftover.reasonCode, leftover.reason)}</span>
                  {/if}
                </div>
                <span class="size">{size(leftover)}</span>
                <div class="actions">
                  <Button size="sm" variant="danger" disabled={!leftover.deletable} onclick={() => page.askLeftover(leftover)}>
                    {msg('diskusage.leftoverDelete')}
                  </Button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <section>
        <div class="section-head">
          <div>
            <h2>{msg('diskusage.screenshotsTitle')}</h2>
            <p class="muted">{msg('diskusage.screenshotsHint')}</p>
          </div>
          <span class="section-total">{total(page.report.totals.screenshotsBytes, screenshots.some((s) => s.sizeUnknown))}</span>
        </div>
        {#if screenshots.length === 0}
          <EmptyState size="inline" title={msg('diskusage.screenshotsEmpty')} />
        {:else}
          <div class="list">
            {#each screenshots as group (group.gameId)}
              <div class="row">
                <div class="main">
                  <span class="title">{group.title}</span>
                  <span class="path">{msg('diskusage.screenshotsCount', { count: group.count })}</span>
                </div>
                <div class="badges">
                  {#if !group.inLibrary}
                    <StatusBadge kind="warning" label={msg('diskusage.leftLibrary')} plain />
                  {/if}
                </div>
                <span class="size">{size(group)}</span>
                <div class="actions">
                  <Button size="sm" variant="danger" onclick={() => page.askScreenshots(group)}>
                    {msg('diskusage.deleteAll')}
                  </Button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <section>
        <div class="section-head">
          <div>
            <h2>{msg('diskusage.snapshotsTitle')}</h2>
            <p class="muted">{msg('diskusage.snapshotsHint')}</p>
          </div>
          <span class="section-total">{total(page.report.totals.snapshotsBytes, snapshots.some((s) => s.sizeUnknown))}</span>
        </div>
        {#if snapshots.length === 0}
          <EmptyState size="inline" title={msg('diskusage.snapshotsEmpty')} />
        {:else}
          <div class="list">
            {#each snapshots as group (group.gameId)}
              <div class="row">
                <div class="main">
                  <span class="title">{group.title}</span>
                  <span class="path">{msg('diskusage.snapshotsCount', { count: group.count })}</span>
                </div>
                <div class="badges">
                  {#if !group.inLibrary}
                    <StatusBadge kind="warning" label={msg('diskusage.leftLibrary')} plain />
                  {/if}
                </div>
                <span class="size">{size(group)}</span>
                <div class="actions">
                  <Button size="sm" variant="danger" onclick={() => page.askSnapshots(group)}>
                    {msg('diskusage.deleteAll')}
                  </Button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>
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
  section {
    margin-top: var(--space-8);
  }

  h2 {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .muted {
    margin-top: 0.3rem;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

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

  .section-head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-3);
  }

  .section-tools {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .section-total {
    font-size: var(--font-md);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .main {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
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

  .reason {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .badges {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .played {
    width: 15rem;
    flex-shrink: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .size {
    width: 14rem;
    flex-shrink: 0;
    text-align: right;
    font-size: var(--font-sm);
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    min-width: 12rem;
    flex-shrink: 0;
  }
</style>
