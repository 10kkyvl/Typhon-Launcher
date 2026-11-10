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
    width: 4.4rem;
    height: 2.42rem;
    border-radius: 108.9rem;
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
    background: var(--accent-bright);
  }

  .toggle.on:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .knob {
    position: absolute;
    top: 0.33rem;
    left: 0.33rem;
    width: 1.76rem;
    height: 1.76rem;
    border-radius: 50%;
    background: #fff;
    transition: transform var(--dur) var(--ease);
  }

  .on .knob {
    background: var(--accent-on, #fff);
    transform: translateX(1.98rem);
  }
</style>
