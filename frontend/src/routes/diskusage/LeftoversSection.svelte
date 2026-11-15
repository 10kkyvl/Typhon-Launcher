<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import { leftoverKindKey, reasonLabel } from '../../lib/diskusage/usage';
  import { msg } from '../../lib/i18n';
  import type { Leftover } from '../../lib/services/diskusage';
  import UsageRow from './UsageRow.svelte';
  import UsageSection from './UsageSection.svelte';
  import { size, total } from './usageSizes';

  // Leftovers of installs, downloads, moves and updates: what each is, why it cannot go, and its Delete.
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
  title={msg('diskusage.leftoversTitle')}
  hint={msg('diskusage.leftoversHint')}
  total={total(totalBytes, leftovers.some((l) => l.sizeUnknown))}
  empty={leftovers.length === 0}
  emptyText={msg('diskusage.leftoversEmpty')}
>
  {#each leftovers as leftover (leftover.path)}
    <UsageRow
      title={msg(leftoverKindKey(leftover.kind))}
      path={leftover.path}
      pathTitle={leftover.path}
      reason={leftover.deletable ? null : reasonLabel(leftover.reasonCode, leftover.reason)}
      size={size(leftover)}
    >
      {#snippet actions()}
        <Button size="sm" variant="danger" disabled={!leftover.deletable} onclick={() => ondelete(leftover)}>
          {msg('diskusage.leftoverDelete')}
        </Button>
      {/snippet}
    </UsageRow>
  {/each}
</UsageSection>
