<script lang="ts">
  import {
    Bookmark,
    ChevronsUpDown,
    Database,
    Download,
    LayoutGrid,
    MonitorDown,
    Settings,
    Trophy,
  } from '@lucide/svelte';
  import { storage, user } from '../mock/user';
  import { navigate, route, type RouteName } from '../stores/router';
  import { gb } from '../utils/format';

  const nav: { name: RouteName; label: string; icon: typeof LayoutGrid }[] = [
    { name: 'library', label: 'Библиотека', icon: LayoutGrid },
    { name: 'installed', label: 'Установлено', icon: MonitorDown },
    { name: 'downloads', label: 'Загрузки', icon: Download },
    { name: 'sources', label: 'Источники', icon: Database },
    { name: 'collections', label: 'Коллекции', icon: Bookmark },
    { name: 'achievements', label: 'Достижения', icon: Trophy },
    { name: 'settings', label: 'Настройки', icon: Settings },
  ];

  let avatarFailed = $state(false);

  const isActive = (name: RouteName) =>
    $route.name === name || (name === 'library' && $route.name === 'game');
</script>

<aside class="sidebar">
  <div class="logo">
    <img src="/aurora.svg" alt="" width="30" height="30" draggable="false" />
    <span class="logo-text">Aurora</span>
  </div>

  <nav>
    {#each nav as item (item.name)}
      <button class="nav-item" class:active={isActive(item.name)} onclick={() => navigate(item.name)}>
        <item.icon size={20} strokeWidth={1.8} />
        <span class="nav-label">{item.label}</span>
      </button>
    {/each}
  </nav>

  <div class="bottom">
    <button class="profile">
      <span class="avatar">
        {#if avatarFailed}
          <span class="avatar-fallback">{user.name.slice(0, 1)}</span>
        {:else}
          <img src={user.avatar} alt="" draggable="false" onerror={() => (avatarFailed = true)} />
        {/if}
        <span class="status-dot"></span>
      </span>
      <span class="profile-text">
        <span class="profile-name">{user.name}</span>
        <span class="profile-status">{user.status}</span>
      </span>
      <ChevronsUpDown size={15} strokeWidth={1.8} />
    </button>

    <div class="storage">
      <div class="storage-head">
        <span>Хранилище</span>
        <span class="storage-nums">{storage.usedGb} ГБ / {gb(storage.totalGb / 1024, 0).replace(' ГБ', ' ТБ')}</span>
      </div>
      <div class="storage-bar">
        <div class="storage-fill" style:width="{(storage.usedGb / storage.totalGb) * 100}%"></div>
      </div>
      <span class="storage-free">Свободно {storage.totalGb - storage.usedGb} ГБ</span>
    </div>
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
    border-right: 1px solid var(--border);
    padding: var(--space-5) var(--space-3) var(--space-4);
  }

  .logo {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 4px 12px 0;
    margin-bottom: var(--space-8);
  }

  .logo-text {
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 40px;
    padding: 0 12px;
    border-radius: var(--radius-md);
    font-size: 14px;
    font-weight: 500;
    color: var(--text-2);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .nav-item:hover {
    background: rgba(255, 255, 255, 0.045);
    color: var(--text);
  }

  .nav-item.active {
    background: var(--accent);
    color: #fff;
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
    gap: 10px;
    padding: 8px 10px;
    border-radius: var(--radius-md);
    text-align: left;
    color: var(--text-3);
    transition: background var(--dur) var(--ease);
  }

  .profile:hover {
    background: rgba(255, 255, 255, 0.045);
  }

  .avatar {
    position: relative;
    width: 34px;
    height: 34px;
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
    font-size: 14px;
    font-weight: 600;
  }

  .status-dot {
    position: absolute;
    right: -1px;
    bottom: -1px;
    width: 10px;
    height: 10px;
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
    font-size: 13.5px;
    font-weight: 550;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .profile-status {
    font-size: 12px;
    color: var(--success);
  }

  .storage {
    padding: 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .storage-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    font-size: 12.5px;
    font-weight: 550;
  }

  .storage-nums {
    font-size: 11.5px;
    font-weight: 400;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .storage-bar {
    height: 5px;
    border-radius: 99px;
    background: rgba(255, 255, 255, 0.07);
    overflow: hidden;
  }

  .storage-fill {
    height: 100%;
    border-radius: 99px;
    background: var(--accent);
  }

  .storage-free {
    font-size: 11.5px;
    color: var(--text-3);
  }

  @media (max-width: 1140px) {
    .sidebar {
      width: 68px;
      padding: var(--space-5) 10px var(--space-4);
      align-items: center;
    }

    .logo {
      padding: 4px 0 0;
      margin-bottom: var(--space-6);
    }

    .logo-text,
    .nav-label,
    .profile-text,
    .profile :global(svg),
    .storage {
      display: none;
    }

    nav {
      width: 100%;
    }

    .nav-item {
      justify-content: center;
      padding: 0;
      width: 100%;
    }

    .profile {
      padding: 6px;
    }
  }
</style>
