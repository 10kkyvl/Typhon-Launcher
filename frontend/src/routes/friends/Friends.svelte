<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { untrack } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { LogIn, ShieldCheck, UserPlus, Users } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import SocialConsentScreen from '../../lib/components/SocialConsentScreen.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import {
    accept,
    block,
    blocks as fetchBlocks,
    decline,
    unblock,
    unfriend,
    type RequestView,
    type UserCard,
  } from '../../lib/services/social';
  import { blockPrompt, unfriendPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { presenceDot } from '../../lib/social/presence';
  import { commonLine, sentAt } from '../../lib/social/view';
  import { navigate } from '../../lib/stores/router';
  import {
    friendsLoaded,
    friendsLoadError,
    friendsPage,
    incomingCount,
    needsSocialConsent,
    reloadFriends,
    showFriendsState,
  } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import AddFriendModal from './AddFriendModal.svelte';
  import FriendCodeCard from './FriendCodeCard.svelte';
  import FriendsList from './FriendsList.svelte';
  import RequestCards from './RequestCards.svelte';
  import { catchLeaving, placeOf, returnTo, type RowPlace } from './rowFocus';
  import { playerFrame } from '../../lib/profile/appearanceCard';
  import { frameReserve } from '../../lib/profile/frameLook';

  let { tab: initialTab }: { tab?: string } = $props();

  let tab = $state('all');
  let addOpen = $state(false);
  let codeOpen = $state(false);
  const uid = $props.id();
  const codeId = `${uid}-code`;
  const incomingHeading = `${uid}-incoming`;
  const outgoingHeading = `${uid}-outgoing`;
  let consentOpen = $state(false);
  let blocked = $state<UserCard[]>([]);
  let blockedLoaded = $state(false);
  let blockedError = $state<unknown>(null);
  let retrying = $state(false);
  let blockedRetrying = $state(false);
  let content = $state<HTMLElement>();
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void>; errorText: (err: unknown) => string } | null>(null);
  // The row a confirmation was asked from: focus goes back by the row rule when the dialog closes.
  let askedFrom: RowPlace | null = null;
  // The actions Go has not answered yet (with the read after them), by the user of their card or row:
  // that card's pressed button is busy and a second press on it sends nothing; other cards act at once.
  const pressed = new SvelteMap<string, string>();
  const busyAs = (action: string, id: string) => pressed.get(id) === action;
  const settling = (id: string) => pressed.has(id);
  let myCode = $state('');
  let search = $state('');
  let sortBy = $state<'name' | 'status'>('status');

  const isGuest = $derived($authState === 'guest');
  const canRead = $derived(!isGuest && !$needsSocialConsent);
  const page = $derived($friendsPage);
  const friendsFailed = $derived(!$friendsLoaded && $friendsLoadError != null);
  const blockedFailed = $derived(!blockedLoaded && blockedError != null);

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

  // A count before its list is read would say "none".
  const counted = (count: number) => ($friendsLoaded ? count : undefined);
  const tabs = $derived([
    { id: 'all', label: msg('social.friendsTabAll'), count: counted(page.friends.length) },
    { id: 'online', label: msg('social.presenceOnline'), count: counted(onlineFriends.length) },
    { id: 'away', label: msg('social.friendsTabAway'), count: counted(awayFriends.length) },
    { id: 'offline', label: msg('social.presenceOffline'), count: counted(offlineFriends.length) },
    { id: 'requests', label: msg('social.friendsTabRequests'), count: counted($incomingCount + page.outgoing.length) },
    { id: 'blocked', label: msg('social.friendsTabBlocked'), count: blockedLoaded ? blocked.length : undefined },
  ]);
  const tabLabel = $derived(tabs.find((option) => option.id === tab)?.label ?? '');

  function report(err: unknown, fallback: string) {
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    toast(accountErrorText(err, fallback), 'danger');
  }

  // The last read of the blocked list asked: the answer of an older one that comes later is dropped.
  let blocksAsked = 0;

  /** Reads the blocked list; answers whether it is there. A failure before any list is kept as state. */
  async function loadBlocks(): Promise<boolean> {
    const mine = ++blocksAsked;
    try {
      const list = await fetchBlocks();
      if (mine !== blocksAsked) return blockedLoaded;
      blocked = list;
      blockedLoaded = true;
      blockedError = null;
      return true;
    } catch (err) {
      if (mine !== blocksAsked) return blockedLoaded;
      if (blockedLoaded) report(err, msg('social.friendsBlockedLoadFailed'));
      else blockedError = err;
      return false;
    }
  }

  /**
   * Runs a card's or a row's action, once per card until it is answered and the lists are read again.
   * Its failure is said once in a toast, or thrown to the confirmation that asked (`confirmed`), which
   * says it and offers Retry; a confirmation's action is always sent (the dialog holds its own second
   * press). Once the action went through its success is said and the lists are read again: a failed
   * read is said as a read failure, not as the action's.
   */
  async function run(action: string, id: string, call: () => Promise<void>, fallback: string, done?: string, confirmed = false) {
    if (pressed.has(id) && !confirmed) return;
    pressed.set(id, action);
    try {
      try {
        await call();
      } catch (err) {
        if (confirmed) throw err;
        report(err, fallback);
        return;
      }
      if (done) toast(done, 'success');
      await reloadFriends();
      if (action === 'block' || action === 'unblock') await loadBlocks();
    } finally {
      if (pressed.get(id) === action) pressed.delete(id);
    }
  }

  function onMenu(user: UserCard, item: string) {
    if (item === 'profile') {
      navigate('user', { username: user.username });
      return;
    }
    const name = user.displayName || user.username;
    askedFrom = placeOf(document.activeElement);
    if (item === 'unfriend') {
      pending = {
        prompt: unfriendPrompt(name),
        run: () =>
          run(
            'unfriend',
            user.id,
            () => unfriend(user.id),
            msg('social.friendsUnfriendFailed'),
            msg('social.friendsUnfriended'),
            true,
          ),
        errorText: (err) => accountErrorText(err, msg('social.friendsUnfriendFailed')),
      };
      return;
    }
    pending = {
      prompt: blockPrompt(name, true),
      run: () => run('block', user.id, () => block(user.id), msg('social.blockFailed'), msg('social.userBlocked'), true),
      errorText: (err) => accountErrorText(err, msg('social.blockFailed')),
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

  // Before any change of the cards and rows shown (an answered request, a removed friend, a list read
  // again), the row rule reads where focus is and hands it on if its card or row went.
  $effect.pre(() => {
    void page;
    void blocked;
    void blockedLoaded;
    void $friendsLoaded;
    untrack(catchLeaving);
  });

  // The screen says a failed first read in its notice: the store's own reads do not toast it too.
  $effect(() => showFriendsState());

  // Read on arrival and again whenever the session changes under the screen (a guest signs in, the
  // connection comes back): the store forgets the lists of a session that ended.
  $effect(() => {
    void $authState;
    if (!canRead) return;
    untrack(() => {
      void reloadFriends();
      void loadBlocks();
    });
  });
</script>

{#snippet incomingCards()}
  <RequestCards users={page.incoming} busy={settling} reserve={incomingReserve} stats={incomingStats} onopen={openProfile}>
    {#snippet actions(request)}
      <Button
        variant="primary"
        size="sm"
        data-row-control="accept"
        busy={busyAs('accept', request.id)}
        onclick={() =>
          run('accept', request.id, () => accept(request.id), msg('social.acceptRequestFailed'), msg('social.friendsNowFriends'))}
      >
        {msg('social.relationIncoming')}
      </Button>
      <Button
        size="sm"
        data-row-control="decline"
        busy={busyAs('decline', request.id)}
        onclick={() => run('decline', request.id, () => decline(request.id), msg('social.declineRequestFailed'))}
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

    {#if tab !== 'blocked'}
      {#if friendsFailed || retrying}
        <!-- On success focus that was on Retry goes to the first control of the lists. -->
        <div class="load-failed">
          <Notice tone="danger" bind:retrying retry={{ label: msg('common.retry'), run: reloadFriends, within: () => content }}>
            {accountErrorText($friendsLoadError, msg('state.socialLoadFriendsFailed'))}
          </Notice>
        </div>
      {/if}

      {#if !$friendsLoaded}
        {#if !friendsFailed}
          <div class="loading"><Spinner label={msg('social.loadingEllipsis')} /></div>
        {/if}
      {:else}
        <div class="content" bind:this={content}>
          {#if presenceTabs.has(tab)}
            {#if page.incoming.length > 0}
              <section class="incoming" data-friends-section="incoming" aria-labelledby={incomingHeading}>
                <SectionHeader id={incomingHeading} title={msg('social.friendsIncomingHeading')} count={page.incoming.length} />
                {@render incomingCards()}
              </section>
            {/if}

            <div data-friends-section="friends">
              <FriendsList
                friends={page.friends}
                group={groupFor}
                {tab}
                heading={tabLabel}
                reserve={listReserve}
                bind:search
                bind:sortBy
                onadd={() => (addOpen = true)}
                onmenu={onMenu}
                onopen={openProfile}
              />
            </div>
          {:else if page.incoming.length === 0 && page.outgoing.length === 0}
            <EmptyState level={2} title={msg('social.friendsEmptyRequestsTitle')} description={msg('social.friendsEmptyRequestsDesc')} />
          {:else}
            {#if page.incoming.length > 0}
              <section class="group" data-friends-section="incoming" aria-labelledby={incomingHeading}>
                <SectionHeader id={incomingHeading} title={msg('social.friendsIncomingHeading')} count={page.incoming.length} />
                {@render incomingCards()}
              </section>
            {/if}

            {#if page.outgoing.length > 0}
              <section class="group" data-friends-section="outgoing" aria-labelledby={outgoingHeading}>
                <SectionHeader id={outgoingHeading} title={msg('social.friendsSentHeading')} count={page.outgoing.length} />
                <RequestCards users={page.outgoing} busy={settling} reserve={outgoingReserve} stats={outgoingStats} onopen={openProfile}>
                  {#snippet actions(request)}
                    <Button
                      variant="ghost"
                      size="sm"
                      data-row-control="cancel"
                      busy={busyAs('cancel', request.id)}
                      onclick={() => run('cancel', request.id, () => decline(request.id), msg('social.cancelRequestFailed'))}
                    >
                      {msg('social.cancelRequestButton')}
                    </Button>
                  {/snippet}
                </RequestCards>
              </section>
            {/if}
          {/if}
        </div>
      {/if}

      {#if tab === 'requests'}
        <div class="safety">
          <Notice announce={false} title={msg('social.friendsSafetyTitle')} level={2}>
            {#snippet icon()}
              <ShieldCheck size="1.6rem" strokeWidth={1.8} />
            {/snippet}
            {msg('social.friendsSafetyDesc')}
          </Notice>
        </div>
      {/if}
    {:else}
      {#if blockedFailed || blockedRetrying}
        <!-- On success focus that was on Retry goes to the first control of the list. -->
        <div class="load-failed">
          <Notice tone="danger" bind:retrying={blockedRetrying} retry={{ label: msg('common.retry'), run: loadBlocks, within: () => content }}>
            {accountErrorText(blockedError, msg('social.friendsBlockedLoadFailed'))}
          </Notice>
        </div>
      {/if}

      {#if !blockedLoaded}
        {#if !blockedFailed}
          <div class="loading"><Spinner label={msg('social.loadingEllipsis')} /></div>
        {/if}
      {:else}
        <div class="content" data-friends-section="blocked" bind:this={content}>
          {#if blocked.length === 0}
            <EmptyState level={2}
              title={msg('social.friendsEmptyBlockedTitle')}
              description={msg('social.friendsEmptyBlockedDesc')}
            />
          {:else}
            <RequestCards users={blocked} busy={settling} reserve={blockedReserve}>
              {#snippet actions(user)}
                <Button
                  size="sm"
                  data-row-control="unblock"
                  busy={busyAs('unblock', user.id)}
                  onclick={() =>
                    run('unblock', user.id, () => unblock(user.id), msg('social.unblockFailed'), msg('social.userUnblocked'))}
                >
                  {msg('social.unblockButton')}
                </Button>
              {/snippet}
            </RequestCards>
          {/if}
        </div>
      {/if}
    {/if}
  {/if}

  {#if !isGuest && !$needsSocialConsent}
    <AddFriendModal bind:open={addOpen} onsent={() => void reloadFriends()} />
  {/if}

  <SocialConsentScreen bind:open={consentOpen} />

  {#if pending}
    <ConfirmModal
      prompt={pending.prompt}
      onconfirm={pending.run}
      onclose={() => (pending = null)}
      errorText={pending.errorText}
      returnFocus={() => returnTo(askedFrom)}
    />
  {/if}
</Page>

<style>
  .code {
    margin-bottom: var(--space-5);
  }

  .load-failed {
    margin-top: var(--space-5);
  }

  .loading {
    display: flex;
    justify-content: center;
    padding: var(--space-10) 0;
  }

  /* The requests waiting for an answer, over the list: an accent plate the kit's Card has no tone for. */
  .incoming {
    margin-top: var(--space-5);
    padding: var(--space-4);
    border: 1px solid var(--accent-ring);
    border-radius: var(--radius-lg);
    background: var(--accent-subtle);
  }

  .group {
    margin-top: var(--space-6);
  }

  .safety {
    margin-top: var(--space-6);
  }
</style>
