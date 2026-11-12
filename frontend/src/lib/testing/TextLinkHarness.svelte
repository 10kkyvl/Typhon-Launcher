<script lang="ts">
  import TextLink from '../components/TextLink.svelte';

  // A link to an address and an action that reads as a link, each in a sentence, as the pages write
  // them. For TextLink.dom.test.ts.
  let { onforgot, disabled = false }: { onforgot: () => void; disabled?: boolean } = $props();
</script>

<p>Read the <TextLink href="https://example.com/guide" target="_blank" rel="noopener noreferrer">guide</TextLink> first.</p>
<p>Forgot your password? <TextLink onclick={onforgot} {disabled} data-kind="forgot">Reset it</TextLink></p>
<button type="button">After</button>
