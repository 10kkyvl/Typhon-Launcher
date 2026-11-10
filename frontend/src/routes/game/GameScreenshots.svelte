<script lang="ts">
  import { FolderOpen, Trash2 } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Lightbox from '../../lib/components/Lightbox.svelte';
  import { deleteScreenshotPrompt } from '../../lib/confirm/prompts';
  import {
    DEFAULT_SCREENSHOT_HOTKEY,
    deleteShot,
    listShots,
    openShotsFolder,
    screenshotsStatus,
    type Shot,
  } from '../../lib/services/screenshots';
  import { lastSavedShot, screenshotErrorText } from '../../lib/stores/screenshots';
  import { toast } from '../../lib/stores/toasts';
  import { dateTime } from '../../lib/utils/format';
  import { errorCode, msg } from '../../lib/i18n';

  let { gameId, title }: { gameId: string; title: string } = $props();

  let shots = $state<Shot[]>([]);
  let loading = $state(true);
  let loadFailed = $state(false);
  let noLibrary = $state(false);
  let enabled = $state(true);
  let statusKnown = $state(false);
  let hotkey = $state<string>(DEFAULT_SCREENSHOT_HOTKEY);
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let pending = $state<{ shot: Shot; run: () => Promise<void> } | null>(null);
  let requestId = 0;

  async function load() {
    const id = ++requestId;
    const target = gameId;
    try {
      const [list, status] = await Promise.allSettled([listShots(target), screenshotsStatus()]);
      if (id !== requestId) return;
      if (status.status === 'fulfilled') {
        enabled = status.value.enabled;
        hotkey = status.value.hotkey || DEFAULT_SCREENSHOT_HOTKEY;
        statusKnown = true;
      } else {
        statusKnown = false;
        toast(screenshotErrorText(status.reason, msg('screenshots.loadError')), 'danger');
      }
      if (list.status === 'rejected') throw list.reason;
      shots = list.value;
      noLibrary = false;
      loadFailed = false;
    } catch (err) {
      if (id !== requestId) return;
      shots = [];
      if (errorCode(err) === 'screenshots.no_library') {
        noLibrary = true;
        loadFailed = false;
      } else {
        noLibrary = false;
        loadFailed = true;
        toast(screenshotErrorText(err, msg('screenshots.loadError')), 'danger');
      }
    } finally {
      if (id === requestId) loading = false;
    }
  }

  $effect(() => {
    void gameId;
    loading = true;
    void load();
  });

  $effect(() => {
    const saved = $lastSavedShot;
    if (saved && saved.gameId === gameId) void load();
  });

  async function openFolder() {
    try {
      await openShotsFolder(gameId);
    } catch (err) {
      toast(screenshotErrorText(err), 'danger');
    }
  }

  function open(index: number) {
    lightboxIndex = index;
    lightboxOpen = true;
  }

  function askDelete(shot: Shot) {
    pending = {
      shot,
      run: async () => {
        try {
          await deleteShot(gameId, shot.name);
          toast(msg('screenshots.deleted'), 'success');
        } catch (err) {
          toast(screenshotErrorText(err), 'danger');
        }
        await load();
      },
    };
  }

  function caption(shot: Shot): string {
    const at = new Date(shot.at);
    return Number.isNaN(at.getTime()) ? shot.name : dateTime(at);
  }
</script>

<section class="mine">
  <div class="head">
    <div class="titles">
      <h2>{msg('screenshots.sectionTitle')}</h2>
      {#if !noLibrary && enabled && statusKnown}
        <p class="hint">{msg('screenshots.hotkeyHint', { hotkey })}</p>
      {/if}
    </div>
    {#if !noLibrary}
      <Button onclick={openFolder}>
        <FolderOpen size="1.5rem" strokeWidth={1.8} />
        {msg('screenshots.openFolder')}
      </Button>
    {/if}
  </div>

  {#if loading}
    <p class="note" role="status">{msg('screenshots.loading')}</p>
  {:else if noLibrary}
    <p class="note">{msg('screenshots.noLibrary')}</p>
  {:else if loadFailed}
    <p class="note danger" role="alert">{msg('screenshots.loadError')}</p>
  {:else if shots.length === 0}
    <p class="note">{!statusKnown ? msg('screenshots.emptyNoHotkey') : enabled ? msg('screenshots.empty', { hotkey }) : msg('screenshots.disabledNote')}</p>
  {:else}
    <div class="grid">
      {#each shots as shot, index (shot.name)}
        <div class="tile">
          <button type="button" class="thumb" onclick={() => open(index)} aria-label={msg('screenshots.open', { name: caption(shot) })}>
            <img src={shot.url} alt="" loading="lazy" draggable="false" />
          </button>
          <span class="when">{caption(shot)}</span>
          <span class="remove">
            <IconButton label={msg('screenshots.delete')} size="sm" onclick={() => askDelete(shot)}>
              <Trash2 size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </span>
        </div>
      {/each}
    </div>
  {/if}
</section>

<Lightbox
  bind:open={lightboxOpen}
  bind:index={lightboxIndex}
  items={shots.map((shot) => ({ id: shot.name, url: shot.url }))}
  label={title}
/>

{#if pending}
  <ConfirmModal prompt={deleteScreenshotPrompt(pending.shot.name)} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .mine {
    margin-bottom: var(--space-8);
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    flex-wrap: wrap;
    margin-bottom: var(--space-4);
  }

  h2 {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .hint {
    margin-top: 0.4rem;
    font-size: 1.2rem;
    color: var(--text-3);
  }

  .note {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .note.danger {
    color: var(--danger);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(24rem, 1fr));
    gap: var(--space-3);
  }

  .tile {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .thumb {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    overflow: hidden;
    transition:
      transform var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }

  .thumb:hover {
    transform: translateY(-2px);
    opacity: 0.92;
  }

  .thumb img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .when {
    font-size: 1.2rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .remove {
    position: absolute;
    top: var(--space-2);
    right: var(--space-2);
    display: flex;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
  }

  .tile:hover .remove,
  .tile:focus-within .remove {
    opacity: 1;
  }
</style>
