<script lang="ts">
  import { untrack } from 'svelte';
  import TextField from '../components/TextField.svelte';

  // A caller of the field: it holds the value through bind:value, sets it from outside, and puts one
  // control at the field's end.
  let { initial = '', error = '' }: { initial?: string; error?: string } = $props();
  let value = $state(untrack(() => initial));
  let field = $state<HTMLInputElement>();
</script>

<TextField label="Name" bind:value bind:input={field} {error}>
  {#snippet trailing()}
    <button type="button" onclick={() => (value = '')}>Clear</button>
  {/snippet}
</TextField>
<output>{value}</output>
<button type="button" onclick={() => (value = 'set from outside')}>Set</button>
<button type="button" onclick={() => field?.focus()}>Focus the field</button>
