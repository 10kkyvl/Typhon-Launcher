import type { RGBA } from './contrastReport';

const SENTINEL = '#010203';

// Theme colours may be var() chains, color-mix() or oklch(); the browser is
// the only parser that agrees with what the window paints, so the value is
// computed on a probe that carries the theme's variables and then drawn on a
// canvas to read it back as sRGB.
export function resolveColors(vars: Record<string, string>, names: readonly string[]): Record<string, RGBA | undefined> {
  const out: Record<string, RGBA | undefined> = {};
  if (typeof document === 'undefined') return out;
  const probe = document.createElement('div');
  probe.style.cssText = 'position:fixed;width:0;height:0;overflow:hidden;visibility:hidden;pointer-events:none';
  for (const [name, value] of Object.entries(vars)) probe.style.setProperty(name, value);
  document.body.appendChild(probe);
  const canvas = document.createElement('canvas');
  canvas.width = 1;
  canvas.height = 1;
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  try {
    for (const name of names) {
      probe.style.color = '';
      probe.style.color = `var(${name})`;
      const computed = getComputedStyle(probe).color;
      if (!ctx || !computed) {
        out[name] = undefined;
        continue;
      }
      ctx.fillStyle = SENTINEL;
      ctx.fillStyle = computed;
      if (String(ctx.fillStyle).toLowerCase() === SENTINEL && computed.toLowerCase() !== SENTINEL) {
        out[name] = undefined;
        continue;
      }
      ctx.clearRect(0, 0, 1, 1);
      ctx.fillRect(0, 0, 1, 1);
      const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data;
      out[name] = [r, g, b, a / 255];
    }
  } finally {
    probe.remove();
  }
  return out;
}
