<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import TextLink from './TextLink.svelte';
  import { msg } from '../i18n';

  /**
   * The question before cloud saves are turned on: what is sent, and the player's yes. The focus opens
   * on Cancel. "Enable" sends `onenable` once; while it runs, and while the caller says it is `busy`,
   * the other controls are held with the reason and the dialog cannot be dismissed. The caller closes
   * the dialog once cloud saves are on: an answer that leaves this opening open did not turn them on,
   * and the dialog says so.
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

  const held = $derived(busy || sending);

  async function enable() {
    if (held) return;
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

<Modal dismissible={!held} bind:open busy={held} title={msg('cloudsaves.consentTitle')} width="48rem">
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
    <Button data-autofocus disabled={held} reason={held ? msg('cloudsaves.consentBusy') : ''} onclick={() => (open = false)}>
      {msg('common.cancel')}
    </Button>
    <Button variant="primary" busy={held} aria-describedby={failed ? failureId : undefined} onclick={enable}>
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
