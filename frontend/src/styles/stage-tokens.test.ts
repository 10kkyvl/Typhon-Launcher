import { describe, expect, it } from 'vitest';

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
// stage and the text plate, and this snapshot holds the launcher to its values. One deliberate departure:
// the prototype's shades are a fixed near-black (rgba(8, 9, 10, a)) with a per-theme override; here a theme
// cannot override the fixed stage tokens (В10), so the shades take the page colour at the prototype's alphas.
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
      '--scrim-copy': 'color-mix(in srgb, var(--bg) 66%, transparent)',
      '--copy-fade-x': '24rem',
      '--copy-fade-y': '8rem',
    });
  });
});
