import { describe, expect, it, vi } from 'vitest';

vi.mock('../../lib/services/backend', () => ({ inWails: false }));

import { msg } from '../../lib/i18n';
import { overlayReason } from './settingsPage.svelte';
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

// The reason the overlay, screenshot and schedule saves show: a code's catalogued message, else nothing,
// so each place says its own catalogued text instead of Go's words.
describe('the reason of a settings save that failed', () => {
  it('is the message of a known code', () => {
    expect(overlayReason(new Error('typhon:overlay.hotkey_unavailable: register F12: taken'))).toBe(msg('overlay.hotkey_unavailable'));
  });

  it('is empty for an error without a known code', () => {
    expect(overlayReason(new Error('register F12: taken'))).toBe('');
    expect(overlayReason(new Error('typhon:nobody.knows_this'))).toBe('');
    expect(overlayReason('a plain string')).toBe('');
  });
});
