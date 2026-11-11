<script lang="ts">
  import { untrack } from 'svelte';
  import Modal from '../components/Modal.svelte';
  import Select from '../components/Select.svelte';
  import type { SelectOption } from '../components/select';

  // A select named by a visible label, on the bare page or inside a dialog, with controls before and
  // after it and an icon button: the shapes Select.dom.test.ts drives. `controlled` passes `value` and
  // `onchange` the way the settings pages do, and `accept` says whether the caller takes the change;
  // otherwise the value is bound. The value the caller holds is shown in `output`.
  let {
    options,
    value: initial = '',
    controlled = false,
    accept = true,
    inDialog = false,
    placeholder,
    disabled = false,
    onchange,
  }: {
    options: SelectOption[];
    value?: string;
    controlled?: boolean;
    accept?: boolean;
    inDialog?: boolean;
    placeholder?: string;
    disabled?: boolean;
    onchange?: (id: string) => void;
  } = $props();

  let value = $state(untrack(() => initial));
  let settings = $state(false);

  function change(id: string) {
    onchange?.(id);
    if (accept) value = id;
  }
</script>

{#snippet field()}
  <button type="button">Before</button>
  <span id="language-label">Language</span>
  {#if controlled}
    <Select {options} {value} labelledby="language-label" {placeholder} {disabled} onchange={change} />
  {:else}
    <Select {options} bind:value labelledby="language-label" {placeholder} {disabled} />
  {/if}
  <button type="button">After</button>
  <button type="button" aria-label="Help">
    <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 0h10v10H0z" /></svg>
  </button>
  <output>{value}</output>
{/snippet}

<button type="button" onclick={() => (settings = true)}>Open settings</button>

{#if inDialog}
  <Modal bind:open={settings} title="Settings">
    {@render field()}
  </Modal>
{:else}
  <section>
    {@render field()}
  </section>
{/if}
