import { describe, expect, it } from 'vitest';
import module from './settingsPage.svelte.ts?raw';

const parts = import.meta.glob<string>(['./*.svelte'], { query: '?raw', import: 'default', eager: true });

// The page object exposes its state through accessors, and a write to a field without a setter throws
// only when the player gets there (a dialog that will not close), unseen by the type check of a `bind:`.
describe('the settings page object', () => {
  it('has a setter for every field a part writes or binds', () => {
    const written = new Set<string>();
    for (const text of Object.values(parts)) {
      for (const m of text.matchAll(/\bpage\.([\w$]+)\s*=(?!=)/g)) written.add(m[1]);
      for (const m of text.matchAll(/bind:[\w$]+=\{page\.([\w$]+)\}/g)) written.add(m[1]);
    }
    expect([...written].sort()).toEqual([
      'cloudConsentOpen',
      'cloudDisableOpen',
      'historyOpen',
      'legalOpen',
      'librarySetupOpen',
      'sentDataOpen',
      'sourcesNoticeReviewOpen',
    ]);
    const missing = [...written].filter((name) => !new RegExp(`\\bset ${name}\\(`).test(module));
    expect(missing).toEqual([]);
  });
});
