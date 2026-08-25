<script lang="ts">
  import { untrack } from 'svelte';
  import { inWails } from '../services/backend';
  import { getLegalDocument, type LegalDocument } from '../services/legal';
  import Modal from './Modal.svelte';

  type Inline = { kind: 'text' | 'bold' | 'code'; value: string };
  type Block =
    | { kind: 'h1' | 'h2' | 'h3'; inline: Inline[] }
    | { kind: 'p'; inline: Inline[] }
    | { kind: 'quote'; lines: Inline[][] }
    | { kind: 'ul' | 'ol'; items: Inline[][] }
    | { kind: 'pre'; text: string };

  const INLINE_RE = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\([^)]+\)/g;

  function parseInline(text: string): Inline[] {
    const tokens: Inline[] = [];
    let last = 0;
    let match: RegExpExecArray | null;
    INLINE_RE.lastIndex = 0;
    while ((match = INLINE_RE.exec(text))) {
      if (match.index > last) tokens.push({ kind: 'text', value: text.slice(last, match.index) });
      if (match[1] !== undefined) tokens.push({ kind: 'bold', value: match[1] });
      else if (match[2] !== undefined) tokens.push({ kind: 'code', value: match[2] });
      else if (match[3] !== undefined) tokens.push({ kind: 'text', value: match[3] });
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

  const blocks = $derived(doc ? parseMarkdown(doc.body) : []);

  $effect(() => {
    const isOpen = open;
    const id = documentId;
    untrack(() => {
      if (!isOpen || !id) return;
      load(id);
    });
  });

  async function load(id: string) {
    status = 'loading';
    errorText = '';
    doc = null;
    try {
      doc = await getLegalDocument(id);
      status = 'ready';
    } catch {
      errorText = inWails
        ? 'Не удалось загрузить документ. Попробуйте ещё раз.'
        : 'Правовые документы недоступны вне приложения.';
      status = 'error';
    }
  }
</script>

{#snippet inlineText(tokens: Inline[])}
  {#each tokens as t}{#if t.kind === 'bold'}<strong>{t.value}</strong>{:else if t.kind === 'code'}<code
        >{t.value}</code
      >{:else}{t.value}{/if}{/each}
{/snippet}

<Modal bind:open {title} width="64rem">
  {#if status === 'loading' || status === 'idle'}
    <p class="state-text">Загрузка документа…</p>
  {:else if status === 'error'}
    <p class="state-text error">{errorText}</p>
  {:else}
    <div class="doc">
      {#each blocks as block}
        {#if block.kind === 'h1'}
          <h2 class="md-h1">{@render inlineText(block.inline)}</h2>
        {:else if block.kind === 'h2'}
          <h3 class="md-h2">{@render inlineText(block.inline)}</h3>
        {:else if block.kind === 'h3'}
          <h4 class="md-h3">{@render inlineText(block.inline)}</h4>
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
  .state-text {
    font-size: var(--font-md);
    color: var(--text-2);
  }

  .state-text.error {
    color: var(--danger);
  }

  .doc {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: var(--prose-max);
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
    font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
    font-size: var(--font-xs);
    line-height: 1.5;
    white-space: pre;
    color: var(--text-2);
  }

  .md-p :global(code),
  .md-quote :global(code),
  .md-list :global(code) {
    font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
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
