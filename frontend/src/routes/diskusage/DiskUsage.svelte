<script lang="ts">
  import { onMount } from 'svelte';
  import { ChevronDown, HardDrive, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import {
    ageOf,
    ageText,
    canManageGame,
    countUnknown,
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
  import { msg } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import {
    cachedUsage,
    deleteDownloadData,
    deleteLeftover,
    deleteScreenshots,
    deleteSnapshots,
    scanUsage,
    type DownloadUsage,
    type GroupUsage,
    type Leftover,
    type Report,
  } from '../../lib/services/diskusage';
  import { activeMove } from '../../lib/stores/relocate';
  import { toast } from '../../lib/stores/toasts';
  import { bytesSize } from '../../lib/utils/format';

  let report = $state<Report | null>(null);
  let scanning = $state(false);
  let scanError = $state('');
  let sort = $state<GameSort>('size');
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let removeOpen = $state(false);
  let removeTarget = $state<{ id: string; title: string } | null>(null);
  let removeMode = $state<'disk' | 'library'>('disk');
  let scanSerial = 0;

  const sortLabels: Record<GameSort, string> = {
    size: msg('diskusage.sortSize'),
    played: msg('diskusage.sortPlayed'),
  };

  const now = $derived(report ? Date.now() : 0);
  const games = $derived(report ? sortGames(report.games, sort) : []);
  const downloads = $derived(report ? sizeSorted(report.downloads) : []);
  const leftovers = $derived(report ? sizeSorted(report.leftovers) : []);
  const screenshots = $derived(report ? sizeSorted(report.screenshots) : []);
  const snapshots = $derived(report ? sizeSorted(report.snapshots) : []);
  const unknownCount = $derived(report ? countUnknown(report) : 0);
  const isEmpty = $derived(
    !!report &&
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

  function failureText(err: unknown) {
    return installErrorText(err, msg('diskusage.deleteFailed'));
  }

  async function refresh() {
    const serial = ++scanSerial;
    scanning = true;
    scanError = '';
    try {
      const next = await scanUsage();
      if (serial === scanSerial) report = next;
    } catch (err) {
      if (serial === scanSerial) scanError = installErrorText(err, msg('diskusage.scanFailed'));
    } finally {
      if (serial === scanSerial) scanning = false;
    }
  }

  onMount(() => {
    void (async () => {
      try {
        const cached = await cachedUsage();
        if (cached && !report) report = cached;
      } catch (err) {
        scanError = installErrorText(err, msg('diskusage.scanFailed'));
      }
      await refresh();
    })();
  });

  let hadMove = false;
  $effect(() => {
    const active = $activeMove !== null;
    if (hadMove && !active) void refresh();
    hadMove = active;
  });

  function ask(prompt: ConfirmPrompt, action: () => Promise<void>, done: string) {
    pending = {
      prompt,
      run: async () => {
        try {
          await action();
          toast(done, 'success');
        } catch (err) {
          toast(failureText(err), 'danger');
        }
        await refresh();
      },
    };
  }

  function askDownload(download: DownloadUsage) {
    ask(
      {
        title: msg('diskusage.downloadConfirmTitle'),
        text: msg('diskusage.downloadConfirmText', { name: download.name }),
        confirm: msg('diskusage.downloadDelete'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteDownloadData(download.id),
      msg('diskusage.downloadDeleted'),
    );
  }

  function askLeftover(leftover: Leftover) {
    ask(
      {
        title: msg('diskusage.leftoverConfirmTitle'),
        text: msg('diskusage.leftoverConfirmText', {
          kind: msg(leftoverKindKey(leftover.kind)),
          path: leftover.path,
        }),
        confirm: msg('diskusage.leftoverDelete'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteLeftover(leftover.path),
      msg('diskusage.leftoverDeleted'),
    );
  }

  function askScreenshots(group: GroupUsage) {
    ask(
      {
        title: msg('diskusage.screenshotsConfirmTitle'),
        text: msg('diskusage.screenshotsConfirmText', { title: group.title, count: group.count }),
        confirm: msg('diskusage.deleteAll'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteScreenshots(group.gameId),
      msg('diskusage.screenshotsDeleted'),
    );
  }

  function askSnapshots(group: GroupUsage) {
    ask(
      {
        title: msg('diskusage.snapshotsConfirmTitle'),
        text: msg('diskusage.snapshotsConfirmText', { title: group.title, count: group.count }),
        note: msg('diskusage.snapshotsConfirmNote'),
        confirm: msg('diskusage.snapshotsConfirmAction'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteSnapshots(group.gameId),
      msg('diskusage.snapshotsDeleted'),
    );
  }

  function openRemove(game: { id: string; title: string }) {
    removeTarget = { id: game.id, title: game.title };
    removeMode = 'disk';
    removeOpen = true;
  }
</script>

<PageHeader title={msg('diskusage.title')} subtitle={msg('diskusage.subtitle')}>
  {#snippet actions()}
    <Button onclick={refresh} disabled={scanning}>
      <RefreshCw size="1.5rem" strokeWidth={2} class={scanning ? 'spin' : ''} />
      {scanning ? msg('diskusage.scanning') : msg('diskusage.rescan')}
    </Button>
  {/snippet}
</PageHeader>

{#if scanError}
  <p class="error" role="alert">{scanError}</p>
{/if}

{#if !report}
  {#if scanning}
    <p class="muted">{msg('diskusage.scanning')}</p>
  {/if}
{:else}
  <Card>
    <div class="summary">
      <span class="summary-icon"><HardDrive size="1.8rem" strokeWidth={1.8} /></span>
      <div class="figure">
        <span class="figure-label">{msg('diskusage.used')}</span>
        <span class="figure-value">{total(report.totals.totalBytes, report.totals.unknownSizes)}</span>
      </div>
      <div class="figure">
        <span class="figure-label">{msg('diskusage.reclaimable')}</span>
        <span class="figure-value">{total(report.totals.reclaimableBytes, report.totals.unknownSizes)}</span>
      </div>
    </div>
    {#if report.totals.unknownSizes}
      <p class="note">{msg('diskusage.unknownNote', { count: unknownCount })}</p>
    {/if}
  </Card>

  {#if report.warnings.length > 0}
    <div class="warnings" role="alert">
      <h2>{msg('diskusage.warningsTitle')}</h2>
      <ul>
        {#each report.warnings as warning, index (index)}
          <li>{msg('diskusage.warningLine', { area: warning.area, message: warning.message })}</li>
        {/each}
      </ul>
    </div>
  {/if}

  {#if isEmpty}
    <EmptyState title={msg('diskusage.emptyTitle')} description={msg('diskusage.emptyText')}>
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
            <span class="section-total">{total(report.totals.gamesBytes, games.some((g) => g.sizeUnknown))}</span>
            <DropdownMenu
              items={[
                { id: 'size', label: sortLabels.size },
                { id: 'played', label: sortLabels.played },
              ]}
              onselect={(id) => (sort = id as GameSort)}
            >
              {#snippet trigger({ open, toggle })}
                <Chip selected={open} onclick={toggle}>
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
                  <span title={msg('diskusage.linkedHint')}>
                    <StatusBadge kind="neutral" label={msg('diskusage.linked')} plain />
                  </span>
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
                  <span title={blocked ? msg(blocked) : undefined}>
                    <Button size="sm" disabled={!manage} onclick={() => openMoveGame(game.id)}>
                      {msg('diskusage.move')}
                    </Button>
                  </span>
                  <span title={blocked ? msg(blocked) : undefined}>
                    <Button size="sm" variant="danger" disabled={!manage} onclick={() => openRemove(game)}>
                      {msg('diskusage.remove')}
                    </Button>
                  </span>
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
        <span class="section-total">{total(report.totals.downloadsBytes, downloads.some((d) => d.sizeUnknown))}</span>
      </div>
      {#if downloads.length === 0}
        <p class="muted">{msg('diskusage.downloadsEmpty')}</p>
      {:else}
        <div class="list">
          {#each downloads as download (download.id)}
            {@const blocked = download.referenced || download.seeding}
            <div class="row">
              <div class="main">
                <span class="title">{download.name}</span>
                <span class="path" title={download.root}>{download.root}</span>
                {#if blocked}
                  <span class="reason">{reasonLabel(download.reasonCode, download.reason)}</span>
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
                <Button size="sm" variant="danger" disabled={blocked} onclick={() => askDownload(download)}>
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
        <span class="section-total">{total(report.totals.leftoversBytes, leftovers.some((l) => l.sizeUnknown))}</span>
      </div>
      {#if leftovers.length === 0}
        <p class="muted">{msg('diskusage.leftoversEmpty')}</p>
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
                <Button size="sm" variant="danger" disabled={!leftover.deletable} onclick={() => askLeftover(leftover)}>
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
        <span class="section-total">{total(report.totals.screenshotsBytes, screenshots.some((s) => s.sizeUnknown))}</span>
      </div>
      {#if screenshots.length === 0}
        <p class="muted">{msg('diskusage.screenshotsEmpty')}</p>
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
                <Button size="sm" variant="danger" onclick={() => askScreenshots(group)}>
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
        <span class="section-total">{total(report.totals.snapshotsBytes, snapshots.some((s) => s.sizeUnknown))}</span>
      </div>
      {#if snapshots.length === 0}
        <p class="muted">{msg('diskusage.snapshotsEmpty')}</p>
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
                <Button size="sm" variant="danger" onclick={() => askSnapshots(group)}>
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
    onremoved={() => void refresh()}
  />
{/if}

{#if pending}
  {@const current = pending}
  <ConfirmModal prompt={current.prompt} onconfirm={current.run} onclose={() => (pending = null)} />
{/if}

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
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--danger);
    border-radius: var(--radius-md);
    color: var(--danger);
    font-size: var(--font-sm);
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
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--danger);
    border-radius: var(--radius-md);
  }

  .warnings h2 {
    font-size: var(--font-md);
    color: var(--danger);
  }

  .warnings ul {
    margin-top: var(--space-2);
    padding-left: var(--space-5);
    font-size: var(--font-sm);
    color: var(--text-2);
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
