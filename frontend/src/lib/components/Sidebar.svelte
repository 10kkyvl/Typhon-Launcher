<script lang="ts">
  import { settings as appearanceSettings } from '../stores/settings';
  import { displayedAccent } from '../theme/apply';
  import { rgb } from '../theme/accent';
  const logoChannels = $derived(rgb($displayedAccent).map(v => v / 255));
  // Replace the blue chroma (B-R), retaining neutral shadows and highlights.
  const logoMatrix = $derived(logoChannels.map(k => `${1-k} 0 ${k} 0 0`).join(' ') + ' 0 0 0 1 0');
  import { Activity, ChartColumn, Database, Download, HardDrive, Gamepad2, History, LayoutGrid, LifeBuoy, MonitorDown, Settings, Users, Wifi } from '@lucide/svelte';
  import { navigate, route, type RouteName } from '../stores/router';
  import { accountErrorText } from '../services/accountMessages';
  import { PRESENCE_STATUSES, type PresenceStatus } from '../services/online';
  import { STATUS_LABELS, statusDot } from '../social/presence';
  import { autoAway, presenceStatus, shownPresence, updatePresenceStatus } from '../stores/presence';
  import { incomingCount } from '../stores/social';
  import { unreadReplies } from '../stores/support';
  import { authState, currentUser, isOffline, signOut } from '../stores/user';
  import { ownFrame } from '../profile/frameSources';
  import { settings } from '../stores/settings';
  import { navBlocks, themeLayout } from '../stores/layout';
  import type { SidebarItem } from '../theme/layout';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';
  import { signOutPrompt, type ConfirmPrompt } from '../confirm/prompts';
  import { Plate, partsThatMove, scrollToShow } from '../motion/plate';
  import Avatar from './Avatar.svelte';
  import BetaBadge from './BetaBadge.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import type { MenuItem } from './DropdownMenu.svelte';
  import SidebarStatus from './SidebarStatus.svelte';
  import { signInFromGuest } from './sidebarStatus';

  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let sidebar = $state<HTMLElement>();
  let plateEl = $state<HTMLElement>();
  let nav = $state<HTMLElement>();
  let plate: Plate | undefined;

  type NavItem = { name: RouteName; label: string; icon: typeof LayoutGrid };

  const navItems: Record<SidebarItem, NavItem> = {
    library: { name: 'library', label: msg('ui.library'), icon: LayoutGrid },
    catalog: { name: 'catalog', label: msg('ui.allGames'), icon: Gamepad2 },
    installed: { name: 'installed', label: msg('ui.installed'), icon: MonitorDown },
    diskusage: { name: 'diskusage', label: msg('diskusage.nav'), icon: HardDrive },
    downloads: { name: 'downloads', label: msg('ui.downloadsNav'), icon: Download },
    sources: { name: 'sources', label: msg('ui.sourcesNav'), icon: Database },
    friends: { name: 'friends', label: msg('ui.friends'), icon: Users },
    activity: { name: 'activity', label: msg('ui.activity'), icon: Activity },
    stats: { name: 'stats', label: msg('stats.nav'), icon: ChartColumn },
    history: { name: 'history', label: msg('ui.history'), icon: History },
    lan: { name: 'lan', label: msg('ui.localNetwork'), icon: Wifi },
  };

  const blocks = $derived(navBlocks($themeLayout.entries, Boolean($settings?.lanSharing)));

  const supportItem: NavItem = { name: 'support', label: msg('tickets.nav'), icon: LifeBuoy };

  const settingsItem: NavItem = { name: 'settings', label: msg('ui.settingsNav'), icon: Settings };

  const isActive = (name: RouteName) =>
    $route.name === name ||
    (name === 'library' && $route.name === 'game') ||
    (name === 'friends' && $route.name === 'user');

  const activeItem = () => sidebar?.querySelector<HTMLElement>('.nav-item.active') ?? null;

  // One plate in the island for the nav and the bottom block alike: it measures nav items through the
  // nav's scroll (motion/plate.ts measure) and follows that scroll.
  $effect(() => {
    if (!plateEl || !nav) return;
    const current = new Plate(plateEl);
    current.follow(nav, activeItem);
    plate = current;
    return () => {
      current.destroy();
      plate = undefined;
    };
  });

  // Every part whose size can move the active item, again when the nav's groups change.
  $effect(() => {
    void blocks;
    if (sidebar) plate?.observe(partsThatMove(sidebar), activeItem);
  });

  $effect(() => {
    void $route.name;
    void $settings?.lanSharing;
    void $themeLayout;
    // After the active class lands in the DOM.
    queueMicrotask(() => {
      const active = activeItem();
      if (nav && active?.offsetParent === nav) {
        nav.scrollTop = scrollToShow(active.offsetTop, active.offsetHeight, nav.scrollTop, nav.clientHeight);
      }
      plate?.to(active);
    });
  });

  const isGuest = $derived($authState === 'guest');

  const avatarName = $derived($currentUser ? $currentUser.displayName || $currentUser.username : isGuest ? msg('ui.guest') : '');

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
          { id: 'profile', label: msg('ui.profile') },
          { id: 'login', label: msg('ui.signIn'), separator: true },
        ]
      : [
          { id: 'profile', label: msg('ui.profile') },
          ...statusItems,
          { id: 'logout', label: msg('ui.signOut'), danger: true, separator: true },
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
        toast(accountErrorText(err, msg('ui.statusChangeFailed')), 'danger');
      }
      return;
    }
    if (id === 'logout') {
      pending = { prompt: signOutPrompt(), run: signOutNow };
      return;
    }
    if (id !== 'login') return;
    await signInFromGuest();
  }

  async function signOutNow() {
    try {
      await signOut();
    } catch (err) {
      toast(accountErrorText(err, msg('ui.signOutFailed')), 'danger');
    }
  }
</script>

{#snippet navButton(item: NavItem)}
  <button
    class="nav-item"
    data-part="sidebar-item"
    data-id={item.name}
    class:active={isActive(item.name)}
    aria-label={item.label}
    aria-current={isActive(item.name) ? 'page' : undefined}
    onclick={() => navigate(item.name)}
  >
    <item.icon size="2rem" strokeWidth={1.8} />
    <span class="nav-label">{item.label}</span>
    {#if item.name === 'friends' && $incomingCount > 0}
      <span class="count">{$incomingCount}</span>
    {:else if item.name === 'support' && $unreadReplies > 0}
      <span class="count">{$unreadReplies}</span>
    {/if}
  </button>
{/snippet}

<aside class="sidebar" class:right={$themeLayout.side === 'right'} data-part="sidebar" bind:this={sidebar}>
  <span class="plate" bind:this={plateEl} aria-hidden="true"></span>
  <div class="logo">
    <svg width="0" height="0" aria-hidden="true" style="position:absolute"><defs><filter id="typhon-accent-logo" color-interpolation-filters="sRGB"><feColorMatrix type="matrix" values={logoMatrix} /></filter></defs></svg>
    <img style:filter={$appearanceSettings?.tintLogo ? 'url(#typhon-accent-logo)' : undefined} class="logo-mark" src="/typhon.png" alt="" draggable="false" />
    <span class="wordmark">Typhon</span>
    <BetaBadge />
  </div>

  <nav data-part="sidebar-nav" bind:this={nav}>
    {#each blocks as block (block.kind === 'group' ? block.ids[0] : block.key)}
      {#if block.kind === 'group'}
        <div class="group">
          {#each block.ids as id (id)}
            {@render navButton(navItems[id])}
          {/each}
        </div>
      {:else}
        <div class="separator" data-part="sidebar-separator"></div>
      {/if}
    {/each}
  </nav>

  <div class="bottom" data-part="sidebar-bottom">
    {#if !$themeLayout.supportHidden}
      {@render navButton(supportItem)}
    {/if}
    {@render navButton(settingsItem)}
    <DropdownMenu items={profileMenu} placement="up" align="left" onselect={onProfileSelect}>
      {#snippet trigger({ open, toggle })}
        <button
          class="profile"
          data-part="sidebar-account"
          class:active={$route.name === 'profile'}
          class:open
          title={$currentUser && $autoAway ? msg('ui.presenceAutoAwayHint') : undefined}
          onclick={toggle}
        >
          <span class="avatar">
            <Avatar
              size="sm"
              name={avatarName}
              src={$currentUser?.avatarUrl}
              status={$currentUser ? statusDot($shownPresence) : undefined}
              {...ownFrame($currentUser)}
            />
          </span>
          <span class="profile-text">
            {#if $currentUser}
              <span class="profile-name">{$currentUser.displayName}</span>
              <span class="profile-status">@{$currentUser.username}{#if $isOffline}<span class="offline-mark" title={msg('ui.offlineProfileTooltip')}>{msg('ui.offlineSuffix')}</span>{/if}</span>
            {:else if isGuest}
              <span class="profile-name">{msg('ui.guest')}</span>
              <span class="profile-status">{msg('ui.noAccount')}</span>
            {:else}
              <span class="profile-name">{msg('ui.notAuthorized')}</span>
            {/if}
          </span>
        </button>
      {/snippet}
    </DropdownMenu>
  </div>

  <SidebarStatus />
</aside>

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .sidebar {
    position: relative;
    display: flex;
    flex-direction: column;
    width: var(--sidebar-w);
    flex-shrink: 0;
    height: calc(100% - 2 * var(--island-gap));
    margin: var(--island-gap) 0 var(--island-gap) var(--island-gap);
    padding: 1.8rem 1.2rem 1.2rem;
    background: var(--island-fill);
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
  }

  .sidebar.right {
    margin: var(--island-gap) var(--island-gap) var(--island-gap) 0;
  }

  .logo {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-2);
    padding: 0.2rem 0.6rem 0;
    margin-bottom: var(--space-8);
  }

  .logo-mark {
    width: 2.8rem;
    height: 2.8rem;
  }

  .wordmark {
    font-family: var(--font-wordmark);
    font-size: var(--font-lg);
    font-weight: 500;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: var(--text);
  }

  /* The one part that gives way in a short window. The gutter is always there (scroll, not auto), so a
     scrollbar appearing moves nothing; it sits in the island's right padding. */
  nav {
    position: relative;
    display: flex;
    flex: 1 1 0;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    margin-right: -0.6rem;
    overflow-x: hidden;
    overflow-y: scroll;
  }

  nav::-webkit-scrollbar {
    width: 0.6rem;
  }

  nav::-webkit-scrollbar-thumb {
    border-width: 0.1rem;
    background-color: var(--hover-strong);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .separator {
    border-top: 1px solid var(--border);
  }

  .nav-item {
    position: relative;
    z-index: 1;
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
      background var(--motion-fast) var(--ease-out),
      color var(--motion-fast) var(--ease-out);
  }

  .nav-item :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
    transition: color var(--motion-fast) var(--ease-out);
  }

  .nav-item:hover {
    background: var(--hover);
    color: var(--text);
  }

  /* Inset: the nav scrolls and would clip an outer ring. */
  .nav-item:focus-visible {
    outline-offset: -2px;
  }

  .nav-item:hover :global(svg) {
    color: var(--text-2);
  }

  .nav-item.active {
    color: var(--text);
  }

  .nav-item.active:hover {
    background: transparent;
  }

  .nav-item.active :global(svg) {
    color: var(--text);
  }

  .plate {
    position: absolute;
    left: 0;
    top: 0;
    z-index: 0;
    transform-origin: 0 0;
    pointer-events: none;
    border-radius: var(--radius-md);
    background: var(--hover-strong);
    transition: opacity var(--motion-fast) var(--ease-out);
  }

  .plate::before {
    content: '';
    position: absolute;
    left: 0.4rem;
    top: 1.1rem;
    bottom: 1.1rem;
    width: 0.3rem;
    border-radius: var(--cut);
    background: var(--accent-bright);
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
    color: var(--accent-on, #fff);
    font-size: var(--font-xs);
    font-weight: 600;
    line-height: 1.8rem;
    text-align: center;
  }

  .bottom {
    flex: none;
    margin-top: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .bottom :global(.dropdown) {
    width: 100%;
    padding-top: var(--space-3);
    border-top: 1px solid var(--border);
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
      width: var(--sidebar-rail-w);
      padding: 1.8rem 1rem 1.2rem;
      align-items: center;
    }

    .logo {
      flex-direction: column;
      gap: 0.4rem;
      padding: 0.2rem 0 0;
      margin-bottom: var(--space-6);
    }

    .wordmark,
    .nav-label,
    .profile-text {
      display: none;
    }

    nav {
      align-self: stretch;
    }

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
