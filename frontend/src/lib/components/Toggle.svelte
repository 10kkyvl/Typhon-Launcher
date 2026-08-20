<script lang="ts">
  let {
    checked = $bindable(false),
    label,
    onchange,
  }: {
    checked?: boolean;
    label?: string;
    onchange?: (checked: boolean) => void;
  } = $props();

  function toggle() {
    checked = !checked;
    onchange?.(checked);
  }
</script>

<button class="toggle" class:on={checked} role="switch" aria-checked={checked} aria-label={label} onclick={toggle}>
  <span class="knob"></span>
</button>

<style>
  .toggle {
    position: relative;
    width: 4.6rem;
    height: 2.6rem;
    border-radius: 9.9rem;
    background: rgba(255, 255, 255, 0.12);
    flex-shrink: 0;
    transition: background var(--dur) var(--ease);
  }

  .toggle:hover {
    background: rgba(255, 255, 255, 0.18);
  }

  .toggle.on {
    background: var(--accent);
  }

  .toggle.on:hover {
    background: var(--accent-hover);
  }

  .knob {
    position: absolute;
    top: 0.3rem;
    left: 0.3rem;
    width: 2rem;
    height: 2rem;
    border-radius: 50%;
    background: #fff;
    transition: transform var(--dur) var(--ease);
  }

  .on .knob {
    transform: translateX(2rem);
  }
</style>
