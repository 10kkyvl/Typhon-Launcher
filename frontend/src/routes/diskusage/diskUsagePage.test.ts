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
