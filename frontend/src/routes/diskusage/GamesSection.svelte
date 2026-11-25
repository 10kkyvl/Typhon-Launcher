<script lang="ts">
  import { ChevronDown } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { ageOf, ageText, canManageGame, gameBlockedKey, isStale, type GameSort } from '../../lib/diskusage/usage';
  import { openMoveGame } from '../../lib/game/actions/move';
  import { msg } from '../../lib/i18n';
  import type { GameUsage } from '../../lib/services/diskusage';
  import { libraryGamesById } from '../../lib/stores/library';
  import { usageSectionIds } from './sections';
  import UsageRow from './UsageRow.svelte';
  import UsageSection from './UsageSection.svelte';
  import { size, total } from './usageSizes';

  // The installed games: their marks, when each was last played, the sort menu, Move and Remove.
  let {
    games,
    totalBytes,
    now,
    sort,
    sortLabels,
    onsort,
    onremove,
  }: {
    /** Already sorted by `sort`. */
    games: GameUsage[];
    totalBytes: number;
    now: number;
    sort: GameSort;
    sortLabels: Record<GameSort, string>;
    onsort: (sort: GameSort) => void;
    onremove: (game: GameUsage) => void;
  } = $props();
</script>

<UsageSection
  id={usageSectionIds.games}
  title={msg('diskusage.gamesTitle')}
  count={games.length}
  hint={msg('diskusage.gamesHint')}
  total={total(totalBytes, games.some((g) => g.sizeUnknown))}
>
  {#snippet tools()}
    <DropdownMenu
      items={(['size', 'played'] as const).map((id) => ({ id, label: sortLabels[id], checked: sort === id }))}
      onselect={(id) => onsort(id as GameSort)}
    >
      {#snippet trigger({ props })}
        <Chip {...props}>
          {msg('diskusage.sortLabel', { sort: sortLabels[sort] })}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </Chip>
      {/snippet}
    </DropdownMenu>
  {/snippet}
  {#each games as game (game.id)}
    {@const blocked = gameBlockedKey(game)}
    {@const manage = canManageGame(game)}
    <UsageRow
      key="games:{game.id}"
      title={game.title}
      gameId={$libraryGamesById.has(game.id) ? game.id : null}
      path={game.installDir}
      played={ageText(ageOf(game.lastPlayed, now))}
      size={size(game)}
    >
      {#snippet badges()}
        {#if game.external}
          <StatusBadge kind="neutral" label={msg('diskusage.external')} plain />
        {/if}
        {#if game.linked}
          <StatusBadge kind="neutral" label={msg('diskusage.linked')} hint={msg('diskusage.linkedHint')} plain />
        {/if}
        {#if game.missing}
          <StatusBadge kind="warning" label={msg('diskusage.missing')} plain />
        {/if}
        {#if game.running}
          <StatusBadge kind="accent" label={msg('diskusage.running')} plain />
        {/if}
        {#if isStale(game.lastPlayed, now)}
          <StatusBadge kind="warning" label={msg('diskusage.stale')} plain />
        {/if}
      {/snippet}
      {#snippet actions()}
        {#if !game.external}
          <Button size="sm" data-row-control="move" disabled={!manage} reason={blocked ? msg(blocked) : ''} onclick={() => openMoveGame(game.id)}>
            {msg('diskusage.move')}
          </Button>
          <Button size="sm" variant="danger" data-row-control="remove" disabled={!manage} reason={blocked ? msg(blocked) : ''} onclick={() => onremove(game)}>
            {msg('diskusage.remove')}
          </Button>
        {/if}
      {/snippet}
    </UsageRow>
  {/each}
</UsageSection>
