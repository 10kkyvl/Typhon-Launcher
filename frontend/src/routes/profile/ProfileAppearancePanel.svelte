<script lang="ts">
  import '../../styles/zoom-slider.css';
  import ColorPalette from '../../lib/components/ColorPalette.svelte';
  import HexColorField from '../../lib/components/HexColorField.svelte';
  import { validAccent } from '../../lib/theme/accent';
  import BannerCropModal from '../../lib/components/BannerCropModal.svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import StyledName from '../../lib/components/StyledName.svelte';
  import { Upload, RotateCcw, Smile, X } from '@lucide/svelte';
  import { onDestroy } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import { removeCoverPrompt, resetProfileAppearancePrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import Toggle from '../../lib/components/Toggle.svelte';
  import {
    AUTO_SOURCES, AVATAR_FRAMES, MAX_STATUS_TEXT, NAME_STYLES,
    type ProfileAppearance, type ProfileSettings,
  } from '../../lib/services/account';
  import { uploadProfileCover } from '../../lib/services/profileCover';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import {
    appearanceAccentStyle, appearanceOf, AUTO_THEME, CUSTOM_THEME, customBanner, DEFAULT_APPEARANCE,
    PROFILE_ACCENTS, PROFILE_THEMES, themeOf,
  } from '../../lib/profile/appearance';
  import { artStatus } from '../../lib/profile/artPalette';
  import { STATUS_EMOJI } from '../../lib/profile/appearanceEmoji';
  import { currentUser } from '../../lib/stores/user';
  import { msg } from '../../lib/i18n';

  let { draft = $bindable(), disabled }: { draft: ProfileSettings; disabled: boolean } = $props();

  const COVER_TYPES = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'];

  const a = $derived(appearanceOf(draft.appearance));
  const previewName = $derived($currentUser?.displayName || $currentUser?.username || msg('social.guestName'));
  const accentStyle = $derived(
    appearanceAccentStyle(a).split(';').filter((rule) => rule.startsWith('--accent')).join(';'),
  );
  const statusLength = $derived([...(draft.statusText ?? '')].length);

  let accentText = $state('');
  let fromText = $state('');
  let toText = $state('');
  $effect(() => { accentText = a.accent; });
  $effect(() => { fromText = a.customFrom; });
  $effect(() => { toText = a.customTo; });
  let palette = $state<'accent' | 'from' | 'to' | null>(null);
  let emojiOpen = $state(false);
  let input: HTMLInputElement | undefined = $state();
  let cropOpen = $state(false);
  let cropSource = $state('');
  let originalSource = $state('');
  let reading = $state(false);
  let uploading = $state(false);
  let error = $state('');
  let alive = true;
  let sequence = 0;
  let pending = $state<{ prompt: ConfirmPrompt; run: () => void } | null>(null);
  const locked = $derived(disabled || reading || uploading || cropOpen);
  onDestroy(() => { alive = false; sequence++; });

  function patch(changes: Partial<ProfileAppearance>) {
    draft.appearance = { ...a, ...changes };
  }
  function setColor(field: 'accent' | 'customFrom' | 'customTo', value: string) {
    if (field === 'accent') accentText = value;
    else if (field === 'customFrom') fromText = value;
    else toText = value;
    if (validAccent(value)) patch({ [field]: value });
  }
  function togglePalette(kind: 'accent' | 'from' | 'to') {
    palette = palette === kind ? null : kind;
  }
  function setStatusText(event: Event) {
    const field = event.currentTarget as HTMLInputElement;
    const text = [...field.value.replace(/\p{Cc}/gu, '')].slice(0, MAX_STATUS_TEXT).join('');
    field.value = text;
    draft.statusText = text;
  }

  async function choose(file?: File) {
    if (!file) return;
    const seq = ++sequence;
    error = '';
    if (!COVER_TYPES.includes(file.type) || file.size > 8 * 1024 * 1024 || file.size === 0) {
      error = msg('profileStyle.coverInvalid'); return;
    }
    reading = true;
    try {
      const url = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result));
        reader.onerror = reject;
        reader.readAsDataURL(file);
      });
      await new Promise<void>((resolve, reject) => {
        const img = new Image();
        img.onload = () => img.naturalWidth > 0 && img.naturalHeight > 0 && img.naturalWidth <= 8000 && img.naturalHeight <= 8000 && img.naturalWidth * img.naturalHeight <= 16_000_000 ? resolve() : reject();
        img.onerror = reject; img.src = url;
      });
      if (!alive || seq !== sequence) return;
      if (file.type === 'image/gif') {
        originalSource = '';
        reading = false;
        await upload(url, seq);
        return;
      }
      cropSource = url;
      cropOpen = true;
    } catch {
      if (alive && seq === sequence) error = msg('profile.coverDecode');
    } finally { if (alive && seq === sequence) reading = false; }
  }
  async function upload(url: string, seq = ++sequence) {
    uploading = true; error = '';
    try {
      const coverUrl = await uploadProfileCover(url.slice(url.indexOf(',') + 1));
      if (!alive || seq !== sequence) return;
      patch({ coverUrl, coverPosition: 50 });
    } catch (err) {
      if (alive && seq === sequence) error = accountErrorText(err, msg('profile.coverError'));
    } finally { if (alive && seq === sequence) uploading = false; }
  }
  function applyCrop(url: string) {
    originalSource = cropSource;
    void upload(url);
  }
  function removeCover() { originalSource = ''; patch({ coverUrl: '' }); error = ''; }
  function reset() { originalSource = ''; draft.appearance = { ...DEFAULT_APPEARANCE }; error = ''; }
</script>

<div class="panel" style={accentStyle}>
  <fieldset disabled={locked}>
    <section>
      <h3>{msg('profile.theme')}</h3>
      <div class="themes">
        {#each PROFILE_THEMES as theme}
          <button type="button" class="theme" class:selected={a.theme === theme.id} aria-pressed={a.theme === theme.id} onclick={() => patch({ theme: theme.id })}>
            <span class="theme-swatch" style:background={theme.banner}></span><span>{msg(`profile.${theme.id}`)}</span>
          </button>
        {/each}
        <button type="button" class="theme" class:selected={a.theme === CUSTOM_THEME} aria-pressed={a.theme === CUSTOM_THEME} onclick={() => patch({ theme: CUSTOM_THEME })}>
          <span class="theme-swatch" style:background={customBanner(a)}></span><span>{msg('profileStyle.themeCustom')}</span>
        </button>
        <button type="button" class="theme" class:selected={a.theme === AUTO_THEME} aria-pressed={a.theme === AUTO_THEME} onclick={() => patch({ theme: AUTO_THEME })}>
          <span class="theme-swatch auto"></span><span>{msg('profileStyle.themeAuto')}</span>
        </button>
      </div>

      {#if a.theme === CUSTOM_THEME}
        <div class="custom">
          <div class="gradient-preview" style:background={customBanner(a)}>
            <span class="gradient-card" style:background={themeOf(a).surface}>{previewName}</span>
          </div>
          <div class="pick">
            <span class="pick-label">{msg('profileStyle.colorFrom')}</span>
            <button type="button" class="color" style:background={a.customFrom} aria-label={msg('profileStyle.colorFrom')} aria-expanded={palette === 'from'} onclick={() => togglePalette('from')}></button>
            <HexColorField value={fromText} onchange={(v) => setColor('customFrom', v)} disabled={locked} />
          </div>
          {#if palette === 'from'}<ColorPalette value={a.customFrom} onchange={(v) => setColor('customFrom', v)} disabled={locked} />{/if}
          <div class="pick">
            <span class="pick-label">{msg('profileStyle.colorTo')}</span>
            <button type="button" class="color" style:background={a.customTo} aria-label={msg('profileStyle.colorTo')} aria-expanded={palette === 'to'} onclick={() => togglePalette('to')}></button>
            <HexColorField value={toText} onchange={(v) => setColor('customTo', v)} disabled={locked} />
          </div>
          {#if palette === 'to'}<ColorPalette value={a.customTo} onchange={(v) => setColor('customTo', v)} disabled={locked} />{/if}
          {#if !validAccent(fromText) || !validAccent(toText)}<p class="hint danger">{msg('settings.accentInvalid')}</p>{/if}
          <label class="slider"><span>{msg('profileStyle.angle')} <output>{a.customAngle}°</output></span>
            <input class="zoom-slider" style:--zoom-progress={`${a.customAngle / 3.6}%`} type="range" aria-label={msg('profileStyle.angle')} min="0" max="360" value={a.customAngle} oninput={(e) => patch({ customAngle: Number(e.currentTarget.value) })} />
          </label>
        </div>
      {:else if a.theme === AUTO_THEME}
        <div class="custom">
          <p class="hint">{msg('profileStyle.autoHint')}</p>
          <div class="options" role="group" aria-label={msg('profileStyle.autoSource')}>
            {#each AUTO_SOURCES as source}
              <button type="button" class="option" class:selected={a.autoSource === source} aria-pressed={a.autoSource === source} onclick={() => patch({ autoSource: source })}>{msg(`profileStyle.autoSource.${source}`)}</button>
            {/each}
          </div>
          {#if $artStatus === 'failed'}<p class="hint danger" role="status">{msg('profileStyle.autoFailed')}</p>{/if}
        </div>
      {/if}
    </section>

    <section>
      <h3>{msg('profile.accent')}</h3>
      <div class="accents">
        {#each PROFILE_ACCENTS as color}
          <button type="button" class="color" style:background={color} class:selected={a.accent.toLowerCase() === color} aria-label={`${msg('profile.accent')} ${color}`} aria-pressed={a.accent.toLowerCase() === color} onclick={() => setColor('accent', color)}></button>
        {/each}
        <Button size="sm" onclick={() => togglePalette('accent')}>{msg('settings.accentCustom')}</Button>
      </div>
      {#if palette === 'accent'}<ColorPalette value={a.accent} onchange={(v) => setColor('accent', v)} disabled={locked} />{/if}
      <div class="hex"><HexColorField value={accentText} onchange={(v) => setColor('accent', v)} disabled={locked} /></div>
      {#if !validAccent(accentText)}<p class="hint danger">{msg('settings.accentInvalid')}</p>{/if}
    </section>

    <section>
      <h3>{msg('profile.cover')}</h3>
      <input class="file" bind:this={input} type="file" accept={COVER_TYPES.join(',')} onchange={() => { void choose(input?.files?.[0]); if (input) input.value = ''; }} />
      <Button onclick={() => input?.click()}><Upload size="1.5rem" />{msg('profile.upload')}</Button>
      <p class="hint">{msg('profileStyle.coverHint')}</p>
      {#if a.coverUrl}
        {#if originalSource}<Button size="sm" variant="ghost" onclick={() => { cropSource = originalSource; cropOpen = true; }}>{msg('profile.editCrop')}</Button>{/if}
        <Button size="sm" variant="ghost" onclick={() => (pending = { prompt: removeCoverPrompt(), run: removeCover })}>{msg('profile.removeCover')}</Button>
      {/if}
      <label class="slider"><span>{msg('profile.dim')} <output>{a.coverDim}%</output></span><input class="zoom-slider" style:--zoom-progress={`${a.coverDim}%`} type="range" aria-label={msg('profile.dim')} min="0" max="100" value={a.coverDim} oninput={(e) => patch({ coverDim: Number(e.currentTarget.value) })} /></label>
      {#if reading || uploading}<p class="hint" role="status">{msg('profile.uploading')}</p>{/if}
      {#if error}<p class="hint danger" role="alert">{error}</p>{/if}
    </section>

    <section>
      <h3>{msg('profileStyle.frame')}</h3>
      <div class="frames">
        {#each AVATAR_FRAMES as frame}
          <button type="button" class="pick-card" class:selected={a.avatarFrame === frame} aria-pressed={a.avatarFrame === frame} onclick={() => patch({ avatarFrame: frame })}>
            <Avatar size="sm" name={previewName} src={$currentUser?.avatarUrl} {frame} />
            <span>{msg(`profileStyle.frame.${frame}`)}</span>
          </button>
        {/each}
      </div>
    </section>

    <section>
      <h3>{msg('profileStyle.nameStyle')}</h3>
      <div class="names">
        {#each NAME_STYLES as style}
          <button type="button" class="pick-card name-card" class:selected={a.nameStyle === style} aria-pressed={a.nameStyle === style} onclick={() => patch({ nameStyle: style })}>
            <StyledName name={previewName} styleName={style} />
            <small>{msg(`profileStyle.nameStyle.${style}`)}</small>
          </button>
        {/each}
      </div>
    </section>

    <section>
      <div class="toggle-row">
        <div>
          <h3>{msg('profileStyle.parallax')}</h3>
          <p class="hint">{msg('profileStyle.parallaxHint')}</p>
        </div>
        <Toggle label={msg('profileStyle.parallax')} checked={a.parallax} disabled={locked} onchange={(value) => patch({ parallax: value })} />
      </div>
    </section>

    <section>
      <h3>{msg('profileStyle.status')}</h3>
      <div class="status-row">
        <button type="button" class="emoji-button" aria-expanded={emojiOpen} aria-label={msg('profileStyle.emojiPick')} onclick={() => (emojiOpen = !emojiOpen)}>
          {#if draft.statusEmoji}<span class="emoji">{draft.statusEmoji}</span>{:else}<Smile size="1.8rem" />{/if}
        </button>
        <input class="text" type="text" autocomplete="off" spellcheck="true" placeholder={msg('profileStyle.statusPlaceholder')}
          aria-label={msg('profileStyle.status')} value={draft.statusText ?? ''} oninput={setStatusText} />
        <span class="counter" class:full={statusLength >= MAX_STATUS_TEXT}>{statusLength}/{MAX_STATUS_TEXT}</span>
      </div>
      {#if emojiOpen}
        <div class="emoji-grid" role="group" aria-label={msg('profileStyle.emojiPick')}>
          {#each STATUS_EMOJI as emoji}
            <button type="button" class="emoji-cell" class:selected={draft.statusEmoji === emoji} aria-pressed={draft.statusEmoji === emoji} onclick={() => { draft.statusEmoji = emoji; emojiOpen = false; }}>{emoji}</button>
          {/each}
        </div>
      {/if}
      {#if draft.statusEmoji}
        <Button size="sm" variant="ghost" onclick={() => (draft.statusEmoji = '')}><X size="1.4rem" />{msg('profileStyle.emojiClear')}</Button>
      {/if}
    </section>

    <Button size="sm" variant="ghost" onclick={() => (pending = { prompt: resetProfileAppearancePrompt(), run: reset })}><RotateCcw size="1.4rem" />{msg('profile.reset')}</Button>
  </fieldset>
</div>
{#if cropOpen}
  <BannerCropModal bind:open={cropOpen} src={cropSource} onsave={applyCrop} />
{/if}
{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}
<style>
  .panel { min-width: 0; }
  h3 { font-size: var(--font-sm); font-weight: 600; margin-bottom: 1.2rem; }
  fieldset { border: 0; padding: 0; min-width: 0; } fieldset:disabled { opacity: .65; }
  section { padding: 1.7rem 0; border-bottom: 1px solid var(--border); }
  .hint { color: var(--text-3); font-size: var(--font-xs); line-height: 1.5; margin-top: .8rem; }
  .danger { color: var(--danger); }
  .themes { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .8rem; }
  .theme { background: transparent; border: 1px solid var(--border); border-radius: var(--radius-md); padding: .5rem; color: var(--text-2); font: inherit; font-size: 1.05rem; cursor: pointer; min-width: 0; }
  .theme-swatch { display: block; height: 4rem; border-radius: .4rem; margin-bottom: .5rem; }
  .theme-swatch.auto { background: linear-gradient(125deg, var(--accent-subtle), var(--accent)); }
  .selected { outline: 2px solid var(--accent); outline-offset: 1px; }
  .custom { display: flex; flex-direction: column; gap: 1rem; margin-top: 1.4rem; }
  .gradient-preview { display: flex; align-items: flex-end; height: 7rem; padding: 1rem; border-radius: var(--radius-md); border: 1px solid var(--border); }
  .gradient-card { padding: .5rem 1rem; border-radius: var(--radius-sm); font-size: var(--font-xs); color: #f1f4f8; }
  .pick { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; }
  .pick-label { font-size: var(--font-xs); color: var(--text-2); min-width: 6rem; }
  .options { display: flex; flex-wrap: wrap; gap: .8rem; }
  .option { background: transparent; border: 1px solid var(--border); border-radius: var(--radius-md); padding: .7rem 1.2rem; color: var(--text-2); font: inherit; font-size: var(--font-xs); cursor: pointer; }
  .file { display: none; } .accents { display: flex; align-items: center; gap: .9rem; flex-wrap: wrap; }
  .color { width: 2.5rem; height: 2.5rem; border-radius: 50%; border: 1px solid var(--border-strong); cursor: pointer; padding: 0; }
  .hex { margin-top: 1.2rem; }
  .slider { display: flex; flex-direction: column; gap: 1rem; margin-top: 1.6rem; font-size: var(--font-xs); }
  .slider > span { display: flex; justify-content: space-between; } input[type=range] { width: 100%; }
  .frames { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .8rem; }
  .names { display: grid; grid-template-columns: minmax(0, 1fr); gap: .8rem; }
  .pick-card { display: flex; flex-direction: column; align-items: center; gap: .9rem; padding: 1.2rem .6rem; background: transparent; border: 1px solid var(--border); border-radius: var(--radius-md); color: var(--text-2); font: inherit; font-size: var(--font-xs); cursor: pointer; min-width: 0; }
  .name-card { align-items: flex-start; padding: 1rem 1.2rem; font-size: var(--font-md); font-weight: 600; color: var(--text); }
  .name-card small { font-size: var(--font-xs); font-weight: 400; color: var(--text-3); }
  .toggle-row { display: flex; align-items: center; justify-content: space-between; gap: 1.6rem; } .toggle-row h3 { margin-bottom: 0; }
  .status-row { display: flex; align-items: center; gap: .8rem; }
  .emoji-button { display: inline-flex; align-items: center; justify-content: center; width: var(--control-md); height: var(--control-md); flex-shrink: 0; background: var(--surface-2); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text-2); cursor: pointer; }
  .emoji { font-size: 1.8rem; line-height: 1; }
  .text { flex: 1; min-width: 0; height: var(--control-md); padding: 0 1.2rem; background: var(--surface-2); border: 1px solid var(--border-strong); border-radius: var(--radius-md); color: var(--text); font: inherit; font-size: var(--font-sm); }
  .text:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-subtle); }
  .counter { flex-shrink: 0; font-size: var(--font-xs); color: var(--text-3); font-variant-numeric: tabular-nums; } .counter.full { color: var(--danger); }
  .emoji-grid { display: grid; grid-template-columns: repeat(8, minmax(0, 1fr)); gap: .3rem; margin: 1rem 0; padding: .6rem; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-md); }
  .emoji-cell { aspect-ratio: 1; background: transparent; border: 0; border-radius: var(--radius-sm); font-size: 1.8rem; line-height: 1; cursor: pointer; }
  .emoji-cell:hover { background: var(--hover); }
  button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
</style>
