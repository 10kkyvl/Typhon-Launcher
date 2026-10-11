<script lang="ts">
  import {
    CircleAlert,
    Download,
    History as HistoryIcon,
    Move,
    PackageCheck,
    PackageX,
    RefreshCw,
    RotateCcw,
    Trash2,
    Wifi,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { clearHistoryPrompt } from '../../lib/confirm/prompts';
  import { filterHistory } from '../../lib/history/historyFilter';
  import { historyLabel } from '../../lib/history/historyText';
  import { Kind, type Record as HistoryRecord } from '../../lib/services/history';
  import { clearHistory, history, historyErrorText, historyStatus } from '../../lib/stores/history';
  import { toast } from '../../lib/stores/toasts';
  import { clockTime, longDate, relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  const segments: { id: string; label: string; kinds?: Kind[] }[] = [
    { id: 'all', label: msg('transfers.historySegmentAll') },
    { id: 'installs', label: msg('transfers.historySegmentInstalls'), kinds: [Kind.KindInstalled, Kind.KindInstallFailed] },
    {
      id: 'updates',
      label: msg('transfers.historySegmentUpdates'),
      kinds: [Kind.KindUpdated, Kind.KindUpdateFailed, Kind.KindRolledBack],
    },
    { id: 'removals', label: msg('transfers.historySegmentRemovals'), kinds: [Kind.KindRemoved, Kind.KindUninstalled] },
  ];


  let segment = $state('all');
  let query = $state('');
  let clearOpen = $state(false);

  const kindsFilter = $derived(segments.find((s) => s.id === segment)?.kinds);
  const filtered = $derived(filterHistory($history, { kinds: kindsFilter, query }));

  const tabs = $derived(
    segments.map((s) => ({
      id: s.id,
      label: s.label,
      count: s.kinds ? $history.filter((r) => s.kinds!.includes(r.kind)).length : $history.length,
    })),
  );

  const icons: Record<Kind, typeof HistoryIcon> = {
    [Kind.$zero]: HistoryIcon,
    [Kind.KindInstalled]: PackageCheck,
    [Kind.KindInstallFailed]: PackageX,
    [Kind.KindUpdated]: RefreshCw,
    [Kind.KindUpdateFailed]: PackageX,
    [Kind.KindRolledBack]: RotateCcw,
    [Kind.KindDownloaded]: Download,
    [Kind.KindUninstalled]: Trash2,
    [Kind.KindRemoved]: Trash2,
    [Kind.KindMoved]: Move,
    [Kind.KindLanReceived]: Wifi,
  };

  function iconFor(kind: Kind) {
    return icons[kind] ?? HistoryIcon;
  }

  function dayStart(date: Date) {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  }

  function dayLabel(iso: string, date: Date) {
    const age = dayStart(new Date()) - dayStart(date);
    return age < 2 * 86_400_000 ? relativeDate(iso) : longDate(date);
  }

  function timeOf(iso: string) {
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? '' : clockTime(date);
  }

  const days = $derived.by(() => {
    const groups: { key: string; label: string; records: HistoryRecord[] }[] = [];
    for (const record of filtered) {
      const date = new Date(record.at);
      const valid = !Number.isNaN(date.getTime());
      const key = valid ? String(dayStart(date)) : 'unknown';
      const last = groups[groups.length - 1];
      if (last && last.key === key) {
        last.records.push(record);
      } else {
        groups.push({ key, label: valid ? dayLabel(record.at, date) : relativeDate(record.at), records: [record] });
      }
    }
    return groups;
  });

  function failed(kind: Kind) {
    return kind === Kind.KindInstallFailed || kind === Kind.KindUpdateFailed;
  }

  async function confirmClear() {
    try {
      await clearHistory();
      toast(msg('transfers.historyCleared'), 'success');
    } catch (err) {
      toast(historyErrorText(err), 'danger');
    }
  }
</script>

<Card surface="panel">
  <PageHeader title={msg('transfers.historyTitle')} subtitle={msg('transfers.historySubtitle')}>
    {#snippet actions()}
      <Button variant="ghost" disabled={$history.length === 0} onclick={() => (clearOpen = true)}>
        <Trash2 size="1.5rem" strokeWidth={1.8} />
        {msg('transfers.historyClearAction')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if $historyStatus.degraded}
    <div class="banner">
      <span class="icon"><CircleAlert size="1.6rem" strokeWidth={1.8} /></span>
      <span class="text">{msg('transfers.historyDegradedBanner', { message: $historyStatus.message })}</span>
    </div>
  {/if}

  {#if $history.length === 0}
    <EmptyState title={msg('transfers.historyEmptyTitle')} description={msg('transfers.historyEmptyDescription')}>
      {#snippet icon()}
        <HistoryIcon size="2rem" strokeWidth={1.8} />
      {/snippet}
    </EmptyState>
  {:else}
    <div class="toolbar">
      <Tabs {tabs} bind:value={segment} variant="pill" />
      <div class="search-slot">
        <SearchInput bind:value={query} placeholder={msg('transfers.historySearchPlaceholder')} />
      </div>
    </div>

    {#if filtered.length === 0}
      <EmptyState title={msg('transfers.historyNoResultsTitle')} description={msg('transfers.historyNoResultsDescription')} />
    {:else}
      {#key segment}
        <div class="days">
          {#each days as day (day.key)}
            <section class="day">
              <h2 class="day-label">{day.label}</h2>
              <div class="table">
                {#each day.records as record (record.id)}
                  {@const label = historyLabel(record)}
                  {@const Icon = iconFor(record.kind)}
                  <div class="row" class:failed={failed(record.kind)}>
                    <span class="icon-cell"><Icon size="1.7rem" strokeWidth={1.8} /></span>
                    <div class="body">
                      <span class="title" title={label.title}>{label.title}</span>
                      {#if label.detail}
                        <span class="detail" title={label.detail}>{label.detail}</span>
                      {/if}
                    </div>
                    <span class="when" title={relativeDate(record.at)}>{timeOf(record.at)}</span>
                  </div>
                {/each}
              </div>
            </section>
          {/each}
        </div>
      {/key}
    {/if}
  {/if}
</Card>

{#if clearOpen}
  <ConfirmModal prompt={clearHistoryPrompt()} onconfirm={confirmClear} onclose={() => (clearOpen = false)} />
{/if}

<style>
  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    margin-bottom: var(--space-5);
    background: var(--danger-subtle);
    border-radius: var(--radius-md);
  }

  .banner .icon {
    display: flex;
    flex-shrink: 0;
    color: var(--danger);
  }

  .banner .text {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-4);
    flex-wrap: wrap;
    max-width: 110rem;
    margin-bottom: var(--space-6);
  }

  .search-slot {
    flex: 1;
    min-width: 22rem;
    max-width: 32rem;
  }

  .days {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    max-width: 110rem;
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .day-label {
    margin-bottom: var(--space-2);
    padding: 0 1.2rem;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
  }

  .table {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
    overflow: hidden;
  }

  .row {
    display: grid;
    grid-template-columns: 3.6rem minmax(0, 1fr) 6rem;
    align-items: center;
    gap: var(--space-4);
    min-height: 5.6rem;
    padding: 0.8rem 1.6rem 0.8rem 1.2rem;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .icon-cell {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .row.failed .icon-cell {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .row.failed .detail {
    color: var(--danger);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .detail {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .when {
    text-align: right;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
</style>
