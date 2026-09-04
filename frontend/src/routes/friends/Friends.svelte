<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import {
    ArrowDownUp,
    ChevronDown,
    EllipsisVertical,
    LayoutGrid,
    List,
    LogIn,
    ShieldCheck,
    UserPlus,
    Users,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import SocialConsentScreen from '../../lib/components/SocialConsentScreen.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import {
    accept,
    block,
    blocks as fetchBlocks,
    decline,
    refresh,
    unblock,
    unfriend,
    type FriendView,
    type RequestView,
    type UserCard,
  } from '../../lib/services/social';
  import { presenceDot, presenceLine, sortFriends } from '../../lib/social/presence';
  import { commonLine, sentAt } from '../../lib/social/view';
  import { friendsView } from '../../lib/stores/ui';
  import { navigate } from '../../lib/stores/router';
  import { friendsPage, incomingCount, needsSocialConsent } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { relativeDate } from '../../lib/utils/format';
  import AddFriendModal from './AddFriendModal.svelte';
  import FriendCodeCard from './FriendCodeCard.svelte';
  import FriendRow from './FriendRow.svelte';

  let { tab: initialTab }: { tab?: string } = $props();

  let tab = $state('all');
  let addOpen = $state(false);
  let codeOpen = $state(false);
  let consentOpen = $state(false);
  let blocked = $state<UserCard[]>([]);
  let busy = $state('');
  let myCode = $state('');
  let search = $state('');
  let sortBy = $state<'name' | 'status'>('name');

  const isGuest = $derived($authState === 'guest');
  const page = $derived($friendsPage);

  const onlineFriends = $derived(
    page.friends.filter((f) => {
      const dot = presenceDot(f.presence);
      return dot === 'online' || dot === 'busy';
    }),
  );
  const awayFriends = $derived(page.friends.filter((f) => presenceDot(f.presence) === 'away'));
  const offlineFriends = $derived(
    page.friends.filter((f) => {
      const dot = presenceDot(f.presence);
      return dot !== 'online' && dot !== 'busy' && dot !== 'away';
    }),
  );

  const presenceTabs = new Set(['all', 'online', 'away', 'offline']);

  const sortLabels: Record<'name' | 'status', string> = {
    name: 'Имя (А-Я)',
    status: 'По статусу',
  };

  const groupEmptyCopy: Record<string, { title: string; description: string }> = {
    online: { title: 'Никто не в сети', description: 'Сейчас никто из друзей не в сети.' },
    away: { title: 'Никто не отошёл', description: 'Сейчас никто из друзей не отходил.' },
    offline: { title: 'Все на связи', description: 'Все друзья сейчас в сети или отошли.' },
  };

  const groupFor = $derived.by(() => {
    if (tab === 'online') return onlineFriends;
    if (tab === 'away') return awayFriends;
    if (tab === 'offline') return offlineFriends;
    return page.friends;
  });

  function sortByName(list: FriendView[]): FriendView[] {
    return [...list].sort((a, b) => (a.displayName || a.username).localeCompare(b.displayName || b.username, 'ru'));
  }

  function matchesSearch(user: UserCard, query: string): boolean {
    return (user.displayName || '').toLowerCase().includes(query) || user.username.toLowerCase().includes(query);
  }

  const visibleFriends = $derived.by(() => {
    const query = search.trim().toLowerCase();
    const filtered = query ? groupFor.filter((f) => matchesSearch(f, query)) : groupFor;
    return sortBy === 'status' ? sortFriends(filtered) : sortByName(filtered);
  });

  function playingGame(friend: FriendView): { igdbId: number; title: string } | null {
    const presence = friend.presence;
    if (!presence || presenceDot(presence) === 'offline' || presence.gameId == null) return null;
    return { igdbId: presence.gameId, title: presence.gameTitle ?? '' };
  }

  function receivedAt(iso: string | null): string {
    const when = relativeDate(iso);
    return when === '—' ? when : `Получена ${when.toLowerCase()}`;
  }

  function incomingStats(request: RequestView): string[] {
    const stats: string[] = [];
    if (request.mutualCount > 0) stats.push(commonLine(request.mutualCount, 0));
    if (request.commonCount > 0) stats.push(commonLine(0, request.commonCount));
    stats.push(receivedAt(request.createdAt));
    return stats;
  }

  function outgoingStats(request: RequestView): string[] {
    const stats: string[] = [];
    if (request.commonCount > 0) stats.push(commonLine(0, request.commonCount));
    stats.push(`Ожидает ответа · ${sentAt(request.createdAt)}`);
    return stats;
  }

  const tabs = $derived([
    { id: 'all', label: 'Все друзья', count: page.friends.length },
    { id: 'online', label: 'В сети', count: onlineFriends.length },
    { id: 'away', label: 'Отошли', count: awayFriends.length },
    { id: 'offline', label: 'Не в сети', count: offlineFriends.length },
    { id: 'requests', label: 'Заявки', count: $incomingCount + page.outgoing.length },
    { id: 'blocked', label: 'Заблокированные', count: blocked.length },
  ]);

  const menuItems = [
    { id: 'profile', label: 'Профиль' },
    { id: 'unfriend', label: 'Удалить из друзей', danger: true, separator: true },
    { id: 'block', label: 'Заблокировать', danger: true },
  ];

  function report(err: unknown, fallback: string) {
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    toast(accountErrorText(err, fallback), 'danger');
  }

  function reload() {
    refresh().catch((err) => report(err, 'Не удалось обновить список друзей'));
  }

  async function loadBlocks() {
    try {
      blocked = await fetchBlocks();
    } catch (err) {
      report(err, 'Не удалось загрузить список заблокированных');
    }
  }

  async function run(id: string, action: () => Promise<void>, fallback: string, done?: string) {
    if (busy) return;
    busy = id;
    try {
      await action();
      if (done) toast(done, 'success');
      await refresh();
      if (id.startsWith('block:') || id.startsWith('unblock:')) await loadBlocks();
    } catch (err) {
      report(err, fallback);
    } finally {
      busy = '';
    }
  }

  function onMenu(user: UserCard, item: string) {
    if (item === 'profile') {
      navigate('user', { username: user.username });
      return;
    }
    if (item === 'unfriend') {
      run(`unfriend:${user.id}`, () => unfriend(user.id), 'Не удалось удалить из друзей', 'Удалён из друзей');
      return;
    }
    run(`block:${user.id}`, () => block(user.id), 'Не удалось заблокировать', 'Пользователь заблокирован');
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => report(err, 'Не удалось открыть вход'));
  }

  function openProfile(user: UserCard) {
    navigate('user', { username: user.username });
  }

  $effect(() => {
    const next = initialTab;
    untrack(() => {
      if (next && tabs.some((t) => t.id === next)) tab = next;
    });
  });

  $effect(() => {
    if (!isGuest && !$needsSocialConsent) loadBlocks();
  });

  onMount(() => {
    if (!isGuest && !$needsSocialConsent) reload();
  });
</script>

<Card surface="panel">
  <PageHeader title="Друзья" subtitle="Заявки, список друзей и заблокированные">
    {#snippet actions()}
      {#if !isGuest && !$needsSocialConsent}
        <Button onclick={() => (codeOpen = !codeOpen)} pressed={codeOpen}>Мой код</Button>
        <Button variant="primary" onclick={() => (addOpen = true)}>
          <UserPlus size="1.5rem" strokeWidth={1.8} />
          Добавить друга
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if isGuest}
    <EmptyState
      title="Друзья доступны с аккаунтом"
      description="Войдите, чтобы добавлять друзей, видеть их профили и общие игры."
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => signIn('login')}>
          <LogIn size="1.5rem" strokeWidth={1.8} />
          Войти
        </Button>
        <Button onclick={() => signIn('register')}>Создать аккаунт</Button>
      {/snippet}
    </EmptyState>
  {:else if $needsSocialConsent}
    <EmptyState
      title="Нужна синхронизация с аккаунтом"
      description="Друзья работают поверх синхронизации: без неё серверу нечего показать вашим друзьям, а вам — их профили."
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (consentOpen = true)}>Включить синхронизацию</Button>
      {/snippet}
    </EmptyState>
  {:else}
    {#if codeOpen}
      <div class="code">
        <FriendCodeCard bind:code={myCode} />
      </div>
    {/if}

    <Tabs {tabs} bind:value={tab} variant="pill" />

    {#if presenceTabs.has(tab)}
      <div class="toolbar">
        <div class="search-wrap">
          <SearchInput bind:value={search} placeholder="Поиск друзей" />
        </div>
        <div class="toolbar-right">
          <DropdownMenu
            items={[
              { id: 'name', label: sortLabels.name },
              { id: 'status', label: sortLabels.status },
            ]}
            onselect={(id) => (sortBy = id as 'name' | 'status')}
          >
            {#snippet trigger({ open, toggle })}
              <button class="sort" class:open onclick={toggle}>
                <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
                Сортировка: {sortLabels[sortBy]}
                <ChevronDown size="1.4rem" strokeWidth={1.8} />
              </button>
            {/snippet}
          </DropdownMenu>
          <SegmentedControl
            bind:value={$friendsView}
            options={[
              { id: 'list', label: 'Список' },
              { id: 'grid', label: 'Сетка' },
            ]}
          >
            {#snippet item(option)}
              {#if option.id === 'list'}
                <List size="1.6rem" strokeWidth={1.8} />
              {:else}
                <LayoutGrid size="1.6rem" strokeWidth={1.8} />
              {/if}
            {/snippet}
          </SegmentedControl>
        </div>
      </div>

      {#if visibleFriends.length === 0}
        {#if page.friends.length === 0}
          <EmptyState title="Пока никого" description="Добавьте друга по имени пользователя или по коду." />
        {:else if search.trim()}
          <EmptyState title="Ничего не найдено" description="Попробуйте изменить запрос поиска." />
        {:else if groupEmptyCopy[tab]}
          <EmptyState title={groupEmptyCopy[tab].title} description={groupEmptyCopy[tab].description} />
        {:else}
          <EmptyState title="Пока никого" description="Добавьте друга по имени пользователя или по коду." />
        {/if}
      {:else if $friendsView === 'grid'}
        <div class="grid">
          {#each visibleFriends as friend (friend.id)}
            <FriendRow
              user={friend}
              status={presenceDot(friend.presence)}
              meta={presenceLine(friend.presence)}
              game={playingGame(friend)}
              variant="grid"
              onopen={() => openProfile(friend)}
            >
              {#snippet actions()}
                <DropdownMenu items={menuItems} onselect={(item) => onMenu(friend, item)}>
                  {#snippet trigger({ toggle })}
                    <IconButton label="Ещё" size="sm" onclick={toggle}>
                      <EllipsisVertical size="1.7rem" strokeWidth={1.8} />
                    </IconButton>
                  {/snippet}
                </DropdownMenu>
              {/snippet}
            </FriendRow>
          {/each}
        </div>
      {:else}
        <div class="list">
          {#each visibleFriends as friend (friend.id)}
            <FriendRow
              user={friend}
              status={presenceDot(friend.presence)}
              meta={presenceLine(friend.presence)}
              game={playingGame(friend)}
              variant="list"
              onopen={() => openProfile(friend)}
            >
              {#snippet actions()}
                <DropdownMenu items={menuItems} onselect={(item) => onMenu(friend, item)}>
                  {#snippet trigger({ toggle })}
                    <IconButton label="Ещё" size="sm" onclick={toggle}>
                      <EllipsisVertical size="1.7rem" strokeWidth={1.8} />
                    </IconButton>
                  {/snippet}
                </DropdownMenu>
              {/snippet}
            </FriendRow>
          {/each}
        </div>
      {/if}
    {:else if tab === 'requests'}
      {#if page.incoming.length === 0 && page.outgoing.length === 0}
        <EmptyState title="Заявок нет" description="Новые заявки появятся здесь." />
      {:else}
        {#if page.incoming.length > 0}
          <h4 class="eyebrow">Входящие заявки</h4>
          <div class="cards">
            {#each page.incoming as request (request.id)}
              <FriendRow user={request} variant="card" stats={incomingStats(request)} onopen={() => openProfile(request)}>
                {#snippet actions()}
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={!!busy}
                    onclick={() =>
                      run(`accept:${request.id}`, () => accept(request.id), 'Не удалось принять заявку', 'Вы теперь друзья')}
                  >
                    Принять
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!!busy}
                    onclick={() => run(`decline:${request.id}`, () => decline(request.id), 'Не удалось отклонить заявку')}
                  >
                    Отклонить
                  </Button>
                {/snippet}
              </FriendRow>
            {/each}
          </div>
        {/if}

        {#if page.outgoing.length > 0}
          <h4 class="eyebrow">Отправленные</h4>
          <div class="cards">
            {#each page.outgoing as request (request.id)}
              <FriendRow user={request} variant="card" stats={outgoingStats(request)} onopen={() => openProfile(request)}>
                {#snippet actions()}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!!busy}
                    onclick={() => run(`cancel:${request.id}`, () => decline(request.id), 'Не удалось отменить заявку')}
                  >
                    Отменить
                  </Button>
                {/snippet}
              </FriendRow>
            {/each}
          </div>
        {/if}
      {/if}

      <div class="safety">
        <ShieldCheck size="2rem" strokeWidth={1.6} />
        <div class="safety-text">
          <h4>Ваша безопасность — наш приоритет</h4>
          <p>Не принимайте заявки от незнакомых пользователей и не делитесь личными данными.</p>
        </div>
      </div>
    {:else if blocked.length === 0}
      <EmptyState
        title="Никого не заблокировано"
        description="Заблокированные не видят ваш профиль и не могут отправить заявку."
      />
    {:else}
      <div class="cards">
        {#each blocked as user (user.id)}
          <FriendRow {user} variant="card">
            {#snippet actions()}
              <Button
                size="sm"
                disabled={!!busy}
                onclick={() =>
                  run(`unblock:${user.id}`, () => unblock(user.id), 'Не удалось разблокировать', 'Пользователь разблокирован')}
              >
                Разблокировать
              </Button>
            {/snippet}
          </FriendRow>
        {/each}
      </div>
    {/if}
  {/if}
</Card>

{#if !isGuest && !$needsSocialConsent}
  <AddFriendModal bind:open={addOpen} onsent={reload} />
{/if}
<SocialConsentScreen bind:open={consentOpen} />

<style>
  .code {
    margin-bottom: var(--space-5);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin: var(--space-5) 0;
    flex-wrap: wrap;
  }

  .search-wrap {
    flex: 1;
    min-width: 20rem;
    max-width: 40rem;
  }

  .toolbar-right {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-shrink: 0;
  }

  .sort {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: var(--control-sm);
    padding: 0 1.1rem;
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .sort:hover,
  .sort.open {
    background: var(--hover);
    color: var(--text);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list :global(.row + .row) {
    border-top: 1px solid var(--border);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(22rem, 1fr));
    gap: var(--space-4);
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .eyebrow {
    margin: var(--space-6) 0 var(--space-3);
    padding: 0 0.2rem;
    font-size: 1.2rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  .safety {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
    margin-top: var(--space-6);
    padding: var(--space-4) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-3);
  }

  .safety-text h4 {
    font-size: var(--font-sm);
    font-weight: 600;
    color: var(--text);
  }

  .safety-text p {
    margin-top: 0.3rem;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }
</style>
