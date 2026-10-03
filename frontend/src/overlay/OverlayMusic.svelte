<script lang="ts">
  import { Music, Pause, Play, SkipBack, SkipForward } from '@lucide/svelte';
  import IconButton from '../lib/components/IconButton.svelte';
  import { msg } from '../lib/i18n';
  import { currentMedia, mediaNext, mediaPrevious, mediaToggle, type MediaState } from '../lib/services/media';

  let { active }: { active: boolean } = $props();

  const POLL_MS = 2000;

  let media = $state<MediaState | null>(null);
  let failed = $state(false);
  let commandFailed = $state(false);

  async function refresh(): Promise<void> {
    try {
      media = await currentMedia();
      failed = false;
    } catch (err) {
      console.error('current media', err);
      failed = true;
    }
  }

  async function command(action: () => Promise<void>): Promise<void> {
    try {
      await action();
      commandFailed = false;
    } catch (err) {
      console.error('media command', err);
      commandFailed = true;
    }
    await refresh();
  }

  $effect(() => {
    if (!active) return;
    void refresh();
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  });

  const track = $derived(media?.active ? media.track : null);
  const line = $derived(track ? [track.artist, track.app].filter(Boolean).join(' · ') : '');
</script>

{#if failed || media?.supported}
  <section class="music" aria-label={msg('overlay.musicLabel')}>
    <span class="icon"><Music size="1.6rem" strokeWidth={1.8} /></span>
    <div class="info">
      {#if failed}
        <span class="muted">{msg('overlay.musicError')}</span>
      {:else if track}
        <strong title={track.title}>{track.title || msg('overlay.musicUntitled')}</strong>
        {#if line}<span title={line}>{line}</span>{/if}
        {#if commandFailed}<span class="error">{msg('overlay.musicCommandError')}</span>{/if}
      {:else}
        <span class="muted">{msg('overlay.musicIdle')}</span>
      {/if}
    </div>
    {#if track}
      <div class="controls">
        <IconButton label={msg('overlay.musicPrevious')} size="sm" disabled={!track.canPrev} onclick={() => command(mediaPrevious)}>
          <SkipBack size="1.6rem" strokeWidth={1.8} />
        </IconButton>
        <IconButton
          label={track.playing ? msg('overlay.musicPause') : msg('overlay.musicPlay')}
          size="sm"
          disabled={!track.canPlayPause}
          onclick={() => command(mediaToggle)}
        >
          {#if track.playing}
            <Pause size="1.6rem" strokeWidth={1.8} />
          {:else}
            <Play size="1.6rem" strokeWidth={1.8} />
          {/if}
        </IconButton>
        <IconButton label={msg('overlay.musicNext')} size="sm" disabled={!track.canNext} onclick={() => command(mediaNext)}>
          <SkipForward size="1.6rem" strokeWidth={1.8} />
        </IconButton>
      </div>
    {/if}
  </section>
{/if}

<style>
  .music {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0 var(--space-4) var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .icon {
    display: inline-flex;
    color: var(--text-3);
    flex-shrink: 0;
  }

  .info {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    gap: 0.1rem;
  }

  .info strong,
  .info span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .info strong {
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .info span {
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .info .error {
    color: var(--danger);
  }

  .controls {
    display: flex;
    flex-shrink: 0;
    gap: 0.2rem;
  }
</style>
