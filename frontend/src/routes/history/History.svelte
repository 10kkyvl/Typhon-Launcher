<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import {
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
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { clearHistoryPrompt } from '../../lib/confirm/prompts';
  import { filterHistory } from '../../lib/history/historyFilter';
  import { historyLabel, historyWhen } from '../../lib/history/historyText';
  import { Kind, type Record as HistoryRecord } from '../../lib/services/history';
  import { libraryGames } from '../../lib/stores/library';
  import {
    clearHistory,
    history,
    historyErrorText,
    historyLoaded,
    historyLoadError,
    historyStatus,
    reloadHistory,
  } from '../../lib/stores/history';
  import { toast } from '../../lib/stores/toasts';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { longDate, relativeDate } from '../../lib/utils/format';
  import { locale, msg } from '../../lib/i18n';

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
  let retrying = $state(false);
  let list = $state<HTMLElement>();

  const loadFailed = $derived(!$historyLoaded && $historyLoadError != null);
  const loading = $derived(!$historyLoaded && !loadFailed);

  const kindsFilter = $derived(segments.find((s) => s.id === segment)?.kinds);
  const filtered = $derived(filterHistory($history, { kinds: kindsFilter, query, locale: $locale }));

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

  const consoleOf = $derived(
    new Map($libraryGames.flatMap((game) => (game.emulation?.platform ? [[game.id, game.emulation.platform] as const] : []))),
  );

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

  const clearButton = () => document.querySelector<HTMLButtonElement>('[data-history-clear]');
  const pageHeading = () => document.querySelector<HTMLElement>('h1[tabindex="-1"]');

  // A refusal rejects: the dialog stays open and says why (errorText).
  async function confirmClear() {
    await clearHistory();
    toast(msg('transfers.historyCleared'), 'success');
  }

  // The dialog hands focus back to Clear; once the list is empty Clear is disabled and cannot take it,
  // so focus goes on to the page heading.
  function closeClear() {
    const handOff = focusHandOff(() => {
      const button = clearButton();
      return button?.disabled ? button : null;
    });
    clearOpen = false;
    void handOff(pageHeading);
  }
</script>

<Page>
  <PageHeader title={msg('transfers.historyTitle')} subtitle={msg('transfers.historySubtitle')}>
    {#snippet actions()}
      <Button variant="ghost" disabled={$history.length === 0} data-history-clear onclick={() => (clearOpen = true)}>
        <Trash2 size="1.5rem" strokeWidth={1.8} />
        {msg('transfers.historyClearAction')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if $historyStatus.degraded}
    <div class="banner">
      <Notice tone="danger" announce={false}>{msg('transfers.historyDegradedBanner', { message: $historyStatus.message })}</Notice>
    </div>
  {/if}

  {#if loadFailed || retrying}
    <!-- On success focus that was on Retry goes to the first control of the list, else the page heading. -->
    <div class="banner">
      <Notice tone="danger" bind:retrying retry={{ label: msg('common.retry'), run: reloadHistory, within: () => list }}>
        {historyErrorText($historyLoadError, msg('transfers.historyLoadFailed'))}
      </Notice>
    </div>
  {/if}

  {#if $history.length === 0}
    {#if loading}
      <EmptyState level={2} title={msg('transfers.historyLoading')} />
    {:else if !loadFailed && !retrying}
      <EmptyState level={2} title={msg('transfers.historyEmptyTitle')} description={msg('transfers.historyEmptyDescription')}>
        {#snippet icon()}
          <HistoryIcon size="2rem" strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
    {/if}
  {:else}
    <div class="toolbar">
      <SegmentedControl options={tabs} label={msg('transfers.historySegmentLabel')} variant="pill" bind:value={segment} />
      <div class="search-slot">
        <SearchInput bind:value={query} label={msg('transfers.historySearchPlaceholder')} placeholder={msg('transfers.historySearchPlaceholder')} />
      </div>
    </div>

    {#if filtered.length === 0}
      <EmptyState level={2} title={msg('transfers.historyNoResultsTitle')} description={msg('transfers.historyNoResultsDescription')} />
    {:else}
      {#key segment}
        <div class="days" bind:this={list}>
          {#each days as day (day.key)}
            <section class="day">
              <h2 class="day-label">{day.label}</h2>
              <div class="table">
                {#each day.records as record (record.id)}
                  {@const label = historyLabel(record)}
                  {@const Icon = iconFor(record.kind)}
                  {@const when = historyWhen(record.at)}
                  <div class="row" class:failed={failed(record.kind)}>
                    <span class="icon-cell"><Icon size="1.7rem" strokeWidth={1.8} /></span>
                    <div class="body">
                      <div class="head">
                        <span class="title">{label.title}</span>
                        {#if record.gameId && consoleOf.has(record.gameId)}<ConsoleTag platform={consoleOf.get(record.gameId) ?? ''} />{/if}
                      </div>
                      {#if label.detail}
                        <span class="detail">{label.detail}</span>
                      {/if}
                    </div>
                    <span class="when">
                      {#if when}
                        <!-- The exact time goes to assistive tech as text and to the pointer as a hint; the time element
                             takes focus only from the pointer, so the row adds no Tab stop for it. -->
                        <Tooltip text={when.exact} describe={false}>
                          <time datetime={record.at} tabindex="-1"><span aria-hidden="true">{when.words}</span><span class="sr-only">{when.exact}</span></time>
                        </Tooltip>
                      {/if}
                    </span>
                  </div>
                {/each}
              </div>
            </section>
          {/each}
        </div>
      {/key}
    {/if}
  {/if}

  {#if clearOpen}
    <ConfirmModal prompt={clearHistoryPrompt()} onconfirm={confirmClear} errorText={(err) => historyErrorText(err)} onclose={closeClear} />
  {/if}
</Page>

<style>
  .banner {
    margin-bottom: var(--space-5);
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

  .head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }

  /* Shown whole, wrapping in the row: an ellipsis would hide words that no keyboard could reach. */
  .title {
    min-width: 0;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
    overflow-wrap: anywhere;
  }

  .detail {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .when {
    text-align: right;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
</style>
