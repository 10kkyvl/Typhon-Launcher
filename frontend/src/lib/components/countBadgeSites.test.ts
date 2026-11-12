import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

/**
 * A CountBadge says its number only as part of the name of the control it sits in (CountBadge.svelte).
 * That holds only where it sits inside a control named by its text, after that text: inside a control
 * named by `aria-label` or `aria-labelledby` it is never heard, beside the control it is loose text,
 * before the text the name reads backwards. IconButton names its own button with the count
 * (`countLabel`) and is the one place that puts a badge inside an `aria-label`.
 *
 * Limits: markup is read as written; a control drawn by a snippet or a component other than the
 * listed ones is not seen as a control.
 */

const OWNER = 'lib/components/IconButton.svelte';
const CONTROLS = new Set(['button', 'a', 'Button', 'Chip', 'TextLink']);
const VOID = new Set(['area', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'source', 'track', 'wbr']);

// Markup only: script and style blocks blanked, so positions stay.
function markup(text: string): string {
  return text.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, (block) => block.replace(/[^\n]/g, ' '));
}

interface Open {
  name: string;
  attrs: string;
  end: number;
}

// The end of the tag starting at `from` (the index after its `>`), skipping braces in attributes, and
// its attributes with every expression replaced by `{}`.
function tagEnd(html: string, from: number): { end: number; attrs: string } {
  let depth = 0;
  let attrs = '';
  for (let i = from; i < html.length; i += 1) {
    const c = html[i];
    if (c === '{') depth += 1;
    else if (c === '}') {
      depth -= 1;
      if (depth === 0) attrs += '{}';
    } else if (depth === 0 && c === '>') return { end: i + 1, attrs };
    else if (depth === 0) attrs += c;
  }
  return { end: html.length, attrs };
}

/** `line problem` for every CountBadge in `text` that would not be heard as part of its control's name. */
function misplacedCounts(text: string): string[] {
  const html = markup(text);
  const found: string[] = [];
  const stack: Open[] = [];
  const line = (at: number) => html.slice(0, at).split('\n').length;
  for (const m of html.matchAll(/<(\/?)([A-Za-z][\w.:-]*)/g)) {
    const [, closing, name] = m;
    if (closing) {
      const at = stack.map((open) => open.name).lastIndexOf(name);
      if (at >= 0) stack.length = at;
      continue;
    }
    const { end, attrs } = tagEnd(html, m.index + m[0].length);
    const selfClosing = /\/\s*$/.test(attrs) || VOID.has(name);
    if (name === 'CountBadge') {
      const control = [...stack].reverse().find((open) => CONTROLS.has(open.name));
      if (!control) found.push(`${line(m.index)} outside a control`);
      else if (/(?:^|\s)aria-label(?:ledby)?=/.test(control.attrs)) found.push(`${line(m.index)} inside a control named by aria-label`);
      else {
        const before = html
          .slice(control.end, m.index)
          .replace(/\{[#:/@][^}]*\}/g, ' ')
          .replace(/<[^>]*>/g, ' ');
        if (!/\S/.test(before)) found.push(`${line(m.index)} before the control's text`);
      }
    }
    if (!selfClosing) stack.push({ name, attrs, end });
  }
  return found;
}

describe('a count is heard with its control', () => {
  it('every CountBadge sits inside a control named by its text, after that text', () => {
    const sites = Object.entries(frontendSources).filter(([path, text]) => path.endsWith('.svelte') && path !== OWNER && text.includes('<CountBadge'));
    expect(sites.length).toBeGreaterThanOrEqual(2);
    expect(sites.flatMap(([path, text]) => misplacedCounts(text).map((problem) => `${path}:${problem}`))).toEqual([]);
  });

  it('sees each way a badge goes unheard', () => {
    const named = `<button aria-label={name}><span>{label}</span><CountBadge count={n} label={t} /></button>`;
    const beside = `<div><button><span>{label}</span></button><CountBadge count={n} label={t} /></div>`;
    const first = `<button><CountBadge count={n} label={t} /><span>{label}</span></button>`;
    const fine = `<button type="button" onclick={() => go({ a: 1 })}><Icon size="2rem" /><span>{label}</span>{#if n > 0}<span class="count"><CountBadge count={n} label={t} /></span>{/if}</button>`;
    expect(misplacedCounts(named)).toEqual(['1 inside a control named by aria-label']);
    expect(misplacedCounts(beside)).toEqual(['1 outside a control']);
    expect(misplacedCounts(first)).toEqual(["1 before the control's text"]);
    expect(misplacedCounts(fine)).toEqual([]);
  });

  it('sees a real site broken: the sidebar item named by aria-label again', () => {
    const sidebar = frontendSources['lib/components/Sidebar.svelte'];
    expect(misplacedCounts(sidebar)).toEqual([]);
    const mutated = sidebar.replace('class="nav-item"', 'class="nav-item"\n    aria-label={item.label}');
    expect(mutated).not.toBe(sidebar);
    expect(misplacedCounts(mutated)).toHaveLength(2);
  });
});
