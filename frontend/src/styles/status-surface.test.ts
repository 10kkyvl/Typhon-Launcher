import { describe, expect, it } from 'vitest';
import { frontendSources, goSources } from '../lib/testing/sources';
import { luminance, rgb } from '../lib/theme/accent';

/**
 * Status messages sit on a surface. The window backdrop is the game's art, and a status colour straight
 * on it does not keep its contrast, so an error, a warning or a success is shown on the opaque plate of
 * a Notice (or on a surface the page already has), never as coloured text on the bare page.
 */

type RGB = [number, number, number];
const BLACK: RGB = [0, 0, 0];

function presets(): Map<string, Record<string, string>> {
  const source = goSources['internal/theme/presets.go'];
  expect(source).toBeTypeOf('string');
  const out = new Map<string, Record<string, string>>();
  for (const m of source.matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)) {
    const id = /\bID:\s*"([^"]+)"/.exec(m[1])?.[1];
    expect(id).toBeTypeOf('string');
    out.set(id as string, Object.fromEntries([...m[1].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]])));
  }
  return out;
}

function ratio(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

const mix = (a: RGB, b: RGB, share: number): RGB => [0, 1, 2].map((i) => a[i] * share + b[i] * (1 - share)) as RGB;

const NOTICE = 'lib/components/Notice.svelte';

function styleOf(path: string): string {
  const source = frontendSources[path];
  expect(source, path).toBeTypeOf('string');
  return /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
}

describe('the plate of a notice', () => {
  const css = styleOf(NOTICE);
  const plate = /\.notice\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
  const fill = /background:\s*color-mix\(in srgb, var\(--notice-tone\) (\d+)%, var\((--[\w-]+)\)\)/.exec(plate);
  const tones = Object.fromEntries(
    [...css.matchAll(/\.notice\[data-tone='(\w+)'\]\s*\{\s*--notice-tone:\s*var\((--[\w-]+)\)/g)].map((m) => [m[1], m[2]]),
  );
  const base = /--notice-tone:\s*var\((--[\w-]+)\)/.exec(plate)?.[1];

  it('is the tone mixed into an opaque surface, never a translucent tint', () => {
    expect(fill, plate).not.toBeNull();
    expect((fill as RegExpExecArray)[2]).toMatch(/^--surface(-\d)?$/);
    expect(plate).not.toMatch(/-subtle|rgba\(|transparent|opacity/);
    expect(base).toBe('--accent-bright');
    expect(tones).toEqual({ success: '--success', warning: '--warning', danger: '--danger' });
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps a notice's text at 4.5:1 and its icon at 3:1 on the plate, whatever the tone`, () => {
      const tokens = presets().get(id);
      expect(tokens, id).toBeDefined();
      const t = tokens as Record<string, string>;
      const share = Number((fill as RegExpExecArray)[1]) / 100;
      const surface = rgb(t[(fill as RegExpExecArray)[2]], rgb(t['--bg'], BLACK));
      const failures: string[] = [];
      for (const [tone, token] of Object.entries({ info: base as string, ...tones })) {
        const colour = rgb(t[token], surface);
        const on = mix(colour, surface, share);
        for (const [name, min] of [['--text', 4.5], ['--text-2', 4.5]] as const) {
          const r = ratio(rgb(t[name], on), on);
          if (r < min) failures.push(`${tone}: ${name} ${r.toFixed(2)} < ${min}`);
        }
        const icon = ratio(colour, on);
        if (icon < 3) failures.push(`${tone}: icon ${icon.toFixed(2)} < 3`);
      }
      expect(failures).toEqual([]);
    });
  }

  it('paints its text in the text colours and only its icon in the tone', () => {
    const coloured = [...css.matchAll(/([^{}]+)\{([^}]*)\}/g)]
      .filter((m) => /(^|[^-])color:\s*var\(--notice-tone\)/.test(m[2]))
      .map((m) => m[1].trim());
    expect(coloured).toEqual(['.icon']);
    expect(css).not.toMatch(/(^|[^-])color:\s*var\(--(danger|warning|success)\)/);
  });
});
