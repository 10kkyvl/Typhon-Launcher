<script lang="ts">
  import { untrack } from 'svelte';
  import OptionList from '../components/OptionList.svelte';
  import type { OptionItem } from '../components/optionList';

  // A caller of the option list between two plain buttons: it holds the choice through bind:value, or
  // through value and onchange (`through`, which may refuse by not writing), and draws its rows itself
  // with `custom`; `activation` goes to the list held through onchange. A test changes the rows under
  // the focus, or holds the list, by rerendering with other `options` or `disabled`. For
  // OptionList.dom.test.ts.
  let {
    options,
    start = '',
    through,
    custom = false,
    labelled = false,
    disabled = false,
    activation,
  }: {
    options: OptionItem[];
    start?: string;
    through?: (id: string) => boolean;
    custom?: boolean;
    labelled?: boolean;
    disabled?: boolean;
    activation?: 'auto' | 'manual';
  } = $props();

  const rows = $derived(options);
  let value = $state(untrack(() => start));
</script>

<button type="button">Before</button>
{#if labelled}<span id="list-label">Save folder</span>{/if}
{#if through}
  <OptionList
    label="Folder"
    options={rows}
    {value}
    {disabled}
    {activation}
    onchange={(id) => {
      if (through(id)) value = id;
    }}
  />
{:else if custom}
  <OptionList label="Folder" options={rows} {disabled} bind:value>
    {#snippet row(option)}<span data-custom={option.id}>{option.title}</span> <span>{option.fact}</span>{/snippet}
  </OptionList>
{:else if labelled}
  <OptionList labelledby="list-label" options={rows} {disabled} bind:value />
{:else}
  <OptionList label="Folder" options={rows} {disabled} bind:value />
{/if}
<button type="button">After</button>
<output>{value}</output>
<button type="button" onclick={() => (value = 'set from outside')}>Set</button>
