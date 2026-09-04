<script lang="ts">
  import { UserRound } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import type { PublicProfile } from '../../lib/services/social';
  import {
    accept,
    block,
    decline,
    profile as fetchProfile,
    refresh,
    sendRequest,
    unfriend,
  } from '../../lib/services/social';
  import { showcaseTitle } from '../../lib/social/view';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import UserCommon from './UserCommon.svelte';
  import UserCovers from './UserCovers.svelte';
  import UserHeader from './UserHeader.svelte';
  import UserMutual from './UserMutual.svelte';
  import UserRecent from './UserRecent.svelte';
  import UserStats from './UserStats.svelte';

  let { username }: { username?: string } = $props();

  let data = $state<PublicProfile | null>(null);
  let loading = $state(true);
  let failure = $state('');
  let missing = $state(false);
  let busy = $state(false);

  const name = $derived(data ? data.displayName || data.username : '');
  const closed = $derived(!!data && data.visibility === 'private' && data.relation !== 'friend');
  const showcase = $derived(data && !closed ? data.showcase : []);
  const common = $derived(data && !closed && data.common && data.common.count > 0 ? data.common : null);
  const recent = $derived(data && !closed ? data.recentlyPlayed : []);
  const favorites = $derived(data && !closed ? data.favorites : []);
  const mutual = $derived(data && !closed && data.mutualCount > 0 ? data.mutualFriends : []);

  async function load(target: string) {
    loading = true;
    failure = '';
    missing = false;
    try {
      const loaded = await fetchProfile(target);
      if (target !== username) return;
      if (loaded.relation === 'self') {
        navigate('profile');
        return;
      }
      data = loaded;
    } catch (err) {
      if (target !== username) return;
      data = null;
      missing = err instanceof AccountError && err.code === 'user_not_found';
      failure = accountErrorText(err, 'Не удалось загрузить профиль');
    } finally {
      if (target === username) loading = false;
    }
  }

  async function act(id: string) {
    const current = data;
    if (!current || busy) return;
    busy = true;
    try {
      if (id === 'add') await sendRequest(current.username);
      else if (id === 'cancel' || id === 'decline') await decline(current.id);
      else if (id === 'accept') await accept(current.id);
      else if (id === 'unfriend') await unfriend(current.id);
      else if (id === 'block') await block(current.id);
      else return;
      await refresh();
      await load(current.username);
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось выполнить действие'), 'danger');
    } finally {
      busy = false;
    }
  }

  $effect(() => {
    const target = username ?? '';
    if (!target) {
      data = null;
      loading = false;
      missing = true;
      failure = '';
      return;
    }
    void load(target);
  });
</script>

<PageHeader title={username ? `@${username}` : 'Профиль'} />

<div class="page">
  {#if loading}
    <p class="muted">Загрузка…</p>
  {:else if missing}
    <EmptyState title="Пользователь не найден" description="Проверьте имя пользователя или код друга">
      {#snippet icon()}
        <UserRound size="2.2rem" strokeWidth={1.6} />
      {/snippet}
    </EmptyState>
  {:else if failure}
    <EmptyState title={failure} description="Попробуйте обновить страницу">
      {#snippet icon()}
        <UserRound size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => load(username ?? '')}>Повторить</Button>
      {/snippet}
    </EmptyState>
  {:else if data}
    <div class="profile">
      <div class="main">
        <div class="area area-header">
          <UserHeader profile={data} {busy} onaction={act} />
          {#if closed}
            <p class="muted closed">Профиль закрыт</p>
          {/if}
        </div>
        {#if !closed}
          {#if common}
            <div class="area area-common">
              <UserCommon {common} {name} />
            </div>
          {/if}
          {#if recent.length > 0}
            <div class="area area-recent">
              <UserRecent games={recent} />
            </div>
          {/if}
          <div class="area area-showcase">
            {#each showcase as fblock (fblock.kind)}
              <UserCovers title={showcaseTitle(fblock.kind)} games={fblock.games} />
            {/each}
          </div>
        {/if}
      </div>
      {#if !closed}
        <div class="side">
          <div class="area area-stats">
            <UserStats stats={data.stats} />
          </div>
          <div class="area area-favorites">
            <UserCovers title="Любимые" games={favorites} />
          </div>
          {#if mutual.length > 0}
            <div class="area area-mutual">
              <UserMutual friends={mutual} count={data.mutualCount} />
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .page {
    max-width: 96rem;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .closed {
    margin-bottom: var(--space-10);
  }

  .profile {
    display: flex;
    flex-direction: column;
  }

  .main,
  .side {
    display: contents;
  }

  .area {
    min-width: 0;
  }

  .area-header {
    order: 1;
  }

  .area-stats {
    order: 2;
  }

  .area-favorites {
    order: 3;
  }

  .area-mutual {
    order: 4;
  }

  .area-common {
    order: 5;
  }

  .area-recent {
    order: 6;
  }

  .area-showcase {
    order: 7;
  }

  @media (min-width: 1600px) {
    .profile {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 40rem;
      gap: 0 var(--space-12);
      align-items: start;
    }

    .main,
    .side {
      display: flex;
      flex-direction: column;
      min-width: 0;
    }
  }
</style>
