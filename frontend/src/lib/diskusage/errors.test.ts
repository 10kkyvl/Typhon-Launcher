import { afterEach, describe, expect, it } from 'vitest';
import { locale, msg } from '../i18n';
import { en } from '../i18n/catalog/en';
import { ru } from '../i18n/catalog/ru';
import { goSources } from '../testing/sources';
import { diskusageErrorText, isScanCancelled, partialText, REASONS } from './errors';

afterEach(() => locale.set('ru'));

// Every code the disk usage service raises itself, read from its Go source.
function goCodes(): string[] {
  const found = new Set<string>();
  for (const [path, text] of Object.entries(goSources)) {
    if (!path.startsWith('internal/diskusage/')) continue;
    for (const m of text.matchAll(/uierr\.(?:New|Wrap)\(\s*"(diskusage\.[a-z_]+)"/g)) found.add(m[1]);
  }
  return [...found].sort();
}

describe('the disk usage error table', () => {
  it('words every code the service raises, and no other', () => {
    const codes = goCodes();
    expect(codes).toContain('diskusage.scan_cancelled');
    expect(Object.keys(REASONS).sort()).toEqual(codes);
  });

  it('points every code at words both languages define', () => {
    for (const [code, key] of Object.entries(REASONS)) {
      expect(Object.hasOwn(ru, key), code).toBe(true);
      expect(Object.hasOwn(en, key), code).toBe(true);
    }
  });

  it.each(Object.entries(REASONS))('says %s in its own words, not the fallback', (code, key) => {
    expect(diskusageErrorText(new Error(`typhon:${code}: go text`), 'fallback')).toBe(msg(key));
  });

  it('hands an owner’s code to the install table the screen read before', () => {
    expect(diskusageErrorText(new Error('typhon:install.download_in_use: x'), 'fallback')).toBe(msg('errInstall.installDownloadInUse'));
    expect(diskusageErrorText(new Error('typhon:relocate.staging_refused: x'), 'fallback')).toBe(msg('errInstall.relocateStagingRefused'));
  });

  it('says the fallback, never Go’s text, for a code no table knows or no code at all', () => {
    expect(diskusageErrorText(new Error('typhon:diskusage.new_one: удалить нельзя'), 'fallback')).toBe('fallback');
    expect(diskusageErrorText(new Error('permission denied'), 'fallback')).toBe('fallback');
  });

  it('tells a cancelled scan apart from a failed one', () => {
    expect(isScanCancelled(new Error('typhon:diskusage.scan_cancelled: подсчёт места отменён'))).toBe(true);
    expect(isScanCancelled(new Error('typhon:diskusage.not_started: x'))).toBe(false);
    expect(isScanCancelled(undefined)).toBe(false);
  });

  // What was already gone is no more on the disk than what this delete took: the player counts both.
  it('says a partial group delete with what went and what stayed, in both languages', () => {
    expect(partialText({ deleted: 3, gone: 1, failed: 2 })).toBe('Удалено: 4, не удалось удалить: 2');
    locale.set('en');
    expect(partialText({ deleted: 3, gone: 1, failed: 2 })).toBe('4 deleted, 2 could not be deleted');
  });
});
