<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { EllipsisVertical, LogIn, UserPlus, Users } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import FeedRow from '../../lib/components/FeedRow.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
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
    type UserCard,
  } from '../../lib/services/social';
  import { feedDayGroups } from '../../lib/social/feed';
  import { presenceDot, presenceLine, sortFriends } from '../../lib/social/presence';
  import { commonLine, sentAt } from '../../lib/social/view';
  import { feedCursor, feedEvents, feedLoading, loadFeed, moreFeed, reactToEvent } from '../../lib/stores/feed';
  import { navigate } from '../../lib/stores/router';
  import { friendsPage, incomingCount, needsSocialConsent } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import AddFriendModal from './AddFriendModal.svelte';
  import FriendCodeCard from './FriendCodeCard.svelte';
  import FriendRow from './FriendRow.svelte';

  let { tab: initialTab }: { tab?: string } = $props();

  let tab = $state('friends');
  let addOpen = $state(false);
  let codeOpen = $state(false);
  let consentOpen = $state(false);
  let blocked = $state<UserCard[]>([]);
  let busy = $state('');
  let myCode = $state('');

  const isGuest = $derived($authState === 'guest');
  const page = $derived($friendsPage);
  const friends = $derived(sortFriends(page.friends));
  const feedGroups = $derived(feedDayGroups($feedEvents));

  const tabs = $derived([
    { id: 'friends', label: `Друзья (${page.friends.length})` },
    { id: 'requests', label: `Заявки (${$incomingCount + page.outgoing.length})` },
    { id: 'feed', label: 'Лента' },
    { id: 'blocked', label: 'Заблокированные' },
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
      if (tab === 'blocked' || id.startsWith('block:')) await loadBlocks();
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
    if (tab === 'blocked' && !isGuest && !$needsSocialConsent) loadBlocks();
  });

  $effect(() => {
    if (tab === 'feed' && !isGuest && !$needsSocialConsent) void loadFeed(true);
  });

  onMount(() => {
    if (!isGuest && !$needsSocialConsent) reload();
  });
</script>

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

<div class="page">
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

    <Tabs {tabs} bind:value={tab} />

    <div class="list">
      {#if tab === 'friends'}
        {#if page.friends.length === 0}
          <EmptyState title="Пока никого" description="Добавьте друга по имени пользователя или по коду." />
        {:else}
          {#each friends as friend (friend.id)}
            <FriendRow
              user={friend}
              status={presenceDot(friend.presence)}
              meta={presenceLine(friend.presence)}
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
        {/if}
      {:else if tab === 'requests'}
        {#if page.incoming.length === 0 && page.outgoing.length === 0}
          <EmptyState title="Заявок нет" description="Новые заявки появятся здесь." />
        {:else}
          {#if page.incoming.length > 0}
            {#each page.incoming as request (request.id)}
              <FriendRow
                user={request}
                meta={commonLine(request.mutualCount, request.commonCount)}
                onopen={() => openProfile(request)}
              >
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
          {/if}

          {#if page.outgoing.length > 0}
            <h4 class="eyebrow">Отправленные</h4>
            {#each page.outgoing as request (request.id)}
              <FriendRow
                user={request}
                meta={sentAt(request.createdAt)}
                onopen={() => openProfile(request)}
              >
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
          {/if}
        {/if}
      {:else if tab === 'feed'}
        {#if $feedEvents.length === 0}
          {#if $feedLoading}
            <p class="muted">Загрузка…</p>
          {:else}
            <EmptyState
              title="Пока тихо"
              description="Здесь появятся события друзей: пройденные игры, новинки и попавшее в любимые."
            />
          {/if}
        {:else}
          {#each feedGroups as group (group.key)}
            <h4 class="eyebrow">{group.label}</h4>
            {#each group.events as event (event.id)}
              <FeedRow {event} onreact={(emoji) => reactToEvent(event.id, emoji)} />
            {/each}
          {/each}
          {#if $feedCursor > 0}
            <div class="more">
              <Button disabled={$feedLoading} onclick={() => moreFeed()}>Показать ещё</Button>
            </div>
          {/if}
        {/if}
      {:else if blocked.length === 0}
        <EmptyState
          title="Никого не заблокировано"
          description="Заблокированные не видят ваш профиль и не могут отправить заявку."
        />
      {:else}
        {#each blocked as user (user.id)}
          <FriendRow {user}>
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
      {/if}
    </div>
  {/if}
</div>

{#if !isGuest && !$needsSocialConsent}
  <AddFriendModal bind:open={addOpen} onsent={reload} />
{/if}
<SocialConsentScreen bind:open={consentOpen} />

<style>
  .page {
    max-width: 96rem;
  }

  .code {
    margin-bottom: var(--space-5);
  }

  .list {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-4);
  }

  .list :global(.row + .row) {
    border-top: 1px solid var(--border);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
    padding: var(--space-5) 0.8rem;
  }

  .more {
    display: flex;
    justify-content: center;
    padding: var(--space-5) 0;
  }

  .eyebrow {
    margin: var(--space-5) 0 var(--space-2);
    padding: 0 0.8rem;
    font-size: 1.2rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }
</style>
