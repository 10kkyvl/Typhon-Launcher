<script lang="ts">
  import { navigate, route } from '../stores/router';
  import { accountErrorText } from '../services/accountMessages';
  import type { PresenceStatus } from '../services/online';
  import { libraryGames, runningGames } from '../stores/library';
  import { autoAway, presenceStatus, shownPresence, updatePresenceStatus } from '../stores/presence';
  import { authState, currentUser, isOffline, signOut } from '../stores/user';
  import { ownFrame } from '../profile/frameSources';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';
  import Tooltip from './Tooltip.svelte';
  import { signOutPrompt, type ConfirmPrompt } from '../confirm/prompts';
  import Avatar from './Avatar.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import { accountMenuItems, chipPresence } from './accountMenu';
  import { signInFromGuest } from './sidebarStatus';

  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);

  const isGuest = $derived($authState === 'guest');

  const avatarName = $derived($currentUser ? $currentUser.displayName || $currentUser.username : isGuest ? msg('ui.guest') : '');

  const profileMenu = $derived(accountMenuItems(isGuest, $presenceStatus));

  const presence = $derived(
    chipPresence($currentUser ? 'user' : isGuest ? 'guest' : 'none', $shownPresence, $runningGames, $libraryGames),
  );

  // What the chip's status line does not say: the status went away by itself, the profile is a cached copy.
  const chipHint = $derived(
    [$currentUser && $autoAway ? msg('ui.presenceAutoAwayHint') : '', $currentUser && $isOffline ? msg('ui.offlineProfileTooltip') : '']
      .filter(Boolean)
      .join(' '),
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

<DropdownMenu items={profileMenu} placement="down" align="right" onselect={onProfileSelect}>
  {#snippet trigger({ props })}
    <Tooltip text={chipHint} placement="bottom">
    <button
      {...props}
      type="button"
      class="chip"
      data-part="topbar-account"
      class:active={$route.name === 'profile'}
    >
      <span class="avatar">
        <Avatar
          size="sm"
          name={avatarName}
          src={$currentUser?.avatarUrl}
          status={presence.dot}
          {...ownFrame($currentUser)}
        />
      </span>
      <span class="chip-text">
        {#if $currentUser}
          <span class="chip-name">{$currentUser.displayName}</span>
          <span class="chip-status">{presence.line}{#if $isOffline}<span class="offline-mark">{msg('ui.offlineSuffix')}</span>{/if}</span>
        {:else if isGuest}
          <span class="chip-name">{msg('ui.guest')}</span>
          <span class="chip-status">{presence.line}</span>
        {:else}
          <span class="chip-name">{msg('ui.notAuthorized')}</span>
        {/if}
      </span>
    </button>
    </Tooltip>
  {/snippet}
</DropdownMenu>

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .chip {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    max-width: var(--topbar-chip-max);
    height: var(--control-md);
    padding: 0 var(--space-3) 0 0.2rem;
    border-radius: var(--radius-control);
    text-align: left;
    color: var(--text-3);
    transition: background var(--dur) var(--ease);
  }

  .chip:hover,
  .chip.active,
  .chip[aria-expanded='true'] {
    background: var(--hover-strong);
  }

  .avatar {
    display: block;
    flex-shrink: 0;
    --avatar-ring: var(--bg-sidebar);
  }

  .chip-text {
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .chip-name,
  .chip-status {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
  }

  .chip-name {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
  }

  .chip-status {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .offline-mark {
    color: var(--warning);
  }

  /* Where the search would start to shrink below its full width (topbarWidths.test.ts): the history pill,
     the right island with three actions and a full chip, two gaps and the full search. The text stays in
     the button's name for assistive tech. */
  @container topbar (max-width: 87.2rem) {
    .chip {
      padding: 0.2rem;
    }

    .chip-text {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
  }

  /* The centred search needs the right island's width on both sides of it. */
  @container topbar-centred (max-width: 112rem) {
    .chip {
      padding: 0.2rem;
    }

    .chip-text {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
  }
</style>
