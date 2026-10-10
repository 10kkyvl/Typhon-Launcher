<script lang="ts">
  import Card from '../Card.svelte';

  let { title, body }: { title: string; body: string } = $props();
</script>

<Card title={title || undefined}>
  <p class="body">{body}</p>
</Card>

<style>
  .body {
    margin: 0;
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
