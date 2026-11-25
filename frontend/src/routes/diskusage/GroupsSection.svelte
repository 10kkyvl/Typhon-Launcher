<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { msg } from '../../lib/i18n';
  import type { GroupUsage } from '../../lib/services/diskusage';
  import { libraryGamesById } from '../../lib/stores/library';
  import { usageSectionIds } from './sections';
  import UsageRow from './UsageRow.svelte';
  import UsageSection from './UsageSection.svelte';
  import { size, total } from './usageSizes';

  // Files kept per game, screenshots or save backups: one row per game with its count and Delete all.
  const WORDS = {
    screenshots: {
      title: 'diskusage.screenshotsTitle',
      hint: 'diskusage.screenshotsHint',
      empty: 'diskusage.screenshotsEmpty',
      count: 'diskusage.screenshotsCount',
    },
    snapshots: {
      title: 'diskusage.snapshotsTitle',
      hint: 'diskusage.snapshotsHint',
      empty: 'diskusage.snapshotsEmpty',
      count: 'diskusage.snapshotsCount',
    },
  } as const;

  let {
    kind,
    groups,
    totalBytes,
    ondelete,
  }: {
    kind: keyof typeof WORDS;
    groups: GroupUsage[];
    totalBytes: number;
    /** Asks before deleting all of the game's files of this kind. */
    ondelete: (group: GroupUsage) => void;
  } = $props();

  const words = $derived(WORDS[kind]);
</script>

<UsageSection
  id={usageSectionIds[kind]}
  title={msg(words.title)}
  count={groups.length}
  hint={msg(words.hint)}
  total={total(totalBytes, groups.some((s) => s.sizeUnknown))}
  empty={groups.length === 0}
  emptyText={msg(words.empty)}
>
  {#each groups as group (group.gameId)}
    <UsageRow
      key="{kind}:{group.gameId}"
      title={group.title}
      gameId={$libraryGamesById.has(group.gameId) ? group.gameId : null}
      path={msg(words.count, { count: group.count })}
      isPath={false}
      size={size(group)}
    >
      {#snippet badges()}
        {#if !group.inLibrary}
          <StatusBadge kind="warning" label={msg('diskusage.leftLibrary')} plain />
        {/if}
      {/snippet}
      {#snippet actions()}
        <Button size="sm" variant="danger" onclick={() => ondelete(group)}>
          {msg('diskusage.deleteAll')}
        </Button>
      {/snippet}
    </UsageRow>
  {/each}
</UsageSection>
