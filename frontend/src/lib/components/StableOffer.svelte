<script lang="ts">
  import { Undo2 } from '@lucide/svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import { stableOfferPrompt } from '../confirm/prompts';
  import { stableOfferVersion } from '../services/selfupdateChannel';
  import { requestInstallStable, selfUpdateBusy, selfUpdateStatus, stableBusy } from '../stores/selfupdate';
  import { msg } from '../i18n';

  let confirmOpen = $state(false);

  const status = $derived($selfUpdateStatus);
  const version = $derived(stableOfferVersion(status));
</script>

{#if version}
  <div class="offer">
    <span class="offer-text">{msg('ui.stableOfferText', { current: status.currentVersion, version })}</span>
    <Button size="sm" disabled={$stableBusy || $selfUpdateBusy} onclick={() => (confirmOpen = true)}>
      <Undo2 size="1.5rem" strokeWidth={1.8} />
      {$stableBusy ? msg('ui.stableOfferBusy') : msg('ui.stableOfferButton', { version })}
    </Button>
  </div>
{/if}

{#if confirmOpen}
  <ConfirmModal prompt={stableOfferPrompt(version)} onconfirm={requestInstallStable} onclose={() => (confirmOpen = false)} />
{/if}

<style>
  .offer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
    border-radius: var(--radius-md);
    background: var(--surface-3);
  }

  .offer-text {
    font-size: var(--font-sm);
    font-weight: 500;
  }
</style>
