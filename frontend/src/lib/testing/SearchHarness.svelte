<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import TopbarSearch from '../components/TopbarSearch.svelte';

  // The top bar's search between two other controls, and a dialog a test opens over the page.
  // For TopbarSearch.dom.test.ts.
  let { dialog = false }: { dialog?: boolean } = $props();
</script>

<button type="button">Before</button>
<TopbarSearch />
<button type="button">After</button>

<Modal open={dialog} title="Other">
  <p>Over it</p>
</Modal>
