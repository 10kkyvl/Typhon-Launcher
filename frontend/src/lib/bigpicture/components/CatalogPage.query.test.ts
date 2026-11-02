import { describe, expect, it } from 'vitest';
import catalogPage from './CatalogPage.svelte?raw';
import shell from '../../components/BigPictureShell.svelte?raw';

describe('поиск из ссылки в Big Picture', () => {
  it('оболочка кладёт запрос ссылки в новую страницу каталога', () => {
    expect(shell).toContain("navigate({ name: 'catalog', query })");
    expect(shell).toContain('initialQuery={page.page.query}');
  });

  it('каталог стартует с этого запроса, первая загрузка идёт через него', () => {
    expect(catalogPage).toContain('let query = $state(untrack(() => initialQuery));');
    expect(catalogPage).toContain('catalog.load({ search: query, genre, sort }, nextPage, pageSize)');
  });

  it('запрос нигде не вставляется как HTML', () => {
    expect(catalogPage).not.toContain('{@html');
    expect(shell).not.toContain('{@html');
  });
});
