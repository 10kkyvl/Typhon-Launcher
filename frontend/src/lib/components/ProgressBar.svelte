<script lang="ts">
  let {
    value,
    indeterminate = false,
    max = 100,
    height = 4,
    color = 'var(--accent)',
  }: {
    value: number;
    indeterminate?: boolean;
    max?: number;
    height?: number;
    color?: string;
  } = $props();

  const pct = $derived(max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0);
</script>

<div class="track" style:height="{height / 10}rem">
  <div class="fill" class:indeterminate style:width="{indeterminate ? 30 : pct}%" style:background={color}></div>
</div>

<style>
  .track {
    width: 100%;
    background: rgba(255, 255, 255, 0.07);
    border-radius: 99rem;
    overflow: hidden;
  }

  .fill {
    height: 100%;
    border-radius: 99rem;
    transition: width 600ms linear;
  }
  .fill.indeterminate { animation: sweep 1.4s ease-in-out infinite; }
  @keyframes sweep { from { transform: translateX(-100%); } to { transform: translateX(340%); } }
  @media (prefers-reduced-motion: reduce) { .fill.indeterminate { animation: none; width: 100% !important; opacity: 0.5; } }
</style>
