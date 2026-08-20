<script lang="ts">
  import { ChevronDown, Check } from '@lucide/svelte';
  import { clickOutside } from '../utils/clickOutside';

  let {
    options,
    value = $bindable(),
    width = '22rem',
    onchange,
  }: {
    options: { id: string; label: string }[];
    value: string;
    width?: string;
    onchange?: (id: string) => void;
  } = $props();

  let open = $state(false);

  const selected = $derived(options.find((o) => o.id === value));
</script>

<div class="select" style:width use:clickOutside={() => (open = false)}>
  <button class="trigger" class:open aria-haspopup="listbox" aria-expanded={open} onclick={() => (open = !open)}>
    <span>{selected?.label ?? ''}</span>
    <ChevronDown size="1.6rem" strokeWidth={1.8} />
  </button>
  {#if open}
    <div class="menu" role="listbox">
      {#each options as option (option.id)}
        <button
          class="option"
          role="option"
          aria-selected={value === option.id}
          onclick={() => {
            value = option.id;
            open = false;
            onchange?.(option.id);
          }}
        >
          <span>{option.label}</span>
          {#if value === option.id}<Check size="1.5rem" strokeWidth={2} />{/if}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .select {
    position: relative;
  }

  .trigger {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    width: 100%;
    height: var(--control-md);
    padding: 0 1.2rem 0 1.4rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 1.5rem;
    color: var(--text);
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .trigger :global(svg) {
    color: var(--text-3);
    transition: transform var(--dur) var(--ease);
  }

  .trigger.open :global(svg),
  .trigger:hover {
    border-color: var(--border-strong);
    background: var(--surface-3);
  }

  .trigger.open :global(svg) {
    transform: rotate(180deg);
  }

  .menu {
    position: absolute;
    z-index: 40;
    top: calc(100% + 0.6rem);
    left: 0;
    right: 0;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .option {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    width: 100%;
    padding: 0.8rem 1rem;
    border-radius: 0.7rem;
    font-size: 1.4rem;
    color: var(--text-2);
    text-align: left;
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .option:hover {
    background: rgba(255, 255, 255, 0.06);
    color: var(--text);
  }

  .option[aria-selected='true'] {
    color: var(--text);
  }

  .option :global(svg) {
    color: var(--accent-text);
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-0.3rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
