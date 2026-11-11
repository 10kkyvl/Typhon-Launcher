<script lang="ts">
  import SegmentedControl from '../components/SegmentedControl.svelte';
  import type { SegmentOption } from '../components/segmented';

  // A segmented control between two plain buttons, on a caller's state: a plain binding, or a
  // function binding as InstallModal and the layout editor use. For SegmentedControl.dom.test.ts.
  let {
    options,
    start,
    variant,
    icons = false,
    disabled = false,
    through,
    labelled = false,
  }: {
    options: SegmentOption[];
    start: string;
    variant?: 'box' | 'pill';
    icons?: boolean;
    disabled?: boolean;
    through?: (id: string) => void;
    labelled?: boolean;
  } = $props();

  // svelte-ignore state_referenced_locally: the starting value only
  let value = $state(start);
</script>

<button type="button">Before</button>
{#if labelled}<span id="view-label">Shown as</span>{/if}
{#if through}
  <SegmentedControl
    {options}
    {variant}
    {disabled}
    label="View"
    bind:value={
      () => value,
      (id) => {
        through(id);
        value = id;
      }
    }
  />
{:else if icons}
  <SegmentedControl {options} {variant} {disabled} label="View" bind:value>
    {#snippet item(option)}<svg aria-hidden="true" data-icon={option.id}></svg>{/snippet}
  </SegmentedControl>
{:else if labelled}
  <SegmentedControl {options} {variant} {disabled} labelledby="view-label" bind:value />
{:else}
  <SegmentedControl {options} {variant} {disabled} label="View" bind:value />
{/if}
<button type="button">After</button>
<output>{value}</output>
