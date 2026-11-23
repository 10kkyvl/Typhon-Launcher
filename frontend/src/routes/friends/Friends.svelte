<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { onMount, untrack } from 'svelte';
  import { LogIn, ShieldCheck, UserPlus, Users } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import SocialConsentScreen from '../../lib/components/SocialConsentScreen.svelte';
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
    type RequestView,
    type UserCard,
  } from '../../lib/services/social';
  import { blockPrompt, unfriendPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { presenceDot } from '../../lib/social/presence';
  import { commonLine, sentAt } from '../../lib/social/view';
  import { navigate } from '../../lib/stores/router';
  import { friendsPage, incomingCount, needsSocialConsent } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import AddFriendModal from './AddFriendModal.svelte';
  import FriendCodeCard from './FriendCodeCard.svelte';
  import FriendsList from './FriendsList.svelte';
  import RequestCards from './RequestCards.svelte';
  import { playerFrame } from '../../lib/profile/appearanceCard';
  import { frameReserve } from '../../lib/profile/frameLook';

  let { tab: initialTab }: { tab?: string } = $props();

  let tab = $state('all');
  let addOpen = $state(false);
  let codeOpen = $state(false);
  const uid = $props.id();
  const codeId = `${uid}-code`;
  let consentOpen = $state(false);
  let blocked = $state<UserCard[]>([]);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let busy = $state('');
  let myCode = $state('');
  let search = $state('');
  let sortBy = $state<'name' | 'status'>('status');

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

  const groupFor = $derived.by(() => {
    if (tab === 'online') return onlineFriends;
    if (tab === 'away') return awayFriends;
    if (tab === 'offline') return offlineFriends;
    return page.friends;
  });

  const reserveOf = (users: readonly UserCard[]) => frameReserve(users.map((user) => playerFrame(user.card)));
  const listReserve = $derived(reserveOf(groupFor));
  const incomingReserve = $derived(reserveOf(page.incoming));
  const outgoingReserve = $derived(reserveOf(page.outgoing));
  const blockedReserve = $derived(reserveOf(blocked));

  function receivedAt(iso: string | null): string {
    const when = relativeDate(iso);
    return when === '—' ? when : msg('friends.receivedAt', { when: when.toLowerCase() });
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
    stats.push(`${msg('friends.pendingReply')} · ${sentAt(request.createdAt)}`);
    return stats;
  }

  const tabs = $derived([
    { id: 'all', label: msg('social.friendsTabAll'), count: page.friends.length },
    { id: 'online', label: msg('social.presenceOnline'), count: onlineFriends.length },
    { id: 'away', label: msg('social.friendsTabAway'), count: awayFriends.length },
    { id: 'offline', label: msg('social.presenceOffline'), count: offlineFriends.length },
    { id: 'requests', label: msg('social.friendsTabRequests'), count: $incomingCount + page.outgoing.length },
    { id: 'blocked', label: msg('social.friendsTabBlocked'), count: blocked.length },
  ]);

  function report(err: unknown, fallback: string) {
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    toast(accountErrorText(err, fallback), 'danger');
  }

  function reload() {
    refresh().catch((err) => report(err, msg('social.friendsRefreshFailed')));
  }

  async function loadBlocks() {
    try {
      blocked = await fetchBlocks();
    } catch (err) {
      report(err, msg('social.friendsBlockedLoadFailed'));
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
    const name = user.displayName || user.username;
    if (item === 'unfriend') {
      pending = {
        prompt: unfriendPrompt(name),
        run: () =>
          run(
            `unfriend:${user.id}`,
            () => unfriend(user.id),
            msg('social.friendsUnfriendFailed'),
            msg('social.friendsUnfriended'),
          ),
      };
      return;
    }
    pending = {
      prompt: blockPrompt(name, true),
      run: () => run(`block:${user.id}`, () => block(user.id), msg('social.blockFailed'), msg('social.userBlocked')),
    };
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => report(err, msg('social.signInFailed')));
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

{#snippet incomingCards()}
  <RequestCards users={page.incoming} {busy} reserve={incomingReserve} stats={incomingStats} onopen={openProfile}>
    {#snippet actions(request)}
      <Button
        variant="primary"
        size="sm"
        disabled={!!busy}
        onclick={() =>
          run(`accept:${request.id}`, () => accept(request.id), msg('social.acceptRequestFailed'), msg('social.friendsNowFriends'))}
      >
        {msg('social.relationIncoming')}
      </Button>
      <Button
        size="sm"
        disabled={!!busy}
        onclick={() => run(`decline:${request.id}`, () => decline(request.id), msg('social.declineRequestFailed'))}
      >
        {msg('social.declineButton')}
      </Button>
    {/snippet}
  </RequestCards>
{/snippet}

<Page>
  <PageHeader title={msg('social.friendsTitle')} subtitle={msg('social.friendsSubtitle')}>
    {#snippet actions()}
      {#if !isGuest && !$needsSocialConsent}
        <Button onclick={() => (codeOpen = !codeOpen)} aria-expanded={codeOpen} aria-controls={codeOpen ? codeId : undefined}>
          {msg('social.friendsMyCodeButton')}
        </Button>
        <Button variant="primary" onclick={() => (addOpen = true)}>
          <UserPlus size="1.5rem" strokeWidth={1.8} />
          {msg('social.friendsAddFriend')}
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if isGuest}
    <EmptyState level={2}
      title={msg('social.friendsGuestTitle')}
      description={msg('social.friendsGuestDesc')}
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => signIn('login')}>
          <LogIn size="1.5rem" strokeWidth={1.8} />
          {msg('social.signInButton')}
        </Button>
        <Button onclick={() => signIn('register')}>{msg('social.createAccountButton')}</Button>
      {/snippet}
    </EmptyState>
  {:else if $needsSocialConsent}
    <EmptyState level={2}
      title={msg('social.friendsConsentTitle')}
      description={msg('social.friendsConsentDesc')}
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (consentOpen = true)}>{msg('social.friendsEnableSync')}</Button>
      {/snippet}
    </EmptyState>
  {:else}
    {#if codeOpen}
      <div class="code" id={codeId}>
        <FriendCodeCard bind:code={myCode} />
      </div>
    {/if}

    <SegmentedControl options={tabs} label={msg('social.friendsTabLabel')} variant="pill" bind:value={tab} />

    {#if presenceTabs.has(tab)}
      {#if page.incoming.length > 0}
        <section class="incoming" aria-label={msg('social.friendsIncomingHeading')}>
          <h4 class="eyebrow">
            <span class="eyebrow-dot"></span>
            {msg('social.friendsIncomingHeading')}
            <span class="eyebrow-count">{page.incoming.length}</span>
          </h4>
          {@render incomingCards()}
        </section>
      {/if}

      <FriendsList
        friends={page.friends}
        group={groupFor}
        {tab}
        reserve={listReserve}
        bind:search
        bind:sortBy
        onadd={() => (addOpen = true)}
        onmenu={onMenu}
        onopen={openProfile}
      />
    {:else if tab === 'requests'}
      {#if page.incoming.length === 0 && page.outgoing.length === 0}
        <EmptyState level={2} title={msg('social.friendsEmptyRequestsTitle')} description={msg('social.friendsEmptyRequestsDesc')} />
      {:else}
        {#if page.incoming.length > 0}
          <h4 class="eyebrow">
            <span class="eyebrow-dot"></span>
            {msg('social.friendsIncomingHeading')}
            <span class="eyebrow-count">{page.incoming.length}</span>
          </h4>
          {@render incomingCards()}
        {/if}

        {#if page.outgoing.length > 0}
          <h4 class="eyebrow">{msg('social.friendsSentHeading')}</h4>
          <RequestCards users={page.outgoing} {busy} reserve={outgoingReserve} stats={outgoingStats} onopen={openProfile}>
            {#snippet actions(request)}
              <Button
                variant="ghost"
                size="sm"
                disabled={!!busy}
                onclick={() => run(`cancel:${request.id}`, () => decline(request.id), msg('social.cancelRequestFailed'))}
              >
                {msg('social.cancelRequestButton')}
              </Button>
            {/snippet}
          </RequestCards>
        {/if}
      {/if}

      <div class="safety">
        <Notice announce={false} title={msg('social.friendsSafetyTitle')} level={4}>
          {#snippet icon()}
            <ShieldCheck size="1.6rem" strokeWidth={1.8} />
          {/snippet}
          {msg('social.friendsSafetyDesc')}
        </Notice>
      </div>
    {:else if blocked.length === 0}
      <EmptyState level={2}
        title={msg('social.friendsEmptyBlockedTitle')}
        description={msg('social.friendsEmptyBlockedDesc')}
      />
    {:else}
      <RequestCards users={blocked} {busy} reserve={blockedReserve}>
        {#snippet actions(user)}
          <Button
            size="sm"
            disabled={!!busy}
            onclick={() =>
              run(`unblock:${user.id}`, () => unblock(user.id), msg('social.unblockFailed'), msg('social.userUnblocked'))}
          >
            {msg('social.unblockButton')}
          </Button>
        {/snippet}
      </RequestCards>
    {/if}
  {/if}

  {#if !isGuest && !$needsSocialConsent}
    <AddFriendModal bind:open={addOpen} onsent={reload} />
  {/if}

  <SocialConsentScreen bind:open={consentOpen} />

  {#if pending}
    <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} errorText={(err) => accountErrorText(err)} />
  {/if}
</Page>

<style>
  .code {
    margin-bottom: var(--space-5);
  }

  .incoming {
    margin-top: var(--space-5);
    padding: 0 var(--space-4) var(--space-4);
    border: 1px solid var(--accent-ring);
    border-radius: var(--radius-lg);
    background: var(--accent-subtle);
  }

  .incoming .eyebrow {
    margin-top: var(--space-4);
    color: var(--accent-text);
  }

  .eyebrow-dot {
    width: 0.8rem;
    height: 0.8rem;
    border-radius: 50%;
    background: var(--accent-bright);
  }

  .eyebrow-count {
    font-variant-numeric: tabular-nums;
    color: var(--text-2);
  }

  .eyebrow {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    margin: var(--space-6) 0 var(--space-3);
    padding: 0 0.2rem;
    font-size: 1.2rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  .safety {
    margin-top: var(--space-6);
  }
</style>
