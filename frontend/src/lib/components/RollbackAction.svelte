<script lang="ts">
  import { Undo2 } from '@lucide/svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import { rollbackPrompt } from '../confirm/prompts';
  import { rollbackReason } from '../services/selfupdateMessages';
  import { rollbackShown } from '../services/selfupdate';
  import { requestRollback, rollbackBusy, rollbackInfo, selfUpdateStatus } from '../stores/selfupdate';
  import { msg } from '../i18n';

  let { compact = false }: { compact?: boolean } = $props();

  let confirmOpen = $state(false);

  const info = $derived($rollbackInfo);
  const shown = $derived(rollbackShown(info));
  const label = $derived(msg('ui.rollbackButton', { version: info.version }));
  const prompt = $derived(rollbackPrompt(info.version, $selfUpdateStatus.currentVersion));
</script>

{#if shown}
  {#if info.available}
    <Button variant={compact ? 'ghost' : 'secondary'} size="sm" disabled={$rollbackBusy} onclick={() => (confirmOpen = true)}>
      <Undo2 size="1.5rem" strokeWidth={1.8} />
      {$rollbackBusy ? msg('settings.aboutRollbackBusy') : label}
    </Button>
  {:else}
    <span title={rollbackReason(info)}>
      <Button variant={compact ? 'ghost' : 'secondary'} size="sm" disabled>
        <Undo2 size="1.5rem" strokeWidth={1.8} />
        {msg('ui.rollbackButtonNone')}
      </Button>
    </span>
  {/if}
{/if}

{#if confirmOpen}
  <ConfirmModal {prompt} onconfirm={requestRollback} onclose={() => (confirmOpen = false)} />
{/if}
