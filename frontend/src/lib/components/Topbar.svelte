<script lang="ts">
  import { Bell, ChevronLeft, ChevronRight, Gamepad2, Users } from '@lucide/svelte';
  import { canGoBack, canGoForward, goBack, goForward, navigate } from '../stores/router';
  import { markAllRead, notifications, openHistory, type Notification } from '../stores/notifications';
  import { incomingCount } from '../stores/social';
  import { toast } from '../stores/toasts';
  import { runWindowAction } from '../stores/window';
  import { clickOutside } from '../utils/clickOutside';
  import { msg } from '../i18n';
  import { enterBigPicture } from '../bigpicture/mode';
  import { themeLayout } from '../stores/layout';
  import IconButton from './IconButton.svelte';
  import TopbarSearch from './TopbarSearch.svelte';

  let notificationsOpen = $state(false);
  let enteringBigPicture = $state(false);

  async function openBigPicture() {
    if (enteringBigPicture) return;
    enteringBigPicture = true;
    try { await enterBigPicture(); }
    catch { toast(msg('bp.enterError'), 'danger'); }
    finally { enteringBigPicture = false; }
  }

  function openNotification(item: Notification) {
    notificationsOpen = false;
    navigate(item.route, item.params);
    item.onOpen?.();
  }

  // Окно без рамки, поэтому поведение нативного заголовка приходится повторять вручную.
  function onTitlebarDoubleClick(event: MouseEvent) {
    const target = event.target as HTMLElement | null;
    if (target?.closest('.no-drag')) return;
    runWindowAction('toggleMaximise');
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<header
  class="topbar"
  class:centered={$themeLayout.search === 'center'}
  data-part="topbar"
  style="--wails-draggable: drag"
  ondblclick={onTitlebarDoubleClick}
>
  <div class="no-drag nav-buttons" data-part="topbar-history">
    <IconButton label={msg('common.back')} disabled={!$canGoBack} onclick={goBack}>
      <ChevronLeft size="1.8rem" strokeWidth={1.8} />
    </IconButton>
    <IconButton label={msg('ui.forward')} disabled={!$canGoForward} onclick={goForward}>
      <ChevronRight size="1.8rem" strokeWidth={1.8} />
    </IconButton>
  </div>

  <TopbarSearch />

  <div class="no-drag right">
    <div class="actions" data-part="topbar-actions">
      {#each $themeLayout.actions as id (id)}
        {#if id === 'bigpicture'}
          <div class="icon-slot" data-part="topbar-action" data-id="bigpicture">
            <IconButton label={msg('bp.enter')} disabled={enteringBigPicture} onclick={openBigPicture}>
              <Gamepad2 size="1.9rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        {:else if id === 'notifications'}
          <div
            class="icon-slot"
            data-part="topbar-action"
            data-id="notifications"
            use:clickOutside={() => (notificationsOpen = false)}
          >
            <IconButton label={msg('ui.notifications')} active={notificationsOpen} onclick={() => (notificationsOpen = !notificationsOpen)}>
              <Bell size="1.8rem" strokeWidth={1.8} />
            </IconButton>
            {#if $notifications.length > 0}
              <span class="badge">{$notifications.length}</span>
            {/if}
            {#if notificationsOpen}
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
                    notificationsOpen = false;
                    openHistory();
                  }}>{msg('ui.allHistory')}</button
                >
              </div>
            {/if}
          </div>
        {:else if id === 'friends'}
          <div class="icon-slot" data-part="topbar-action" data-id="friends">
            <IconButton label={msg('ui.friends')} onclick={() => navigate('friends')}>
              <Users size="1.8rem" strokeWidth={1.8} />
            </IconButton>
            {#if $incomingCount > 0}
              <span class="badge">{$incomingCount}</span>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  </div>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    height: var(--topbar-h);
    padding: 0 var(--space-3) 0 var(--page-x);
    flex-shrink: 0;
  }

  .no-drag {
    --wails-draggable: no-drag;
  }

  .nav-buttons {
    display: flex;
    gap: 0;
    margin-left: -0.8rem;
  }

  .topbar.centered {
    display: grid;
    grid-template-columns: 1fr minmax(0, 44rem) 1fr;
  }

  .topbar.centered .nav-buttons {
    justify-self: start;
  }

  .topbar.centered .right {
    justify-self: end;
    margin-left: 0;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

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

  .badge {
    position: absolute;
    top: 0;
    right: 0;
    pointer-events: none;
    min-width: 1.6rem;
    height: 1.6rem;
    padding: 0 0.4rem;
    border-radius: 0.8rem;
    background: var(--accent);
    color: var(--accent-on, #fff);
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.6rem;
    text-align: center;
  }

  .right {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-left: auto;
  }

  .icon-slot {
    position: relative;
  }

  .notifications {
    position: absolute;
    z-index: 50;
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
