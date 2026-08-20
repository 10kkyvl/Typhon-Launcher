<script lang="ts">
  let {
    value,
    max = 100,
    height = 6,
    color = 'var(--accent)',
  }: {
    value: number;
    max?: number;
    height?: number;
    color?: string;
  } = $props();

  const pct = $derived(Math.min(100, Math.max(0, (value / max) * 100)));
</script>

<div class="track" style:height="{height / 10}rem">
  <div class="fill" style:width="{pct}%" style:background={color}></div>
</div>

<style>
  .track {
    width: 100%;
    background: rgba(255, 255, 255, 0.07);
    border-radius: 9.9rem;
    overflow: hidden;
  }

  .fill {
    height: 100%;
    border-radius: 9.9rem;
    transition: width 600ms linear;
  }
</style>
