<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { Activity, LifeBuoy, Plus } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { msg } from '../../lib/i18n';
  import { openStatusPage } from '../../lib/services/account';
  import { listTickets, type Ticket } from '../../lib/services/support';
  import { supportErrorText } from '../../lib/services/supportMessages';
  import { statusView, topicLabel } from '../../lib/support/topics';
  import { navigate } from '../../lib/stores/router';
  import { unreadReplies } from '../../lib/stores/support';
  import { toast } from '../../lib/stores/toasts';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { relativeDate } from '../../lib/utils/format';

  let tickets = $state<Ticket[]>([]);
  let next = $state('');
  let loading = $state(true);
  let loadingMore = $state(false);
  let failure = $state('');
  // Which request the failure is about, so Retry asks for that one again.
  let failedPage: 'first' | 'refresh' | 'more' | null = null;
  let loaded = $state(false);
  let retrying = $state(false);
  let list = $state<HTMLElement>();
  let firstNew = -1;
  // Pages on screen, and a refresh a reply asked for while a Retry or a later page was on its way.
  let pages = 0;
  let refreshPending = false;
  // The refresh on its way, by its place in `run`; 0 when none is.
  let refreshRun = 0;
  let run = 0;

  // A failure stays on screen until an answer replaces it: a refresh that starts does not clear it. A
  // refresh (`keepLater`) puts page one in again and keeps the pages loaded after it.
  async function load(keepLater = false): Promise<boolean> {
    const current = ++run;
    refreshRun = keepLater ? current : 0;
    try {
      const page = await listTickets('');
      if (current !== run) return false;
      if (keepLater && pages > 1) {
        const fresh = new Set(page.tickets.map((item) => item.number));
        tickets = [...page.tickets, ...tickets.filter((item) => !fresh.has(item.number))];
      } else {
        tickets = page.tickets;
        next = page.next;
        pages = 1;
      }
      loaded = true;
      failure = '';
      failedPage = null;
      return true;
    } catch (err) {
      if (current !== run) return false;
      failure = supportErrorText(err, msg('tickets.loadFailed'));
      failedPage = keepLater ? 'refresh' : 'first';
      return false;
    } finally {
      if (current === run) loading = false;
      if (refreshRun === current) refreshRun = 0;
    }
  }

  async function more(): Promise<boolean> {
    if (loadingMore || !next) return false;
    // A refresh on its way is overtaken here and its answer dropped: it is owed again once this page
    // has answered.
    if (refreshRun) {
      refreshRun = 0;
      refreshPending = true;
    }
    const current = ++run;
    loadingMore = true;
    try {
      const page = await listTickets(next);
      if (current !== run) return false;
      const known = new Set(tickets.map((item) => item.number));
      tickets = [...tickets, ...page.tickets.filter((item) => !known.has(item.number))];
      next = page.next;
      pages += 1;
      failure = '';
      failedPage = null;
      return true;
    } catch (err) {
      if (current !== run) return false;
      failure = supportErrorText(err, msg('tickets.loadFailed'));
      failedPage = 'more';
      return false;
    } finally {
      loadingMore = false;
      settle();
    }
  }

  // The refresh a reply asked for while it had to wait runs once the request it waited for answers.
  function settle() {
    if (!refreshPending || loadingMore) return;
    refreshPending = false;
    void load(true);
  }

  // Retry repeats the request that failed: a later page is asked for again and the pages on screen
  // stay; on success focus goes to the first request it brought.
  async function retry(): Promise<boolean> {
    try {
      if (failedPage === 'more') {
        firstNew = tickets.length;
        return await more();
      }
      firstNew = -1;
      return await load(failedPage === 'refresh');
    } finally {
      settle();
    }
  }

  // The last page takes "Load more" away: focus it held goes to the first request that page brought.
  async function showMore() {
    const from = tickets.length;
    const hand = focusHandOff();
    if (await more()) void hand(() => list?.querySelectorAll<HTMLElement>('button.row')[from]);
  }

  function retryFocus(): HTMLElement | undefined {
    return firstNew < 0 ? undefined : list?.querySelectorAll<HTMLElement>('button.row')[firstNew];
  }

  onMount(() => {
    void load();
    return () => {
      run += 1;
    };
  });

  let lastUnread: number | null = null;
  $effect(() => {
    const count = $unreadReplies;
    untrack(() => {
      const changed = lastUnread !== null && lastUnread !== count;
      lastUnread = count;
      if (!changed || !loaded) return;
      // A retry or a later page on its way is not overtaken: its notice stays and its answer is kept;
      // the refresh runs once it has answered.
      if (loadingMore || retrying) refreshPending = true;
      else void load(true);
    });
  });

  function open(item: Ticket) {
    navigate('support', { ticket: String(item.number) });
  }

  function newTicket() {
    navigate('support', { view: 'new' });
  }

  async function openStatus() {
    try {
      await openStatusPage();
    } catch (err) {
      console.error('open status page', err);
      toast(msg('tickets.statusOpenFailed'), 'danger');
    }
  }
</script>

<PageHeader title={msg('tickets.title')} subtitle={msg('tickets.subtitle')}>
  {#snippet actions()}
    <Button variant="ghost" onclick={openStatus}>
      <Activity size="1.5rem" strokeWidth={1.8} />
      {msg('tickets.statusPage')}
    </Button>
    <Button variant="primary" onclick={newTicket}>
      <Plus size="1.5rem" strokeWidth={1.8} />
      {msg('tickets.new')}
    </Button>
  {/snippet}
</PageHeader>

{#if failure || retrying}
  <div class="banner">
    <Notice tone="danger" bind:retrying retry={{ label: msg('common.retry'), run: retry, focus: retryFocus, within: () => list }}>
      {failure}
    </Notice>
  </div>
{/if}

{#if loading}
  <Spinner label={msg('tickets.loading')} />
{:else if tickets.length === 0 && !failure}
  <EmptyState level={2} title={msg('tickets.emptyTitle')} description={msg('tickets.emptyText')}>
    {#snippet icon()}
      <LifeBuoy size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={newTicket}>{msg('tickets.new')}</Button>
    {/snippet}
  </EmptyState>
{:else}
  <ul class="list" bind:this={list}>
    {#each tickets as item (item.number)}
      {@const status = statusView(item.status)}
      <li>
        <button class="row" class:unread={item.unread} onclick={() => open(item)}>
          <span class="dot" class:on={item.unread} role={item.unread ? 'img' : undefined} aria-label={item.unread ? msg('tickets.unreadMark') : undefined}></span>
          <span class="main">
            <span class="subject" dir="auto">{item.subject}</span>
            <span class="meta">
              <span class="ref">{item.ref}</span>
              <span>{topicLabel(item.topic)}</span>
              <span>{msg('tickets.updated', { when: relativeDate(item.updatedAt) })}</span>
            </span>
          </span>
          {#if status}
            <StatusBadge kind={status.kind} label={status.label} />
          {/if}
        </button>
      </li>
    {/each}
  </ul>
  {#if next}
    <div class="more">
      <Button busy={loadingMore} onclick={showMore}>
        {loadingMore ? msg('tickets.loadingMore') : msg('tickets.loadMore')}
      </Button>
    </div>
  {/if}
{/if}

<style>
  .banner {
    margin-bottom: var(--space-4);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    width: 100%;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    text-align: left;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
    border-color: var(--border-strong);
  }

  .dot {
    flex-shrink: 0;
    width: 0.9rem;
    height: 0.9rem;
    border-radius: 50%;
    background: transparent;
  }

  .dot.on {
    background: var(--accent-bright);
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .subject {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text);
  }

  .unread .subject {
    font-weight: 600;
  }

  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem var(--space-3);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .ref {
    font-variant-numeric: tabular-nums;
    color: var(--text-2);
  }

  .more {
    display: flex;
    justify-content: center;
    margin-top: var(--space-5);
  }
</style>
