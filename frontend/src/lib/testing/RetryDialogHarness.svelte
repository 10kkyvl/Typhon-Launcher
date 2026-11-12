<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Notice from '../components/Notice.svelte';

  // A page with a dialog over it whose list failed to load: the error and its Retry are in the dialog,
  // and what replaces them on success has no control of its own. `aim: 'page'` names a focus target
  // on the page under the dialog, which is inert. For Notice.retry.dom.test.ts.
  let { run, open = true, aim = 'none' }: { run: () => Promise<boolean>; open?: boolean; aim?: 'none' | 'page' } = $props();

  let pageButton = $state<HTMLElement>();

  let failed = $state(true);
  let retrying = $state(false);

  async function attempt(): Promise<boolean> {
    const ok = await run();
    failed = !ok;
    return ok;
  }
</script>

<main>
  <h1 tabindex="-1">Library</h1>
  <button type="button" bind:this={pageButton}>On the page</button>
</main>

<Modal {open} title="Pick a game">
  {#if failed || retrying}
    <Notice tone="danger" compact bind:retrying retry={{ label: 'Retry', run: attempt, focus: aim === 'page' ? () => pageButton : undefined }}>The list could not be loaded</Notice>
  {:else}
    <p>Nothing to pick yet</p>
  {/if}
</Modal>
