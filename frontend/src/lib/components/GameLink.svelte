<script lang="ts">
  import type { Snippet } from 'svelte';
  import { navigate } from '../stores/router';
  import TextLink from './TextLink.svelte';

  /**
   * A game's title or cover that opens the game's page (decision В13: where a game is only mentioned, a
   * download or a list row, its name and its picture lead to its page; no separate "game page" button).
   * A button, so the keyboard reaches it and Enter opens it, named by the game's title.
   *
   * Without `children` it is the title: the launcher's text link in the colour of the text around it
   * (`truncate` keeps it on one line). With `children` it is the cover holding that picture; `repeat`
   * when the title beside it already links to the page: the cover is then for the pointer only, out of
   * the Tab order and hidden from assistive tech, so the page is one stop and one name, not two. With no
   * `id` there is no page to open: the title is plain text and the cover the picture alone.
   */
  let {
    id,
    name,
    children,
    repeat = false,
    truncate = false,
    testid,
  }: {
    /** The game's id as the game route takes it (a library id, or the catalog id of a download's game). */
    id: string | null | undefined;
    /** The game's title: the link's text, or the cover's name. */
    name: string;
    /** The cover's picture. */
    children?: Snippet;
    repeat?: boolean;
    truncate?: boolean;
    testid?: string;
  } = $props();

  function open() {
    if (id) navigate('game', { id });
  }
</script>

{#if children}
  {#if id}
    <button
      type="button"
      class="cover"
      aria-label={repeat ? undefined : name}
      aria-hidden={repeat || undefined}
      tabindex={repeat ? -1 : undefined}
      data-testid={testid}
      onclick={open}>{@render children()}</button
    >
  {:else}
    {@render children()}
  {/if}
{:else if id}
  <TextLink tone="plain" {truncate} data-testid={testid} onclick={open}>{name}</TextLink>
{:else}
  {name}
{/if}

<style>
  .cover {
    display: block;
    width: 100%;
    padding: 0;
    border: 0;
    border-radius: inherit;
    background: none;
    color: inherit;
    cursor: pointer;
  }
</style>
