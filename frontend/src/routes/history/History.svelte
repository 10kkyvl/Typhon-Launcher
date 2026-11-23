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
  import { historyArt, historyGames } from '../../lib/history/historyGame';
  import { Kind, type Record as HistoryRecord } from '../../lib/services/history';
  import { libraryGames } from '../../lib/stores/library';
  import { requestArt } from '../../lib/stores/metadata';
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
  import { focusableIn } from '../../lib/layers/focus';
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

  // A long history comes a page at a time, as on the prototype.
  const PAGE = 60;

  const uid = $props.id();

  let segment = $state('all');
  let query = $state('');
  let clearOpen = $state(false);
  let retrying = $state(false);
  let list = $state<HTMLElement>();

  const loadFailed = $derived(!$historyLoaded && $historyLoadError != null);
  const loading = $derived(!$historyLoaded && !loadFailed);
  // The history the page shows: none until a whole list was read. A record Go sends before that stays in
  // the store and comes with the list, but alone it is not the history, so nothing counts or lists it.
  const records = $derived($historyLoaded ? $history : []);

  const kindsFilter = $derived(segments.find((s) => s.id === segment)?.kinds);
  const filtered = $derived(filterHistory(records, { kinds: kindsFilter, query, locale: $locale }));

  // How far "Show more" went. Every change of the segment or the query starts again, a change back
  // to the filter it was pressed under included.
  let limit = $state(PAGE);
  const shown = $derived(filtered.slice(0, limit));

  function pickSegment(next: string) {
    if (next === segment) return;
    segment = next;
    limit = PAGE;
  }

  function setQuery(next: string) {
    if (next === query) return;
    query = next;
    limit = PAGE;
  }

  const tabs = $derived(
    segments.map((s) => ({
      id: s.id,
      label: s.label,
      count: s.kinds ? records.filter((r) => s.kinds!.includes(r.kind)).length : records.length,
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
    for (const record of shown) {
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

  // The covers of the records shown, through the art store's one request path; a key it already holds
  // or has just asked for is not asked again.
  $effect(() => {
    const ids = [...new Set(shown.map((record) => historyArt(record, gameOf(record.gameId), {}).key).filter(Boolean))];
    if (ids.length > 0) requestArt(ids);
  });

  /**
   * The next page. Show more keeps focus while records are left; the press that brings the last of them
   * takes the button away, and focus goes to the first record it brought that has a control (Tab goes on
   * through the new ones from there), else to the heading of that record's day.
   */
  async function showMore() {
    const fresh = new Set(filtered.slice(shown.length, shown.length + PAGE).map((record) => record.id));
    const handOff = focusHandOff();
    limit += PAGE;
    const picked: { target: HTMLElement | null } = { target: null };
    await handOff(() => (picked.target = firstFresh(fresh)));
    const { target } = picked;
    if (target && document.activeElement === target) {
      const box = target.closest<HTMLElement>('[data-history-row]') ?? target;
      if (typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    }
  }

  function firstFresh(fresh: ReadonlySet<string>): HTMLElement | null {
    const rows = Array.from(list?.querySelectorAll<HTMLElement>('[data-history-row]') ?? []).filter((row) => fresh.has(row.dataset.historyRow ?? ''));
    for (const row of rows) {
      const control = focusableIn(row)[0];
      if (control) return control;
    }
    return rows[0]?.closest('section')?.querySelector<HTMLElement>('h2[tabindex="-1"]') ?? null;
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

{#snippet count()}
  <span>{msg('transfers.historyCount', { count: records.length })}</span>
{/snippet}

<Page>
  <PageHeader title={msg('transfers.historyTitle')} subtitle={msg('transfers.historySubtitle')} meta={records.length > 0 ? count : undefined}>
    {#snippet actions()}
      <Button variant="ghost" disabled={records.length === 0} data-history-clear onclick={() => (clearOpen = true)}>
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

  {#if records.length === 0}
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
      <SegmentedControl options={tabs} label={msg('transfers.historySegmentLabel')} variant="pill" bind:value={() => segment, pickSegment} />
      <div class="search-slot">
        <SearchInput bind:value={() => query, setQuery} label={msg('transfers.historySearchPlaceholder')} placeholder={msg('transfers.historySearchPlaceholder')} />
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
      {#if filtered.length > shown.length}
        <div class="more">
          <Button onclick={showMore}>{msg('transfers.historyShowMore')}</Button>
          <span class="shown" role="status">{msg('transfers.historyShownOf', { shown: shown.length, total: filtered.length })}</span>
        </div>
      {/if}
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

  .more {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-4);
    margin-top: var(--space-6);
  }

  .shown {
    font-size: var(--font-sm);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }
</style>
