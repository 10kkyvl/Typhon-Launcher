import { describe, expect, it } from 'vitest';
import source from './ThemePreview.svelte?raw';
import { ru } from '../../lib/i18n/catalog/ru';
import { en } from '../../lib/i18n/catalog/en';
import { tokenGroups } from '../../lib/theme/tokens';

const notPreviewed = new Set([
  '--page-x',
  '--page-max',
  '--prose-max',
  '--sidebar-w',
  '--topbar-h',
  '--space-8',
  '--space-10',
  '--space-12',
  '--cut',
  '--ease',
  '--ease-spring',
  '--dur',
  '--dur-fast',
  '--dur-panel',
  '--dur-slow',
  '--shadow-lift',
  '--font-hero',
]);

const allTokens = tokenGroups.flatMap((g) => g.tokens);

describe('ThemePreview', () => {
  it('only excludes tokens that exist', () => {
    for (const name of notPreviewed) {
      expect(allTokens, name).toContain(name);
    }
  });

  it('uses every theme token that is not explicitly excluded', () => {
    const missing = allTokens.filter((name) => !notPreviewed.has(name) && !source.includes(`var(${name})`) && !source.includes(`var(${name},`));
    expect(missing).toEqual([]);
  });

  it('does not truncate or clip text', () => {
    expect(source).not.toContain('text-overflow: ellipsis');
    expect(source).not.toContain('white-space: nowrap');
  });

  it('has a label in both languages for every preview message key', () => {
    const keys = [...source.matchAll(/msg\('(settings\.themePreview\w+)'\)/g)].map((m) => m[1]);
    expect(keys.length).toBeGreaterThan(0);
    for (const key of keys) {
      expect(ru, key).toHaveProperty(key);
      expect(en, key).toHaveProperty(key);
    }
  });
});
