<script lang="ts">
  import { EyeOff } from '@lucide/svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { msg } from '../../lib/i18n';

  let { text = msg('social.hiddenDefaultHint') }: { text?: string } = $props();
</script>

<Tooltip {text}>
  <span class="hidden-badge">
    <EyeOff size="1.43rem" strokeWidth={1.8} />
    {msg('social.hiddenLabel')}
  </span>
</Tooltip>

<style>
  .hidden-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.66rem;
    height: 2.64rem;
    padding: 0 0.99rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-3);
    font-size: 1.32rem;
    font-weight: 500;
    white-space: nowrap;
  }
</style>
