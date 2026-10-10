<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { CircleAlert, LifeBuoy, Plus } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { msg } from '../../lib/i18n';
  import { listTickets, type Ticket } from '../../lib/services/support';
  import { supportErrorText } from '../../lib/services/supportMessages';
  import { statusView, topicLabel } from '../../lib/support/topics';
  import { navigate } from '../../lib/stores/router';
  import { unreadReplies } from '../../lib/stores/support';
  import { relativeDate } from '../../lib/utils/format';

  let tickets = $state<Ticket[]>([]);
  let next = $state('');
  let loading = $state(true);
  let loadingMore = $state(false);
  let failure = $state('');
  let loaded = $state(false);
  let run = 0;

  async function load() {
    const current = ++run;
    failure = '';
    try {
      const page = await listTickets('');
      if (current !== run) return;
      tickets = page.tickets;
      next = page.next;
      loaded = true;
    } catch (err) {
      if (current !== run) return;
      failure = supportErrorText(err, msg('tickets.loadFailed'));
    } finally {
      if (current === run) loading = false;
    }
  }

  async function more() {
    if (loadingMore || !next) return;
    const current = ++run;
    loadingMore = true;
    failure = '';
    try {
      const page = await listTickets(next);
      if (current !== run) return;
      const known = new Set(tickets.map((item) => item.number));
      tickets = [...tickets, ...page.tickets.filter((item) => !known.has(item.number))];
      next = page.next;
    } catch (err) {
      if (current !== run) return;
      failure = supportErrorText(err, msg('tickets.loadFailed'));
    } finally {
      loadingMore = false;
    }
  }

  function retry() {
    loading = tickets.length === 0;
    void load();
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
      if (changed && loaded && !loadingMore) void load();
    });
  });

  function open(item: Ticket) {
    navigate('support', { ticket: String(item.number) });
  }

  function newTicket() {
    navigate('support', { view: 'new' });
  }
</script>

<Card surface="panel">
  <PageHeader title={msg('tickets.title')} subtitle={msg('tickets.subtitle')}>
    {#snippet actions()}
      <Button variant="primary" onclick={newTicket}>
        <Plus size="1.5rem" strokeWidth={1.8} />
        {msg('tickets.new')}
      </Button>
    {/snippet}
  </PageHeader>

  {#if failure}
    <div class="banner" role="alert">
      <CircleAlert size="1.6rem" strokeWidth={1.8} />
      <span class="banner-text">{failure}</span>
      <Button size="sm" onclick={retry}>{msg('common.retry')}</Button>
    </div>
  {/if}

  {#if loading}
    <p class="muted">{msg('tickets.loading')}</p>
  {:else if tickets.length === 0 && !failure}
    <EmptyState title={msg('tickets.emptyTitle')} description={msg('tickets.emptyText')}>
      {#snippet icon()}
        <LifeBuoy size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={newTicket}>{msg('tickets.new')}</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <ul class="list">
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
        <Button disabled={loadingMore} onclick={more}>
          {loadingMore ? msg('tickets.loadingMore') : msg('tickets.loadMore')}
        </Button>
      </div>
    {/if}
  {/if}
</Card>

<style>
  .muted {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    padding: 0.9rem 1.2rem;
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
    color: var(--danger);
    font-size: var(--font-sm);
  }

  .banner :global(svg) {
    flex-shrink: 0;
  }

  .banner-text {
    flex: 1;
    min-width: 0;
    line-height: 1.4;
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
    background: var(--accent);
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
