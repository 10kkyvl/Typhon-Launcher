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
    width: 42px;
    height: 24px;
    border-radius: 99px;
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
    top: 3px;
    left: 3px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #fff;
    transition: transform var(--dur) var(--ease);
  }

  .on .knob {
    transform: translateX(18px);
  }
</style>
