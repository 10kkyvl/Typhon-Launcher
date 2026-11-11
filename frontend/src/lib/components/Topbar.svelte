<script lang="ts">
  import { ChevronLeft, ChevronRight, Gamepad2, Users } from '@lucide/svelte';
  import { canGoBack, canGoForward, goBack, goForward, navigate } from '../stores/router';
  import { notifications } from '../stores/notifications';
  import { incomingCount } from '../stores/social';
  import { toast } from '../stores/toasts';
  import { runWindowAction } from '../stores/window';
  import { msg } from '../i18n';
  import { enterBigPicture } from '../bigpicture/mode';
  import { themeLayout } from '../stores/layout';
  import AccountMenu from './AccountMenu.svelte';
  import IconButton from './IconButton.svelte';
  import NotificationsPanel from './NotificationsPanel.svelte';
  import TopbarSearch from './TopbarSearch.svelte';

  let enteringBigPicture = $state(false);

  async function openBigPicture() {
    if (enteringBigPicture) return;
    enteringBigPicture = true;
    try { await enterBigPicture(); }
    catch { toast(msg('bp.enterError'), 'danger'); }
    finally { enteringBigPicture = false; }
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
  <div class="no-drag island history" data-part="topbar-history">
    <IconButton label={msg('common.back')} disabled={!$canGoBack} onclick={goBack}>
      <ChevronLeft size="1.8rem" strokeWidth={1.8} />
    </IconButton>
    <IconButton label={msg('ui.forward')} disabled={!$canGoForward} onclick={goForward}>
      <ChevronRight size="1.8rem" strokeWidth={1.8} />
    </IconButton>
  </div>

  <TopbarSearch />

  <div class="no-drag island right">
    <div class="actions" data-part="topbar-actions">
      {#each $themeLayout.actions as id (id)}
        {#if id === 'bigpicture'}
          <div class="icon-slot" data-part="topbar-action" data-id="bigpicture">
            <IconButton label={msg('bp.enter')} disabled={enteringBigPicture} onclick={openBigPicture}>
              <Gamepad2 size="1.9rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        {:else if id === 'notifications'}
          <div class="icon-slot" data-part="topbar-action" data-id="notifications">
            <NotificationsPanel />
            {#if $notifications.length > 0}
              <span class="badge" aria-hidden="true">{$notifications.length}</span>
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
    <AccountMenu />
  </div>
</header>

<style>
  /* The account chip reads this container's width (AccountMenu) to drop its text before the search shrinks;
     the centred row has its own name because it runs out of room sooner. */
  .topbar {
    container: topbar / inline-size;
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: var(--topbar-h);
    padding: 0 var(--page-x);
    flex-shrink: 0;
  }

  .no-drag {
    --wails-draggable: no-drag;
  }

  .island {
    display: flex;
    flex: none;
    align-items: center;
    padding: 0.3rem;
    background: var(--island-fill);
    border: 1px solid var(--border);
    border-radius: var(--radius-control);
  }

  .history {
    gap: 0;
  }

  .topbar.centered {
    container-name: topbar-centred;
    display: grid;
    grid-template-columns: 1fr minmax(var(--topbar-search-min), var(--topbar-search-max)) 1fr;
  }

  .topbar.centered .history {
    justify-self: start;
  }

  .topbar.centered .right {
    justify-self: end;
    margin-left: 0;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
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
    gap: var(--space-1);
    margin-left: auto;
  }

  .icon-slot {
    position: relative;
  }
</style>
