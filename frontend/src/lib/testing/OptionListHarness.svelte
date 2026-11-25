<script lang="ts">
  import { untrack } from 'svelte';
  import OptionList from '../components/OptionList.svelte';
  import type { OptionItem } from '../components/optionList';

  // A caller of the option list between two plain buttons: it holds the choice through bind:value, or
  // through value and onchange (`through`, which may refuse by not writing), and draws its rows itself
  // with `custom`. A test changes the rows under the focus by rerendering with other `options`. For
  // OptionList.dom.test.ts.
  let {
    options,
    start = '',
    through,
    custom = false,
    labelled = false,
  }: {
    options: OptionItem[];
    start?: string;
    through?: (id: string) => boolean;
    custom?: boolean;
    labelled?: boolean;
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
    onchange={(id) => {
      if (through(id)) value = id;
    }}
  />
{:else if custom}
  <OptionList label="Folder" options={rows} bind:value>
    {#snippet row(option)}<span data-custom={option.id}>{option.title}</span> <span>{option.fact}</span>{/snippet}
  </OptionList>
{:else if labelled}
  <OptionList labelledby="list-label" options={rows} bind:value />
{:else}
  <OptionList label="Folder" options={rows} bind:value />
{/if}
<button type="button">After</button>
<output>{value}</output>
<button type="button" onclick={() => (value = 'set from outside')}>Set</button>
