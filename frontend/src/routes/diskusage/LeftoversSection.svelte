<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import { gameTitle, leftoverKindKey, leftoverOfGame, reasonLabel, reasonTone } from '../../lib/diskusage/usage';
  import { msg } from '../../lib/i18n';
  import type { Leftover } from '../../lib/services/diskusage';
  import { libraryGamesById } from '../../lib/stores/library';
  import { usageSectionIds } from './sections';
  import UsageRow from './UsageRow.svelte';
  import UsageSection from './UsageSection.svelte';
  import { size, total } from './usageSizes';

  // Leftovers of installs, downloads, moves, updates and removals: what each is, why it cannot go, and
  // its Delete only where Go lets it go (the screen never offers a delete Go did not).
  let {
    leftovers,
    totalBytes,
    ondelete,
  }: {
    leftovers: Leftover[];
    totalBytes: number;
    /** Asks before deleting the leftover. */
    ondelete: (leftover: Leftover) => void;
  } = $props();
</script>

<UsageSection
  id={usageSectionIds.leftovers}
  title={msg('diskusage.leftoversTitle')}
  count={leftovers.length}
  hint={msg('diskusage.leftoversHint')}
  total={total(totalBytes, leftovers.some((l) => l.sizeUnknown))}
  empty={leftovers.length === 0}
  emptyText={msg('diskusage.leftoversEmpty')}
>
  {#each leftovers as leftover (leftover.path)}
    <UsageRow
      key="leftovers:{leftover.path}"
      title={msg(leftoverKindKey(leftover.kind))}
      game={leftoverOfGame(leftover)
        ? { id: leftover.gameId && $libraryGamesById.has(leftover.gameId) ? leftover.gameId : null, name: gameTitle(leftover) }
        : undefined}
      path={leftover.path}
      reason={leftover.deletable ? null : reasonLabel(leftover.reasonCode)}
      reasonTone={reasonTone(leftover.reasonCode)}
      size={size(leftover)}
    >
      {#snippet actions()}
        {#if leftover.deletable}
          <Button size="sm" variant="danger" data-row-control="delete" onclick={() => ondelete(leftover)}>
            {msg('diskusage.leftoverDelete')}
          </Button>
        {/if}
      {/snippet}
    </UsageRow>
  {/each}
</UsageSection>
