<script lang="ts">
  import Checkbox from '../components/Checkbox.svelte';

  // A checkbox row as a dialog builds it, with what the caller holds shown next to it. `mode` is who
  // holds the state: the caller, which may refuse the change (`refuse`), or the box through a binding.
  // For Checkbox.dom.test.ts.
  let {
    mode = 'controlled',
    refuse = false,
    start = false,
    disabled = false,
    reason,
    aside,
    asked,
  }: {
    mode?: 'controlled' | 'bound';
    refuse?: boolean;
    start?: boolean | 'mixed';
    disabled?: boolean;
    reason?: string;
    aside?: string;
    asked?: (next: boolean) => void;
  } = $props();

  // svelte-ignore state_referenced_locally: the starting value only
  let held = $state<boolean | 'mixed'>(start);
</script>

<button type="button">Before</button>
{#if mode === 'bound'}
  <Checkbox bind:checked={held} label="Install when done" description="Runs the installer after the download" {disabled} {reason} {aside} />
{:else}
  <Checkbox
    checked={held}
    label="Install when done"
    description="Runs the installer after the download"
    {disabled}
    {reason}
    {aside}
    data-row="install"
    onchange={(next) => {
      asked?.(next);
      if (!refuse) held = next;
    }}
  />
{/if}
<button type="button">After</button>
<output>{String(held)}</output>
