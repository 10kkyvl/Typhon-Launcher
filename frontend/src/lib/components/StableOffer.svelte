<script lang="ts">
  import { Undo2 } from '@lucide/svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Notice from './Notice.svelte';
  import { stableOfferPrompt } from '../confirm/prompts';
  import { stableOfferVersion } from '../services/selfupdateChannel';
  import { requestInstallStable, selfUpdateBusy, selfUpdateStatus, stableBusy } from '../stores/selfupdate';
  import { msg } from '../i18n';
  import { updateReason } from '../services/selfupdateMessages';

  let confirmOpen = $state(false);

  const status = $derived($selfUpdateStatus);
  const version = $derived(stableOfferVersion(status));
</script>

{#if version}
  <Notice announce={false}>
    {msg('ui.stableOfferText', { current: status.currentVersion, version })}
    {#snippet action()}
      <Button size="sm" disabled={$stableBusy || $selfUpdateBusy} onclick={() => (confirmOpen = true)}>
        <Undo2 size="1.5rem" strokeWidth={1.8} />
        {$stableBusy ? msg('ui.stableOfferBusy') : msg('ui.stableOfferButton', { version })}
      </Button>
    {/snippet}
  </Notice>
{/if}

{#if confirmOpen}
  <ConfirmModal prompt={stableOfferPrompt(version)} onconfirm={requestInstallStable} onclose={() => (confirmOpen = false)} errorText={updateReason} />
{/if}
