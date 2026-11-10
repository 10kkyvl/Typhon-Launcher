<script lang="ts">
  import { navigate, route } from '../stores/router';
  import { accountErrorText } from '../services/accountMessages';
  import { PRESENCE_STATUSES, type PresenceStatus } from '../services/online';
  import { STATUS_LABELS, statusDot } from '../social/presence';
  import { autoAway, presenceStatus, shownPresence, updatePresenceStatus } from '../stores/presence';
  import { authState, currentUser, isOffline, signOut } from '../stores/user';
  import { ownFrame } from '../profile/frameSources';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';
  import { signOutPrompt, type ConfirmPrompt } from '../confirm/prompts';
  import Avatar from './Avatar.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import type { MenuItem } from './DropdownMenu.svelte';
  import { signInFromGuest } from './sidebarStatus';

  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);

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

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
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
    .profile-text {
      display: none;
    }

    .profile {
      justify-content: center;
      padding: 0.6rem;
    }
  }
</style>
