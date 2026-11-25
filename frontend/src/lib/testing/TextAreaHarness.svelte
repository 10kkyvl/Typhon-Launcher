<script lang="ts">
  import { untrack } from 'svelte';
  import TextArea from '../components/TextArea.svelte';

  // A caller of the multi-line field: it holds the value through bind:value, sets it from outside,
  // counts it against a limit and sends it with Ctrl/Cmd+Enter, as a message form does.
  let { initial = '', max = 20, grow }: { initial?: string; max?: number; grow?: number } = $props();
  let value = $state(untrack(() => initial));
  let field = $state<HTMLTextAreaElement>();
  let sent = $state<string[]>([]);

  function onkeydown(event: KeyboardEvent) {
    if (event.key !== 'Enter' || !(event.ctrlKey || event.metaKey)) return;
    event.preventDefault();
    sent = [...sent, value];
  }
</script>

<TextArea
  label="Message"
  bind:value
  bind:textarea={field}
  {grow}
  counter={{ max, text: (count, limit) => `${count} of ${limit}`, limitText: (count, limit) => (count > limit ? `${count - limit} over` : `${limit - count} left`) }}
  {onkeydown}
/>
<output>{value}</output>
<ul aria-label="Sent">
  {#each sent as text, index (index)}<li>{text}</li>{/each}
</ul>
<button type="button" onclick={() => (value = '')}>Clear</button>
<button type="button" onclick={() => field?.focus()}>Focus the field</button>
