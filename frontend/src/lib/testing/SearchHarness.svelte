<script lang="ts">
  import ConfirmModal from '../components/ConfirmModal.svelte';
  import Modal from '../components/Modal.svelte';
  import TopbarSearch from '../components/TopbarSearch.svelte';
  import { confirmLeave, leaveRequest, stayHere } from '../stores/router';

  // The top bar's search between two other controls, a dialog a test opens over the page, and the
  // leave confirmation App.svelte shows while a page's leave guard holds a navigation.
  // For TopbarSearch.dom.test.ts.
  let { dialog = false }: { dialog?: boolean } = $props();
</script>

<button type="button">Before</button>
<TopbarSearch />
<button type="button">After</button>

<Modal open={dialog} title="Other">
  <p>Over it</p>
</Modal>

{#if $leaveRequest}
  <ConfirmModal prompt={$leaveRequest.prompt()} onconfirm={confirmLeave} onclose={stayHere} />
{/if}
