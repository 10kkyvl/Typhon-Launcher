<script lang="ts">
  import { tick } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import { msg } from '../../lib/i18n';
  import { focusableIn, focusQuietly } from '../../lib/layers/focus';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { userCommon, type CommonGame } from '../../lib/services/social';
  import UserCommonRow from './UserCommonRow.svelte';

  let {
    open = $bindable(false),
    username,
    name,
    oninvite,
  }: {
    open?: boolean;
    username: string;
    name: string;
    oninvite?: (game: CommonGame) => void;
  } = $props();

  let games = $state<CommonGame[]>([]);
  let next = $state('');
  let loading = $state(false);
  let loaded = $state(false);
  let failure = $state('');
  let serial = 0;

  // A retry keeps the failure on screen, its Retry busy, until the answer replaces it.
  async function load(reset: boolean, retry = false): Promise<void> {
    const run = ++serial;
    const cursor = reset ? '' : next;
    loading = true;
    if (!retry) failure = '';
    try {
      const page = await userCommon(username, cursor);
      if (run !== serial) return;
      games = reset ? page.games : [...games, ...page.games];
      next = page.next;
      failure = '';
      loaded = true;
    } catch (err) {
      if (run !== serial) return;
      failure = accountErrorText(err, msg('social.userCommonFailed'));
    } finally {
      if (run === serial) loading = false;
    }
  }

  let retrying = $state(false);
  let content = $state<HTMLElement>();
  let list = $state<HTMLElement>();
  let moreSlot = $state<HTMLElement>();
  let freshFrom = 0;

  // The notice's retry asks again for the page that failed. On success the focus goes to the first
  // game it brought; with none, it stays in the dialog, on its first control.
  async function retry(): Promise<boolean> {
    freshFrom = games.length;
    await load(!loaded, true);
    return !failure;
  }

  function afterRetry(): HTMLElement | undefined {
    const row = list?.children[freshFrom];
    if (row instanceof HTMLElement) return focusableIn(row)[0];
    const dialog = content?.closest<HTMLElement>('[role="dialog"]');
    return dialog ? focusableIn(dialog)[0] : undefined;
  }

  // "Show more" goes when its page fails and the failure takes its place: focus that was on it moves
  // to that notice's Retry instead of falling out of the list.
  async function more() {
    const pressed = Boolean(moreSlot?.contains(document.activeElement));
    await load(false);
    if (!pressed || !failure) return;
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected && content?.contains(active)) return;
    const again = moreSlot ? focusableIn(moreSlot)[0] : undefined;
    if (again) focusQuietly(again);
  }

  $effect(() => {
    if (!open) return;
    const target = username;
    if (!target) return;
    games = [];
    next = '';
    loaded = false;
    void load(true);
    return () => {
      serial++;
    };
  });
</script>

<!-- dismissible while loading: loading more of the list is a read; closing drops it. -->
<Modal bind:open title={msg('social.userCommonAllTitle')} width="64rem">
  <div bind:this={content}>
    {#if !loaded && !failure && !retrying}
      <Spinner label={msg('social.loadingEllipsis')} />
    {:else if loaded && games.length === 0 && !failure && !retrying}
      <EmptyState size="inline" title={msg('social.userCommonEmpty')} />
    {:else}
      {#if games.length > 0}
        <div class="list" bind:this={list}>
          {#each games as game (game.igdbId)}
            <UserCommonRow {game} {name} {oninvite} />
          {/each}
        </div>
      {/if}
      <!-- One place for what follows the list: "Show more", or the failure of the page it asked for. -->
      <div class="more" bind:this={moreSlot}>
        {#if failure || retrying}
          <Notice tone="danger" compact bind:retrying retry={{ label: msg('common.retry'), run: retry, focus: afterRetry }}>
            {failure}
          </Notice>
        {:else if next}
          <Button busy={loading} onclick={more}>{msg('social.userCommonMore')}</Button>
        {/if}
      </div>
    {/if}
  </div>
</Modal>

<style>
  .list {
    display: flex;
    flex-direction: column;
  }

  .list :global(.item + .item) {
    border-top: 1px solid var(--border);
  }

  .list + .more:not(:empty) {
    margin-top: var(--space-3);
  }
</style>
