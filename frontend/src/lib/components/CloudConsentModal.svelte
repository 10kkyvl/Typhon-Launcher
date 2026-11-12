<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import TextLink from './TextLink.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    busy = false,
    onenable,
    onpolicy,
  }: {
    open?: boolean;
    busy?: boolean;
    onenable: () => void;
    onpolicy: () => void;
  } = $props();
</script>

<Modal dismissible={!busy} bind:open title={msg('cloudsaves.consentTitle')} width="48rem">
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

  {#snippet footer()}
    <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" {busy} onclick={onenable}>{msg('cloudsaves.consentEnable')}</Button>
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
