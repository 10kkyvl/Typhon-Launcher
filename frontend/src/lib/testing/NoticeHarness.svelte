<script lang="ts">
  import Button from '../components/Button.svelte';
  import Notice from '../components/Notice.svelte';

  // A notice between two other controls, as it sits on a page; `closable` removes it on close, the way
  // a page does. For Notice.dom.test.ts.
  let {
    tone = 'info',
    title,
    text = 'The catalog could not be loaded',
    compact = false,
    closable = false,
    withAction = false,
    onretry,
  }: {
    tone?: 'info' | 'success' | 'warning' | 'danger';
    title?: string;
    text?: string;
    compact?: boolean;
    closable?: boolean;
    withAction?: boolean;
    onretry?: () => void;
  } = $props();

  let shown = $state(true);
</script>

<button type="button">Before</button>
{#if shown}
  <Notice {tone} {title} {compact} onclose={closable ? () => (shown = false) : undefined}>
    {text}
    {#snippet action()}
      {#if withAction}<Button size="sm" onclick={onretry}>Retry</Button>{/if}
    {/snippet}
  </Notice>
{/if}
<button type="button">After</button>
