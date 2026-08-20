<script lang="ts">
  let {
    kind = 'neutral',
    label,
    dot = true,
  }: {
    kind?: 'success' | 'warning' | 'danger' | 'accent' | 'neutral';
    label: string;
    dot?: boolean;
  } = $props();
</script>

<span class="badge {kind}">
  {#if dot}<span class="dot"></span>{/if}
  {label}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 26px;
    padding: 0 11px;
    border-radius: 7px;
    font-size: 12.5px;
    font-weight: 500;
    white-space: nowrap;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }

  .success {
    background: var(--success-subtle);
    color: var(--success);
  }

  .warning {
    background: var(--warning-subtle);
    color: var(--warning);
  }

  .danger {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .accent {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .neutral {
    background: rgba(255, 255, 255, 0.06);
    color: var(--text-2);
  }
</style>
