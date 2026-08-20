<script lang="ts">
  import {
    Bell,
    ChevronDown,
    ChevronLeft,
    ChevronRight,
    CircleHelp,
    Minus,
    Settings,
    Square,
    X,
  } from '@lucide/svelte';
  import { Window } from '@wailsio/runtime';
  import { user } from '../mock/user';
  import { canGoBack, canGoForward, goBack, goForward, navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { clickOutside } from '../utils/clickOutside';
  import IconButton from './IconButton.svelte';
  import SearchInput from './SearchInput.svelte';

  let search = $state('');
  let notificationsOpen = $state(false);
  let avatarFailed = $state(false);

  const notifications = [
    { id: 1, title: 'Hogwarts Legacy', text: 'Обновление 1.0.3.0 загружается' },
    { id: 2, title: 'ELDEN RING', text: 'Доступно дополнение Shadow of the Erdtree' },
    { id: 3, title: 'Источники', text: 'Aurora Core Library обновлена' },
  ];

  const inWails = typeof (window as unknown as { _wails?: unknown })._wails !== 'undefined';

  function win(action: 'minimise' | 'maximise' | 'close') {
    if (!inWails) {
      toast('Доступно только в desktop-сборке');
      return;
    }
    if (action === 'minimise') Window.Minimise();
    else if (action === 'maximise') Window.ToggleMaximise();
    else Window.Close();
  }
</script>

<header class="topbar" style="--wails-draggable: drag">
  <div class="no-drag nav-buttons">
    <IconButton label="Назад" onclick={goBack}>
      <span class="arrow" class:dim={!$canGoBack}><ChevronLeft size={19} strokeWidth={1.8} /></span>
    </IconButton>
    <IconButton label="Вперёд" onclick={goForward}>
      <span class="arrow" class:dim={!$canGoForward}><ChevronRight size={19} strokeWidth={1.8} /></span>
    </IconButton>
  </div>

  <div class="no-drag search-wrap">
    <SearchInput bind:value={search} placeholder="Поиск игр, дополнений, коллекций..." shortcut="Ctrl + K" />
  </div>

  <div class="no-drag right">
    <div class="bell" use:clickOutside={() => (notificationsOpen = false)}>
      <IconButton label="Уведомления" active={notificationsOpen} onclick={() => (notificationsOpen = !notificationsOpen)}>
        <Bell size={19} strokeWidth={1.8} />
      </IconButton>
      {#if notificationsOpen}
        <div class="notifications">
          <div class="notifications-head">Уведомления</div>
          {#each notifications as n (n.id)}
            <button class="notification" onclick={() => (notificationsOpen = false)}>
              <span class="notification-title">{n.title}</span>
              <span class="notification-text">{n.text}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
    <IconButton label="Справка" onclick={() => toast('Справка недоступна в demo')}>
      <CircleHelp size={19} strokeWidth={1.8} />
    </IconButton>
    <IconButton label="Настройки" onclick={() => navigate('settings')}>
      <Settings size={19} strokeWidth={1.8} />
    </IconButton>

    <button class="account" onclick={() => navigate('settings')}>
      <span class="avatar">
        {#if avatarFailed}
          <span class="avatar-fallback">{user.name.slice(0, 1)}</span>
        {:else}
          <img src={user.avatar} alt="" draggable="false" onerror={() => (avatarFailed = true)} />
        {/if}
      </span>
      <span class="account-text">
        <span class="account-name">{user.name}</span>
        <span class="account-status"><span class="dot"></span>{user.status}</span>
      </span>
      <ChevronDown size={14} strokeWidth={1.8} />
    </button>

    <div class="window-controls">
      <button class="wc" aria-label="Свернуть" onclick={() => win('minimise')}>
        <Minus size={16} strokeWidth={1.6} />
      </button>
      <button class="wc" aria-label="Развернуть" onclick={() => win('maximise')}>
        <Square size={13} strokeWidth={1.6} />
      </button>
      <button class="wc close" aria-label="Закрыть" onclick={() => win('close')}>
        <X size={16} strokeWidth={1.6} />
      </button>
    </div>
  </div>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    height: var(--topbar-h);
    padding: 0 var(--space-4) 0 var(--space-6);
    flex-shrink: 0;
  }

  .no-drag {
    --wails-draggable: no-drag;
  }

  .nav-buttons {
    display: flex;
    gap: 2px;
  }

  .arrow {
    display: inline-flex;
  }

  .arrow.dim {
    opacity: 0.35;
  }

  .search-wrap {
    flex: 1;
    max-width: 560px;
  }

  .right {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-left: auto;
  }

  .bell {
    position: relative;
  }

  .notifications {
    position: absolute;
    z-index: 50;
    top: calc(100% + 8px);
    right: 0;
    width: 320px;
    padding: 6px;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .notifications-head {
    padding: 8px 11px 6px;
    font-size: 12px;
    font-weight: 550;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-3);
  }

  .notification {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: 9px 11px;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .notification:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  .notification-title {
    font-size: 13.5px;
    font-weight: 550;
  }

  .notification-text {
    font-size: 12.5px;
    color: var(--text-3);
  }

  .account {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 44px;
    margin-left: 6px;
    padding: 0 10px;
    border-radius: var(--radius-md);
    color: var(--text-3);
    transition: background var(--dur) var(--ease);
  }

  .account:hover {
    background: rgba(255, 255, 255, 0.045);
  }

  .avatar {
    width: 32px;
    height: 32px;
    flex-shrink: 0;
  }

  .avatar img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
  }

  .avatar-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    background: var(--accent-subtle);
    color: var(--accent-text);
    font-size: 13px;
    font-weight: 600;
  }

  .account-text {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }

  .account-name {
    font-size: 13.5px;
    font-weight: 550;
    color: var(--text);
    line-height: 1.25;
  }

  .account-status {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 11.5px;
    color: var(--text-3);
    line-height: 1.25;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--success);
  }

  .window-controls {
    display: flex;
    margin-left: 10px;
  }

  .wc {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 34px;
    border-radius: var(--radius-sm);
    color: var(--text-3);
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .wc:hover {
    background: rgba(255, 255, 255, 0.07);
    color: var(--text);
  }

  .wc.close:hover {
    background: #c9403f;
    color: #fff;
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-3px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (max-width: 1240px) {
    .account-text {
      display: none;
    }

    .account :global(svg:last-child) {
      display: none;
    }
  }
</style>
