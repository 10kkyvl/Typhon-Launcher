<script lang="ts">
  let {
    src,
    name,
    size = 'md',
    status,
  }: {
    src?: string;
    name: string;
    size?: 'sm' | 'md' | 'lg';
    status?: 'online' | 'away' | 'busy' | 'offline';
  } = $props();

  let failed = $state(false);

  $effect(() => {
    src;
    failed = false;
  });

  const initial = $derived(name.trim().slice(0, 1).toUpperCase() || '?');
</script>

<span class="avatar {size}">
  {#if !src || failed}
    <span class="fallback">{initial}</span>
  {:else}
    <img {src} alt="" draggable="false" onerror={() => (failed = true)} />
  {/if}
  {#if status}
    <span class="dot {status}"></span>
  {/if}
</span>

<style>
  .avatar {
    position: relative;
    flex-shrink: 0;
    display: block;
  }

  .avatar.sm {
    width: 3.2rem;
    height: 3.2rem;
  }

  .avatar.md {
    width: 4.8rem;
    height: 4.8rem;
  }

  .avatar.lg {
    width: 9.6rem;
    height: 9.6rem;
  }

  img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
  }

  .fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-weight: 600;
  }

  .sm .fallback {
    font-size: var(--font-sm);
  }

  .md .fallback {
    font-size: var(--font-lg);
  }

  .lg .fallback {
    font-size: 3.6rem;
  }

  .dot {
    position: absolute;
    right: -1px;
    bottom: -1px;
    border-radius: 50%;
    border: 2px solid var(--avatar-ring, var(--bg));
  }

  .sm .dot {
    width: 0.9rem;
    height: 0.9rem;
  }

  .md .dot {
    width: 1.2rem;
    height: 1.2rem;
  }

  .lg .dot {
    width: 2rem;
    height: 2rem;
    border-width: 3px;
  }

  .dot.online {
    background: var(--success);
  }

  .dot.away {
    background: var(--warning);
  }

  .dot.busy {
    background: var(--danger);
  }

  .dot.offline {
    background: var(--text-3);
  }
</style>
