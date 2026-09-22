<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { t, locale } from '../../i18n';
  import type { TextRequest } from '../contracts';
  import { appendText, eraseCharacter, keyboardRows } from '../keyboard';
  import { focusControl } from '../navigation';

  let { request, ondone }: { request: TextRequest; ondone: (value: string | null) => void } = $props();
  let value = $state(untrack(() => request.initialValue ?? ''));
  let language = $state<'en' | 'ru'>($locale);
  let upper = $state(false);
  let root: HTMLElement;
  let input: HTMLInputElement;
  const limit = $derived(request.maxLength ?? 300);
  function write(text: string) { value = appendText(value, text, limit); }

  function physical(event: KeyboardEvent) {
    if (event.isComposing || event.metaKey || event.ctrlKey || event.altKey) return;
    if (event.key === 'Escape') {
      event.preventDefault(); event.stopImmediatePropagation(); ondone(null); return;
    }
    if (event.target === input) {
      if (event.key === 'Enter') {
        event.preventDefault(); event.stopImmediatePropagation(); ondone(value);
      }
      return;
    }
    // Capture characters before the shell interprets Q/E, Space or repeats.
    if (event.key.length === 1 || event.key === 'Backspace' || event.key === 'Delete') {
      event.preventDefault(); event.stopImmediatePropagation();
      if (event.key === 'Backspace' || event.key === 'Delete') value = eraseCharacter(value);
      else write(event.key);
    }
  }

  onMount(() => {
    window.addEventListener('keydown', physical, true);
    focusControl(root.querySelector<HTMLButtonElement>('[data-bp-default]') ?? undefined);
    return () => window.removeEventListener('keydown', physical, true);
  });
</script>

<div class="keyboard-veil">
  <div class="keyboard" role="dialog" aria-modal="true" aria-labelledby="bp-keyboard-title" tabindex="-1" data-bp-scope bind:this={root}>
    <h2 id="bp-keyboard-title">{request.title}</h2>
    <input aria-label={request.title} maxlength={limit} bind:this={input} bind:value autocomplete="off" spellcheck="false" />
    <p>{$t('bp.textHint')}</p>
    <div class="keys">
      {#each keyboardRows[language] as row, index}
        <div class="key-row">
          {#each Array.from(row) as char, column}
            <button class="bp-button key" data-bp-focus={`key-${index}-${column}`} data-bp-default={index === 1 && column === 0 ? '' : undefined}
              onclick={() => write(upper ? char.toUpperCase() : char)}>{upper ? char.toUpperCase() : char}</button>
          {/each}
        </div>
      {/each}
    </div>
    <div class="bp-actions tools">
      <button class="bp-button" data-bp-focus="key-language" onclick={() => language = language === 'en' ? 'ru' : 'en'}>EN / РУ</button>
      <button class="bp-button" data-bp-focus="key-shift" aria-pressed={upper} onclick={() => upper = !upper}>⇧ {$t('bp.uppercase')}</button>
      <button class="bp-button space" data-bp-focus="key-space" onclick={() => write(' ')}>{$t('bp.space')}</button>
      <button class="bp-button" data-bp-focus="key-erase" onclick={() => value = eraseCharacter(value)}>⌫</button>
      <button class="bp-button" data-bp-focus="key-clear" onclick={() => value = ''}>{$t('bp.clear')}</button>
    </div>
    <div class="bp-actions finish">
      <button class="bp-button" data-bp-focus="key-cancel" onclick={() => ondone(null)}>{$t('common.cancel')}</button>
      <button class="bp-button bp-primary" data-bp-focus="key-done" onclick={() => ondone(value)}>{$t('bp.done')}</button>
    </div>
  </div>
</div>

<style>
  .keyboard-veil { position: absolute; inset: 0 0 68px; z-index: 4; display: flex; align-items: flex-start; justify-content: center; padding: 24px; background: #050910dd; backdrop-filter: blur(12px); overflow: auto; }
  .keyboard { width: min(920px, 100%); flex-shrink: 0; margin-block: auto; padding: 28px; border: 1px solid #ffffff24; border-radius: 20px; background: #152033; }
  h2 { font-size: 1.4em; margin-bottom: 16px; }
  input { width: 100%; padding: 14px 18px; background: #080e19; border: 1px solid #ffffff40; border-radius: 10px; color: white; font: inherit; }
  input:focus { outline: 3px solid var(--accent); }
  p { margin: 10px 0 20px; color: #b6c2d4; font-size: .8em; }
  .keys { display: grid; gap: 10px; }
  .key-row { display: flex; gap: 10px; justify-content: center; }
  .keyboard .key { width: 56px; min-width: 0; padding: 6px; }
  .tools { margin-top: 16px; gap: 10px; }.space { flex: 1; }.finish { justify-content: flex-end; margin-top: 22px; }
  @media (max-height: 760px) { .keyboard { padding: 20px; }.keyboard .key { min-height: 38px; }.keys { gap: 8px; }p { margin-bottom: 12px; }.finish { margin-top: 12px; } }
</style>
