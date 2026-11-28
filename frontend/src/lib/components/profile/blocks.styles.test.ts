import { describe, expect, it } from 'vitest';
import { declarations, ruleBody, styleOf, topLevel } from '../../testing/css';
import { luminance, rgb } from '../../theme/accent';

type RGB = [number, number, number];

/**
 * How the profile's blocks are painted: in tokens, never a colour of their own; what stands over a
 * game's picture in the fixed --art-* colours (styles/shell.css), which do not follow the theme because
 * the picture and not the theme is under them, measured over a white and a black picture; spacing on
 * the --space-* scale. A source tripwire: what a light preset paints over real artwork is checked in the
 * running app.
 */
const sources = import.meta.glob<string>('./*.svelte', { query: '?raw', import: 'default', eager: true });

const css = (name: string) => styleOf(sources[`./${name}.svelte`], name);
// Read from disk: a stylesheet imported in a test is no text.
type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const shellCss = fs.readFileSync(new URL('../../../styles/shell.css', import.meta.url), 'utf8');
const shell = Object.fromEntries([...shellCss.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));

const WHITE: RGB = [255, 255, 255];
const BLACK: RGB = [0, 0, 0];

function token(value: string): string {
  const name = /^var\((--[\w-]+)\)$/.exec(value.trim())?.[1];
  expect(name, `${value} is one fixed token`).toBeTypeOf('string');
  const fixed = shell[name as string];
  expect(fixed, `${name} is in shell.css`).toBeTypeOf('string');
  return fixed;
}

function ratio(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** The words' colour on the plate over the worst picture: a white one and a black one. */
function worst(words: string, plate: string): number {
  return Math.min(...[WHITE, BLACK].map((picture) => {
    const under = rgb(token(plate), picture);
    return ratio(rgb(token(words), under), under);
  }));
}

const COLOUR_LITERAL = /#[0-9a-f]{3,8}\b|\brgba?\(|\bhsla?\(|\b(?:white|black)\b/i;

describe("the profile blocks' paint", () => {
  it('is all tokens: no colour is written in a block', () => {
    const literal = Object.entries(sources).flatMap(([path, source]) =>
      /<style>/.test(source)
        ? declarations(styleOf(source, path)).filter((d) => COLOUR_LITERAL.test(d.value)).map((d) => `${path}: ${d.prop}: ${d.value}`)
        : [],
    );
    expect(literal).toEqual([]);
  });

  it("the pinned game's words stand on the shade over its picture in the fixed art colours, at 4.5:1 over any picture", () => {
    const pinned = css('PinnedBlock');
    const stops = topLevel(/^linear-gradient\(([\s\S]*)\)$/.exec(ruleBody(pinned, '.shade').background)?.[1] ?? '');
    const foot = /^(var\(--[\w-]+\))/.exec(stops.at(-1) ?? '')?.[1] ?? '';

    expect(ruleBody(pinned, '.info').color).toBe('var(--art-text)');
    for (const part of ['.eyebrow', '.title', '.meta']) expect(ruleBody(pinned, part).color, part).toBeUndefined();
    expect(worst(ruleBody(pinned, '.info').color, foot)).toBeGreaterThanOrEqual(4.5);
  });

  it("a collection's marks on a cover stand on the art plate in the art colour, at 4.5:1 over any picture", () => {
    const collection = css('CollectionBlock');
    for (const mark of ['.heart', '.mastered']) {
      const rule = ruleBody(collection, mark);
      expect(worst(rule.color, rule.background), mark).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('spaces the read-only blocks on the scale: a gap, a padding, a margin or an offset is a --space-* token, 0, auto, or a hairline', () => {
    const SPACING = /^(?:gap|row-gap|column-gap|padding(?:-\w+)?|margin(?:-\w+)?|inset|top|right|bottom|left)$/;
    const off = ['PinnedBlock', 'CollectionBlock', 'CompletedBlock', 'GenresBlock', 'FingerprintBlock', 'TextBlock', 'AboutBlock', 'BlockGrid'].flatMap((name) =>
      declarations(css(name))
        .filter((d) => SPACING.test(d.prop))
        .filter((d) => d.value.split(/\s+/).some((part) => !/^(?:0|auto|var\(--space-\d+\)|0\.2rem|1px|2px|-?var\(--[\w-]+\))$/.test(part) && !/^calc\(/.test(part)))
        .map((d) => `${name}: ${d.prop}: ${d.value}`),
    );
    expect(off).toEqual([]);
  });
});
