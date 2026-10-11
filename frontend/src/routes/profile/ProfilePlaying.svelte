<script lang="ts">
  import { ChevronRight } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { msg } from '../../lib/i18n';
  import HiddenBadge from './HiddenBadge.svelte';

  let {
    title,
    art,
    hidden = false,
    disabled = false,
    onopen,
  }: {
    title: string;
    art: string;
    hidden?: boolean;
    disabled?: boolean;
    onopen: () => void;
  } = $props();
</script>

<section class="playing" aria-label={msg('social.nowPlayingTitle')}>
  <button class="open" type="button" {disabled} onclick={onopen}>
    <span class="cover">
      <Artwork src={art} alt="" label={title} ratio="16 / 9" radius="var(--radius-md)" />
    </span>
    <span class="text">
      <span class="eyebrow">{msg('social.nowPlayingTitle')}</span>
      <span class="title">{title}</span>
      <StatusBadge kind="success" label={msg('social.playing')} plain />
    </span>
    {#if !disabled}
      <span class="go"><ChevronRight size="2rem" strokeWidth={1.8} /></span>
    {/if}
  </button>
  {#if hidden}
    <span class="flag"><HiddenBadge text={msg('social.hiddenGenericHint')} /></span>
  {/if}
</section>

<style>
  .playing {
    position: relative;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background:
      linear-gradient(100deg, var(--accent-subtle), transparent 55%),
      var(--surface-2);
    transition: border-color var(--dur) var(--ease);
  }

  .playing:hover,
  .playing:focus-within {
    border-color: var(--border-strong);
  }

  .open {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    width: 100%;
    padding: var(--space-4);
    border-radius: var(--radius-lg);
    color: inherit;
    text-align: left;
  }

  .open:disabled {
    cursor: default;
  }

  .cover {
    display: block;
    flex-shrink: 0;
    width: 19rem;
    border-radius: var(--radius-md);
    overflow: hidden;
  }

  .cover :global(img) {
    transition: transform var(--dur-slow) var(--ease);
  }

  .open:hover:not(:disabled) .cover :global(img) {
    transform: scale(1.04);
  }

  .text {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.6rem;
    flex: 1;
    min-width: 0;
  }

  .eyebrow {
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--text-3);
  }

  .title {
    max-width: 100%;
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.2;
    overflow-wrap: anywhere;
  }

  .go {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--text-3);
    transition:
      transform var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .open:hover .go {
    transform: translateX(0.3rem);
    color: var(--text);
  }

  .flag {
    position: absolute;
    top: var(--space-4);
    right: var(--space-4);
  }
</style>
