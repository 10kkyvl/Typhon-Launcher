<script lang="ts">
  import { ChevronLeft, ChevronRight, Gamepad2, Users } from '@lucide/svelte';
  import { canGoBack, canGoForward, goBack, goForward, navigate } from '../stores/router';
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
          </div>
        {:else if id === 'friends'}
          <div class="icon-slot" data-part="topbar-action" data-id="friends">
            <IconButton
              label={msg('ui.friends')}
              count={$incomingCount}
              countLabel={msg('ui.friendsIncoming', { count: $incomingCount })}
              onclick={() => navigate('friends')}
            >
              <Users size="1.8rem" strokeWidth={1.8} />
            </IconButton>
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

  .right {
    gap: var(--space-1);
    margin-left: auto;
  }

  .icon-slot {
    position: relative;
  }
</style>
