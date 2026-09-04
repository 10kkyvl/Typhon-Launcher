<script lang="ts">
  import { LogIn, Users } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import FeedRow from '../../lib/components/FeedRow.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SocialConsentScreen from '../../lib/components/SocialConsentScreen.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { feedDayGroups } from '../../lib/social/feed';
  import { feedCursor, feedEvents, feedLoading, loadFeed, moreFeed, reactToEvent } from '../../lib/stores/feed';
  import { needsSocialConsent } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';

  let consentOpen = $state(false);

  const isGuest = $derived($authState === 'guest');
  const feedGroups = $derived(feedDayGroups($feedEvents));

  function report(err: unknown, fallback: string) {
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    toast(accountErrorText(err, fallback), 'danger');
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => report(err, 'Не удалось открыть вход'));
  }

  $effect(() => {
    if (!isGuest && !$needsSocialConsent) void loadFeed(true);
  });
</script>

<PageHeader title="Активность" subtitle="Лента событий ваших друзей" />

<div class="page">
  {#if isGuest}
    <EmptyState
      title="Активность доступна с аккаунтом"
      description="Войдите, чтобы видеть события друзей: пройденные игры, новинки и попавшее в любимые."
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
      description="Лента активности работает поверх синхронизации: без неё серверу нечего показывать в ленте друзей."
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (consentOpen = true)}>Включить синхронизацию</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="list">
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
    </div>
  {/if}
</div>

<SocialConsentScreen bind:open={consentOpen} />

<style>
  .page {
    max-width: 96rem;
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
