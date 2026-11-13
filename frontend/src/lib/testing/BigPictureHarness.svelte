<script lang="ts">
  import BigPictureShell from '../components/BigPictureShell.svelte';
  import CloudConflictModal from '../components/CloudConflictModal.svelte';
  import EmulatorNeededModal from '../components/EmulatorNeededModal.svelte';
</script>

<!-- What App.svelte mounts while Big Picture is active: the shell, and the shared prompts after it. -->
<BigPictureShell />
<EmulatorNeededModal />
<CloudConflictModal />
