<script lang="ts">
  import { LogIn, UserRound } from '@lucide/svelte';
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
  import { authState, leaveGuest } from '../../lib/stores/user';
  import UserActivity from './UserActivity.svelte';
  import UserCommon from './UserCommon.svelte';
  import UserCovers from './UserCovers.svelte';
  import UserHeader from './UserHeader.svelte';
  import UserMutual from './UserMutual.svelte';
  import UserRecent from './UserRecent.svelte';
  import UserStats from './UserStats.svelte';

  let { username }: { username?: string } = $props();

  let data = $state<PublicProfile | null>(null);
  let loading = $state(true);
  let refreshing = $state(false);
  let failure = $state('');
  let missing = $state(false);
  let busy = $state(false);

  const isGuest = $derived($authState === 'guest');
  const name = $derived(data ? data.displayName || data.username : '');
  const stranger = $derived(!!data && data.relation !== 'friend' && data.relation !== 'self');
  const closed = $derived(!!data && data.visibility === 'private' && data.relation !== 'self');
  const restricted = $derived(!!data && data.visibility === 'friends' && stranger);
  const showcase = $derived(data && !closed ? data.showcase : []);
  const common = $derived(data && !closed && data.common && data.common.count > 0 ? data.common : null);
  const recent = $derived(data && !closed ? data.recentlyPlayed : []);
  const activity = $derived(data && !closed ? data.recentActivity : []);
  const favorites = $derived(data && !closed ? data.favorites : []);
  const mutual = $derived(data && !closed && data.mutualCount > 0 ? data.mutualFriends : []);

  async function load(target: string, quiet = false) {
    if (quiet) {
      refreshing = true;
    } else {
      loading = true;
      failure = '';
      missing = false;
    }
    try {
      const loaded = await fetchProfile(target);
      if (target !== username) return;
      if (loaded.relation === 'self') {
        navigate('profile');
        return;
      }
      data = loaded;
      failure = '';
      missing = false;
    } catch (err) {
      if (target !== username) return;
      if (quiet) {
        toast(accountErrorText(err, 'Не удалось обновить профиль'), 'danger');
        return;
      }
      data = null;
      missing = err instanceof AccountError && err.code === 'user_not_found';
      failure = accountErrorText(err, 'Не удалось загрузить профиль');
    } finally {
      if (target === username) {
        loading = false;
        refreshing = false;
      }
    }
  }

  async function act(id: string) {
    const current = data;
    if (!current || busy) return;
    busy = true;
    try {
      let done = '';
      if (id === 'add') {
        const result = await sendRequest(current.username);
        done = result.accepted ? 'Вы теперь друзья' : 'Заявка отправлена';
      } else if (id === 'cancel') {
        await decline(current.id);
        done = 'Заявка отменена';
      } else if (id === 'decline') {
        await decline(current.id);
        done = 'Заявка отклонена';
      } else if (id === 'accept') {
        await accept(current.id);
        done = 'Вы теперь друзья';
      } else if (id === 'unfriend') {
        await unfriend(current.id);
        done = 'Удалён из друзей';
      } else if (id === 'block') {
        await block(current.id);
        await refresh();
        toast('Пользователь заблокирован', 'success');
        navigate('friends', { tab: 'blocked' });
        return;
      } else {
        return;
      }
      toast(done, 'success');
      await refresh();
      await load(current.username, true);
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось выполнить действие'), 'danger');
    } finally {
      busy = false;
    }
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => toast(accountErrorText(err, 'Не удалось открыть вход'), 'danger'));
  }

  $effect(() => {
    const target = username ?? '';
    if (isGuest) {
      data = null;
      loading = false;
      missing = false;
      failure = '';
      return;
    }
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

{#if isGuest}
  <EmptyState
    title="Профили доступны с аккаунтом"
    description="Войдите, чтобы смотреть профили других игроков, их общие с вами игры и друзей."
  >
    {#snippet icon()}
      <UserRound size="2.2rem" strokeWidth={1.6} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => signIn('login')}>
        <LogIn size="1.5rem" strokeWidth={1.8} />
        Войти
      </Button>
      <Button onclick={() => signIn('register')}>Создать аккаунт</Button>
    {/snippet}
  </EmptyState>
{:else if loading}
  <p class="muted">Загрузка…</p>
{:else if missing}
  <EmptyState title="Пользователь не найден" description="Проверьте имя пользователя или код друга">
    {#snippet icon()}
      <UserRound size="2.2rem" strokeWidth={1.6} />
    {/snippet}
  </EmptyState>
{:else if failure}
  <EmptyState title="Не удалось загрузить профиль" description={failure}>
    {#snippet icon()}
      <UserRound size="2.2rem" strokeWidth={1.6} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => load(username ?? '')}>Повторить</Button>
    {/snippet}
  </EmptyState>
{:else if data}
  <div class="profile" class:refreshing>
    <div class="main">
      <div class="area area-header">
        <UserHeader profile={data} {busy} onaction={act} />
        {#if closed}
          <p class="muted note">Профиль закрыт</p>
        {:else if restricted}
          <p class="muted note">Остальное видно друзьям</p>
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
        {#if activity.length > 0}
          <div class="area area-activity">
            <UserActivity items={activity} />
          </div>
        {/if}
        <div class="area area-showcase">
          {#each showcase as fblock (fblock.kind)}
            <UserCovers title={showcaseTitle(fblock.kind)} games={fblock.games} columns="main" />
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

<style>
  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .note {
    margin-bottom: var(--space-10);
  }

  .profile {
    display: flex;
    flex-direction: column;
    transition: opacity var(--dur) var(--ease);
  }

  .profile.refreshing {
    opacity: 0.6;
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

  .area-activity {
    order: 7;
  }

  .area-showcase {
    order: 8;
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
