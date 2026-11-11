<script lang="ts">
  import Toggle from '../components/Toggle.svelte';

  // A settings row with a switch, as the pages build it: the row's label and description are the
  // visible text next to the switch. `mode` is who holds the state: the caller, which may refuse the
  // change (`refuse`), or the switch through a binding. For Toggle.dom.test.ts.
  let {
    mode = 'controlled',
    refuse = false,
    start = false,
    disabled = false,
    asked,
  }: {
    mode?: 'controlled' | 'bound';
    refuse?: boolean;
    start?: boolean;
    disabled?: boolean;
    asked?: (next: boolean) => void;
  } = $props();

  // svelte-ignore state_referenced_locally: the starting value only
  let held = $state(start);
</script>

<div class="row">
  <div class="row-text">
    <span id="tray-label">Minimise to tray</span>
    <span id="tray-sub">Keeps the launcher running in the background</span>
  </div>
  {#if mode === 'bound'}
    <Toggle bind:checked={held} labelledby="tray-label" describedby="tray-sub" {disabled} />
  {:else}
    <Toggle
      checked={held}
      labelledby="tray-label"
      describedby="tray-sub"
      {disabled}
      onchange={(next) => {
        asked?.(next);
        if (!refuse) held = next;
      }}
    />
  {/if}
</div>
<output>{held ? 'on' : 'off'}</output>
