<script lang="ts">
  import Button from '../components/Button.svelte';
  import Notice from '../components/Notice.svelte';

  // A notice between two other controls, as it sits on a page; `closable` removes it on close, the way
  // a page does, and `withAction` is the only way an action reaches it. For Notice.dom.test.ts.
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

{#snippet retry()}
  <Button size="sm" onclick={onretry}>Retry</Button>
{/snippet}

<button type="button">Before</button>
{#if shown}
  <Notice {tone} {title} {compact} action={withAction ? retry : undefined} onclose={closable ? () => (shown = false) : undefined}>
    {text}
  </Notice>
{/if}
<button type="button">After</button>
