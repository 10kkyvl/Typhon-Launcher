<script lang="ts">
  import {
    ChevronDown,
    CircleCheck,
    Download,
    FolderOpen,
    Menu,
    Plus,
    Settings,
    X,
  } from '@lucide/svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
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
  import { active, completed, forceStart, moveDown, moveUp, queue, remove, stats } from '../../lib/stores/downloads';
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

  const summary = $derived(
    [
      `↓ ${speedBytes($stats.downSpeed)}`,
      `↑ ${speedBytes($stats.upSpeed)}`,
      msg('downloads.active', { count: $stats.activeCount }),
      msg('transfers.downloadsQueuedCount', { count: $queue.length }),
    ].join(' · '),
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
    const ids = [...$queue, ...$completed]
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

<PageHeader title={msg('transfers.downloadsTitle')} subtitle={summary}>
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

<section class="section">
  <h2>{msg('transfers.downloadsActiveHeading')} <span class="count">{$active.length}</span></h2>
  {#if $active.length === 0}
    <EmptyState
      title={msg('transfers.downloadsEmptyActiveTitle')}
      description={msg('transfers.downloadsEmptyActiveDescription')}
    >
      {#snippet icon()}
        <Download size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (addOpen = true)}>{msg('transfers.downloadsAddAction')}</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="rows">
      {#each $active as download (download.id)}
        <DownloadItem {download} onopen={(d) => openDetails(d.id)} />
      {/each}
    </div>
  {/if}
</section>

<section class="section">
  <h2>{msg('transfers.downloadsQueueHeading')} <span class="count">{$queue.length}</span></h2>
  {#if $queue.length === 0}
    <p class="muted">{msg('transfers.downloadsQueueEmpty')}</p>
  {:else}
    <div class="rows">
      {#each $queue as q, i (q.id)}
        <div class="row">
          <div class="thumb">
            <Artwork src={coverOf(q)} alt={q.name} ratio="3 / 4" radius="var(--radius-sm)" />
          </div>
          <div class="info">
            <span class="title">{q.name}</span>
            {#if typeTag(q) || sourceTag(q)}
              <div class="tags">
                {#if typeTag(q)}<StatusBadge kind="neutral" label={typeTag(q)} dot={false} />{/if}
                {#if sourceTag(q)}<StatusBadge kind="neutral" label={sourceTag(q)} dot={false} />{/if}
              </div>
            {/if}
          </div>
          <div class="status">
            <span class="status-main">{msg('transfers.downloadsStatusQueued')}</span>
            <span class="status-sub">{msg('transfers.downloadsStatusWaiting')}</span>
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
  {/if}
</section>

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
          <div class="info">
            <button class="title link" onclick={() => openDetails(item.id)}>{item.name}</button>
            {#if typeTag(item) || sourceTag(item)}
              <div class="tags">
                {#if typeTag(item)}<StatusBadge kind="neutral" label={typeTag(item)} dot={false} />{/if}
                {#if sourceTag(item)}<StatusBadge kind="neutral" label={sourceTag(item)} dot={false} />{/if}
              </div>
            {/if}
          </div>
          <div class="status">
            <span class="status-main">{msg('transfers.downloadsDoneSize', { size: bytesSize(item.total) })}</span>
            <span class="status-sub">{msg('transfers.downloadsDoneWhen', { when: completedWhen(item.completedAt) })}</span>
          </div>
          <span class="done-check" aria-hidden="true">
            <CircleCheck size="1.8rem" strokeWidth={1.8} />
          </span>
          <div class="install-cell">
            {#if !install}
              <Button size="sm" variant="primary" onclick={() => openInstall(item.id)}>{msg('transfers.downloadsInstallAction')}</Button>
            {:else if installActive(install.status)}
              <div class="install-progress">
                <span class="install-status">{installStatusLabels(install.status)}</span>
                <ProgressBar value={install.progress * 100} height={4} />
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

<AddDownloadModal bind:open={addOpen} />
<DownloadDetailsModal bind:open={detailsOpen} id={detailsId} />
<InstallModal bind:open={installOpen} downloadId={installDownloadId} />

<style>
  .section {
    margin-bottom: var(--space-10);
    max-width: 140rem;
  }

  .section h2 {
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    font-size: var(--font-xl);
    margin-bottom: var(--space-4);
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
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .thumb {
    width: 5.6rem;
    flex-shrink: 0;
  }

  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .title {
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
    min-width: 15rem;
    font-size: var(--font-sm);
  }

  .status-main {
    color: var(--text-2);
  }

  .status-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .done-check {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--success);
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
    min-width: 15rem;
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
    text-align: center;
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
