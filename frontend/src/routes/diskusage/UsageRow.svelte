<script lang="ts">
  import type { Snippet } from 'svelte';
  import { TriangleAlert } from '@lucide/svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import { msg } from '../../lib/i18n';

  // One row of a section, an item of its list: what it is (a game of the library leads to its page),
  // whose game it is, where (a path shown whole, wrapping), why it cannot go, its marks, its size and
  // its actions.
  let {
    key,
    title,
    gameId,
    game,
    path,
    isPath = true,
    reason,
    reasonTone = 'plain',
    badges,
    played,
    size,
    actions,
  }: {
    /** The row's id within the page, for the focus rule: `<section>:<item>`. */
    key: string;
    title: string;
    /** The library game the title names: the title then leads to its page. */
    gameId?: string | null;
    /** The game a leftover belongs to, said as "Game: <name>"; the name leads to its page with an id. */
    game?: { id: string | null; name: string };
    /** The path, or for a group the line that counts its files (`isPath` false). */
    path: string;
    isPath?: boolean;
    /**
     * The line under the path: rows that can have one pass it, or null while there is none; rows that
     * never have one leave it out.
     */
    reason?: string | null;
    /** `warning`: the folder matters to a game (it may be its only copy); said with a mark and the tone. */
    reasonTone?: 'warning' | 'plain';
    badges?: Snippet;
    /** When the game was last played; only rows of games have it. */
    played?: string;
    size: string;
    actions?: Snippet;
  } = $props();

  // The words around the game's name in "Game: <name>", so the name alone is the link.
  const MARK = '\u0000';
  const around = $derived(game ? msg('diskusage.copyOf', { title: MARK }).split(MARK) : []);
</script>

<!-- Spaces between the parts are written out: a part that is absent leaves no space behind. -->
<li class="row" data-usage-row={key}
  ><div class="main"
    ><span class="title"><GameLink id={gameId} name={title} /></span>{#if game}{' '}<span class="game"
        >{around[0]}<GameLink id={game.id} name={game.name} />{around[1] ?? ''}</span
      >{/if}{' '}<span class="path" class:words={!isPath}>{path}</span
    >{#if reason !== undefined}{' '}{#if reason !== null}<span class="reason" data-tone={reasonTone}
          >{#if reasonTone === 'warning'}<TriangleAlert size="1.3rem" strokeWidth={2} aria-hidden="true" />{/if}{reason}</span
        >{/if}{/if}</div
  >{#if badges}{' '}<div class="badges">{@render badges()}</div>{/if}{#if played !== undefined}{' '}<span class="played"
      >{played}</span
    >{/if}{' '}<span class="size">{size}</span>{' '}<div class="actions">{@render actions?.()}</div></li
>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .main {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  /* A path is shown whole: it wraps at any character rather than being cut. */
  .path {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .path.words {
    font-size: var(--font-sm);
  }

  .game {
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .reason {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  /* On the row's own fill (.row), which status-surface.test.ts names as its surface. */
  .reason[data-tone='warning'] {
    color: var(--warning);
    font-weight: 600;
  }

  .badges {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .played {
    width: 12rem;
    flex-shrink: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .size {
    width: 10rem;
    flex-shrink: 0;
    text-align: right;
    font-size: var(--font-sm);
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    min-width: 12rem;
    flex-shrink: 0;
  }
</style>
