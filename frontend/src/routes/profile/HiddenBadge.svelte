<script lang="ts">
  import { EyeOff } from '@lucide/svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
</script>

<Tooltip text="Скрыто от других. Вы видите этот блок, остальные — нет.">
  <span class="hidden-badge">
    <EyeOff size="1.3rem" strokeWidth={1.8} />
    Скрыто
  </span>
</Tooltip>

<style>
  .hidden-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.7rem;
    border-radius: var(--radius-xs);
    background: var(--surface-3);
    color: var(--text-3);
    font-size: var(--font-xs);
  }
</style>
