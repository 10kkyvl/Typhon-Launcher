<script lang="ts">
  import AppShell from '../components/AppShell.svelte';
  import Modal from '../components/Modal.svelte';
  import { toast } from '../stores/toasts';

  // The main window's shell with a dialog that reports a failure as a toast and stays open, as
  // InstallModal does. For AppShell.dom.test.ts.
  // `confirm` opens a second, small dialog over the first.
  let { onretry, confirm = false }: { onretry: () => void; confirm?: boolean } = $props();

  let open = $state(true);
</script>

<AppShell>
  <p>Library</p>
</AppShell>

<Modal bind:open title="Install">
  <button type="button" onclick={() => toast('Install failed', 'danger', { label: 'Retry', run: onretry })}>Install</button>
</Modal>

<Modal open={confirm} title="Confirm">
  <p>Sure?</p>
</Modal>
