<script lang="ts">
  import { ChevronLeft, ExternalLink } from '@lucide/svelte';
  import TextLink from '../components/TextLink.svelte';

  // A link to an address and an action that reads as a link, each in a sentence, as the pages write
  // them, a muted link in a path, and links with a mark beside their text (an external link, a way
  // back), and a link and an action kept to one line in a row too narrow for them. For
  // TextLink.dom.test.ts.
  let { onforgot, disabled = false }: { onforgot: () => void; disabled?: boolean } = $props();
</script>

<p>Read the <TextLink href="https://example.com/guide" target="_blank" rel="noopener noreferrer">guide</TextLink> first.</p>
<p>Forgot your password? <TextLink onclick={onforgot} {disabled} data-kind="forgot">Reset it</TextLink></p>
<button type="button">After</button>
<nav aria-label="Path"><TextLink href="#library" tone="muted">Library</TextLink></nav>
<p>
  See the
  <TextLink href="https://example.com/wiki" target="_blank" rel="noopener noreferrer">
    wiki
    {#snippet end()}<ExternalLink />{/snippet}
  </TextLink>.
</p>
<p>
  <TextLink href="#back" tone="muted">
    {#snippet start()}<ChevronLeft />{/snippet}
    Back to the library
  </TextLink>
</p>
<div class="row">
  <TextLink href="#game" truncate>The Legend of Heroes: Trails in the Sky the 3rd</TextLink>
  <TextLink onclick={onforgot} truncate data-kind="author">A player with a very long display name</TextLink>
</div>
