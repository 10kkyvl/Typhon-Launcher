<script lang="ts">
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import { ticketLogsPrompt } from '../../lib/support/logsPrompt';

  let settle = $state<((confirmed: boolean) => void) | null>(null);

  export function ask(): Promise<boolean> {
    settle?.(false);
    return new Promise<boolean>((resolve) => {
      settle = resolve;
    });
  }

  function finish(confirmed: boolean) {
    const resolve = settle;
    settle = null;
    resolve?.(confirmed);
  }
</script>

{#if settle}
  <ConfirmModal prompt={ticketLogsPrompt()} onconfirm={() => finish(true)} onclose={() => finish(false)} />
{/if}
