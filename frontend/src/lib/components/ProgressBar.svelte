<script lang="ts">
  let {
    value,
    max = 100,
    height = 4,
    color = 'var(--accent)',
  }: {
    value: number;
    max?: number;
    height?: number;
    color?: string;
  } = $props();

  const pct = $derived(max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0);
</script>

<div class="track" style:height="{height / 10}rem">
  <div class="fill" style:width="{pct}%" style:background={color}></div>
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
</style>
