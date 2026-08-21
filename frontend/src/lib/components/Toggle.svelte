<script lang="ts">
  let {
    checked = $bindable(false),
    label,
    disabled = false,
    onchange,
  }: {
    checked?: boolean;
    label?: string;
    disabled?: boolean;
    onchange?: (checked: boolean) => void;
  } = $props();

  function toggle() {
    if (disabled) return;
    checked = !checked;
    onchange?.(checked);
  }
</script>

<button
  class="toggle"
  class:on={checked}
  {disabled}
  role="switch"
  aria-checked={checked}
  aria-label={label}
  onclick={toggle}
>
  <span class="knob"></span>
</button>

<style>
  .toggle {
    position: relative;
    width: 4rem;
    height: 2.2rem;
    border-radius: 99rem;
    background: rgba(255, 255, 255, 0.12);
    flex-shrink: 0;
    transition: background var(--dur) var(--ease);
  }

  .toggle:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .toggle:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.18);
  }

  .toggle.on {
    background: var(--accent);
  }

  .toggle.on:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .knob {
    position: absolute;
    top: 0.3rem;
    left: 0.3rem;
    width: 1.6rem;
    height: 1.6rem;
    border-radius: 50%;
    background: #fff;
    transition: transform var(--dur) var(--ease);
  }

  .on .knob {
    transform: translateX(1.8rem);
  }
</style>
