import { describe, expect, it } from 'vitest';
import { ruleBody, styleOf } from '../lib/testing/css';
import backdropSource from '../lib/components/ShellBackdrop.svelte?raw';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

function rootTokens(file: string): Record<string, string> {
  const css = fs.readFileSync(new URL(file, import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
  const start = css.indexOf(':root {');
  expect(start, file).toBeGreaterThanOrEqual(0);
  const body = css.slice(css.indexOf('{', start) + 1, css.indexOf('\n}', start));
  return Object.fromEntries([...body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
}

const pick = (tokens: Record<string, string>, names: string[]) => Object.fromEntries(names.map((name) => [name, tokens[name]]));

// Decision В12 of the redesign: the prototype (typhon-ui, src/styles/tokens.css) is the standard for the
// stage and the text plate, and this snapshot holds the launcher to its values. The plate is the
// prototype's near-black in the dark presets and a pale one in the light presets, as the prototype's
// blush. One deliberate departure: a theme cannot override the fixed top bar scrim (В10), so the scrim is
// drawn from the theme's own colours instead of a per-theme override.
describe('the stage and its text plate keep the prototype values', () => {
  it('geometry, focus, the top bar scrim and the mask stops (fixed, shell.css)', () => {
    expect(pick(rootTokens('./shell.css'), ['--stage-foot', '--stage-tail', '--stage-focus-x', '--stage-focus-y', '--stage-scrim', '--mask-solid', '--mask-most', '--mask-some'])).toEqual({
      '--stage-foot': '64vh',
      '--stage-tail': '6vh',
      '--stage-focus-x': '62%',
      '--stage-focus-y': '26%',
      '--stage-scrim': 'linear-gradient(180deg, color-mix(in srgb, var(--bg) 46%, transparent) 0, color-mix(in srgb, var(--bg) 16%, transparent) 8rem, transparent 14rem)',
      '--mask-solid': '#000',
      '--mask-most': 'rgba(0, 0, 0, 0.7)',
      '--mask-some': 'rgba(0, 0, 0, 0.28)',
    });
  });

  it('the plate under words on the stage (themable, tokens.css)', () => {
    expect(pick(rootTokens('./tokens.css'), ['--scrim-copy', '--copy-fade-x', '--copy-fade-y'])).toEqual({
      '--scrim-copy': 'rgba(8, 9, 10, 0.66)',
      '--copy-fade-x': '24rem',
      '--copy-fade-y': '8rem',
    });
  });
});

// The prototype's ShellBackdrop.svelte: the stage's height from the foot, the melt into the blurred
// picture, a plate 14 % taller than the stage, and the two fades of a change of game.
describe('the stage layer keeps the prototype geometry', () => {
  const css = styleOf(backdropSource, 'ShellBackdrop.svelte');
  const melt = 'linear-gradient(180deg, var(--mask-solid) 0, var(--mask-solid) 78%, var(--mask-most) 87%, var(--mask-some) 94%, transparent 100%)';

  it('the stage: from the top of the window to the foot and its tail, solid to 78 % and gone at its end', () => {
    expect(ruleBody(css, '.stage')).toMatchObject({
      position: 'absolute',
      inset: '0 0 auto',
      height: 'calc(var(--stage-foot) + var(--stage-tail))',
      '-webkit-mask-image': melt,
      'mask-image': melt,
    });
  });

  it('the plate: taller than the stage, the picture covering it from the focus point', () => {
    expect(ruleBody(css, '.plate').inset).toBe('0 0 -14%');
    expect(ruleBody(css, '.plate img')).toMatchObject({ 'object-fit': 'cover', 'object-position': 'var(--stage-focus-x) var(--stage-focus-y)' });
    expect(ruleBody(css, '.scrim').background).toBe('var(--stage-scrim)');
  });

  it('a change of game: the new picture arrives over --motion-backdrop, the old one leaves over 200 ms', () => {
    expect(backdropSource).toMatch(/way === 'in' \? tokenMs\('--motion-backdrop', 600\) : tokenMs\('--dur-panel', 200\)/);
  });
});

// The prototype's global.css, desktop rules: the plate hugs its block, eases out to the right and below,
// and is one text colour. The phone band (max-width 820px) belongs to the narrow layout, not ported yet.
describe('the text plate keeps the prototype rule', () => {
  const global = fs.readFileSync(new URL('./global.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

  it('.stage-copy: one text colour, a positioned block for its plate', () => {
    expect(ruleBody(global, '.stage-copy')).toEqual({
      '--copy-lead': '0rem',
      '--copy-rise': '0rem',
      '--text-2': 'var(--text)',
      position: 'relative',
    });
  });

  it('.stage-copy::before: the plate under the block, masked across and down', () => {
    expect(ruleBody(global, '.stage-copy::before')).toEqual({
      '--copy-across':
        'linear-gradient(90deg, transparent 0, var(--mask-solid) var(--copy-lead), var(--mask-solid) calc(100% - var(--copy-fade-x)), var(--mask-most) calc(100% - var(--copy-fade-x) * 0.72), var(--mask-some) calc(100% - var(--copy-fade-x) * 0.36), transparent 100%)',
      '--copy-down':
        'linear-gradient(180deg, transparent 0, var(--mask-some) calc(var(--copy-fade-y) * 0.5 + var(--copy-rise) * 0.4), var(--mask-most) calc(var(--copy-fade-y) * 0.8 + var(--copy-rise) * 0.75), var(--mask-solid) calc(var(--copy-fade-y) + var(--copy-rise)), var(--mask-solid) calc(100% - var(--copy-fade-y)), var(--mask-most) calc(100% - var(--copy-fade-y) * 0.7), var(--mask-some) calc(100% - var(--copy-fade-y) * 0.34), transparent 100%)',
      content: "''",
      position: 'absolute',
      // The prototype's -1, from the window's scale.
      'z-index': 'var(--z-backdrop)',
      inset: 'calc(-1 * var(--copy-fade-y)) calc(-1 * var(--copy-fade-x)) calc(-1 * var(--copy-fade-y)) calc(-1 * max(var(--page-x), var(--copy-lead)))',
      background: 'var(--scrim-copy)',
      '-webkit-mask-image': 'var(--copy-across), var(--copy-down)',
      '-webkit-mask-composite': 'source-in',
      'mask-image': 'var(--copy-across), var(--copy-down)',
      'mask-composite': 'intersect',
      'pointer-events': 'none',
    });
  });
});
