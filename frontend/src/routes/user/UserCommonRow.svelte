<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import type { CommonGame } from '../../lib/services/social';
  import { bothPlayedRecently, commonGameLabel, commonPlaytimeLine } from '../../lib/social/view';
  import { msg } from '../../lib/i18n';
  import GameRow from './GameRow.svelte';

  let {
    game,
    name,
    oninvite,
    offline = false,
  }: {
    game: CommonGame;
    name: string;
    oninvite?: (game: CommonGame) => void;
    /** The invitation opens the chat, which needs the server: held, saying why. */
    offline?: boolean;
  } = $props();
</script>

<div class="item">
  <GameRow
    {game}
    meta={commonGameLabel(game.viewerOwned, game.targetOwned, name)}
    detail={commonPlaytimeLine(game)}
    tag={bothPlayedRecently(game) ? msg('social.userCommonBothRecent') : ''}
  />
  {#if oninvite}
    <Button size="sm" disabled={offline} reason={offline ? msg('social.userActionsNeedConnection') : ''} onclick={() => oninvite(game)}>{msg('social.userCommonInvite')}</Button>
  {/if}
</div>

<style>
  .item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .item :global(.row) {
    flex: 1;
    min-width: 0;
  }
</style>
