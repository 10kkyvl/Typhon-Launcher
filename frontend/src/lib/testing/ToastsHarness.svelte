<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Toasts from '../components/Toasts.svelte';

  // The toast stack as a shell mounts it: after the page, in a part the layer stack leaves live, with
  // a dialog a test can open over the page. For Toasts.dom.test.ts.
  let { dialog = false }: { dialog?: boolean } = $props();
</script>

<main>
  <button type="button">Before</button>
</main>
<div data-layer-keep>
  <Toasts />
</div>

<Modal open={dialog} title="Install">
  <input aria-label="Folder" />
</Modal>
