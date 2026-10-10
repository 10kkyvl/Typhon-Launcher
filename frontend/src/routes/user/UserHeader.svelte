<script lang="ts">
  import { Calendar, EllipsisVertical, Gamepad2 } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import StyledName from '../../lib/components/StyledName.svelte';
  import { appearanceOf } from '../../lib/profile/appearance';
  import type { PublicProfile } from '../../lib/services/social';
  import { presenceDot } from '../../lib/social/presence';
  import { memberSince, relationLabel } from '../../lib/social/view';
  import { msg } from '../../lib/i18n';
  import UserStats from './UserStats.svelte';

  let {
    profile,
    busy,
    onaction,
    onmessage,
  }: {
    profile: PublicProfile;
    busy: boolean;
    onaction: (id: string) => void;
    onmessage?: () => void;
  } = $props();

  type MenuItem = { id: string; label: string; danger?: boolean; separator?: boolean };

  const name = $derived(profile.displayName || profile.username);
  const look = $derived(appearanceOf(profile.appearance));
  const since = $derived(memberSince(profile.createdAt));
  const presence = $derived(presenceDot(profile.presence));
  const playing = $derived(presence !== 'offline' && profile.presence?.gameId != null);
  const playingLine = $derived(
    playing ? (profile.presence?.gameTitle ? msg('social.playingIn', { name: profile.presence.gameTitle }) : msg('social.playing')) : '',
  );
  const pending = $derived(profile.relation === 'none' || profile.relation === 'outgoing' || profile.relation === 'incoming');

  const friendMenu: MenuItem[] = [
    { id: 'unfriend', label: msg('social.unfriendLabel'), danger: true },
    { id: 'block', label: msg('social.blockLabel'), danger: true },
  ];

  const blockMenu: MenuItem[] = [{ id: 'block', label: msg('social.blockLabel'), danger: true }];
</script>

<section class="user-header">
  <div class="header-surface">
    <div class="head">
      <Avatar size="lg" name={name} src={profile.avatarUrl} status={presence} frame={look.avatarFrame} />

      <div class="identity">
        <h2 class="display-name"><StyledName {name} styleName={look.nameStyle} /></h2>
        <span class="username">@{profile.username}</span>
        {#if profile.statusEmoji || profile.statusText}
          <p class="status">{#if profile.statusEmoji}<span class="status-emoji">{profile.statusEmoji}</span>{/if}{#if profile.statusText}<span>{profile.statusText}</span>{/if}</p>
        {/if}
        {#if profile.bio}<p class="bio">{profile.bio}</p>{/if}
        <div class="meta">
          {#if playingLine}
            <span class="meta-item now">
              <Gamepad2 size="1.5rem" strokeWidth={1.8} />
              {playingLine}
            </span>
          {/if}
          {#if since}
            <span class="meta-item">
              <Calendar size="1.5rem" strokeWidth={1.8} />
              {since}
            </span>
          {/if}
        </div>
      </div>

      <div class="right">
        <div class="head-actions">
          {#if profile.relation === 'friend'}
            <StatusBadge kind="success" label={relationLabel('friend')} />
            {#if onmessage}<Button variant="primary" onclick={onmessage}>{msg('social.chatWrite')}</Button>{/if}
            <DropdownMenu items={friendMenu} onselect={onaction}>
              {#snippet trigger({ toggle })}
                <IconButton label={msg('social.moreLabel')} onclick={toggle}>
                  <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          {:else if pending}
            {#if profile.relation === 'none'}
              <Button variant="primary" disabled={busy} onclick={() => onaction('add')}>
                {relationLabel('none')}
              </Button>
            {:else if profile.relation === 'outgoing'}
              <StatusBadge kind="neutral" label={relationLabel('outgoing')} />
              <Button variant="ghost" disabled={busy} onclick={() => onaction('cancel')}>{msg('social.cancelRequestButton')}</Button>
            {:else}
              <Button variant="primary" disabled={busy} onclick={() => onaction('accept')}>
                {relationLabel('incoming')}
              </Button>
              <Button disabled={busy} onclick={() => onaction('decline')}>{msg('social.declineButton')}</Button>
            {/if}
            <DropdownMenu items={blockMenu} onselect={onaction}>
              {#snippet trigger({ toggle })}
                <IconButton label={msg('social.moreLabel')} onclick={toggle}>
                  <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          {/if}
        </div>
        <div class="stats">
          <UserStats stats={profile.stats} />
        </div>
      </div>
    </div>
  </div>
</section>

<style>
  .header-surface { padding-bottom: 2rem; position: relative; }
  .user-header {
    container-type: inline-size;
    display: block;
    margin-bottom: var(--space-6);
  }

  .head {
    display: flex;
    align-items: flex-start;
    gap: var(--space-5);
  }

  .identity {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .display-name {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
    overflow-wrap: anywhere;
  }

  .username {
    font-size: var(--font-md);
    color: var(--text-3);
  }

  .status {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .bio {
    max-width: 60rem;
    margin-top: 0.4rem;
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-2);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-4);
    margin-top: 0.4rem;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .meta-item :global(svg) {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .meta-item.now {
    min-width: 0;
    height: 2.8rem;
    padding: 0 1.1rem;
    border-radius: var(--radius-xl);
    background: var(--accent-subtle);
    color: var(--accent-text);
    font-weight: 500;
  }

  .meta-item.now :global(svg) {
    color: inherit;
  }

  .identity,
  .right {
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .right {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-4);
    flex-shrink: 0;
  }

  .head-actions {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  @container (max-width: 900px) {
    .head {
      flex-wrap: wrap;
    }

    .right {
      align-items: flex-start;
      width: 100%;
    }
  }
</style>
