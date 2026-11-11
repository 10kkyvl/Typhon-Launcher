<script lang="ts">
  import Modal from '../components/Modal.svelte';

  // A page with a button that opens a dialog, and a button inside that dialog that opens another one:
  // the shape every Modal caller has. For Modal.dom.test.ts.
  let {
    dismissible = true,
    onclose,
    oninnerclose,
  }: { dismissible?: boolean; onclose?: () => void; oninnerclose?: () => void } = $props();

  let open = $state(false);
  let inner = $state(false);
</script>

<main>
  <button type="button" onclick={() => (open = true)}>Open settings</button>
  <button type="button">Elsewhere</button>
</main>

<Modal bind:open title="Settings" {dismissible} {onclose}>
  <input aria-label="Name" />
  <button type="button" onclick={() => (inner = true)}>More</button>
  {#snippet footer()}
    <button type="button" onclick={() => (open = false)}>Save</button>
  {/snippet}
</Modal>

<Modal bind:open={inner} title="More settings" onclose={oninnerclose}>
  <button type="button">Inner action</button>
</Modal>
