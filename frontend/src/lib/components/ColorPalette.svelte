<script lang="ts">
  import { untrack } from 'svelte';
  import { msg } from '../i18n';
  import { hexToHsv, hsvToHex, clampUnit } from '../theme/colorPicker';
  let { value, onchange }: { value: string; onchange: (color: string) => void } = $props();
  let hsv = $state(untrack(() => hexToHsv(value)));
  let lastColor = untrack(() => value);
  $effect(() => {
    if (value.toUpperCase() !== lastColor.toUpperCase()) {
      hsv = hexToHsv(value, hsv.h);
      lastColor = value;
    }
  });
  function emit() {
    lastColor = hsvToHex(hsv);
    onchange(lastColor);
  }
  function pick(event: PointerEvent) {
    const bounds = event.currentTarget instanceof HTMLElement ? event.currentTarget.getBoundingClientRect() : null;
    if (!bounds || !bounds.width || !bounds.height) return;
    hsv = { ...hsv, s: clampUnit((event.clientX - bounds.left) / bounds.width), v: 1 - clampUnit((event.clientY - bounds.top) / bounds.height) };
    emit();
  }
  function start(event: PointerEvent) {
    if (event.button !== 0) return;
    const target = event.currentTarget as HTMLElement;
    target.focus();
    target.setPointerCapture(event.pointerId);
    pick(event);
  }
  function move(event: PointerEvent) {
    if ((event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId)) pick(event);
  }
  function key(event: KeyboardEvent) {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    const step = event.shiftKey ? .1 : .01;
    hsv = { ...hsv,
      s: clampUnit(hsv.s + (event.key === 'ArrowRight' ? step : event.key === 'ArrowLeft' ? -step : 0)),
      v: clampUnit(hsv.v + (event.key === 'ArrowUp' ? step : event.key === 'ArrowDown' ? -step : 0)),
    };
    emit();
  }
</script>

<div class="palette">
  <div class="color-field">
    <div class="sample" style:background={value} aria-hidden="true"></div>
    <div class="saturation" style:--hue={`hsl(${hsv.h} 100% 50%)`}
      role="slider" tabindex="0" aria-label={msg('settings.accentShade')}
      aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(hsv.s * 100)}
      aria-valuetext={msg('settings.accentShadeValue', { saturation: Math.round(hsv.s * 100), brightness: Math.round(hsv.v * 100) })}
      onpointerdown={start} onpointermove={move} onkeydown={key}>
      <span class="cursor" style:left={`${hsv.s * 100}%`} style:top={`${(1 - hsv.v) * 100}%`}></span>
    </div>
  </div>
  <input class="hue" type="range" min="0" max="360" step="1" value={hsv.h}
    aria-label={msg('settings.accentHue')}
    oninput={event => { hsv = { ...hsv, h: Number(event.currentTarget.value) }; emit(); }} />
</div>

<style>
  .palette { width: 100%; max-width: 44rem; margin-top: var(--space-4); }
  .color-field { display: flex; height: 18rem; border: 1px solid var(--border-strong); border-radius: var(--radius-md); overflow: hidden; }
  .sample { width: 22%; flex-shrink: 0; }
  .saturation { position: relative; flex: 1; background: linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, transparent), var(--hue); cursor: crosshair; touch-action: none; user-select: none; }
  .saturation:focus-visible { outline: 2px solid #fff; outline-offset: -4px; box-shadow: inset 0 0 0 5px #000; }
  .cursor { position: absolute; width: 1.6rem; height: 1.6rem; border: 2px solid #fff; border-radius: 50%; box-shadow: 0 0 0 1px #0009; transform: translate(-50%, -50%); pointer-events: none; }
  .hue { display: block; appearance: none; -webkit-appearance: none; width: 100%; height: 2.8rem; padding: 0; margin: var(--space-2) 0 0; background: transparent; cursor: pointer; touch-action: none; }
  .hue::-webkit-slider-runnable-track { height: .8rem; border-radius: 99px; background: linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00); }
  .hue::-moz-range-track { height: .8rem; border-radius: 99px; background: linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00); }
  .hue::-webkit-slider-thumb { appearance: none; -webkit-appearance: none; width: 2rem; height: 2rem; margin-top: -.6rem; border: 2px solid #fff; border-radius: 50%; background: transparent; box-shadow: 0 0 0 1px #0009; }
  .hue::-moz-range-thumb { width: 1.6rem; height: 1.6rem; border: 2px solid #fff; border-radius: 50%; background: transparent; box-shadow: 0 0 0 1px #0009; }
</style>
