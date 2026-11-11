import { describe, expect, it } from 'vitest';
import { balanced } from '../testing/scan';

/**
 * A confirmation whose action can reject shows the error in the dialog, so its text must come from the
 * action's domain: every `<ConfirmModal>` whose `onconfirm` may reject passes an explicit `errorText`.
 * The default (backend code → message, else "error") is only a safety net.
 *
 * "May reject", read from the caller's own file: the handler has no `catch`, or a `catch` that throws
 * again. The handler is the function `onconfirm` names or calls (`fn`, `() => fn(x)`), an inline
 * arrow's block, or, for `pending.run`, the `run` of every object assigned to `pending`, resolved the
 * same way. A handler that cannot be resolved in the file counts as one that may reject.
 *
 * Limits: only one level is followed (a handler that calls a helper which catches still needs
 * `errorText`, which is harmless); a rethrow hidden in a called function is not seen; a `pending`
 * assigned through a helper that builds the object elsewhere is not resolved.
 */

const sources = import.meta.glob<string>(['../../**/*.svelte'], { query: '?raw', import: 'default', eager: true });

function bodyAfter(text: string, at: number): string {
  const open = text.indexOf('{', at);
  return open < 0 ? '' : (balanced(text, open) ?? '');
}

function safe(body: string): boolean {
  const at = body.search(/\bcatch\b/);
  if (at < 0) return false;
  return !/\bthrow\b/.test(bodyAfter(body, at));
}

function definition(text: string, name: string): string | null {
  const found = new RegExp(`(?:function\\s+${name}\\s*\\(|(?:const|let)\\s+${name}\\s*=)`).exec(text);
  return found ? bodyAfter(text, found.index + found[0].length) : null;
}

// What runs for a handler expression: a named function, an arrow's block, or the function an arrow calls.
function resolve(text: string, expression: string): string | null {
  const value = expression.trim();
  const named = /^([\w$]+)$/.exec(value) ?? /^(?:async\s*)?\([^)]*\)\s*=>\s*(?:await\s+)?([\w$]+)\s*\(/.exec(value);
  if (named) return definition(text, named[1]);
  const arrow = /^(?:async\s*)?\([^)]*\)\s*=>\s*\{/.exec(value);
  return arrow ? bodyAfter(value, arrow[0].length - 1) : null;
}

// `pending.run`: the `run` of every object assigned to `pending` (to `pending` too when the dialog reads
// it through a `{@const current = pending}`).
function pendingRuns(text: string, base: string): (string | null)[] | null {
  const names = new Set([base, 'pending']);
  const runs: (string | null)[] = [];
  for (const name of names) {
    for (const m of text.matchAll(new RegExp(`\\b${name}\\s*=\\s*\\{`, 'g'))) {
      const object = bodyAfter(text, (m.index ?? 0) + m[0].length - 1);
      const run = /\brun\s*:\s*/.exec(object);
      if (!run) continue;
      const from = run.index + run[0].length;
      const rest = object.slice(from);
      const block = /^(?:async\s*)?\([^)]*\)\s*=>\s*\{/.exec(rest);
      const value = block ? rest.slice(0, block[0].length - 1) + bodyAfter(rest, block[0].length - 1) : rest.split(/,\s*\n|\n|\s*\}$/)[0];
      runs.push(resolve(text, value));
    }
  }
  return runs.length ? runs : null;
}

function handlerBodies(text: string, handler: string): (string | null)[] | null {
  const base = /^\s*([\w$]+)\.run\s*$/.exec(handler)?.[1];
  if (base) return pendingRuns(text, base);
  return [resolve(text, handler)];
}

function unmapped(): string[] {
  const missing: string[] = [];
  for (const [path, text] of Object.entries(sources)) {
    if (path.endsWith('/ConfirmModal.svelte') || path.includes('/testing/')) continue;
    for (const tag of text.matchAll(/<ConfirmModal\b/g)) {
      const end = text.indexOf('/>', tag.index);
      const props = text.slice(tag.index, end);
      if (/\berrorText=/.test(props)) continue;
      const at = props.indexOf('onconfirm=');
      const handler = at < 0 ? '' : (balanced(props, at + 'onconfirm='.length) ?? '');
      const bodies = handlerBodies(text, handler);
      if (bodies && bodies.every((body) => body !== null && safe(body))) continue;
      missing.push(`${path}:${text.slice(0, tag.index).split('\n').length} onconfirm={${handler.trim()}}`);
    }
  }
  return missing;
}

describe('ConfirmModal callers', () => {
  it('reads the callers', () => {
    expect(Object.values(sources).filter((text) => text.includes('<ConfirmModal')).length).toBeGreaterThan(20);
  });

  it('pass errorText wherever the action may reject', () => {
    expect(unmapped()).toEqual([]);
  });
});
