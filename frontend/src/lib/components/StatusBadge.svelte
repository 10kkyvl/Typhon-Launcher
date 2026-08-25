<script lang="ts">
  let {
    kind = 'neutral',
    label,
    dot = true,
    plain = false,
  }: {
    kind?: 'success' | 'warning' | 'danger' | 'accent' | 'neutral';
    label: string;
    dot?: boolean;
    plain?: boolean;
  } = $props();
</script>

<span class="badge {kind}" class:plain>
  {#if dot}<span class="dot"></span>{/if}
  {label}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: 2.4rem;
    padding: 0 0.9rem;
    border-radius: var(--radius-sm);
    font-size: 1.2rem;
    font-weight: 500;
    white-space: nowrap;
  }

  .badge.plain {
    height: auto;
    padding: 0;
    background: none;
    font-size: var(--font-xs);
    font-weight: 400;
    color: var(--text-2);
  }

  .dot {
    width: 0.6rem;
    height: 0.6rem;
    border-radius: 50%;
    background: currentColor;
    flex-shrink: 0;
  }

  .plain .dot {
    width: 0.7rem;
    height: 0.7rem;
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
    background: var(--hover-strong);
    color: var(--text-2);
  }

  .plain.success .dot {
    background: var(--success);
  }

  .plain.warning .dot {
    background: var(--warning);
  }

  .plain.danger .dot {
    background: var(--danger);
  }

  .plain.accent .dot {
    background: var(--accent);
  }

  .plain.neutral .dot {
    background: var(--text-3);
  }
</style>
