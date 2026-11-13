<script lang="ts">
  import LiveIsland from '../components/LiveIsland.svelte';
  import Modal from '../components/Modal.svelte';

  // The live island as the shell mounts it: after the page, with a dialog a test can open over the
  // page. For LiveIsland.dom.test.ts.
  // `onescape`: a page field that answers Escape itself (a search clearing its text).
  let { dialog = $bindable(false), onescape }: { dialog?: boolean; onescape?: () => void } = $props();
</script>

<main>
  <input aria-label="Search" onkeydown={(event) => event.key === 'Escape' && onescape?.()} />
  <button type="button">Before</button>
</main>

<LiveIsland />

<Modal bind:open={dialog} title="Install">
  <input aria-label="Folder" />
</Modal>
