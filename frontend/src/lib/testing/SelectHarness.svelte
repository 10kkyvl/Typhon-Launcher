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
    naming = 'labelledby',
    onchange,
  }: {
    options: SelectOption[];
    value?: string;
    controlled?: boolean;
    accept?: boolean;
    inDialog?: boolean;
    placeholder?: string;
    disabled?: boolean;
    /** How the field gets its name "Language": a visible label it points at, `label`, or `<label for>` + `id`. */
    naming?: 'labelledby' | 'label' | 'for';
    onchange?: (id: string) => void;
  } = $props();

  const names = $derived(
    naming === 'label' ? { label: 'Language' } : naming === 'for' ? { id: 'language-field' } : { labelledby: 'language-label' },
  );

  let value = $state(untrack(() => initial));
  let settings = $state(false);

  function change(id: string) {
    onchange?.(id);
    if (accept) value = id;
  }
</script>

{#snippet field()}
  <button type="button">Before</button>
  {#if naming === 'labelledby'}
    <span id="language-label">Language</span>
  {:else if naming === 'for'}
    <label for="language-field">Language</label>
  {/if}
  {#if controlled}
    <Select {options} {value} {...names} {placeholder} {disabled} onchange={change} />
  {:else}
    <Select {options} bind:value {...names} {placeholder} {disabled} />
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
