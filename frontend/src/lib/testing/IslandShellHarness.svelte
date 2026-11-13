<script lang="ts">
  import AppShell from '../components/AppShell.svelte';
  import CloudConflictModal from '../components/CloudConflictModal.svelte';
  import Modal from '../components/Modal.svelte';

  // The main window's shell, which mounts the live island while motion is on and the toast stack
  // always, with a dialog a test can open over it and the save conflict dialog App.svelte mounts
  // after it. For LiveIsland.toasts.dom.test.ts and the producers' tests.
  let { dialog = $bindable(false) }: { dialog?: boolean } = $props();
</script>

<AppShell>
  <button type="button">Page</button>
</AppShell>

<Modal bind:open={dialog} title="Install">
  <input aria-label="Folder" />
</Modal>

<CloudConflictModal />
