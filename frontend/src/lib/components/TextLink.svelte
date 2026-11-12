<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes, HTMLAttributes } from 'svelte/elements';

  /**
   * An action that reads as a link inside a sentence or a list ("Forgot your password?", "Show all", a
   * breadcrumb, "Open folder"). Given `href` it is a real link (`<a href>`): it goes somewhere, and the
   * browser offers what it offers for links. Given `onclick` instead it is a `<button>`: it does
   * something here, and only a button can be disabled. It keeps the font and the line height of the
   * text around it, so a line holding it is as high as one without; it is underlined, so it is told
   * from the text by more than its colour, and it shows the focus ring every control has. `tone`:
   * `accent` in running text; `muted` where the link is part of the furniture and must not pull the
   * eye (a breadcrumb, a path). Other attributes (target, rel, aria-*, data-*) go to the element.
   */
  type Shared = Omit<HTMLAttributes<HTMLElement>, 'class' | 'children' | 'title' | 'onclick'> & {
    children: Snippet;
    tone?: 'accent' | 'muted';
  };
  type AsLink = Shared & {
    href: string;
    target?: HTMLAnchorAttributes['target'];
    rel?: HTMLAnchorAttributes['rel'];
    onclick?: (e: MouseEvent) => void;
    disabled?: undefined;
  };
  type AsButton = Shared & {
    href?: undefined;
    target?: undefined;
    rel?: undefined;
    onclick: (e: MouseEvent) => void;
    disabled?: boolean;
  };

  let { href, disabled = false, tone = 'accent', children, ...rest }: AsLink | AsButton = $props();
</script>

{#if href !== undefined}
  <a {...rest} class="text-link" class:muted={tone === 'muted'} {href}>{@render children()}</a>
{:else}
  <button {...rest} class="text-link" class:muted={tone === 'muted'} type="button" {disabled}>{@render children()}</button>
{/if}

<style>
  .text-link {
    display: inline;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    line-height: inherit;
    vertical-align: baseline;
    text-align: inherit;
    color: var(--accent-text);
    text-decoration-line: underline;
    text-decoration-thickness: var(--link-underline);
    text-decoration-color: color-mix(in srgb, currentColor 45%, transparent);
    text-underline-offset: var(--link-underline-offset);
    border-radius: var(--radius-xs);
    cursor: pointer;
    transition:
      color var(--dur) var(--ease),
      text-decoration-color var(--dur) var(--ease);
  }

  .text-link.muted {
    color: var(--text-2);
  }

  .text-link:hover:not(:disabled) {
    color: var(--text);
    text-decoration-color: currentColor;
  }

  .text-link:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>
