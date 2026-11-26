<script lang="ts">
  import { tick } from 'svelte';
  import { Calendar, EllipsisVertical, Gamepad2, LogIn } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import { frameLook } from '../../lib/profile/frameLook';
  import AvatarEditor from '../../lib/components/AvatarEditor.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import type { MenuItem } from '../../lib/components/menu';
  import IconButton from '../../lib/components/IconButton.svelte';
  import MaskedEmail from '../../lib/components/MaskedEmail.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StyledName from '../../lib/components/StyledName.svelte';
  import TextArea from '../../lib/components/TextArea.svelte';
  import TextField from '../../lib/components/TextField.svelte';
  import type { TextAreaCounter } from '../../lib/components/textArea';
  import { appearanceOf } from '../../lib/profile/appearance';
  import type { ProfileAppearance } from '../../lib/services/account';
  import { signOutPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { accountErrorField, accountErrorText } from '../../lib/services/accountMessages';
  import type { GameRef, ProfileStats as ProfileStatsData } from '../../lib/services/profile';
  import { joinDate } from '../../lib/social/view';
  import { statusDot } from '../../lib/social/presence';
  import { shownPresence } from '../../lib/stores/presence';
  import { authState, currentUser, isOffline, leaveGuest, saveProfile, savingProfile, signOut } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { msg } from '../../lib/i18n';
  import HiddenBadge from './HiddenBadge.svelte';
  import ProfileStats from './ProfileStats.svelte';

  let {
    running,
    stats,
    statsFailed = false,
    showOnline,
    showPlaying,
    showStats,
    onsettings,
    onedit,
    appearance,
    statusEmoji,
    statusText,
  }: {
    running: GameRef[];
    /** The figures, or null until the first read has answered them. */
    stats: ProfileStatsData | null;
    /** The read of the figures failed: the page says so in its notice, the header shows none. */
    statsFailed?: boolean;
    showOnline: boolean;
    showPlaying: boolean;
    showStats: boolean;
    onsettings: () => void;
    onedit: () => void;
    appearance: ProfileAppearance;
    statusEmoji: string;
    statusText: string;
  } = $props();

  const uid = $props.id();
  const look = $derived(appearanceOf(appearance));

  const BIO_LIMIT = 150;
  // Counted as the browser's maxlength counts, so the count and the limit it refuses at agree.
  const bioCounter: TextAreaCounter = {
    max: BIO_LIMIT,
    measure: (value) => value.length,
    text: (count, max) => msg('profile.bioCounter', { count, max }),
    limitText: (count, max) => {
      if (count > max) return msg('profile.bioOver', { count: count - max });
      if (count === max) return msg('profile.bioAt');
      return msg('profile.bioNear', { count: max - count });
    },
  };

  let editing = $state(false);
  let draft = $state({ displayName: '', username: '', bio: '' });
  let fieldErrors = $state<{ displayName?: string; username?: string; bio?: string; general?: string }>({});
  let busy = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let root = $state<HTMLElement>();
  let displayNameField = $state<HTMLInputElement>();
  let usernameField = $state<HTMLInputElement>();
  let bioField = $state<HTMLTextAreaElement>();
  // The editor is opened from the menu, which gives focus back to its trigger: the fields leaving hand
  // focus back there.
  const opener = () => root?.querySelector<HTMLElement>('[data-header-menu]');

  const isGuest = $derived($authState === 'guest');
  const avatarFrame = $derived(isGuest ? frameLook('none', '') : frameLook(look.avatarFrame, look.frameColor, $currentUser?.frames));

  const avatarName = $derived(
    !isGuest && $currentUser ? $currentUser.displayName || $currentUser.username : msg('social.guestName'),
  );

  const memberSince = $derived($currentUser ? joinDate($currentUser.createdAt) : '');
  const playing = $derived(running[0] ?? null);
  const playingHidden = $derived(!!playing && !showPlaying);

  const dirty = $derived(
    !!$currentUser &&
      (draft.displayName !== $currentUser.displayName ||
        draft.username !== $currentUser.username ||
        draft.bio !== $currentUser.bio),
  );

  const bio = $derived(!isGuest ? ($currentUser?.bio ?? '') : '');

  const menuItems = $derived<MenuItem[]>([
    { id: 'edit', label: msg('social.editProfileLabel') },
    { id: 'settings', label: msg('profile.privacy') },
    { id: 'signout', label: busy ? msg('social.signingOut') : msg('social.signOutLabel'), danger: true, separator: true },
  ]);

  function startEditing() {
    if (!$currentUser || $isOffline) return;
    draft = { displayName: $currentUser.displayName, username: $currentUser.username, bio: $currentUser.bio };
    fieldErrors = {};
    editing = true;
  }

  function close(handOff: ReturnType<typeof focusHandOff>) {
    fieldErrors = {};
    editing = false;
    void handOff(opener);
  }

  function cancelEditing() {
    close(focusHandOff());
  }

  // A refusal about one field is said there and that field takes focus; any other once, in the notice.
  async function refuse(err: unknown) {
    const message = accountErrorText(err, msg('social.saveFailed'));
    const field = accountErrorField(err);
    if (field === 'username') fieldErrors = { username: message };
    else if (field === 'displayName') fieldErrors = { displayName: message };
    else if (field === 'bio') fieldErrors = { bio: message };
    else fieldErrors = { general: message };
    await tick();
    if (fieldErrors.username) usernameField?.focus();
    else if (fieldErrors.displayName) displayNameField?.focus();
    else if (fieldErrors.bio) bioField?.focus();
  }

  async function save() {
    if (!$currentUser || !dirty || $savingProfile || $isOffline) return;
    const handOff = focusHandOff();
    fieldErrors = {};
    const patch: { displayName?: string; username?: string; bio?: string } = {};
    if (draft.displayName !== $currentUser.displayName) patch.displayName = draft.displayName;
    if (draft.username !== $currentUser.username) patch.username = draft.username;
    if (draft.bio !== $currentUser.bio) patch.bio = draft.bio;
    try {
      if (!(await saveProfile(patch))) {
        fieldErrors = { general: msg('profile.saveBusy') };
        return;
      }
      close(handOff);
      toast(msg('social.profileUpdated'), 'success');
    } catch (err) {
      await refuse(err);
    }
  }

  function onMenu(id: string) {
    if (id === 'edit') {
      if ($isOffline) toast(msg('social.editRequiresConnection'));
      else startEditing();
    } else if (id === 'settings') {
      onsettings();
    } else if (id === 'signout') {
      pending = { prompt: signOutPrompt(), run: () => run(signOut, msg('social.signOutFailed')) };
    }
  }

  async function run(fn: () => Promise<unknown>, fallback: string) {
    if (busy) return;
    busy = true;
    try {
      await fn();
    } catch (err) {
      toast(accountErrorText(err, fallback), 'danger');
    } finally {
      busy = false;
    }
  }
</script>

<section class="profile-header" bind:this={root}>
  <div class="header-surface">
    <div class="head">
      <Avatar
        size="lg"
        name={avatarName}
        src={isGuest ? undefined : $currentUser?.avatarUrl}
        status={isGuest ? undefined : statusDot($shownPresence)}
        {...avatarFrame}
      />

      <div class="identity">
        {#if isGuest}
          <h2 class="display-name">{msg('social.guestName')}</h2>
          <span class="username">{msg('social.guestProfileHint')}</span>
        {:else if $currentUser}
          <h2 class="display-name"><StyledName name={$currentUser.displayName} styleName={look.nameStyle} nameColor={look.nameColor} /></h2>
          <span class="username">@{$currentUser.username}</span>
          {#if statusEmoji || statusText}
            <p class="status">{#if statusEmoji}<span class="status-emoji">{statusEmoji}</span>{/if}{#if statusText}<span>{statusText}</span>{/if}</p>
          {/if}
          {#if bio}
            <p class="bio">{bio}</p>
          {/if}
          <div class="meta">
            {#if playing}
              <StatusBadge kind="accent" wrap label={msg('social.playingIn', { name: playing.title })}>
                {#snippet icon()}<Gamepad2 strokeWidth={1.8} />{/snippet}
              </StatusBadge>
              {#if playingHidden}<HiddenBadge text={msg('social.hiddenStatusHint')} />{/if}
            {/if}
            {#if memberSince}
              <span class="meta-item">
                <span class="meta-icon" aria-hidden="true"><Calendar size="1.5rem" strokeWidth={1.8} /></span>
                {msg('social.memberSince', { date: memberSince })}
              </span>
            {/if}
            {#if !showOnline}
              <HiddenBadge text={msg('social.onlineStatusHiddenHint')} />
            {/if}
          </div>
        {/if}
      </div>

      <div class="right">
        <div class="head-actions">
          {#if isGuest}
            <Button variant="primary" disabled={busy} onclick={() => run(() => leaveGuest('login'), msg('social.signInFailed'))}>
              <LogIn size="1.5rem" strokeWidth={1.8} />
              {msg('social.signInButton')}
            </Button>
            <Button disabled={busy} onclick={() => run(() => leaveGuest('register'), msg('social.signInFailed'))}>
              {msg('social.createAccountButton')}
            </Button>
          {:else}
            <Button size="sm" data-profile-edit onclick={onedit}>{msg('profileStyle.edit')}</Button>
            <AvatarEditor size="sm" disabled={$isOffline} />
            <DropdownMenu items={menuItems} onselect={onMenu}>
              {#snippet trigger({ props })}
                <IconButton {...props} data-header-menu label={msg('social.moreLabel')}>
                  <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          {/if}
        </div>
        {#if !isGuest && stats}
          <div class="stats">
            <ProfileStats {stats} hidden={!showStats} />
          </div>
        {:else if !isGuest && !statsFailed}
          <!-- The page's status says what is loading; this mark only holds the figures' place. -->
          <div class="stats"><Spinner size="sm" /></div>
        {/if}
      </div>
    </div>

    {#if editing && $currentUser}
      <div class="fields">
        {#if $isOffline}
          <Notice tone="warning" compact>{msg('profile.editOffline')}</Notice>
        {/if}
        <!-- While the save waits, and while the connection is gone, the fields refuse edits by being
             read-only: a field that turns disabled drops the focus it holds. -->
        <TextField
          id="{uid}-display-name"
          label={msg('social.displayNameLabel')}
          maxlength={32}
          readonly={$isOffline || $savingProfile}
          error={fieldErrors.displayName}
          bind:value={draft.displayName}
          bind:input={displayNameField}
        />
        <TextField
          id="{uid}-username"
          label={msg('social.usernameLabel')}
          maxlength={24}
          autocomplete="off"
          hint={msg('profile.usernameHint', { username: draft.username })}
          readonly={$isOffline || $savingProfile}
          error={fieldErrors.username}
          bind:value={draft.username}
          bind:input={usernameField}
        />
        <TextArea
          id="{uid}-bio"
          label={msg('social.aboutTitle')}
          rows={3}
          maxlength={BIO_LIMIT}
          counter={bioCounter}
          readonly={$isOffline || $savingProfile}
          error={fieldErrors.bio}
          bind:value={draft.bio}
          bind:textarea={bioField}
        />
        <div class="field">
          <span class="field-label">{msg('common.email')}</span>
          <MaskedEmail email={$currentUser.email} />
          <span class="hint">{msg('social.emailHint')}</span>
        </div>
        <div class="field">
          <span class="field-label">{msg('social.memberSinceLabel')}</span>
          <span class="hint">{memberSince}</span>
        </div>
      </div>
      <div class="foot">
        {#if fieldErrors.general}<div class="general"><Notice tone="danger" compact>{fieldErrors.general}</Notice></div>{/if}
        <Button variant="ghost" disabled={$savingProfile} onclick={cancelEditing}>{msg('common.cancel')}</Button>
        <!-- Busy, not disabled, while it waits: it keeps the focus it was pressed with. Offline it stays
             in the Tab order, refused, and says why. -->
        <Button
          variant="primary"
          busy={$savingProfile}
          disabled={!$savingProfile && (!dirty || $isOffline)}
          reason={$isOffline ? msg('social.editRequiresConnection') : ''}
          onclick={save}
        >
          {$savingProfile ? msg('social.saving') : msg('common.save')}
        </Button>
      </div>
    {/if}
  </div>
</section>

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .header-surface { padding: 0 0 var(--space-5); position: relative; }
  .profile-header {
    container-type: inline-size;
    display: block;
    margin-bottom: var(--space-6);
  }

  .head {
    display: flex;
    align-items: flex-end;
    gap: var(--space-5);
  }

  .identity {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .display-name {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
    overflow-wrap: anywhere;
  }

  .username {
    font-size: var(--font-md);
    color: var(--text-3);
  }

  .status {
    display: flex;
    align-items: baseline;
    gap: var(--space-1);
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .bio {
    font-size: var(--font-sm);
    color: var(--text-2);
    max-width: 56rem;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-4);
    margin-top: var(--space-1);
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .meta-icon {
    display: inline-flex;
    color: var(--text-3);
    flex-shrink: 0;
  }

  .right {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-4);
    flex-shrink: 0;
  }

  .head-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 44rem;
    margin-top: var(--space-5);
    padding-top: var(--space-5);
    border-top: 1px solid var(--border);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .field-label {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .foot {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-5);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }

  .general {
    flex: 1;
    min-width: 0;
    margin-right: auto;
  }

  @container (max-width: 900px) {
    .head {
      flex-wrap: wrap;
    }

    .right {
      align-items: center;
      flex-direction: row;
      justify-content: space-between;
      width: 100%;
      flex-wrap: wrap;
    }
  }
</style>
