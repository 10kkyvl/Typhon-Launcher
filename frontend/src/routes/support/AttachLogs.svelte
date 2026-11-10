<script lang="ts">
  import Toggle from '../../lib/components/Toggle.svelte';
  import { msg } from '../../lib/i18n';

  let {
    checked = $bindable(false),
    disabled = false,
    onchange,
  }: {
    checked?: boolean;
    disabled?: boolean;
    onchange?: (checked: boolean) => void;
  } = $props();
</script>

<div class="attach">
  <Toggle bind:checked {disabled} label={msg('tickets.attachLogs')} {onchange} />
  <div class="text">
    <span class="label">{msg('tickets.attachLogs')}</span>
    <span class="hint">{msg('tickets.attachLogsHint')}</span>
  </div>
</div>

<style>
  .attach {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
  }

  .text {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .label {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .hint {
    font-size: var(--font-xs);
    line-height: 1.45;
    color: var(--text-3);
  }
</style>
