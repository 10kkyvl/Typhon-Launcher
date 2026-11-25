<script lang="ts">
  import { untrack } from 'svelte';
  import PathField from '../components/PathField.svelte';

  // A caller of the path field: it holds the path through bind:value, writes what its picker answers
  // (`pick`, a promise the test settles) and sets the path from outside. For PathField.dom.test.ts.
  let {
    initial = '',
    editable = false,
    pick,
  }: { initial?: string; editable?: boolean; pick: () => Promise<string> } = $props();
  let value = $state(untrack(() => initial));
  let field = $state<HTMLInputElement>();

  async function browse() {
    const picked = await pick();
    if (picked) value = picked;
  }
</script>

<PathField label="Install to" placeholder="No folder chosen" browseLabel="Browse" onbrowse={browse} {editable} bind:value bind:input={field}>
  {#snippet facts()}<span>Free: 12 GB</span>{/snippet}
</PathField>
<output>{value}</output>
<button type="button" onclick={() => (value = 'set from outside')}>Set</button>
<button type="button" onclick={() => field?.focus()}>Focus the field</button>
