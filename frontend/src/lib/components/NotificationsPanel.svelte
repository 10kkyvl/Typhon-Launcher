<script lang="ts">
  import { Bell } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { navigate, route } from '../stores/router';
  import { markAllRead, notifications, openHistory, type Notification } from '../stores/notifications';
  import { msg } from '../i18n';
  import IconButton from './IconButton.svelte';
  import Popover from './Popover.svelte';

  /**
   * The top bar's notifications: the bell and, under it, the unread notifications with "mark all read"
   * and the way to the full history. Pressing a notification closes the panel and opens its page.
   */
  let open = $state(false);

  // The panel belongs to the page it was opened on: a route change, whatever made it, closes it.
  const routeKey = $derived($route.key);
  $effect(() => {
    void routeKey;
    untrack(() => (open = false));
  });

  function openNotification(item: Notification, close: () => void) {
    close();
    navigate(item.route, item.params);
    item.onOpen?.();
  }
</script>

<Popover bind:open label={msg('ui.notifications')} placement="bottom-end" width="32rem" partId="notifications">
  {#snippet trigger({ open, props })}
    <IconButton label={msg('ui.notifications')} active={open} {...props}>
      <Bell size="1.8rem" strokeWidth={1.8} />
    </IconButton>
  {/snippet}
  {#snippet children({ close })}
    <div class="notifications-head-row">
      <div class="notifications-head" aria-hidden="true">{msg('ui.notifications')}</div>
      {#if $notifications.length > 0}
        <button type="button" class="notifications-mark-read" onclick={markAllRead}>{msg('ui.markAllRead')}</button>
      {/if}
    </div>
    {#if $notifications.length === 0}
      <div class="notifications-empty">{msg('ui.nothingNewYet')}</div>
    {:else}
      <div role="list">
        {#each $notifications as n (n.id)}
          <div role="listitem">
            <button type="button" class="notification" onclick={() => openNotification(n, close)}>
              <span class="notification-title">{n.title}</span>
              <span class="notification-text">{n.text}</span>
            </button>
          </div>
        {/each}
      </div>
    {/if}
    <button
      type="button"
      class="notifications-all"
      onclick={() => {
        close();
        openHistory();
      }}>{msg('ui.allHistory')}</button
    >
  {/snippet}
</Popover>

<style>
  .notifications-head {
    padding: 0.8rem 1rem 0.5rem;
    font-size: 1.1rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
  }

  .notifications-head-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
  }

  .notifications-mark-read {
    flex-shrink: 0;
    padding: 0.3rem 0.8rem;
    border-radius: var(--radius-xs);
    font-size: var(--font-xs);
    color: var(--text-3);
    transition: background var(--dur-fast) var(--ease), color var(--dur-fast) var(--ease);
  }

  .notifications-mark-read:hover {
    background: var(--hover-strong);
    color: var(--text-2);
  }

  .notification {
    display: flex;
    flex-direction: column;
    gap: 1px;
    width: 100%;
    padding: 0.8rem 1rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .notification:hover {
    background: var(--hover-strong);
  }

  .notification-title {
    font-size: var(--font-sm);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notification-text {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .notifications-all {
    display: block;
    width: 100%;
    padding: 1rem 1.4rem;
    border-top: 1px solid var(--border);
    text-align: center;
    font-size: var(--font-sm);
    color: var(--text-2);
    transition: background var(--dur) var(--ease);
  }

  .notifications-all:hover {
    background: var(--hover);
    color: var(--text);
  }

  .notifications-empty {
    padding: 0.8rem 1rem 1.2rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
