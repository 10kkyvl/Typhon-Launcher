import { describe, expect, it } from 'vitest';

/**
 * Hints go through Tooltip (on the layer stack, reachable by keyboard, a description where they add
 * to the name) and the shared hover intent. Two ways back to what was there before are held here:
 *
 * - a native `title` on a control (`button`, `a`, `input`, `select`, `textarea`, `[role=button]`):
 *   shown late and unstyled, never on keyboard focus, read twice or not at all by screen readers.
 *   Big Picture and the overlay window are not on this path yet. Named exceptions: IconButton's
 *   `tooltip={false}` (the overlay keeps the native title) and ActivityDock (replaced by the activity
 *   widget). A `title` on non-interactive text (the full form of a truncated value, an exact time) is
 *   not a control's and stays;
 * - a component of its own that waits on hover with a timer: the delays, the grace and the sticky
 *   dismissal live in hoverIntent.ts only.
 */

type Fs = {
  readFileSync(path: URL, encoding: 'utf8'): string;
  readdirSync(path: URL, options: { recursive: true }): string[];
};
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

const SRC = new URL('../../', import.meta.url);
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');
const files = fs
  .readdirSync(SRC, { recursive: true })
  .map((path) => path.replaceAll('\\', '/'))
  .filter((path) => /\.(svelte|ts)$/.test(path) && !/\.test\.ts$/.test(path));

const NOT_YET = [/^lib\/bigpicture\//, /^overlay\//];
const TITLE_EXCEPTIONS = ['lib/components/IconButton.svelte', 'lib/components/ActivityDock.svelte'];

// Markup only: script and style blocks blanked, so line numbers stay.
function markup(text: string): string {
  return text.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, (block) => block.replace(/[^\n]/g, ' '));
}

// Every opening tag of an HTML element, with its attributes as written outside braces: each
// expression, at any depth of braces, becomes `{}`, the shorthand `{title}` stays as it is.
function tags(html: string): { name: string; attrs: string; at: number }[] {
  const found: { name: string; attrs: string; at: number }[] = [];
  for (const start of html.matchAll(/<([a-z][\w-]*)\b/g)) {
    let attrs = '';
    let expression = '';
    let depth = 0;
    for (let i = start.index + start[0].length; i < html.length && !(depth === 0 && html[i] === '>'); i += 1) {
      const c = html[i];
      if (depth === 0 && c !== '{') attrs += c;
      else if (c === '{') {
        depth += 1;
        if (depth > 1) expression += c;
      } else if (c === '}') {
        depth -= 1;
        if (depth > 0) expression += c;
        else {
          attrs += expression.trim() === 'title' ? '{title}' : '{}';
          expression = '';
        }
      } else expression += c;
    }
    found.push({ name: start[1], attrs, at: start.index });
  }
  return found;
}

/** `path:line <tag>` for every native title on a control in the markup of `path`. */
function titledControls(path: string, text: string): string[] {
  const html = markup(text);
  return tags(html)
    .filter(({ name, attrs }) => {
      const control = ['button', 'a', 'input', 'select', 'textarea'].includes(name) || /\brole=["']button["']/.test(attrs);
      return control && /(^|\s)(title=|\{title\})/.test(attrs);
    })
    .map(({ name, at }) => `${path}:${html.slice(0, at).split('\n').length} <${name}>`);
}

describe('hints', () => {
  it('reads the sources', () => {
    expect(files).toContain('lib/components/Tooltip.svelte');
    expect(files).toContain('lib/components/hoverIntent.ts');
  });

  it('finds a native title on a control, on one line or over several', () => {
    expect(titledControls('x.svelte', '<button title={a}>x</button>')).toEqual(['x.svelte:1 <button>']);
    expect(titledControls('x.svelte', '<div>\n  <input\n    type="text"\n    {title}\n  />\n</div>')).toEqual(['x.svelte:2 <input>']);
    expect(titledControls('x.svelte', '<span role="button" tabindex="0" title="t">x</span>')).toEqual(['x.svelte:1 <span>']);
    expect(titledControls('x.svelte', '<span title={full}>{short}</span><button aria-label={a}>x</button>')).toEqual([]);
    expect(titledControls('x.svelte', '<script>const b = "<button title=1>";</script><p>x</p>')).toEqual([]);
    expect(titledControls('x.svelte', '<button onclick={() => go({ a: 1 })} title={t}>x</button>')).toEqual(['x.svelte:1 <button>']);
    expect(titledControls('x.svelte', '<input bind:value={title} />')).toEqual([]);
  });

  it('puts no native title on a control outside Big Picture and the overlay', () => {
    const found = files
      .filter((path) => path.endsWith('.svelte') && !NOT_YET.some((re) => re.test(path)) && !TITLE_EXCEPTIONS.includes(path))
      .flatMap((path) => titledControls(path, read(path)));
    expect(found).toEqual([]);
  });

  it('keeps the named exceptions true: each still has the title it is excused for', () => {
    for (const path of TITLE_EXCEPTIONS) expect(titledControls(path, read(path)), path).not.toEqual([]);
  });

  it('times hover in the hover intent only', () => {
    const hover = /\b(pointerenter|mouseenter|pointerover|mouseover)\b/;
    const timer = /\b(setTimeout|setInterval)\s*\(/;
    const own = files.filter((path) => path !== 'lib/components/hoverIntent.ts' && hover.test(read(path)) && timer.test(read(path)));
    expect(own).toEqual([]);
  });
});
