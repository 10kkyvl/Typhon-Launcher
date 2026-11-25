import { describe, expect, it } from 'vitest';
import module from './diskUsagePage.svelte.ts?raw';

const parts = import.meta.glob<string>(['./*.svelte'], { query: '?raw', import: 'default', eager: true });

// The page object exposes its state through getters, and a write to a field without a setter throws
// only when the player gets there, unseen by the type check of a `bind:`. Today no part writes or
// binds a field (the confirmation closes through `closePending()`); a new write needs a setter.
describe('the disk usage page object', () => {
  it('reads every part of the page', () => {
    expect(Object.keys(parts).sort()).toEqual([
      './DiskUsage.svelte',
      './DownloadsSection.svelte',
      './GamesSection.svelte',
      './GroupsSection.svelte',
      './LeftoversSection.svelte',
      './UsageRow.svelte',
      './UsageSection.svelte',
      './UsageWarnings.svelte',
    ]);
  });

  // A negative claim of the whole screen: Go's prose (a row's `reason`, a warning's `message`, an
  // error's text) never reaches the window; the words come from the codes. A source tripwire over every
  // file of the screen and of lib/diskusage; the dom tests prove the words shown.
  it('reads no Go prose anywhere in the screen', () => {
    const code = { ...parts, ...import.meta.glob<string>(['./*.ts', '!./*.test.ts', '!./diskUsageTesting.ts', '../../lib/diskusage/*.ts', '!../../lib/diskusage/*.test.ts'], { query: '?raw', import: 'default', eager: true }) };
    expect(Object.keys(code)).toContain('./diskUsagePage.svelte.ts');
    expect(Object.keys(code)).toContain('../../lib/diskusage/usage.ts');
    const raw = Object.entries(code).flatMap(([path, text]) =>
      [...text.replace(/<style[\s\S]*?<\/style>/g, '').matchAll(/(?<!diskusage)\.(reason|message)\b(?!Code|\.)/g)].map((m) => `${path}: .${m[1]}`),
    );
    expect(raw).toEqual([]);
  });

  it('has a setter for every field a part writes or binds', () => {
    const written = new Set<string>();
    for (const text of Object.values(parts)) {
      for (const m of text.matchAll(/\bpage\.([\w$]+)\s*=(?!=)/g)) written.add(m[1]);
      for (const m of text.matchAll(/bind:[\w$]+=\{page\.([\w$]+)\}/g)) written.add(m[1]);
    }
    expect([...written].sort()).toEqual([]);
    const missing = [...written].filter((name) => !new RegExp(`\\bset ${name}\\(`).test(module));
    expect(missing).toEqual([]);
  });
});
