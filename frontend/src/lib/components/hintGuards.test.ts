import { describe, expect, it } from 'vitest';

/**
 * Hints go through Tooltip (on the layer stack, reachable by keyboard, a description where they add
 * to the name) and the shared hover intent. Two ways back to what was there before are held here:
 *
 * - a native `title` on a control (`button`, `a`, `input`, `select`, `textarea`, `[role=button]`):
 *   shown late and unstyled, never on keyboard focus, read twice or not at all by screen readers.
 *   Big Picture and the overlay window are not on this path yet. Named exceptions: IconButton, whose
 *   native title is its label only with `tooltip={false}` (the overlay) or while it is disabled with no
 *   reason (no focus, no tooltip, nothing announced twice), and ActivityDock (replaced by the activity widget).
 *   A component that spreads the attributes it is given onto its control (Button, Chip, IconButton,
 *   TextLink: found, not listed by hand) counts as that control. A `title` on non-interactive text
 *   (the full form of a truncated value, an exact time) is not a control's and stays;
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

// Every opening tag (an element or a component), with its attributes as written outside braces: each
// expression, at any depth of braces, becomes `{}`; the shorthand `{title}` and a spread of the
// component's own `rest` stay as they are.
function tags(html: string): { name: string; attrs: string; at: number }[] {
  const found: { name: string; attrs: string; at: number }[] = [];
  for (const start of html.matchAll(/<([A-Za-z][\w.-]*)\b/g)) {
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
          const inner = expression.trim();
          attrs += inner === 'title' ? '{title}' : inner === '...rest' ? '{...rest}' : '{}';
          expression = '';
        }
      } else expression += c;
    }
    found.push({ name: start[1], attrs, at: start.index });
  }
  return found;
}

const CONTROLS = ['button', 'a', 'input', 'select', 'textarea'];
const isControl = (name: string, attrs: string) => CONTROLS.includes(name) || /\brole=["']button["']/.test(attrs);

/** Components (file names) whose markup spreads the attributes they are given onto a control. */
function forwardersIn(sources: { path: string; text: string }[]): string[] {
  return sources
    .filter(({ text }) => tags(markup(text)).some(({ name, attrs }) => isControl(name, attrs) && attrs.includes('{...rest}')))
    .map(({ path }) => path.slice(path.lastIndexOf('/') + 1).replace(/\.svelte$/, ''))
    .sort();
}

/** `path:line <tag>` for every native title on a control, or on a component that forwards it to one, in `path`. */
function titledControls(path: string, text: string, forwarders: readonly string[] = []): string[] {
  const html = markup(text);
  return tags(html)
    .filter(({ name, attrs }) => (isControl(name, attrs) || forwarders.includes(name)) && /(^|\s)(title=|\{title\})/.test(attrs))
    .map(({ name, at }) => `${path}:${html.slice(0, at).split('\n').length} <${name}>`);
}

/** The source times something on hover itself: it listens for the pointer coming over and starts a timer. */
function timesHover(text: string): boolean {
  return /(pointerenter|mouseenter|pointerover|mouseover)\b/i.test(text) && /\b(setTimeout|setInterval)\s*\(/.test(text);
}

const svelteSources = () => files.filter((path) => path.endsWith('.svelte')).map((path) => ({ path, text: read(path) }));

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

  it('counts a component that spreads what it is given onto its control as that control', () => {
    const chip = { path: 'lib/components/Chip.svelte', text: '<script>let { ...rest } = $props();</script><button {...rest}>x</button>' };
    const card = { path: 'lib/components/Card.svelte', text: '<section><h2>{title}</h2></section>' };
    expect(forwardersIn([chip, card])).toEqual(['Chip']);
    expect(titledControls('x.svelte', '<Chip title={hint} onclick={go}>x</Chip>', ['Chip'])).toEqual(['x.svelte:1 <Chip>']);
    expect(titledControls('x.svelte', '<Card title={heading}>x</Card>', ['Chip'])).toEqual([]);
  });

  it('knows the components that forward attributes to a control', () => {
    expect(forwardersIn(svelteSources())).toEqual(['Button', 'Checkbox', 'Chip', 'IconButton', 'SearchInput', 'TextLink', 'Toggle']);
  });

  it('sees a hover timer in a Svelte event attribute as well as in addEventListener', () => {
    expect(timesHover('<div onmouseenter={() => (timer = setTimeout(show, 300))}>x</div>')).toBe(true);
    expect(timesHover("node.addEventListener('pointerenter', () => setTimeout(show, 300));")).toBe(true);
    expect(timesHover('<div onpointerover={show}>x</div>')).toBe(false);
  });

  it('puts no native title on a control outside Big Picture and the overlay', () => {
    const forwarders = forwardersIn(svelteSources());
    const found = files
      .filter((path) => path.endsWith('.svelte') && !NOT_YET.some((re) => re.test(path)) && !TITLE_EXCEPTIONS.includes(path))
      .flatMap((path) => titledControls(path, read(path), forwarders));
    expect(found).toEqual([]);
  });

  it('keeps the named exceptions true: each still has the title it is excused for', () => {
    for (const path of TITLE_EXCEPTIONS) expect(titledControls(path, read(path)), path).not.toEqual([]);
  });

  it("gives IconButton's native title its label only without a tooltip or while disabled without a reason", () => {
    const source = read('lib/components/IconButton.svelte');
    expect(source).toContain('const nativeTitle = $derived(!tooltip || (disabled && !refused) ? label : undefined);');
    expect(titledControls('lib/components/IconButton.svelte', source)).toHaveLength(1);
    expect(markup(source).match(/\btitle=\{[^}]*\}/g)).toEqual(['title={nativeTitle}']);
  });

  it('times hover in the hover intent only', () => {
    const own = files.filter((path) => path !== 'lib/components/hoverIntent.ts' && timesHover(read(path)));
    expect(own).toEqual([]);
  });
});
