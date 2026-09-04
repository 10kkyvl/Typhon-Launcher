<script lang="ts">
  import Avatar from '../../lib/components/Avatar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { AccountError } from '../../lib/services/account';
  import { statusBadgeKind, statusLabel } from '../../lib/game/status';
  import { gameFriends, type GameFriends } from '../../lib/services/social';
  import { friendsPage } from '../../lib/stores/social';
  import { navigate } from '../../lib/stores/router';
  import { authState } from '../../lib/stores/user';
  import { playtime, plural } from '../../lib/utils/format';

  let { canonicalGameId }: { canonicalGameId: string } = $props();

  let page = $state<GameFriends | null>(null);
  let warned = false;

  const enabled = $derived(
    !!canonicalGameId && $authState === 'authenticated' && $friendsPage.friends.length > 0,
  );

  $effect(() => {
    const id = canonicalGameId;
    if (!enabled) {
      page = null;
      return;
    }
    let cancelled = false;
    gameFriends(id)
      .then((result) => {
        if (!cancelled) page = result;
      })
      .catch((err) => {
        if (cancelled) return;
        page = null;
        if (err instanceof AccountError && err.code === 'unknown_game') return;
        if (warned) return;
        warned = true;
        console.warn('game friends request failed', err);
      });
    return () => {
      cancelled = true;
    };
  });

  const played = $derived(page?.played ?? []);
  const playingNow = $derived(page?.playingNow ?? []);
  const playedLine = $derived(
    `${played.length} ${plural(played.length, 'друг', 'друга', 'друзей')} ${plural(played.length, 'играл', 'играли', 'играли')}`,
  );
</script>

{#if played.length > 0 || playingNow.length > 0}
  <section class="panel">
    <h2 class="heading sm">Друзья</h2>

    {#if played.length > 0}
      <p class="line">{playedLine}</p>
      <ul class="people">
        {#each played as friend (friend.id)}
          <li class="person">
            <button class="who" type="button" onclick={() => navigate('user', { username: friend.username })}>
              <Avatar size="sm" name={friend.displayName || friend.username} src={friend.avatarUrl} />
              <span class="names">
                <span class="name">{friend.displayName || friend.username}</span>
                <span class="meta">
                  {#if friend.playtimeSeconds}
                    <span class="time">{playtime(friend.playtimeSeconds)}</span>
                  {/if}
                  {#if friend.status}
                    <StatusBadge plain kind={statusBadgeKind(friend.status)} label={statusLabel(friend.status)} />
                  {/if}
                </span>
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    {#if playingNow.length > 0}
      <p class="line sub">Играют сейчас</p>
      <ul class="people">
        {#each playingNow as friend (friend.id)}
          <li class="person">
            <button class="who" type="button" onclick={() => navigate('user', { username: friend.username })}>
              <Avatar size="sm" name={friend.displayName || friend.username} src={friend.avatarUrl} status="online" />
              <span class="names">
                <span class="name">{friend.displayName || friend.username}</span>
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .panel {
    padding: var(--space-5);
    background: var(--surface);
    border-radius: var(--radius-lg);
  }

  .heading.sm {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    color: var(--text-2);
    margin-bottom: var(--space-2);
  }

  .line {
    font-size: var(--font-xs);
    color: var(--text-3);
    margin-bottom: var(--space-2);
  }

  .line.sub {
    margin-top: var(--space-4);
  }

  .people {
    display: flex;
    flex-direction: column;
    list-style: none;
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.6rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .who:hover {
    background: var(--hover);
  }

  .names {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .name {
    font-size: var(--font-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .time {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
