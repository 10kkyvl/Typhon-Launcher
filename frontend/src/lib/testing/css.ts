import type { RGBA } from '../theme/contrastReport';

// For guards that read a component's styles and measure what they draw, against a token set.

/** The style block of a component's source, without comments. */
export function styleOf(source: string, name: string): string {
  const block = /<style>([\s\S]*)<\/style>/.exec(source)?.[1];
  if (block === undefined) throw new Error(`${name} has no style block`);
  return block.replace(/\/\*[\s\S]*?\*\//g, '');
}

export function declarations(css: string): { prop: string; value: string }[] {
  return [...css.matchAll(/(?:^|[;{])\s*([-\w]+)\s*:\s*([^;{}]+)/g)].map((m) => ({ prop: m[1], value: m[2].trim() }));
}

/** The declarations of the rule written with exactly this selector. */
export function ruleBody(css: string, selector: string): Record<string, string> {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const body = new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`, 'm').exec(css)?.[1];
  if (body === undefined) throw new Error(`no rule ${selector}`);
  return Object.fromEntries(declarations(`{${body}`).map((d) => [d.prop, d.value]));
}

/** Splits on the commas outside parentheses. */
export function topLevel(text: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '(') depth++;
    else if (text[i] === ')') depth--;
    else if (text[i] === ',' && depth === 0) {
      parts.push(text.slice(from, i).trim());
      from = i + 1;
    }
  }
  return [...parts, text.slice(from).trim()];
}

// What the browser computes for a colour written with tokens: var(), hex, rgb(a), transparent and
// color-mix() in srgb, which mixes premultiplied by alpha.
export function cssColor(expr: string, tokens: Record<string, string>): RGBA {
  const value = expr.trim();
  const ref = /^var\((--[\w-]+)\)$/.exec(value);
  if (ref) {
    const token = tokens[ref[1]];
    if (token === undefined) throw new Error(`${ref[1]} is not in the token set`);
    return cssColor(token, tokens);
  }
  if (value === 'transparent') return [0, 0, 0, 0];
  const hex = /^#([0-9a-f]{6})$/i.exec(value);
  if (hex) return [0, 2, 4].map((i) => parseInt(hex[1].slice(i, i + 2), 16)).concat(1) as RGBA;
  const fn = /^rgba?\(([^()]*)\)$/.exec(value);
  if (fn) {
    const [r, g, b, a = '1'] = fn[1].split(',').map((s) => s.trim());
    return [Number(r), Number(g), Number(b), Number(a)];
  }
  const mix = /^color-mix\(([\s\S]*)\)$/.exec(value);
  if (!mix) throw new Error(`cannot measure ${value}`);
  const [space, ...sides] = topLevel(mix[1]);
  if (space !== 'in srgb') throw new Error(`cannot mix ${space}`);
  const [a, b] = sides.map((side) => {
    const m = /^([\s\S]*?)(?:\s+([\d.]+)%)?$/.exec(side) as RegExpExecArray;
    return { c: cssColor(m[1], tokens), p: m[2] === undefined ? undefined : Number(m[2]) / 100 };
  });
  const p = a.p ?? (b.p === undefined ? 0.5 : 1 - b.p);
  const alpha = a.c[3] * p + b.c[3] * (1 - p);
  if (alpha === 0) return [0, 0, 0, 0];
  const channel = (i: number) => (a.c[i] * a.c[3] * p + b.c[i] * b.c[3] * (1 - p)) / alpha;
  return [channel(0), channel(1), channel(2), alpha];
}
