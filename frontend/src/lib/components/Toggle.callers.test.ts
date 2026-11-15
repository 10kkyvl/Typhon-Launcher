import { describe, expect, it } from 'vitest';
import { fromSettingsRow, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use Toggle, read from their markup. With `onchange` the caller holds the state
 * and Toggle never writes it (Toggle.svelte), so a `bind:checked` next to `onchange` would never be
 * written: such a caller sets its own state in `onchange`.
 *
 * Limit: see lib/testing/markup.ts; `labelledby` and `describedby` are checked only for an `id=` with
 * the same expression inside the element the switch sits in, or the ids a SettingsRow hands it.
 */

const toggles = (path: string, text: string): Use[] => usesIn(path, text, 'Toggle');

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'));
const uses = callers.flatMap(([path, text]) => toggles(path, text));

// The attribute itself, not one whose name only ends the same way (`data-onchange=`).
const binds = (tag: string) => /(?<![\w:-])bind:checked(?![\w-])/.test(tag);
const reports = (tag: string) => /(?<![\w:-])onchange=|\{\s*onchange\s*\}/.test(tag);

function bothWays(tag: string): boolean {
  return binds(tag) && reports(tag);
}

// A switch with neither does nothing when pressed: nothing reads what it would write.
function wired(tag: string): boolean {
  return binds(tag) || reports(tag);
}

function spreads(tag: string): boolean {
  return /\{\s*\.\.\./.test(tag.replace(/=\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}/g, ''));
}

// The name comes from the visible label next to the switch (`labelledby`, with the same expression as
// an `id=` inside the element the switch sits in: its row); `label` is for a switch whose row shows no
// label of its own, so a `label` whose text the same file also shows as an element's content is a
// hidden copy and is refused.
function named({ path, tag, row }: Use, text = frontendSources[path] ?? ''): boolean {
  const own = /\blabel=\{(msg\('[\w.]+'\))\}/.exec(tag)?.[1];
  if (own) return !text.includes(`>{${own}}<`);
  const by = /\blabelledby=("[^"]*"|\{[^}]*\})/.exec(tag)?.[1];
  return by !== undefined && (row.includes(`id=${by}`) || fromSettingsRow(row, 'labelId', by));
}

// The description, where there is one, is the row's own text too. A description shown only in some
// rows is written `{cond ? `${a}-sub` : undefined}`: each template in it is the id `"{a}-sub"`.
function describedInRow({ tag, row }: Use): boolean {
  const by = /\bdescribedby=("[^"]*"|\{(?:[^{}]|\{[^{}]*\})*\})/.exec(tag)?.[1];
  if (by === undefined) return true;
  const templates = [...by.matchAll(/`([^`]*)`/g)].map((m) => `"${m[1].replace(/\$\{/g, '{')}"`);
  return [by, ...templates].some((id) => row.includes(`id=${id}`)) || fromSettingsRow(row, 'hintId', by);
}

const at = (row: string, tag: string): Use => ({ path: 'x.svelte', tag, row });

describe('Toggle call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBeGreaterThanOrEqual(50);
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<Toggle\b/g)?.length ?? 0), 0));
  });

  it('finds the row a switch sits in', () => {
    const text = '<div class="row"><span id="a">A</span>{#if x > 1}<Toggle labelledby="a" />{/if}</div><span id="b">B</span>';
    expect(toggles('x.svelte', text).map((use) => use.row)).toEqual(['<div class="row"><span id="a">A</span>{#if x > 1}<Toggle labelledby="a" />{/if}</div>']);
  });

  it('name every switch: from the visible label in its row, or with label where none is shown', () => {
    const row = '<div class="row"><span id="row">Sound</span><Toggle /></div>';
    expect(named(at(row, "<Toggle checked={on} label={msg('a.b')} />"))).toBe(true);
    expect(named(at(row, "<Toggle checked={on} label={msg('a.b')} />"), "<span>{msg('a.b')}</span>")).toBe(false);
    expect(named(at(row, '<Toggle checked={on} labelledby="row" />'))).toBe(true);
    expect(named(at(row, '<Toggle checked={on} />'))).toBe(false);
    expect(named(at(row, '<Toggle checked={on} labelledby="nowhere" />'))).toBe(false);
    // The label is in the file, but in another row.
    expect(named(at('<div><Toggle /></div>', '<Toggle checked={on} labelledby="row" />'), row)).toBe(false);
    const settings = '<SettingsRow label="Sound">{#snippet children({ labelId, hintId })}<Toggle labelledby={labelId} />{/snippet}</SettingsRow>';
    expect(named(at(settings, '<Toggle labelledby={labelId} />'))).toBe(true);
    expect(named(at(settings.replace('labelId, ', ''), '<Toggle labelledby={labelId} />'))).toBe(false);
    expect(named(at('<div>{#snippet children({ labelId })}<Toggle />{/snippet}</div>', '<Toggle labelledby={labelId} />'))).toBe(false);
    expect(uses.filter((use) => !named(use)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('describe a switch by the text of its own row', () => {
    const row = '<div class="row"><span id="sub">Keeps it running</span><Toggle /></div>';
    expect(describedInRow(at(row, '<Toggle labelledby="row" describedby="sub" />'))).toBe(true);
    expect(describedInRow(at(row, '<Toggle labelledby="row" />'))).toBe(true);
    expect(describedInRow(at(row, '<Toggle labelledby="row" describedby="elsewhere" />'))).toBe(false);
    const some = '<div><span id="{uid}-sub">Only here</span><Toggle /></div>';
    expect(describedInRow(at(some, '<Toggle describedby={shown ? `${uid}-sub` : undefined} />'))).toBe(true);
    expect(describedInRow(at(some, '<Toggle describedby={shown ? `${uid}-other` : undefined} />'))).toBe(false);
    const settings = '<SettingsRow label="Sound" hint="Plays a chime">{#snippet children({ labelId, hintId })}<Toggle />{/snippet}</SettingsRow>';
    expect(describedInRow(at(settings, '<Toggle labelledby={labelId} describedby={hintId} />'))).toBe(true);
    expect(describedInRow(at(settings.replace(', hintId', ''), '<Toggle labelledby={labelId} describedby={hintId} />'))).toBe(false);
    expect(uses.filter((use) => !describedInRow(use)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('bind the state or report it with onchange: a switch with neither is dead', () => {
    expect(wired('<Toggle bind:checked={x} />')).toBe(true);
    expect(wired('<Toggle checked={x} onchange={save} />')).toBe(true);
    expect(wired('<Toggle checked={x} {onchange} />')).toBe(true);
    expect(wired('<Toggle checked={x} labelledby="a" />')).toBe(false);
    expect(wired('<Toggle checked={x} data-onchange="a" />')).toBe(false);
    expect(wired('<Toggle checked={x} data-bind:checked={y} />')).toBe(false);
    expect(uses.filter(({ tag }) => !wired(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('bind the state or hold it with onchange, never both', () => {
    expect(bothWays('<Toggle bind:checked={x} onchange={save} />')).toBe(true);
    expect(bothWays('<Toggle bind:checked {onchange} />')).toBe(true);
    expect(bothWays('<Toggle checked={x} {onchange} />')).toBe(false);
    expect(bothWays('<Toggle bind:checked={x} />')).toBe(false);
    expect(uses.filter(({ tag }) => bothWays(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  // A spread hides which of the two ways a caller took, and whether it names the switch.
  it('spread nothing onto a switch', () => {
    expect(spreads('<Toggle {...props} />')).toBe(true);
    expect(spreads('<Toggle checked={[...a].length > 0} label={msg("x")} />')).toBe(false);
    expect(uses.filter(({ tag }) => spreads(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});
