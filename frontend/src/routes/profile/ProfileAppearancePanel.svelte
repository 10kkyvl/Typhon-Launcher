<script lang="ts">
  import { ArrowDown, ArrowUp, Upload, X, RotateCcw } from '@lucide/svelte';
  import { onDestroy } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { SHOWCASE_KINDS, type ProfileSettings, type AvatarImage } from '../../lib/services/account';
  import { uploadProfileCover } from '../../lib/services/profileCover';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { appearanceAccentStyle, appearanceOf, DEFAULT_APPEARANCE, PROFILE_ACCENTS, PROFILE_THEMES } from '../../lib/profile/appearance';
  import { showcaseLabel } from '../../lib/profile/view';
  import { currentUser, isOffline, saveProfile, savingProfile } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

  let { settings, onpreview, onclose }: {
    settings: ProfileSettings; onpreview: (value: ProfileSettings) => void; onclose: () => void;
  } = $props();
  function initial() { return { ...settings, showcase: [...settings.showcase], appearance: appearanceOf(settings.appearance) }; }
  let draft = $state<ProfileSettings>(initial());
  let appearance = $state(initial().appearance);
  let selectedImage = $state<AvatarImage | null>(null);
  let input: HTMLInputElement;
  let busy = $state(false);
  let reading = $state(false);
  let error = $state('');
  let alive = true;
  let sequence = 0;
  const owner = $currentUser?.id;
  const disabled = $derived(busy || reading || $savingProfile || $isOffline);
  $effect(() => onpreview({ ...draft, appearance: { ...appearance } }));
  onDestroy(() => { alive = false; sequence++; });

  async function choose(file?: File) {
    if (!file) return;
    const seq = ++sequence;
    error = '';
    if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type) || file.size > 8 * 1024 * 1024 || file.size === 0) {
      error = msg('profile.coverInvalid'); return;
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
      selectedImage = { data: url.slice(url.indexOf(',') + 1), mime: file.type };
      appearance.coverUrl = url;
      appearance.coverPosition = 50;
    } catch {
      if (alive && seq === sequence) error = msg('profile.coverDecode');
    } finally { if (alive && seq === sequence) reading = false; }
  }
  function removeCover() { selectedImage = null; appearance.coverUrl = ''; error = ''; }
  function reset() { selectedImage = null; appearance = { ...DEFAULT_APPEARANCE }; error = ''; }
  function toggle(kind: typeof SHOWCASE_KINDS[number], checked: boolean) {
    draft.showcase = checked ? [...draft.showcase, kind] : draft.showcase.filter((k) => k !== kind);
  }
  function move(index: number, delta: number) {
    const next = [...draft.showcase];
    [next[index], next[index + delta]] = [next[index + delta], next[index]];
    draft.showcase = next;
  }
  async function save() {
    if (disabled || owner !== $currentUser?.id) return;
    busy = true; error = '';
    try {
      let coverUrl = appearance.coverUrl;
      if (selectedImage) coverUrl = await uploadProfileCover(selectedImage.data);
      if (!alive || owner !== $currentUser?.id) return;
      // Keep a completed upload for retry if saving the profile fails.
      selectedImage = null;
      appearance.coverUrl = coverUrl;
      await saveProfile({ profile: { ...$state.snapshot(draft), appearance: { ...$state.snapshot(appearance), coverUrl } } });
      if (!alive) return;
      toast(msg('social.settingsSaved'), 'success');
      onclose();
    } catch (err) { if (alive) error = accountErrorText(err, msg('profile.coverError')); }
    finally { if (alive) busy = false; }
  }
</script>

<aside style={appearanceAccentStyle(appearance)} aria-label={msg('profile.appearance')}>
  <div class="panel-head"><h2>{msg('profile.appearance')}</h2><IconButton label={msg('common.cancel')} disabled={busy} onclick={onclose}><X size="1.8rem" /></IconButton></div>
  <p class="hint preview">{msg('profile.preview')}</p>
  {#if $isOffline}<p class="hint">{msg('social.settingsRequireConnection')}</p>{/if}
  <fieldset disabled={disabled}>
    <section>
      <h3>{msg('profile.theme')}</h3>
      <div class="themes">
        {#each PROFILE_THEMES as theme}
          <button class="theme" class:selected={appearance.theme === theme.id} aria-pressed={appearance.theme === theme.id} onclick={() => (appearance.theme = theme.id)}>
            <span class="theme-swatch" style:background={theme.banner}></span><span>{msg(`profile.${theme.id}`)}</span>
          </button>
        {/each}
      </div>
    </section>
    <section>
      <h3>{msg('profile.cover')}</h3>
      <input class="file" bind:this={input} type="file" accept="image/jpeg,image/png,image/webp" onchange={() => { void choose(input.files?.[0]); input.value = ''; }} />
      <Button onclick={() => input.click()}><Upload size="1.5rem" />{msg('profile.upload')}</Button>
      <p class="hint">{msg('profile.coverHint')}</p>
      {#if appearance.coverUrl}
        <label class="slider">{msg('profile.coverPosition')}<input type="range" min="0" max="100" bind:value={appearance.coverPosition} /></label>
        <Button size="sm" variant="ghost" onclick={removeCover}>{msg('profile.removeCover')}</Button>
      {/if}
    </section>
    <section>
      <h3>{msg('profile.accent')}</h3>
      <div class="accents">
        {#each PROFILE_ACCENTS as color}
          <button class="color" style:background={color} class:selected={appearance.accent === color} aria-label={`${msg('profile.accent')} ${color}`} aria-pressed={appearance.accent === color} onclick={() => (appearance.accent = color)}></button>
        {/each}
        <input type="color" aria-label={msg('profile.accent')} bind:value={appearance.accent} />
      </div>
      <label class="slider"><span>{msg('profile.dim')} <output>{appearance.coverDim}%</output></span><input type="range" aria-label={msg('profile.dim')} min="0" max="100" bind:value={appearance.coverDim} /></label>
    </section>
    <section>
      <h3>{msg('profile.showcases')}</h3>
      {#each draft.showcase as kind, index (kind)}
        <div class="showcase-row">
          <span>{showcaseLabel(kind)}</span>
          <IconButton size="sm" label={msg('social.moveUp', { title: showcaseLabel(kind) })} disabled={index === 0 || disabled} onclick={() => move(index, -1)}><ArrowUp size="1.4rem" /></IconButton>
          <IconButton size="sm" label={msg('social.moveDown', { title: showcaseLabel(kind) })} disabled={index === draft.showcase.length - 1 || disabled} onclick={() => move(index, 1)}><ArrowDown size="1.4rem" /></IconButton>
          <Toggle label={showcaseLabel(kind)} checked disabled={disabled} onchange={(v) => toggle(kind, v)} />
        </div>
      {/each}
      {#each SHOWCASE_KINDS.filter((kind) => !draft.showcase.includes(kind)) as kind}
        <div class="showcase-row"><span>{showcaseLabel(kind)}</span><Toggle label={showcaseLabel(kind)} checked={false} disabled={disabled} onchange={(v) => toggle(kind, v)} /></div>
      {/each}
    </section>
    <Button size="sm" variant="ghost" onclick={reset}><RotateCcw size="1.4rem" />{msg('profile.reset')}</Button>
  </fieldset>
  <div class="foot">
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    {#if busy || reading}<p class="hint" role="status">{reading || selectedImage ? msg('profile.uploading') : msg('social.saving')}</p>{/if}
    <Button variant="primary" disabled={disabled} onclick={save}>{busy ? msg('social.saving') : msg('common.save')}</Button>
    <Button variant="ghost" disabled={busy} onclick={onclose}>{msg('common.cancel')}</Button>
  </div>
</aside>
<style>
  aside { width: 30rem; flex-shrink: 0; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 1.8rem; align-self: start; position: sticky; top: 1.6rem; max-height: calc(100vh - 5rem); overflow-y: auto; }
  .panel-head { display: flex; align-items: center; justify-content: space-between; }
  h2 { font-size: var(--font-lg); } h3 { font-size: var(--font-sm); font-weight: 600; margin-bottom: 1.2rem; }
  fieldset { border: 0; padding: 0; min-width: 0; } fieldset:disabled { opacity: .65; }
  section { padding: 1.7rem 0; border-bottom: 1px solid var(--border); }
  .hint { color: var(--text-3); font-size: var(--font-xs); line-height: 1.5; margin-top: .8rem; } .preview { color: var(--accent-text); }
  .themes { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .8rem; }
  .theme { background: transparent; border: 1px solid var(--border); border-radius: var(--radius-md); padding: .5rem; color: var(--text-2); font: inherit; font-size: 1.05rem; cursor: pointer; min-width: 0; }
  .theme-swatch { display: block; height: 4rem; border-radius: .4rem; margin-bottom: .5rem; }
  .selected { outline: 2px solid var(--accent); outline-offset: 1px; }
  .file { display: none; } .accents { display: flex; align-items: center; gap: .9rem; flex-wrap: wrap; }
  .color { width: 2.5rem; height: 2.5rem; border-radius: 50%; border: 0; cursor: pointer; }
  input[type=color] { width: 2.8rem; height: 2.8rem; border: 1px solid var(--border); background: transparent; padding: 0; cursor: pointer; }
  .slider { display: flex; flex-direction: column; gap: 1rem; margin-top: 1.6rem; font-size: var(--font-xs); }
  .slider > span { display: flex; justify-content: space-between; } input[type=range] { width: 100%; accent-color: var(--accent); }
  .showcase-row { display: flex; gap: .3rem; align-items: center; min-height: 4rem; font-size: var(--font-xs); }
  .showcase-row > span { flex: 1; } .foot { position: sticky; bottom: -1.8rem; background: var(--surface-2); padding: 1rem 0; display: flex; flex-direction: column; gap: .6rem; margin-top: 1.5rem; }
  .error { font-size: var(--font-xs); color: var(--danger); line-height: 1.5; }
  button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  @media (max-width: 1000px) { aside { width: 100%; position: static; max-height: none; } }
</style>
