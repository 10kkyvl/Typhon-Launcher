<script lang="ts">
  import { platformBadge } from '../library/emulation';

  let { platform }: { platform: string } = $props();
</script>

<span class="console-tag">{platformBadge(platform)}</span>

<style>
  .console-tag {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    height: 1.98rem;
    padding: 0 0.77rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-2);
    font-size: var(--font-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
  }
</style>
