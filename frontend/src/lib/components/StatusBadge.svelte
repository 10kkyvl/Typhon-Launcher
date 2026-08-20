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
    gap: 0.7rem;
    height: 2.6rem;
    padding: 0 1.1rem;
    border-radius: 0.7rem;
    font-size: 1.3rem;
    font-weight: 500;
    white-space: nowrap;
  }

  .dot {
    width: 0.6rem;
    height: 0.6rem;
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
