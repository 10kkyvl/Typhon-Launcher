<script lang="ts">
  import Avatar from './Avatar.svelte';
  import StyledName from './StyledName.svelte';
  import { playerCardOf } from '../profile/appearanceCard';
  import { presenceDot, presenceLine } from '../social/presence';
  import { wideArt } from '../social/art';
  import type { PresenceView, UserCard } from '../services/social';
  import { msg } from '../i18n';

  let { user, presence }: { user: UserCard; presence?: PresenceView | null } = $props();

  const view = $derived(playerCardOf(user.card));
  const name = $derived(user.displayName || user.username);
  const art = $derived(view.coverUrl || wideArt(view.pinned));
  const status = $derived(presence ? presenceDot(presence) : undefined);
  const line = $derived(presence ? presenceLine(presence) : '');
</script>

<div class="card" style={view.style} style:--card-bg={view.theme.background} style:--card-surface={view.theme.surface}>
  <div class="banner" style:background={view.theme.banner}>
    {#if art}<img src={art} alt="" loading="lazy" draggable="false" />{/if}
    <div class="tint" style:background={view.theme.banner}></div>
    <div class="fade"></div>
  </div>
  <div class="body">
    <div class="head">
      <span class="avatar"><Avatar size="md" {name} src={user.avatarUrl} {status} frame={view.appearance.avatarFrame} /></span>
      <div class="ident">
        <span class="name"><StyledName {name} styleName={view.appearance.nameStyle} /></span>
        <span class="handle">@{user.username}</span>
      </div>
    </div>
    {#if view.statusEmoji || view.statusText}
      <p class="status">{#if view.statusEmoji}<span>{view.statusEmoji}</span>{/if}{#if view.statusText}<span>{view.statusText}</span>{/if}</p>
    {/if}
    {#if line}<p class="presence">{line}</p>{/if}
    {#if view.pinned}
      <div class="pinned">
        {#if view.pinned.coverUrl}<img class="thumb" src={view.pinned.coverUrl} alt="" loading="lazy" draggable="false" />{/if}
        <span class="pinned-text">
          <span class="pinned-label">{msg('profileStyle.cardPinned')}</span>
          <span class="pinned-title">{view.pinned.title}</span>
        </span>
      </div>
    {/if}
  </div>
</div>

<style>
  .card {
    --text: #f1f4f8;
    --text-2: #bec7d2;
    --text-3: #99a5b5;
    --avatar-ring: var(--card-bg);
    width: 32rem;
    max-width: 100%;
    overflow: hidden;
    border: 1px solid rgba(200, 220, 240, 0.14);
    border-radius: var(--radius-lg);
    background: var(--card-bg);
    color: var(--text);
    text-align: left;
  }

  .banner {
    position: relative;
    height: 7.2rem;
    overflow: hidden;
  }

  .banner img,
  .tint,
  .fade {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }

  .banner img {
    object-fit: cover;
  }

  .tint {
    opacity: 0.55;
  }

  .fade {
    background: linear-gradient(transparent 40%, var(--card-bg));
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    padding: 0 1.4rem 1.4rem;
  }

  .head {
    display: flex;
    align-items: flex-end;
    gap: 1.2rem;
    margin-top: -2.6rem;
    min-width: 0;
  }

  .avatar {
    flex-shrink: 0;
    padding: 0.3rem;
    border-radius: 50%;
    background: var(--card-bg);
  }

  .ident {
    display: flex;
    flex-direction: column;
    min-width: 0;
    padding-bottom: 0.2rem;
  }

  .name {
    font-size: var(--font-lg);
    font-weight: 600;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .handle {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status,
  .presence {
    display: flex;
    gap: 0.6rem;
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .presence {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .pinned {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.8rem;
    border-radius: var(--radius-md);
    background: var(--card-surface);
  }

  .thumb {
    width: 3.2rem;
    height: 4.2rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    object-fit: cover;
  }

  .pinned-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .pinned-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .pinned-title {
    font-size: var(--font-sm);
    color: var(--accent-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
