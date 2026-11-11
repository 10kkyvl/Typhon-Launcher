<script lang="ts">
  import { EyeOff } from '@lucide/svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { msg } from '../../lib/i18n';

  let { text = msg('social.hiddenDefaultHint') }: { text?: string } = $props();
</script>

<Tooltip {text}>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (A stop in the Tab order so the hint, who it is hidden from, reaches keyboard users; pressing it does nothing.) -->
  <span class="hidden-badge" tabindex="0">
    <EyeOff size="1.3rem" strokeWidth={1.8} />
    {msg('social.hiddenLabel')}
  </span>
</Tooltip>

<style>
  .hidden-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: 2.4rem;
    padding: 0 0.9rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-3);
    font-size: 1.2rem;
    font-weight: 500;
    white-space: nowrap;
  }
</style>
