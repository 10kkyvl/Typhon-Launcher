<script lang="ts">
  import { Database, Download, Gamepad2, History, LayoutGrid, MonitorDown, Settings, Users, Wifi } from '@lucide/svelte';
  import { navigate, route, type RouteName } from '../stores/router';
  import { accountErrorText } from '../services/accountMessages';
  import { PRESENCE_STATUSES, type PresenceStatus } from '../services/online';
  import { STATUS_LABELS, statusDot } from '../social/presence';
  import { presenceStatus, updatePresenceStatus } from '../stores/presence';
  import { incomingCount } from '../stores/social';
  import { authState, currentUser, isOffline, leaveGuest, signOut } from '../stores/user';
  import { settings } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import Avatar from './Avatar.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import type { MenuItem } from './DropdownMenu.svelte';

  type NavItem = { name: RouteName; label: string; icon: typeof LayoutGrid };

  const groups: NavItem[][] = [
    [
      { name: 'library', label: 'Библиотека', icon: LayoutGrid },
      { name: 'catalog', label: 'Все игры', icon: Gamepad2 },
      { name: 'installed', label: 'Установлено', icon: MonitorDown },
    ],
    [
      { name: 'downloads', label: 'Загрузки', icon: Download },
      { name: 'sources', label: 'Источники', icon: Database },
      { name: 'friends', label: 'Друзья', icon: Users },
      { name: 'history', label: 'История', icon: History },
    ],
  ];

  const lanItem: NavItem = { name: 'lan', label: 'Локальная сеть', icon: Wifi };

  const settingsItem: NavItem = { name: 'settings', label: 'Настройки', icon: Settings };

  const isActive = (name: RouteName) =>
    $route.name === name ||
    (name === 'library' && $route.name === 'game') ||
    (name === 'friends' && $route.name === 'user');

  const isGuest = $derived($authState === 'guest');

  const avatarName = $derived($currentUser ? $currentUser.displayName || $currentUser.username : isGuest ? 'Гость' : '');

  const statusItems = $derived<MenuItem[]>(
    PRESENCE_STATUSES.map((status, index) => ({
      id: `status:${status}`,
      label: STATUS_LABELS[status],
      dot: statusDot(status),
      checked: $presenceStatus === status,
      separator: index === 0,
    })),
  );

  const profileMenu = $derived<MenuItem[]>(
    isGuest
      ? [
          { id: 'profile', label: 'Профиль' },
          { id: 'login', label: 'Войти в аккаунт', separator: true },
        ]
      : [
          { id: 'profile', label: 'Профиль' },
          ...statusItems,
          { id: 'logout', label: 'Выйти', danger: true, separator: true },
        ],
  );

  async function onProfileSelect(id: string) {
    if (id === 'profile') {
      navigate('profile');
      return;
    }
    if (id.startsWith('status:')) {
      try {
        await updatePresenceStatus(id.slice('status:'.length) as PresenceStatus);
      } catch (err) {
        toast(accountErrorText(err, 'Не удалось сменить статус'), 'danger');
      }
      return;
    }
    try {
      if (id === 'login') await leaveGuest();
      else await signOut();
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось выйти'), 'danger');
    }
  }
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
    {#if item.name === 'friends' && $incomingCount > 0}
      <span class="count">{$incomingCount}</span>
    {/if}
  </button>
{/snippet}

<aside class="sidebar">
  <div class="logo">
    <img class="logo-mark" src="/typhon.png" alt="" draggable="false" />
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
    {#if $settings?.lanSharing}
      <div class="group">
        {@render navButton(lanItem)}
      </div>
    {/if}
  </nav>

  <div class="bottom">
    {@render navButton(settingsItem)}
    <DropdownMenu items={profileMenu} placement="up" align="left" onselect={onProfileSelect}>
      {#snippet trigger({ open, toggle })}
        <button
          class="profile"
          class:active={$route.name === 'profile'}
          class:open
          onclick={toggle}
        >
          <span class="avatar">
            <Avatar
              size="sm"
              name={avatarName}
              src={$currentUser?.avatarUrl}
              status={$currentUser ? statusDot($presenceStatus) : undefined}
            />
          </span>
          <span class="profile-text">
            {#if $currentUser}
              <span class="profile-name">{$currentUser.displayName}</span>
              <span class="profile-status">@{$currentUser.username}{#if $isOffline}<span class="offline-mark" title="Нет связи с сервером аккаунтов, показан кэш профиля"> · офлайн</span>{/if}</span>
            {:else if isGuest}
              <span class="profile-name">Гость</span>
              <span class="profile-status">Без аккаунта</span>
            {:else}
              <span class="profile-name">Не авторизован</span>
            {/if}
          </span>
        </button>
      {/snippet}
    </DropdownMenu>
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

  .count {
    margin-left: auto;
    min-width: 1.8rem;
    padding: 0 0.5rem;
    border-radius: 0.9rem;
    background: var(--accent);
    color: #fff;
    font-size: var(--font-xs);
    font-weight: 600;
    line-height: 1.8rem;
    text-align: center;
  }

  .bottom {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .bottom :global(.dropdown) {
    width: 100%;
  }

  .profile {
    display: flex;
    align-items: center;
    width: 100%;
    gap: 1rem;
    padding: 0.6rem 0.8rem;
    border-radius: var(--radius-md);
    text-align: left;
    color: var(--text-3);
    transition: background var(--dur) var(--ease);
  }

  .profile:hover,
  .profile.active,
  .profile.open {
    background: var(--hover);
  }

  .avatar {
    display: block;
    flex-shrink: 0;
    --avatar-ring: var(--bg-sidebar);
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

  .offline-mark {
    color: var(--warning);
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

    .count {
      position: absolute;
      top: 0.4rem;
      right: 0.4rem;
      margin-left: 0;
      min-width: 1.6rem;
      padding: 0 0.4rem;
      border-radius: 0.8rem;
      font-size: 1rem;
      line-height: 1.6rem;
    }

    .profile {
      justify-content: center;
      padding: 0.6rem;
    }
  }
</style>
