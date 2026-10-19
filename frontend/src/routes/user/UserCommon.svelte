<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { CommonGame, CommonGames } from '../../lib/services/social';
  import { commonGamesTitle } from '../../lib/social/view';
  import { msg } from '../../lib/i18n';
  import UserCommonModal from './UserCommonModal.svelte';
  import UserCommonRow from './UserCommonRow.svelte';

  let {
    common,
    name,
    username,
    oninvite,
  }: {
    common: CommonGames;
    name: string;
    username: string;
    oninvite?: (game: CommonGame) => void;
  } = $props();

  const rows = $derived(common.games.slice(0, 6));
  let all = $state(false);
</script>

<Card title={msg('social.userCommonTitle')}>
  <p class="count">{commonGamesTitle(common.count)}</p>
  <div class="list">
    {#each rows as game (game.igdbId)}
      <UserCommonRow {game} {name} {oninvite} />
    {/each}
  </div>
  {#if common.count > rows.length}
    <div class="more">
      <Button onclick={() => (all = true)}>{msg('social.userCommonAll', { count: common.count })}</Button>
    </div>
  {/if}
</Card>

<UserCommonModal bind:open={all} {username} {name} {oninvite} />

<style>
  .count {
    font-size: var(--font-sm);
    color: var(--text-2);
    margin-bottom: var(--space-3);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list :global(.item + .item) {
    border-top: 1px solid var(--border);
  }

  .more {
    margin-top: var(--space-3);
  }
</style>
