<script lang="ts">
  let {
    id,
    name,
    value,
    describedBy,
    onchange,
  }: {
    id: string;
    name: string;
    value: string;
    describedBy?: string;
    onchange: (value: string) => void;
  } = $props();
</script>

<input
  {id}
  class="input sm token-value"
  type="text"
  title={name}
  aria-describedby={describedBy}
  {value}
  oninput={(e) => onchange((e.currentTarget as HTMLInputElement).value)}
/>

<style>
  .token-value {
    flex: 1 1 auto;
    min-width: 0;
  }
</style>
