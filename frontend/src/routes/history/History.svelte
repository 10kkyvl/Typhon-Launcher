<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { History as HistoryIcon, Trash2 } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { clearHistoryPrompt } from '../../lib/confirm/prompts';
  import { filterHistory } from '../../lib/history/historyFilter';
  import { historyGames } from '../../lib/history/historyGame';
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
  import HistoryDay from './HistoryDay.svelte';

  // Every known kind is under one segment besides All: a download and a copy from the local network
  // brought a game in, as an install did.
  const segments: { id: string; label: string; kinds?: Kind[] }[] = [
    { id: 'all', label: msg('transfers.historySegmentAll') },
    {
      id: 'installs',
      label: msg('transfers.historySegmentInstalls'),
      kinds: [Kind.KindInstalled, Kind.KindInstallFailed, Kind.KindDownloaded, Kind.KindLanReceived],
    },
    {
      id: 'updates',
      label: msg('transfers.historySegmentUpdates'),
      kinds: [Kind.KindUpdated, Kind.KindUpdateFailed, Kind.KindRolledBack],
    },
    { id: 'removals', label: msg('transfers.historySegmentRemovals'), kinds: [Kind.KindRemoved, Kind.KindUninstalled] },
    { id: 'moves', label: msg('transfers.historySegmentMoves'), kinds: [Kind.KindMoved] },
  ];

  const uid = $props.id();

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

  function dayStart(date: Date) {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  }

  function dayLabel(iso: string, date: Date) {
    const age = dayStart(new Date()) - dayStart(date);
    return age < 2 * 86_400_000 ? relativeDate(iso) : longDate(date);
  }

  const gameOf = $derived(historyGames($libraryGames));

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

{#snippet count()}
  <span>{msg('transfers.historyCount', { count: $history.length })}</span>
{/snippet}

<Page>
  <PageHeader title={msg('transfers.historyTitle')} subtitle={msg('transfers.historySubtitle')} meta={$history.length > 0 ? count : undefined}>
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
      <div class="days" bind:this={list}>
        {#each days as day (day.key)}
          <HistoryDay id="{uid}-day-{day.key}" label={day.label} records={day.records} {gameOf} />
        {/each}
      </div>
    {/if}
  {/if}

  {#if clearOpen}
    <ConfirmModal prompt={clearHistoryPrompt()} onconfirm={confirmClear} errorText={(err) => historyErrorText(err)} onclose={closeClear} />
  {/if}
</Page>

<style>
  .banner {
    max-width: 110rem;
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
    gap: var(--space-8);
    max-width: 110rem;
  }
</style>
