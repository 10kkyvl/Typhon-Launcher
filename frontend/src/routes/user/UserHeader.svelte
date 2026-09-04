<script lang="ts">
  import { ChevronDown, EllipsisVertical } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { PublicProfile } from '../../lib/services/social';
  import { dotKind, presenceDot, presenceLine } from '../../lib/social/presence';
  import { memberSince, relationLabel } from '../../lib/social/view';

  let {
    profile,
    busy,
    onaction,
  }: {
    profile: PublicProfile;
    busy: boolean;
    onaction: (id: string) => void;
  } = $props();

  type MenuItem = { id: string; label: string; danger?: boolean; separator?: boolean };

  const name = $derived(profile.displayName || profile.username);
  const since = $derived(memberSince(profile.createdAt));
  const presence = $derived(presenceDot(profile.presence));
  const presenceText = $derived(presenceLine(profile.presence, new Date(), profile.relation === 'self'));
  const pending = $derived(profile.relation === 'none' || profile.relation === 'outgoing' || profile.relation === 'incoming');

  const friendMenu: MenuItem[] = [
    { id: 'unfriend', label: 'Удалить из друзей', danger: true },
    { id: 'block', label: 'Заблокировать', danger: true },
  ];

  const blockMenu: MenuItem[] = [{ id: 'block', label: 'Заблокировать', danger: true }];
</script>

<section class="user-header">
  <Card>
    <div class="head">
      <Avatar size="lg" name={name} src={profile.avatarUrl} status={presence} />

      <div class="identity">
        <h2 class="display-name">{name}</h2>
        <span class="username">@{profile.username}</span>
        {#if profile.bio}<p class="bio">{profile.bio}</p>{/if}
        <div class="status">
          <StatusBadge plain kind={dotKind(presence)} label={presenceText} />
        </div>
        {#if since}<span class="since">{since}</span>{/if}
      </div>

      <div class="head-actions">
        {#if profile.relation === 'friend'}
          <DropdownMenu items={friendMenu} onselect={onaction}>
            {#snippet trigger({ toggle })}
              <Button disabled={busy} onclick={toggle}>
                {relationLabel('friend')}
                <ChevronDown size="1.5rem" strokeWidth={1.8} />
              </Button>
            {/snippet}
          </DropdownMenu>
        {:else if pending}
          {#if profile.relation === 'none'}
            <Button variant="primary" disabled={busy} onclick={() => onaction('add')}>
              {relationLabel('none')}
            </Button>
          {:else if profile.relation === 'outgoing'}
            <Button disabled>{relationLabel('outgoing')}</Button>
            <Button variant="ghost" disabled={busy} onclick={() => onaction('cancel')}>Отменить</Button>
          {:else}
            <Button variant="primary" disabled={busy} onclick={() => onaction('accept')}>
              {relationLabel('incoming')}
            </Button>
            <Button variant="ghost" disabled={busy} onclick={() => onaction('decline')}>Отклонить</Button>
          {/if}
          <DropdownMenu items={blockMenu} onselect={onaction}>
            {#snippet trigger({ toggle })}
              <IconButton label="Ещё" onclick={toggle}>
                <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
              </IconButton>
            {/snippet}
          </DropdownMenu>
        {/if}
      </div>
    </div>
  </Card>
</section>

<style>
  .user-header {
    display: block;
    margin-bottom: var(--space-10);
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

  .bio {
    max-width: 60rem;
    margin-top: 0.4rem;
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-2);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .status {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .since {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .head-actions {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  @media (max-width: 1200px) {
    .head {
      flex-wrap: wrap;
    }
  }
</style>
