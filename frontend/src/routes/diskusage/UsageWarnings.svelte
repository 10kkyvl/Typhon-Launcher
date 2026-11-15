<script lang="ts">
  import Notice from '../../lib/components/Notice.svelte';
  import { msg } from '../../lib/i18n';
  import type { Warning } from '../../lib/services/diskusage';

  // The areas the scan could not read, one line each; nothing while there are none.
  let { warnings }: { warnings: Warning[] } = $props();
</script>

{#if warnings.length > 0}
  <div class="warnings">
    <Notice tone="danger" title={msg('diskusage.warningsTitle')} level={2}>
      <ul>
        {#each warnings as warning, index (index)}
          <li>{msg('diskusage.warningLine', { area: warning.area, message: warning.message })}</li>
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
