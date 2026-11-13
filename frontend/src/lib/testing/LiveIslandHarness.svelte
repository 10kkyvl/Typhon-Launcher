<script lang="ts">
  import LiveIsland from '../components/LiveIsland.svelte';
  import Modal from '../components/Modal.svelte';

  // The live island as the shell mounts it: after the page, with a dialog a test can open over the
  // page. For LiveIsland.dom.test.ts.
  let { dialog = $bindable(false) }: { dialog?: boolean } = $props();
</script>

<main>
  <button type="button">Before</button>
</main>

<LiveIsland />

<Modal bind:open={dialog} title="Install">
  <input aria-label="Folder" />
</Modal>
