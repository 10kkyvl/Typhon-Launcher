<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import TextLink from './TextLink.svelte';
  import { msg } from '../i18n';

  /**
   * The question before cloud saves are turned on: what is sent, and the player's yes. The focus opens
   * on Cancel. "Enable" sends `onenable` once; while it runs it is busy, Cancel is held with the reason
   * and the dialog cannot be dismissed. A caller that says it is `busy` with other work holds only
   * "Enable", with its reason: the dialog can still be left. The caller closes the dialog once cloud
   * saves are on: an answer that leaves this opening open did not turn them on, and the dialog says so.
   */
  let {
    open = $bindable(false),
    busy = false,
    onenable,
    onpolicy,
  }: {
    open?: boolean;
    busy?: boolean;
    onenable: () => Promise<unknown>;
    onpolicy: () => void;
  } = $props();

  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let sending = $state(false);
  let failed = $state(false);
  // Each opening: an answer to an earlier one paints nothing.
  let opening = 0;
  $effect(() => {
    if (!open) return;
    untrack(() => {
      opening += 1;
      sending = false;
      failed = false;
    });
  });

  // The caller's work that is not this press.
  const foreign = $derived(busy && !sending);

  async function enable() {
    if (sending || busy) return;
    const mine = opening;
    sending = true;
    failed = false;
    try {
      await onenable();
    } catch (err) {
      console.error('cloud saves consent', err);
    }
    if (mine !== opening) return;
    sending = false;
    if (open) failed = true;
  }
</script>

<Modal dismissible={!sending} bind:open busy={sending} title={msg('cloudsaves.consentTitle')} width="48rem">
  <p class="text">{msg('cloudsaves.consentIntro')}</p>
  <ul class="items">
    <li>{msg('cloudsaves.consentArchive')}</li>
    <li>{msg('cloudsaves.consentGameId')}</li>
    <li>{msg('cloudsaves.consentDeviceId')}</li>
    <li>{msg('cloudsaves.consentSizeDigest')}</li>
  </ul>
  <p class="text">{msg('cloudsaves.consentRetention')}</p>
  <p class="text">
    {msg('cloudsaves.consentPolicy')}
    <TextLink onclick={onpolicy}>{msg('cloudsaves.consentPolicyLink')}</TextLink>
  </p>
  {#if failed}
    <Notice tone="danger" compact id={failureId}>{msg('cloudsaves.consentFailed')}</Notice>
  {/if}

  {#snippet footer()}
    <Button data-autofocus disabled={sending} reason={sending ? msg('cloudsaves.consentBusy') : ''} onclick={() => (open = false)}>
      {msg('common.cancel')}
    </Button>
    <Button
      variant="primary"
      busy={sending}
      disabled={foreign}
      reason={foreign ? msg('cloudsaves.consentHeld') : ''}
      aria-describedby={failed ? failureId : undefined}
      onclick={enable}
    >
      {msg('cloudsaves.consentEnable')}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: 0 0 var(--space-3);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .items {
    margin: 0 0 var(--space-3);
    padding-left: var(--space-5);
    font-size: var(--font-sm);
    color: var(--text);
  }
</style>
