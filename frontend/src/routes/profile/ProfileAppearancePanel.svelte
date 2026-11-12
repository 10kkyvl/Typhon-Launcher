<script lang="ts">
  import '../../styles/zoom-slider.css';
  import ColorPalette from '../../lib/components/ColorPalette.svelte';
  import HexColorField from '../../lib/components/HexColorField.svelte';
  import { validAccent } from '../../lib/theme/accent';
  import BannerCropModal from '../../lib/components/BannerCropModal.svelte';
  import ProfileCanvas from '../../lib/components/ProfileCanvas.svelte';
  import ProfileCover from '../../lib/components/ProfileCover.svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import StyledName from '../../lib/components/StyledName.svelte';
  import { Upload, RotateCcw, Smile, X } from '@lucide/svelte';
  import { onDestroy, tick } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
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
    pickCustomColor, PROFILE_ACCENTS, PROFILE_THEMES, themeOf,
  } from '../../lib/profile/appearance';
  import { toast } from '../../lib/stores/toasts';
  import { launcherAccent } from '../../lib/stores/profileAccent';
  import { artStatus } from '../../lib/profile/artPalette';
  import { STATUS_EMOJI } from '../../lib/profile/appearanceEmoji';
  import { layoutErrorText, statusIssue } from '../../lib/profile/layout';
  import { currentUser } from '../../lib/stores/user';
  import { frameLook, isCustomFrame, ownedFrame } from '../../lib/profile/frameLook';
  import { msg } from '../../lib/i18n';
  import { nextIndex } from '../../lib/utils/roving';

  let { draft = $bindable(), disabled, autoArt }: { draft: ProfileSettings; disabled: boolean; autoArt?: string } = $props();

  const uid = $props.id();

  const COVER_TYPES = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'];

  const a = $derived(appearanceOf(draft.appearance));
  const previewName = $derived($currentUser?.displayName || $currentUser?.username || msg('social.guestName'));
  const fromLabel = $derived(a.theme === CUSTOM_THEME ? msg('profileStyle.colorFrom') : msg('profileStyle.customColor'));
  const accentStyle = $derived(
    appearanceAccentStyle(a).split(';').filter((rule) => rule.startsWith('--accent')).join(';'),
  );
  const preview = $derived({ ...a, parallax: false });
  const ownedFrames = $derived($currentUser?.frames ?? []);
  const activeFrame = $derived(ownedFrame(ownedFrames, a.avatarFrame));
  const frameRevoked = $derived(isCustomFrame(a.avatarFrame) && !activeFrame);
  const statusLength = $derived([...(draft.statusText ?? '')].length);
  const statusError = $derived(statusIssue(draft.statusText));

  const THEME_IDS = [...PROFILE_THEMES.map((theme) => theme.id), CUSTOM_THEME, AUTO_THEME];
  const accentSources = [
    { id: 'custom', label: msg('profileStyle.accentOwn') },
    { id: 'theme', label: msg('profileStyle.accentTheme') },
  ];
  const autoSources = AUTO_SOURCES.map((source) => ({ id: source, label: msg(`profileStyle.autoSource.${source}`) }));
  const paletteId = (kind: string) => `${uid}-palette-${kind}`;
  const hexError = (kind: string) => `${uid}-hex-${kind}-error`;
  const emojiId = `${uid}-emoji`;
  const emojiGridId = `${uid}-emoji-grid`;

  // A radio group is one Tab stop: its chosen option, or the first while none is chosen.
  const stop = (value: string, all: readonly string[]) => (all.includes(value) ? value : all[0]);
  const ownAccent = $derived(a.accentSource === 'custom' ? a.accent.toLowerCase() : '');
  const themeStop = $derived(stop(a.theme, THEME_IDS));
  const accentStop = $derived(stop(ownAccent, PROFILE_ACCENTS));
  const frameStop = $derived(
    stop(activeFrame ? `custom:${activeFrame.id}` : frameRevoked ? 'none' : a.avatarFrame, [
      ...AVATAR_FRAMES,
      ...ownedFrames.map((owned) => `custom:${owned.id}`),
    ]),
  );
  const nameStop = $derived(stop(a.nameStyle, NAME_STYLES));
  const emojiStop = $derived(stop(draft.statusEmoji ?? '', STATUS_EMOJI));

  let accentText = $state('');
  let fromText = $state('');
  let toText = $state('');
  $effect(() => { accentText = a.accent; });
  $effect(() => { fromText = a.customFrom; });
  $effect(() => { toText = a.customTo; });
  let nameText = $state('');
  let frameText = $state('');
  $effect(() => { nameText = a.nameColor; });
  $effect(() => { frameText = a.frameColor; });
  // A colour field that holds an incomplete colour, and is described by its message while it does.
  const fromBad = $derived(!validAccent(fromText));
  const toBad = $derived(!validAccent(toText));
  const accentBad = $derived(!validAccent(accentText));
  const frameBad = $derived(frameText !== '' && !validAccent(frameText));
  const nameBad = $derived(nameText !== '' && !validAccent(nameText));
  let palette = $state<'accent' | 'from' | 'to' | 'name' | 'frame' | null>(null);
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
    if (!validAccent(value)) return;
    if (field === 'customFrom') draft.appearance = pickCustomColor(a, value);
    else if (field === 'accent') patch({ accent: value, accentSource: 'custom' });
    else patch({ [field]: value });
  }
  function useLauncherAccent() {
    const accent = launcherAccent();
    if (!accent) {
      toast(msg('profileStyle.accentThemeUnreadable'), 'danger');
      return;
    }
    patch({ accentSource: 'theme', accent });
  }
  function setOptionalColor(field: 'nameColor' | 'frameColor', value: string) {
    if (field === 'nameColor') nameText = value;
    else frameText = value;
    if (value === '' || validAccent(value)) patch({ [field]: value });
  }
  function togglePalette(kind: 'accent' | 'from' | 'to' | 'name' | 'frame') {
    palette = palette === kind ? null : kind;
  }
  function chooseAccentSource(source: string) {
    if (source === 'theme') useLauncherAccent();
    else patch({ accentSource: 'custom' });
  }
  // The arrows, Home and End move along a radio group and choose the option they land on (WAI-ARIA
  // radio group). Choosing is a press of that option, unless its press does more (an emoji also
  // closes its grid): `choose` is then given the option landed on and its index.
  function roam(
    event: KeyboardEvent,
    choose: (radio: HTMLElement, index: number) => void = (radio) => radio.click(),
  ) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radio = event.currentTarget as HTMLElement;
    const all = [...(radio.closest('[role="radiogroup"]')?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
    const next = nextIndex(all.indexOf(radio), all.length, event.key, { orientation: 'both' });
    if (next === null) return;
    event.preventDefault();
    choose(all[next], next);
    all[next].focus();
  }
  async function pickEmoji(emoji: string) {
    draft.statusEmoji = emoji;
    emojiOpen = false;
    await tick();
    document.getElementById(emojiId)?.focus();
  }
  function setStatusText(event: Event) {
    const field = event.currentTarget as HTMLInputElement;
    const text = [...field.value].slice(0, MAX_STATUS_TEXT).join('');
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
      <h3 id="{uid}-theme-label">{msg('profile.theme')}</h3>
      <div class="themes" role="radiogroup" aria-labelledby="{uid}-theme-label">
        {#each PROFILE_THEMES as theme}
          <button type="button" role="radio" class="theme" class:selected={a.theme === theme.id} aria-checked={a.theme === theme.id} tabindex={themeStop === theme.id ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => patch({ theme: theme.id })}>
            <span class="theme-swatch" style:background={theme.banner}></span><span>{msg(`profile.${theme.id}`)}</span>
          </button>
        {/each}
        <button type="button" role="radio" class="theme" class:selected={a.theme === CUSTOM_THEME} aria-checked={a.theme === CUSTOM_THEME} tabindex={themeStop === CUSTOM_THEME ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => patch({ theme: CUSTOM_THEME })}>
          <span class="theme-swatch" style:background={customBanner(a)}></span><span>{msg('profileStyle.themeCustom')}</span>
        </button>
        <button type="button" role="radio" class="theme" class:selected={a.theme === AUTO_THEME} aria-checked={a.theme === AUTO_THEME} tabindex={themeStop === AUTO_THEME ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => patch({ theme: AUTO_THEME })}>
          <span class="theme-swatch auto"></span><span>{msg('profileStyle.themeAuto')}</span>
        </button>
      </div>

      <div class="preview" aria-hidden="true">
        <div class="preview-scale">
          <ProfileCanvas appearance={preview} {autoArt} quiet>
            <ProfileCover appearance={preview} />
            <span class="preview-card" style:background="var(--surface-2)" style:color="var(--text)">{previewName}</span>
          </ProfileCanvas>
        </div>
      </div>

      <div class="custom">
        <div class="pick">
          <span class="pick-label">{fromLabel}</span>
          <button type="button" class="color" style:background={a.customFrom} aria-label={fromLabel} aria-expanded={palette === 'from'} aria-controls={palette === 'from' ? paletteId('from') : undefined} onclick={() => togglePalette('from')}></button>
          <HexColorField label={fromLabel} describedby={fromBad ? hexError('from') : undefined} value={fromText} onchange={(v) => setColor('customFrom', v)} disabled={locked} />
        </div>
        {#if palette === 'from'}<div id={paletteId('from')}><ColorPalette value={a.customFrom} onchange={(v) => setColor('customFrom', v)} disabled={locked} /></div>{/if}
        {#if fromBad}<p class="hint danger" id={hexError('from')} role="alert">{msg('settings.accentInvalid')}</p>{/if}
        {#if a.theme === CUSTOM_THEME}
          <div class="pick">
            <span class="pick-label">{msg('profileStyle.colorTo')}</span>
            <button type="button" class="color" style:background={a.customTo} aria-label={msg('profileStyle.colorTo')} aria-expanded={palette === 'to'} aria-controls={palette === 'to' ? paletteId('to') : undefined} onclick={() => togglePalette('to')}></button>
            <HexColorField label={msg('profileStyle.colorTo')} describedby={toBad ? hexError('to') : undefined} value={toText} onchange={(v) => setColor('customTo', v)} disabled={locked} />
            {#if a.customTo !== a.customFrom}<Button size="sm" variant="ghost" onclick={() => patch({ customTo: a.customFrom })}>{msg('profileStyle.singleColor')}</Button>{/if}
          </div>
          {#if palette === 'to'}<div id={paletteId('to')}><ColorPalette value={a.customTo} onchange={(v) => setColor('customTo', v)} disabled={locked} /></div>{/if}
          {#if toBad}<p class="hint danger" id={hexError('to')} role="alert">{msg('settings.accentInvalid')}</p>{/if}
          <label class="slider"><span>{msg('profileStyle.angle')} <output>{a.customAngle}°</output></span>
            <input class="zoom-slider" style:--zoom-progress={`${a.customAngle / 3.6}%`} type="range" aria-label={msg('profileStyle.angle')} min="0" max="360" value={a.customAngle} oninput={(e) => patch({ customAngle: Number(e.currentTarget.value) })} />
          </label>
          <div class="toggle-row">
            <div>
              <span class="pick-label" id="{uid}-custom-pure-label">{msg('profileStyle.customPure')}</span>
              <p class="hint" id="{uid}-custom-pure-sub">{msg('profileStyle.customPureHint')}</p>
            </div>
            <Toggle labelledby="{uid}-custom-pure-label" describedby="{uid}-custom-pure-sub" checked={a.customPure} disabled={locked} onchange={(value) => patch({ customPure: value })} />
          </div>
        {/if}
      </div>
      {#if a.theme === AUTO_THEME}
        <div class="custom">
          <p class="hint">{msg('profileStyle.autoHint')}</p>
          <SegmentedControl
            options={autoSources}
            label={msg('profileStyle.autoSource')}
            disabled={locked}
            bind:value={() => a.autoSource, (source) => patch({ autoSource: source as ProfileAppearance['autoSource'] })}
          />
          {#if $artStatus === 'failed'}<Notice tone="warning" compact>{msg('profileStyle.autoFailed')}</Notice>{/if}
        </div>
      {/if}
    </section>

    <section>
      <h3 id="{uid}-accent-label">{msg('profile.accent')}</h3>
      <SegmentedControl options={accentSources} labelledby="{uid}-accent-label" disabled={locked} bind:value={() => a.accentSource, chooseAccentSource} />
      {#if a.accentSource === 'theme'}<p class="hint">{msg('profileStyle.accentThemeHint')}</p>{/if}
      <div class="accents">
        <div class="swatches" role="radiogroup" aria-label={msg('profileStyle.accentOwn')}>
          {#each PROFILE_ACCENTS as color}
            <button type="button" role="radio" class="color" style:background={color} class:selected={ownAccent === color} aria-label={`${msg('profile.accent')} ${color}`} aria-checked={ownAccent === color} tabindex={accentStop === color ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => setColor('accent', color)}></button>
          {/each}
        </div>
        <Button size="sm" aria-expanded={palette === 'accent'} aria-controls={palette === 'accent' ? paletteId('accent') : undefined} onclick={() => togglePalette('accent')}>{msg('settings.accentCustom')}</Button>
      </div>
      {#if palette === 'accent'}<div id={paletteId('accent')}><ColorPalette value={a.accent} onchange={(v) => setColor('accent', v)} disabled={locked} /></div>{/if}
      <div class="hex"><HexColorField label={msg('profile.accent')} describedby={accentBad ? hexError('accent') : undefined} value={accentText} onchange={(v) => setColor('accent', v)} disabled={locked} /></div>
      {#if accentBad}<p class="hint danger" id={hexError('accent')} role="alert">{msg('settings.accentInvalid')}</p>{/if}
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
      {#if reading || uploading}<div class="message"><Spinner size="sm" label={msg('profile.uploading')} /></div>{/if}
      {#if error}<div class="message"><Notice tone="danger" compact>{error}</Notice></div>{/if}
    </section>

    <section>
      <h3 id="{uid}-frame-label">{msg('profileStyle.frame')}</h3>
      <div class="frame-choices" role="radiogroup" aria-labelledby="{uid}-frame-label">
        <div class="frames">
          {#each AVATAR_FRAMES as frame}
            {@const picked = frame === 'none' ? a.avatarFrame === 'none' || frameRevoked : a.avatarFrame === frame}
            <button type="button" role="radio" class="pick-card" class:selected={picked} aria-checked={picked} tabindex={frameStop === frame ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => patch({ avatarFrame: frame })}>
              <span class="sample" aria-hidden="true"><Avatar size="sm" name={previewName} src={$currentUser?.avatarUrl} {frame} frameColor={a.frameColor} /></span>
              <span>{msg(`profileStyle.frame.${frame}`)}</span>
            </button>
          {/each}
        </div>
        {#if ownedFrames.length > 0}
          <h4 class="sub">{msg('profileStyle.frameOwned')}</h4>
          <div class="frames owned">
            {#each ownedFrames as owned (owned.id)}
              {@const picked = activeFrame?.id === owned.id}
              <button type="button" role="radio" class="pick-card" class:selected={picked} aria-checked={picked} tabindex={frameStop === `custom:${owned.id}` ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => patch({ avatarFrame: `custom:${owned.id}` })}>
                <span class="checker"><img src={owned.url} alt="" draggable="false" loading="lazy" decoding="async" /></span>
                <span class="owned-name">{owned.name}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
      {#if frameRevoked}<div class="message"><Notice tone="warning" compact announce={false}>{msg('profileStyle.frameRevoked')}</Notice></div>{/if}
      {#if !activeFrame}
      <div class="pick">
        <span class="pick-label">{msg('profileStyle.frameColor')}</span>
        <button type="button" class="color" style:background={a.frameColor || a.accent} aria-label={msg('profileStyle.frameColor')} aria-expanded={palette === 'frame'} aria-controls={palette === 'frame' ? paletteId('frame') : undefined} onclick={() => togglePalette('frame')}></button>
        <HexColorField label={msg('profileStyle.frameColor')} describedby={frameBad ? hexError('frame') : undefined} optional value={frameText} onchange={(v) => setOptionalColor('frameColor', v)} disabled={locked} />
        {#if a.frameColor}<Button size="sm" variant="ghost" onclick={() => setOptionalColor('frameColor', '')}>{msg('profileStyle.colorAsAccent')}</Button>{/if}
      </div>
      {#if palette === 'frame'}<div id={paletteId('frame')}><ColorPalette value={a.frameColor || a.accent} onchange={(v) => setOptionalColor('frameColor', v)} disabled={locked} /></div>{/if}
      {#if frameBad}<p class="hint danger" id={hexError('frame')} role="alert">{msg('settings.accentInvalid')}</p>{/if}
      {/if}
    </section>

    <section>
      <h3 id="{uid}-name-label">{msg('profileStyle.nameStyle')}</h3>
      <div class="names" role="radiogroup" aria-labelledby="{uid}-name-label">
        {#each NAME_STYLES as style}
          <button type="button" role="radio" class="pick-card name-card" class:selected={a.nameStyle === style} aria-checked={a.nameStyle === style} tabindex={nameStop === style ? 0 : -1} onkeydown={(e) => roam(e)} onclick={() => patch({ nameStyle: style })}>
            <span class="sample" aria-hidden="true"><StyledName name={previewName} styleName={style} nameColor={a.nameColor} /></span>
            <small>{msg(`profileStyle.nameStyle.${style}`)}</small>
          </button>
        {/each}
      </div>
      <div class="pick">
        <span class="pick-label">{msg('profileStyle.nameColor')}</span>
        <button type="button" class="color" style:background={a.nameColor || a.accent} aria-label={msg('profileStyle.nameColor')} aria-expanded={palette === 'name'} aria-controls={palette === 'name' ? paletteId('name') : undefined} onclick={() => togglePalette('name')}></button>
        <HexColorField label={msg('profileStyle.nameColor')} describedby={nameBad ? hexError('name') : undefined} optional value={nameText} onchange={(v) => setOptionalColor('nameColor', v)} disabled={locked} />
        {#if a.nameColor}<Button size="sm" variant="ghost" onclick={() => setOptionalColor('nameColor', '')}>{msg('profileStyle.colorAsAccent')}</Button>{/if}
      </div>
      {#if palette === 'name'}<div id={paletteId('name')}><ColorPalette value={a.nameColor || a.accent} onchange={(v) => setOptionalColor('nameColor', v)} disabled={locked} /></div>{/if}
      {#if nameBad}<p class="hint danger" id={hexError('name')} role="alert">{msg('settings.accentInvalid')}</p>{/if}
    </section>

    <section>
      <div class="toggle-row">
        <div>
          <h3 id="{uid}-parallax-label">{msg('profileStyle.parallax')}</h3>
          <p class="hint" id="{uid}-parallax-sub">{msg('profileStyle.parallaxHint')}</p>
        </div>
        <Toggle labelledby="{uid}-parallax-label" describedby="{uid}-parallax-sub" checked={a.parallax} disabled={locked} onchange={(value) => patch({ parallax: value })} />
      </div>
    </section>

    <section>
      <h3>{msg('profileStyle.status')}</h3>
      <div class="status-row">
        <IconButton
          id={emojiId}
          variant="secondary"
          label={msg('profileStyle.emojiPick')}
          disabled={locked}
          aria-expanded={emojiOpen}
          aria-controls={emojiOpen ? emojiGridId : undefined}
          onclick={() => (emojiOpen = !emojiOpen)}
        >
          {#if draft.statusEmoji}<span class="emoji">{draft.statusEmoji}</span>{:else}<Smile size="1.8rem" />{/if}
        </IconButton>
        <input class="input text" type="text" autocomplete="off" spellcheck="true" placeholder={msg('profileStyle.statusPlaceholder')}
          aria-label={msg('profileStyle.status')} aria-invalid={statusError ? 'true' : undefined}
          aria-describedby={statusError ? `${uid}-status-error` : undefined} value={draft.statusText ?? ''} oninput={setStatusText} />
        <span class="counter" class:full={statusLength >= MAX_STATUS_TEXT}>{statusLength}/{MAX_STATUS_TEXT}</span>
      </div>
      {#if statusError}<p class="status-error" id="{uid}-status-error" role="alert">{layoutErrorText(statusError)}</p>{/if}
      {#if emojiOpen}
        <div class="emoji-grid" id={emojiGridId} role="radiogroup" aria-label={msg('profileStyle.emojiPick')}>
          {#each STATUS_EMOJI as emoji}
            <button type="button" role="radio" class="emoji-cell" class:selected={draft.statusEmoji === emoji} aria-checked={draft.statusEmoji === emoji} tabindex={emojiStop === emoji ? 0 : -1} onkeydown={(e) => roam(e, (_, index) => (draft.statusEmoji = STATUS_EMOJI[index]))} onclick={() => pickEmoji(emoji)}>{emoji}</button>
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
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} errorText={(err) => accountErrorText(err)} />
{/if}
<style>
  .panel { min-width: 0; }
  h3 { font-size: var(--font-sm); font-weight: 600; margin-bottom: 1.2rem; }
  fieldset { border: 0; padding: 0; min-width: 0; } fieldset:disabled { opacity: .65; }
  section { padding: 1.7rem 0; border-bottom: 1px solid var(--border); }
  .status-error { width: fit-content; padding: 0 var(--space-1); border-radius: var(--radius-xs); background: var(--surface-2); color: var(--danger); font-size: var(--font-xs); line-height: 1.5; margin-top: .8rem; }
  .hint { color: var(--text-3); font-size: var(--font-xs); line-height: 1.5; margin-top: .8rem; }
  .danger { width: fit-content; padding: 0 var(--space-1); border-radius: var(--radius-xs); background: var(--surface-2); color: var(--danger); }
  .themes { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .8rem; }
  .theme { background: transparent; border: 1px solid var(--border); border-radius: var(--radius-md); padding: .5rem; color: var(--text-2); font: inherit; font-size: 1.05rem; cursor: pointer; min-width: 0; }
  .theme-swatch { display: block; height: 4rem; border-radius: .4rem; margin-bottom: .5rem; }
  .theme-swatch.auto { background: linear-gradient(125deg, var(--accent-subtle), var(--accent)); }
  .custom { display: flex; flex-direction: column; gap: 1rem; margin-top: 1.4rem; }
  .preview { height: 9rem; margin-top: 1.4rem; overflow: hidden; border-radius: var(--radius-md); border: 1px solid var(--border); }
  .preview-scale { width: 250%; transform: scale(.4); transform-origin: 0 0; pointer-events: none; }
  .preview-card { display: block; margin-top: 4rem; padding: 1.4rem 2rem; border-radius: var(--radius-md); font-size: var(--font-md); }
  .pick { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; }
  .pick-label { font-size: var(--font-xs); color: var(--text-2); min-width: 6rem; }
  .frame-choices + .pick, .names + .pick { margin-top: 1.2rem; }
  .file { display: none; } .accents, .swatches { display: flex; align-items: center; gap: .9rem; flex-wrap: wrap; }
  .color { width: 2.5rem; height: 2.5rem; border-radius: 50%; border: 1px solid var(--border-strong); cursor: pointer; padding: 0; }
  .hex { margin-top: 1.2rem; }
  .message { margin-top: .8rem; }
  .slider { display: flex; flex-direction: column; gap: 1rem; margin-top: 1.6rem; font-size: var(--font-xs); }
  .slider > span { display: flex; justify-content: space-between; } input[type=range] { width: 100%; }
  .frames { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .8rem; }
  .names { display: grid; grid-template-columns: minmax(0, 1fr); gap: .8rem; }
  h4.sub { margin: 1.4rem 0 .8rem; font-size: var(--font-xs); font-weight: 600; color: var(--text-3); }
  .frames.owned { margin-top: 0; }
  .checker { display: flex; align-items: center; justify-content: center; width: 4.8rem; height: 4.8rem; border-radius: var(--radius-sm); background: repeating-conic-gradient(var(--surface-3) 0% 25%, var(--surface-4) 0% 50%) 0 0 / 1.2rem 1.2rem; overflow: hidden; }
  .checker img { max-width: 100%; max-height: 100%; object-fit: contain; }
  .owned-name { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pick-card { display: flex; flex-direction: column; align-items: center; gap: .9rem; padding: 1.2rem .6rem; background: transparent; border: 1px solid var(--border); border-radius: var(--radius-md); color: var(--text-2); font: inherit; font-size: var(--font-xs); cursor: pointer; min-width: 0; }
  .name-card { align-items: flex-start; padding: 1rem 1.2rem; font-size: var(--font-md); font-weight: 600; color: var(--text); }
  .name-card small { font-size: var(--font-xs); font-weight: 400; color: var(--text-3); }
  /* Hides the sample from the option's name and takes no box of its own. */
  .sample { display: contents; }
  .toggle-row { display: flex; align-items: center; justify-content: space-between; gap: 1.6rem; } .toggle-row h3 { margin-bottom: 0; }
  .status-row { display: flex; align-items: center; gap: .8rem; }
  .emoji { font-size: 1.8rem; line-height: 1; }
  .text { flex: 1; }
  .counter { flex-shrink: 0; padding: 0 var(--space-1); border-radius: var(--radius-xs); font-size: var(--font-xs); color: var(--text-3); font-variant-numeric: tabular-nums; } .counter.full { background: var(--surface-2); color: var(--danger); }
  .emoji-grid { display: grid; grid-template-columns: repeat(8, minmax(0, 1fr)); gap: .3rem; margin: 1rem 0; padding: .6rem; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-md); }
  .emoji-cell { aspect-ratio: 1; background: transparent; border: 0; border-radius: var(--radius-sm); font-size: 1.8rem; line-height: 1; cursor: pointer; }
  .emoji-cell:hover { background: var(--hover); }
  /* The chosen option's mark is its edge in the accent, not an outline: the shared focus ring, drawn a
     step further out, shows apart from it. A chosen colour swatch is often the accent itself, so its
     accent edge is parted from its colour by a ring of the window's ground. Last: it wins over each
     option's own border. */
  .selected { border-color: var(--accent-bright); box-shadow: 0 0 0 1px var(--accent-bright); }
  .color.selected { border-width: 2px; box-shadow: inset 0 0 0 2px var(--bg); }
</style>
