<script lang="ts">
  import Notice from '../../lib/components/Notice.svelte';
  import { warningText } from '../../lib/diskusage/usage';
  import { msg } from '../../lib/i18n';
  import type { Warning } from '../../lib/services/diskusage';

  // The parts the scan could not read, one line each in the words of its code; nothing while there are none.
  let { warnings }: { warnings: Warning[] } = $props();
</script>

{#if warnings.length > 0}
  <div class="warnings">
    <Notice tone="danger" title={msg('diskusage.warningsTitle')} level={2}>
      <ul>
        {#each warnings as warning, index (index)}
          <li>{warningText(warning)}</li>
        {/each}
      </ul>
    </Notice>
  </div>
{/if}

<style>
  .warnings {
    margin-top: var(--space-4);
  }

  .warnings ul {
    padding-left: var(--space-5);
  }
</style>
