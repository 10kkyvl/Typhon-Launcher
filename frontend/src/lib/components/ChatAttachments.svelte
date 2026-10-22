<script lang="ts">
  import { msg } from '../i18n';
  import type { Attachment } from '../services/messaging';
  import { safeImageUrl } from '../social/chatImage';

  let {
    attachments,
    onopen,
    onload,
  }: {
    attachments: Attachment[];
    onopen?: (urls: string[], position: number) => void;
    onload?: () => void;
  } = $props();

  let broken = $state<Record<string, boolean>>({});

  const urls = $derived(attachments.map((item) => safeImageUrl(item.url)).filter((url) => url !== ''));
</script>

{#if urls.length > 0}
  <div class="attachments" class:single={urls.length === 1}>
    {#each urls as url, position (position)}
      {#if broken[`${position}:${url.length}`]}
        <span class="broken">{msg('ui.imageUnavailable')}</span>
      {:else if onopen}
        <button class="tile" type="button" aria-label={msg('social.chatImageOpen')} onclick={() => onopen(urls, position)}>
          <img src={url} alt={msg('social.chatImageLabel')} loading="lazy" onload={() => onload?.()} onerror={() => (broken = { ...broken, [`${position}:${url.length}`]: true })} />
        </button>
      {:else}
        <span class="tile">
          <img src={url} alt={msg('social.chatImageLabel')} loading="lazy" onload={() => onload?.()} onerror={() => (broken = { ...broken, [`${position}:${url.length}`]: true })} />
        </span>
      {/if}
    {/each}
  </div>
{/if}

<style>
  .attachments { display: grid; grid-template-columns: repeat(2, 12rem); gap: 0.4rem; margin-top: 0.5rem; }
  .attachments.single { grid-template-columns: auto; }
  .tile { display: block; padding: 0; border: 0; background: transparent; line-height: 0; overflow: hidden; border-radius: var(--radius-sm); }
  button.tile { cursor: zoom-in; }
  .tile img { display: block; width: 12rem; height: 12rem; border-radius: var(--radius-sm); object-fit: cover; }
  .single .tile img { width: auto; height: auto; max-width: 24rem; max-height: 24rem; }
  .broken { display: grid; place-items: center; width: 12rem; height: 12rem; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-3); font-size: var(--font-xs); text-align: center; }
</style>
