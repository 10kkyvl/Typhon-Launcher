<script lang="ts">
  import { EyeOff } from '@lucide/svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';

  let { text = 'Скрыто от других. Вы видите этот блок, остальные — нет.' }: { text?: string } = $props();
</script>

<Tooltip {text}>
  <span class="hidden-badge">
    <EyeOff size="1.3rem" strokeWidth={1.8} />
    Скрыто
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
