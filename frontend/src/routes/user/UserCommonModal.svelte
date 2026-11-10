<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import { msg } from '../../lib/i18n';
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

  async function load(reset: boolean): Promise<void> {
    const run = ++serial;
    const cursor = reset ? '' : next;
    loading = true;
    failure = '';
    try {
      const page = await userCommon(username, cursor);
      if (run !== serial) return;
      games = reset ? page.games : [...games, ...page.games];
      next = page.next;
      loaded = true;
    } catch (err) {
      if (run !== serial) return;
      failure = accountErrorText(err, msg('social.userCommonFailed'));
    } finally {
      if (run === serial) loading = false;
    }
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

<Modal bind:open title={msg('social.userCommonAllTitle')} width="64rem">
  {#if failure && games.length === 0}
    <p class="note error" role="alert">{failure}</p>
    <Button variant="primary" onclick={() => load(true)}>{msg('common.retry')}</Button>
  {:else if loaded && games.length === 0}
    <p class="note">{msg('social.userCommonEmpty')}</p>
  {:else}
    <div class="list">
      {#each games as game (game.igdbId)}
        <UserCommonRow {game} {name} {oninvite} />
      {/each}
    </div>
    {#if failure}
      <p class="note error" role="alert">{failure}</p>
    {/if}
    {#if next || failure}
      <div class="more">
        <Button disabled={loading} onclick={() => load(false)}>
          {failure ? msg('common.retry') : msg('social.userCommonMore')}
        </Button>
      </div>
    {/if}
  {/if}
</Modal>

<style>
  .list {
    display: flex;
    flex-direction: column;
  }

  .list :global(.item + .item) {
    border-top: 1px solid var(--border);
  }

  .note {
    font-size: var(--font-sm);
    color: var(--text-2);
    margin-bottom: var(--space-3);
  }

  .error {
    color: var(--danger);
  }

  .more {
    margin-top: var(--space-3);
  }
</style>
