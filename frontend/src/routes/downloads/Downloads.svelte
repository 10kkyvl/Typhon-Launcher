<script lang="ts">
  import {
    ArrowDown,
    ArrowUp,
    ChevronDown,
    CircleAlert,
    Download,
    FolderOpen,
    Menu,
    Plus,
    RotateCcw,
    Settings,
    X,
  } from '@lucide/svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DownloadDetailsModal from '../../lib/components/DownloadDetailsModal.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { Download as DownloadRecord } from '../../lib/services/downloads';
  import { maxActiveDownloadOptions, openFolder } from '../../lib/services/settings';
  import {
    active,
    completed,
    failed,
    forceStart,
    moveDown,
    moveUp,
    queue,
    remove,
    resume,
    stats,
  } from '../../lib/stores/downloads';
  import { installErrorText } from '../../lib/install/installErrors';
  import { installIndeterminate } from '../../lib/install/progress';
  import { installActive, installStatusLabels, installationsByDownload } from '../../lib/stores/install';
  import { gameArt, requestArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { sources } from '../../lib/stores/sources';
  import { toast } from '../../lib/stores/toasts';
  import { errorMessage } from '../../lib/utils/errors';
  import { bytesSize, clockTime, relativeDate, speedBytes } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  const concurrencyValue = $derived(String($settings?.maxActiveDownloads ?? 2));

  function pickConcurrency(id: string) {
    updateSettings({ maxActiveDownloads: Number(id) });
  }

  let addOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let installOpen = $state(false);
  let installDownloadId = $state<string | null>(null);

  const nothing = $derived(
    $active.length === 0 && $queue.length === 0 && $completed.length === 0 && $failed.length === 0,
  );

  function openDetails(id: string) {
    detailsId = id;
    detailsOpen = true;
  }

  function openInstall(id: string) {
    installDownloadId = id;
    installOpen = true;
  }

  function typeTag(d: DownloadRecord) {
    if (d.origin.purpose === 'update') return msg('transfers.downloadsTypeUpdate');
    if (d.origin.purpose === 'repair') return msg('transfers.downloadsTypeRepair');
    if (d.origin.gameId || d.origin.releaseId) return msg('transfers.downloadsTypeGame');
    return '';
  }

  function sourceTag(d: DownloadRecord) {
    return $sources.find((s) => s.id === d.origin.sourceId)?.name ?? '';
  }

  function coverOf(d: DownloadRecord) {
    return (d.origin.gameId && $gameArt[d.origin.gameId]?.cover) || '';
  }

  $effect(() => {
    const ids = [...$queue, ...$failed, ...$completed]
      .map((d) => d.origin.gameId)
      .filter((id): id is string => Boolean(id));
    if (ids.length > 0) requestArt(ids);
  });

  function completedWhen(iso: string | null) {
    if (!iso) return '—';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    const time = clockTime(date);
    const rel = relativeDate(iso);
    return msg('transfers.downloadsCompletedAt', { rel: `${rel.charAt(0).toLowerCase()}${rel.slice(1)}`, time });
  }

  async function openDestination(path: string) {
    try {
      await openFolder(path);
    } catch {
      toast(msg('transfers.downloadsFolderUnavailable'), 'danger');
    }
  }
</script>

<Card surface="panel">
<PageHeader title={msg('transfers.downloadsTitle')}>
  {#snippet actions()}
    <DropdownMenu
      items={maxActiveDownloadOptions.map((o) => ({ ...o, checked: o.id === concurrencyValue }))}
      onselect={pickConcurrency}
    >
      {#snippet trigger({ toggle })}
        <Button onclick={toggle}>
          {msg('transfers.downloadsConcurrency', { value: concurrencyValue })}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </Button>
      {/snippet}
    </DropdownMenu>
    <IconButton label={msg('transfers.downloadsSettingsLabel')} onclick={() => navigate('settings', { tab: 'downloads' })}>
      <Settings size="1.7rem" strokeWidth={1.8} />
    </IconButton>
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Plus size="1.5rem" strokeWidth={2} />
      {msg('transfers.downloadsAddAction')}
    </Button>
  {/snippet}
</PageHeader>

{#if nothing}
  <EmptyState
    title={msg('transfers.downloadsEmptyActiveTitle')}
    description={msg('transfers.downloadsEmptyActiveDescription')}
  >
    {#snippet icon()}
      <Download size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => (addOpen = true)}>
        <Plus size="1.5rem" strokeWidth={2} />
        {msg('transfers.downloadsAddAction')}
      </Button>
    {/snippet}
  </EmptyState>
{:else}
  <div class="summary">
    <span class="sum speed"><ArrowDown size="1.5rem" strokeWidth={2} />{speedBytes($stats.downSpeed)}</span>
    <span class="sum speed dim"><ArrowUp size="1.5rem" strokeWidth={2} />{speedBytes($stats.upSpeed)}</span>
    <span class="sum-sep" aria-hidden="true"></span>
    <span class="sum dim">{msg('downloads.active', { count: $stats.activeCount })}</span>
    <span class="sum dim">{msg('transfers.downloadsQueuedCount', { count: $queue.length })}</span>
  </div>

  <section class="section">
    <h2>{msg('transfers.downloadsActiveHeading')} <span class="count">{$active.length}</span></h2>
    {#if $active.length === 0}
      <p class="muted">{msg('transfers.downloadsEmptyActiveTitle')}</p>
    {:else}
      <div class="rows">
        {#each $active as download (download.id)}
          <DownloadItem {download} onopen={(d) => openDetails(d.id)} />
        {/each}
      </div>
    {/if}
  </section>
{/if}

{#if $failed.length > 0}
  <section class="section">
    <h2>{msg('transfers.downloadsFailedHeading')} <span class="count">{$failed.length}</span></h2>
    <div class="rows">
      {#each $failed as item (item.id)}
        <div class="row failed">
          <div class="thumb">
            <Artwork src={coverOf(item)} alt={item.name} ratio="3 / 4" radius="var(--radius-sm)" />
          </div>
          <div class="info">
            <button class="title link" title={item.name} onclick={() => openDetails(item.id)}>{item.name}</button>
            <span class="error-text">
              <CircleAlert size="1.4rem" strokeWidth={1.8} />
              <span>{installErrorText(item.error)}</span>
            </span>
          </div>
          <div class="row-actions">
            <Button size="sm" onclick={() => resume(item.id)}>
              <RotateCcw size="1.4rem" strokeWidth={1.8} />
              {msg('common.retry')}
            </Button>
            <IconButton label={msg('transfers.downloadsRemoveFromListLabel')} size="sm" onclick={() => remove(item.id)}>
              <X size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    </div>
  </section>
{/if}

{#if $queue.length > 0}
<section class="section">
  <h2>{msg('transfers.downloadsQueueHeading')} <span class="count">{$queue.length}</span></h2>
    <div class="rows">
      {#each $queue as q, i (q.id)}
        <div class="row">
          <div class="thumb">
            <Artwork src={coverOf(q)} alt={q.name} ratio="3 / 4" radius="var(--radius-sm)" />
          </div>
          <div class="info inline">
            <span class="title" title={q.name}>{q.name}</span>
            {#if typeTag(q) || sourceTag(q)}
              <div class="tags">
                {#if typeTag(q)}<StatusBadge kind="neutral" label={typeTag(q)} dot={false} />{/if}
                {#if sourceTag(q)}<StatusBadge kind="neutral" label={sourceTag(q)} dot={false} />{/if}
              </div>
            {/if}
          </div>
          <div class="status">
            <span class="status-main">{msg('transfers.downloadsStatusWaiting')}</span>
          </div>
          <div class="row-actions">
            <DropdownMenu
              items={[
                ...(i > 0 ? [{ id: 'up', label: msg('transfers.downloadsMoveUp') }] : []),
                ...(i < $queue.length - 1 ? [{ id: 'down', label: msg('transfers.downloadsMoveDown') }] : []),
                { id: 'start', label: msg('transfers.downloadsStartNow'), separator: i > 0 || i < $queue.length - 1 },
              ]}
              onselect={(id) => {
                if (id === 'up') moveUp(q.id);
                else if (id === 'down') moveDown(q.id);
                else forceStart(q.id);
              }}
            >
              {#snippet trigger({ toggle })}
                <IconButton label={msg('transfers.downloadsQueueActionsLabel')} size="sm" onclick={toggle}>
                  <Menu size="1.6rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
            <IconButton label={msg('transfers.downloadsCancelLabel')} size="sm" onclick={() => remove(q.id)}>
              <X size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    </div>
</section>
{/if}

{#if $completed.length > 0}
  <section class="section">
    <h2>{msg('transfers.downloadsCompletedHeading')} <span class="count">{$completed.length}</span></h2>
    <div class="rows">
      {#each $completed as item (item.id)}
        {@const install = $installationsByDownload.get(item.id)}
        <div class="row">
          <div class="thumb">
            <Artwork src={coverOf(item)} alt={item.name} ratio="3 / 4" radius="var(--radius-sm)" />
          </div>
          <div class="info inline">
            <button class="title link" title={item.name} onclick={() => openDetails(item.id)}>{item.name}</button>
            {#if typeTag(item) || sourceTag(item)}
              <div class="tags">
                {#if typeTag(item)}<StatusBadge kind="neutral" label={typeTag(item)} dot={false} />{/if}
                {#if sourceTag(item)}<StatusBadge kind="neutral" label={sourceTag(item)} dot={false} />{/if}
              </div>
            {/if}
          </div>
          <div class="status" title={msg('transfers.downloadsDoneWhen', { when: completedWhen(item.completedAt) })}>
            <span class="status-main">{bytesSize(item.total)}</span>
            <span class="status-sub">{completedWhen(item.completedAt)}</span>
          </div>
          <div class="install-cell">
            {#if !install}
              <Button size="sm" variant="primary" onclick={() => openInstall(item.id)}>{msg('transfers.downloadsInstallAction')}</Button>
            {:else if installActive(install.status)}
              <div class="install-progress">
                <span class="install-status">{installStatusLabels(install.status)}</span>
                <ProgressBar value={install.progress * 100} indeterminate={installIndeterminate(install)} height={4} />
              </div>
            {:else if install.status === 'waiting_for_user'}
              <Button size="sm" variant="primary" onclick={() => openInstall(item.id)}>{msg('transfers.downloadsContinueInstallAction')}</Button>
            {:else if install.status === 'completed'}
              <StatusBadge kind="success" label={msg('transfers.downloadsInstalledStatus')} plain />
            {:else if install.status === 'failed'}
              <Button size="sm" variant="danger" onclick={() => openInstall(item.id)}>{msg('transfers.downloadsInstallErrorAction')}</Button>
            {:else}
              <Button size="sm" onclick={() => openInstall(item.id)}>
                {install.status === 'cancelled' ? msg('transfers.downloadsCancelledStatus') : msg('transfers.downloadsInterruptedStatus')}
              </Button>
            {/if}
          </div>
          <div class="row-actions">
            <IconButton label={msg('transfers.downloadsShowInFolderLabel')} size="sm" onclick={() => openDestination(item.destination)}>
              <FolderOpen size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton label={msg('transfers.downloadsRemoveFromListLabel')} size="sm" onclick={() => remove(item.id)}>
              <X size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    </div>
  </section>
{/if}

<p class="footer-hint">
  {msg('transfers.downloadsFooterHintQuestion')}
  <button class="link" onclick={() => navigate('history')}>{msg('transfers.downloadsOpenHistoryLog')}</button>
</p>
</Card>

<AddDownloadModal bind:open={addOpen} />
<DownloadDetailsModal bind:open={detailsOpen} id={detailsId} />
<InstallModal bind:open={installOpen} downloadId={installDownloadId} />

<style>
  .summary {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    max-width: 140rem;
    margin-bottom: var(--space-6);
    padding: var(--space-3) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .sum {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-weight: 500;
    color: var(--text);
  }

  .sum.speed {
    min-width: 10.5rem;
  }

  .sum.dim {
    font-weight: 400;
    color: var(--text-2);
  }

  .sum :global(svg) {
    color: var(--accent-text);
  }

  .sum.dim :global(svg) {
    color: var(--text-3);
  }

  .sum-sep {
    width: 1px;
    height: 1.8rem;
    background: var(--border-strong);
  }

  .section {
    margin-bottom: var(--space-8);
    max-width: 140rem;
  }

  .section h2 {
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    font-size: var(--font-lg);
    margin-bottom: var(--space-3);
  }

  .count {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    min-height: 6.4rem;
    padding: var(--space-2) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    transition: border-color var(--dur) var(--ease);
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .row:hover {
    border-color: var(--border-strong);
  }

  .row.failed {
    border-color: color-mix(in srgb, var(--danger) 35%, var(--border));
  }

  .error-text {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    max-width: 100%;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .error-text span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .error-text :global(svg) {
    flex-shrink: 0;
  }

  .thumb {
    width: 3.6rem;
    flex-shrink: 0;
  }

  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }

  .info.inline {
    flex-direction: row;
    align-items: center;
    gap: var(--space-3);
  }

  .tags {
    flex-shrink: 0;
  }

  .title {
    min-width: 0;
    max-width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .title.link {
    text-align: left;
    transition: color var(--dur) var(--ease);
  }

  .title.link:hover {
    color: var(--accent-text);
  }

  .tags {
    display: flex;
    gap: 0.6rem;
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    flex-shrink: 0;
    width: 17rem;
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
  }

  .status-main {
    color: var(--text-2);
  }

  .status-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .row-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  .install-cell {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    width: 17rem;
    flex-shrink: 0;
  }

  .install-progress {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    width: 100%;
  }

  .install-status {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: right;
  }

  .footer-hint {
    margin-top: var(--space-6);
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .footer-hint .link {
    color: var(--accent-text);
  }

  .footer-hint .link:hover {
    text-decoration: underline;
  }


  @media (max-width: 1300px) {
    .status {
      display: none;
    }
  }
</style>
