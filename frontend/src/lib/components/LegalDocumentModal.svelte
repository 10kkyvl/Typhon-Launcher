<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { inWails } from '../services/backend';
  import { getLegalDocument, type LegalDocument } from '../services/legal';
  import { createLatest } from '../services/latest';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import TextLink from './TextLink.svelte';
  import { msg } from '../i18n';

  /**
   * A legal document, read from the app and drawn from its Markdown. Its headings sit under the
   * dialog's title in the outline; a link to a web address opens outside, as every link of the app
   * does (`target="_blank"`, which Wails hands to the system browser). Once the document shows, the
   * focus goes to it, a region the keys scroll, unless the player has moved it meanwhile. A failed read
   * is said with Retry in the app, and as unavailable outside it; an empty document in its own words.
   */
  type Inline = { kind: 'text' | 'bold' | 'code'; value: string } | { kind: 'link'; value: string; href: string };
  type Block =
    | { kind: 'h1' | 'h2' | 'h3'; inline: Inline[] }
    | { kind: 'p'; inline: Inline[] }
    | { kind: 'quote'; lines: Inline[][] }
    | { kind: 'ul' | 'ol'; items: Inline[][] }
    | { kind: 'pre'; text: string };

  const INLINE_RE = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)]+)\)/g;
  // Only a web address is a link: the documents are the app's own, but a link is never a script.
  const WEB = /^https?:\/\//i;

  function parseInline(text: string): Inline[] {
    const tokens: Inline[] = [];
    let last = 0;
    let match: RegExpExecArray | null;
    INLINE_RE.lastIndex = 0;
    while ((match = INLINE_RE.exec(text))) {
      if (match.index > last) tokens.push({ kind: 'text', value: text.slice(last, match.index) });
      if (match[1] !== undefined) tokens.push({ kind: 'bold', value: match[1] });
      else if (match[2] !== undefined) tokens.push({ kind: 'code', value: match[2] });
      else if (match[3] !== undefined) tokens.push(WEB.test(match[4]) ? { kind: 'link', value: match[3], href: match[4] } : { kind: 'text', value: match[3] });
      last = INLINE_RE.lastIndex;
    }
    if (last < text.length) tokens.push({ kind: 'text', value: text.slice(last) });
    return tokens;
  }

  const HEADING_RE = /^(#{1,3})\s+(.*)$/;
  const UL_RE = /^[-*]\s+/;
  const OL_RE = /^\d+\.\s+/;

  function parseMarkdown(body: string): Block[] {
    const lines = body.replace(/\r\n/g, '\n').split('\n');
    const blocks: Block[] = [];
    let i = 0;
    while (i < lines.length) {
      const line = lines[i];
      if (line.trim() === '') {
        i++;
        continue;
      }
      if (line.trimStart().startsWith('<!--')) {
        while (i < lines.length && !lines[i].includes('-->')) {
          i++;
        }
        i++;
        continue;
      }
      if (line.startsWith('```')) {
        const collected: string[] = [];
        i++;
        while (i < lines.length && !lines[i].startsWith('```')) {
          collected.push(lines[i]);
          i++;
        }
        i++;
        blocks.push({ kind: 'pre', text: collected.join('\n') });
        continue;
      }
      if (line.trimStart().startsWith('|')) {
        const collected: string[] = [];
        while (i < lines.length && lines[i].trimStart().startsWith('|')) {
          collected.push(lines[i]);
          i++;
        }
        blocks.push({ kind: 'pre', text: collected.join('\n') });
        continue;
      }
      const heading = HEADING_RE.exec(line);
      if (heading) {
        const level = heading[1].length;
        blocks.push({ kind: level === 1 ? 'h1' : level === 2 ? 'h2' : 'h3', inline: parseInline(heading[2]) });
        i++;
        continue;
      }
      if (line.startsWith('>')) {
        const collected: Inline[][] = [];
        while (i < lines.length && lines[i].startsWith('>')) {
          collected.push(parseInline(lines[i].replace(/^>\s?/, '')));
          i++;
        }
        blocks.push({ kind: 'quote', lines: collected });
        continue;
      }
      if (UL_RE.test(line)) {
        const items: Inline[][] = [];
        while (i < lines.length && UL_RE.test(lines[i])) {
          items.push(parseInline(lines[i].replace(UL_RE, '')));
          i++;
        }
        blocks.push({ kind: 'ul', items });
        continue;
      }
      if (OL_RE.test(line)) {
        const items: Inline[][] = [];
        while (i < lines.length && OL_RE.test(lines[i])) {
          items.push(parseInline(lines[i].replace(OL_RE, '')));
          i++;
        }
        blocks.push({ kind: 'ol', items });
        continue;
      }
      const collected: string[] = [];
      while (
        i < lines.length &&
        lines[i].trim() !== '' &&
        !lines[i].startsWith('```') &&
        !lines[i].trimStart().startsWith('|') &&
        !HEADING_RE.test(lines[i]) &&
        !lines[i].startsWith('>') &&
        !UL_RE.test(lines[i]) &&
        !OL_RE.test(lines[i])
      ) {
        collected.push(lines[i]);
        i++;
      }
      blocks.push({ kind: 'p', inline: parseInline(collected.join(' ')) });
    }
    return blocks;
  }

  let {
    open = $bindable(false),
    documentId,
    title,
  }: {
    open?: boolean;
    documentId: string | null;
    title: string;
  } = $props();

  let status = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let doc = $state<LegalDocument | null>(null);
  let errorText = $state('');
  let retrying = $state(false);
  let region = $state<HTMLElement>();

  const blocks = $derived(doc ? parseMarkdown(doc.body) : []);

  $effect(() => {
    const isOpen = open;
    const id = documentId;
    untrack(() => {
      if (!isOpen || !id) return;
      void load(id);
    });
  });

  // An earlier opening's answer, landing last, must not replace the document of this one.
  const latest = createLatest();

  // True when the document came, false when it failed; undefined when a newer read has taken over.
  async function fetchDoc(id: string): Promise<boolean | undefined> {
    const isCurrent = latest.start();
    try {
      const next = await getLegalDocument(id);
      if (!isCurrent()) return undefined;
      doc = next;
      errorText = '';
      status = 'ready';
      return true;
    } catch (err) {
      if (!isCurrent()) return undefined;
      console.error('legal document', err);
      errorText = inWails ? msg('modals.legalLoadFailed') : msg('modals.legalUnavailableOutsideApp');
      status = 'error';
      return false;
    }
  }

  async function load(id: string) {
    status = 'loading';
    errorText = '';
    retrying = false;
    doc = null;
    if (!(await fetchDoc(id))) return;
    // The dialog took the focus on its own while the document loaded: it goes to the document. A
    // control the player moved to meanwhile keeps it.
    await tick();
    const active = document.activeElement;
    if (!region || region.closest('[role="dialog"]')?.contains(active)) return;
    region.focus();
  }

  const retry = $derived(
    inWails && documentId ? { label: msg('common.retry'), run: () => fetchDoc(documentId as string), focus: () => region } : undefined,
  );
</script>

{#snippet inlineText(tokens: Inline[])}
  {#each tokens as t}{#if t.kind === 'bold'}<strong>{t.value}</strong>{:else if t.kind === 'code'}<code
        >{t.value}</code
      >{:else if t.kind === 'link'}<TextLink href={t.href} target="_blank" rel="noopener noreferrer">{t.value}</TextLink
      >{:else}{t.value}{/if}{/each}
{/snippet}

<Modal bind:open {title} width="64rem">
  {#if status === 'loading' || status === 'idle'}
    <Spinner label={msg('modals.legalLoading')} />
  {:else if status === 'error' || retrying}
    <Notice tone="danger" compact bind:retrying retry={retry}>{errorText}</Notice>
  {:else if blocks.length === 0}
    <p class="md-p">{msg('modals.legalEmpty')}</p>
  {:else}
    <!-- The dialog's title is an h3: the document's own headings come under it. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (A stop in the Tab order so the keys scroll the document.) -->
    <div class="doc" role="document" aria-label={title} tabindex="0" bind:this={region}>
      {#each blocks as block}
        {#if block.kind === 'h1'}
          <h4 class="md-h1">{@render inlineText(block.inline)}</h4>
        {:else if block.kind === 'h2'}
          <h5 class="md-h2">{@render inlineText(block.inline)}</h5>
        {:else if block.kind === 'h3'}
          <h6 class="md-h3">{@render inlineText(block.inline)}</h6>
        {:else if block.kind === 'p'}
          <p class="md-p">{@render inlineText(block.inline)}</p>
        {:else if block.kind === 'quote'}
          <blockquote class="md-quote">
            {#each block.lines as line}
              <p>{@render inlineText(line)}</p>
            {/each}
          </blockquote>
        {:else if block.kind === 'ul'}
          <ul class="md-list">
            {#each block.items as item}
              <li>{@render inlineText(item)}</li>
            {/each}
          </ul>
        {:else if block.kind === 'ol'}
          <ol class="md-list">
            {#each block.items as item}
              <li>{@render inlineText(item)}</li>
            {/each}
          </ol>
        {:else if block.kind === 'pre'}
          <pre class="md-pre">{block.text}</pre>
        {/if}
      {/each}
    </div>
  {/if}
</Modal>

<style>

  .doc {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: var(--prose-max);
    border-radius: var(--radius-xs);
  }

  .md-h1 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-top: var(--space-3);
  }

  .md-h2 {
    font-size: var(--font-lg);
    font-weight: 600;
    margin-top: var(--space-3);
  }

  .md-h3 {
    font-size: var(--font-md);
    font-weight: 600;
    margin-top: var(--space-2);
  }

  .md-p {
    font-size: var(--font-sm);
    line-height: 1.6;
    color: var(--text-2);
  }

  .md-quote {
    padding: var(--space-2) var(--space-4);
    border-left: 2px solid var(--border-strong);
    color: var(--text-3);
  }

  .md-quote p {
    font-size: var(--font-sm);
    line-height: 1.6;
  }

  .md-list {
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: var(--font-sm);
    line-height: 1.6;
    color: var(--text-2);
  }

  .md-pre {
    max-width: 100%;
    overflow-x: auto;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    font-size: var(--font-xs);
    line-height: 1.5;
    white-space: pre;
    color: var(--text-2);
  }

  .md-p :global(code),
  .md-quote :global(code),
  .md-list :global(code) {
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    font-size: 0.92em;
    padding: 0.1em 0.4em;
    border-radius: var(--radius-xs);
    background: var(--surface-3);
  }

  .md-p :global(strong),
  .md-quote :global(strong),
  .md-list :global(strong) {
    font-weight: 600;
    color: var(--text);
  }
</style>
