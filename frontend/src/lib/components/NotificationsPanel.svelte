<script lang="ts">
  import { Bell } from '@lucide/svelte';
  import { navigate } from '../stores/router';
  import { markAllRead, notifications, openHistory, type Notification } from '../stores/notifications';
  import { msg } from '../i18n';
  import IconButton from './IconButton.svelte';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  function openNotification(item: Notification) {
    open = false;
    navigate(item.route, item.params);
    item.onOpen?.();
  }
</script>

<IconButton label={msg('ui.notifications')} active={open} onclick={() => (open = !open)}>
  <Bell size="1.8rem" strokeWidth={1.8} />
</IconButton>
{#if open}
  <div class="notifications">
    <div class="notifications-head-row">
      <div class="notifications-head">{msg('ui.notifications')}</div>
      {#if $notifications.length > 0}
        <button class="notifications-mark-read" onclick={markAllRead}>{msg('ui.markAllRead')}</button>
      {/if}
    </div>
    {#if $notifications.length === 0}
      <div class="notifications-empty">{msg('ui.nothingNewYet')}</div>
    {:else}
      {#each $notifications as n (n.id)}
        <button class="notification" onclick={() => openNotification(n)}>
          <span class="notification-title">{n.title}</span>
          <span class="notification-text">{n.text}</span>
        </button>
      {/each}
    {/if}
    <button
      class="notifications-all"
      onclick={() => {
        open = false;
        openHistory();
      }}>{msg('ui.allHistory')}</button
    >
  </div>
{/if}

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

  .notifications {
    position: absolute;
    z-index: var(--z-popover);
    top: calc(100% + 0.6rem);
    right: 0;
    width: 32rem;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-0.3rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
