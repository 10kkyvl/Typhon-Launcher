<script lang="ts">
  import { Bookmark, Database, Download, LayoutGrid, MonitorDown, Settings, Trophy } from '@lucide/svelte';
  import { navigate, route, type RouteName } from '../stores/router';
  import { currentUser } from '../stores/user';

  type NavItem = { name: RouteName; label: string; icon: typeof LayoutGrid };

  const groups: NavItem[][] = [
    [
      { name: 'library', label: 'Библиотека', icon: LayoutGrid },
      { name: 'installed', label: 'Установлено', icon: MonitorDown },
    ],
    [
      { name: 'downloads', label: 'Загрузки', icon: Download },
      { name: 'sources', label: 'Источники', icon: Database },
    ],
    [
      { name: 'collections', label: 'Коллекции', icon: Bookmark },
      { name: 'achievements', label: 'Достижения', icon: Trophy },
    ],
  ];

  const settingsItem: NavItem = { name: 'settings', label: 'Настройки', icon: Settings };

  let avatarFailed = $state(false);

  const isActive = (name: RouteName) =>
    $route.name === name || (name === 'library' && $route.name === 'game');

  const avatarInitial = $derived(
    $currentUser ? ($currentUser.displayName || $currentUser.username).slice(0, 1).toUpperCase() : '?',
  );

  $effect(() => {
    $currentUser?.avatarUrl;
    avatarFailed = false;
  });
</script>

{#snippet navButton(item: NavItem)}
  <button
    class="nav-item"
    class:active={isActive(item.name)}
    aria-current={isActive(item.name) ? 'page' : undefined}
    onclick={() => navigate(item.name)}
  >
    <span class="indicator"></span>
    <item.icon size="2rem" strokeWidth={1.8} />
    <span class="nav-label">{item.label}</span>
  </button>
{/snippet}

<aside class="sidebar">
  <div class="logo">
    <img class="logo-mark" src="/typhon.svg" alt="" draggable="false" />
    <span class="logo-text">Typhon</span>
  </div>

  <nav>
    {#each groups as group, i (i)}
      <div class="group">
        {#each group as item (item.name)}
          {@render navButton(item)}
        {/each}
      </div>
    {/each}
  </nav>

  <div class="bottom">
    {@render navButton(settingsItem)}
    <button class="profile" onclick={() => navigate('settings')}>
      <span class="avatar">
        {#if avatarFailed || !$currentUser?.avatarUrl}
          <span class="avatar-fallback">{avatarInitial}</span>
        {:else}
          <img src={$currentUser.avatarUrl} alt="" draggable="false" onerror={() => (avatarFailed = true)} />
        {/if}
        {#if $currentUser}
          <span class="status-dot"></span>
        {/if}
      </span>
      <span class="profile-text">
        {#if $currentUser}
          <span class="profile-name">{$currentUser.displayName}</span>
          <span class="profile-status">@{$currentUser.username}</span>
        {:else}
          <span class="profile-name">Не авторизован</span>
        {/if}
      </span>
    </button>
  </div>
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    width: var(--sidebar-w);
    flex-shrink: 0;
    height: 100%;
    background: var(--bg-sidebar);
    padding: 1.8rem 1.2rem 1.4rem;
  }

  .logo {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.2rem 1rem 0;
    margin-bottom: var(--space-8);
  }

  .logo-mark {
    width: 2.8rem;
    height: 2.8rem;
  }

  .logo-text {
    font-size: 1.9rem;
    font-weight: 600;
    letter-spacing: var(--tracking-title);
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .nav-item {
    position: relative;
    display: flex;
    align-items: center;
    gap: 1.2rem;
    height: 4rem;
    padding: 0 1rem;
    border-radius: var(--radius-md);
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text-2);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .nav-item :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .nav-item:hover {
    background: var(--hover);
    color: var(--text);
  }

  .nav-item:hover :global(svg) {
    color: var(--text-2);
  }

  .nav-item.active {
    background: var(--hover-strong);
    color: var(--text);
  }

  .nav-item.active :global(svg) {
    color: var(--text);
  }

  .indicator {
    position: absolute;
    left: 0;
    top: 1.1rem;
    bottom: 1.1rem;
    width: 0.3rem;
    border-radius: var(--cut) 0.3rem 0.3rem var(--cut);
    background: var(--accent);
    opacity: 0;
    transform: scaleY(0.5);
    transition:
      opacity var(--dur) var(--ease),
      transform var(--dur) var(--ease);
  }

  .nav-item.active .indicator {
    opacity: 1;
    transform: scaleY(1);
  }

  .nav-label {
    white-space: nowrap;
  }

  .bottom {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .profile {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.6rem 0.8rem;
    border-radius: var(--radius-md);
    text-align: left;
    color: var(--text-3);
    transition: background var(--dur) var(--ease);
  }

  .profile:hover {
    background: var(--hover);
  }

  .avatar {
    position: relative;
    width: 3.2rem;
    height: 3.2rem;
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
    background: var(--surface-3);
    color: var(--text-2);
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .status-dot {
    position: absolute;
    right: -1px;
    bottom: -1px;
    width: 0.9rem;
    height: 0.9rem;
    border-radius: 50%;
    background: var(--success);
    border: 2px solid var(--bg-sidebar);
  }

  .profile-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .profile-name {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
  }

  .profile-status {
    font-size: 1.2rem;
    color: var(--text-3);
    line-height: 1.3;
  }

  @media (max-width: 1140px) {
    .sidebar {
      width: 6.4rem;
      padding: 1.8rem 1rem 1.4rem;
      align-items: center;
    }

    .logo {
      padding: 0.2rem 0 0;
      margin-bottom: var(--space-6);
    }

    .logo-text,
    .nav-label,
    .profile-text {
      display: none;
    }

    nav,
    .bottom {
      width: 100%;
    }

    .nav-item {
      justify-content: center;
      padding: 0;
      width: 100%;
    }

    .profile {
      justify-content: center;
      padding: 0.6rem;
    }
  }
</style>
