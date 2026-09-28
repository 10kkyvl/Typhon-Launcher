import { describe, expect, it } from 'vitest';
import { entry, popPage, pushPage } from './router';

describe('Big Picture page history', () => {
  it('retains the catalog instance and focus when returning from a game', () => {
    const catalog = entry({ name: 'catalog' });
    const pages = pushPage([catalog], { name: 'game', id: 'canonical-42' }, 'catalog-game:42');
    expect(popPage(pages)).toEqual([{ ...catalog, focus: 'catalog-game:42' }]);
    expect(catalog.focus).toBe('');
  });
  it('does not pop the last page', () => {
    const pages = [entry({ name: 'home' })];
    expect(popPage(pages)).toBe(pages);
  });
});
