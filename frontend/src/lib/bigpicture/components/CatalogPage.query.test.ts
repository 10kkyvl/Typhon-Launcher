import { describe, expect, it } from 'vitest';
import catalogPage from './CatalogPage.svelte?raw';
import shell from '../../components/BigPictureShell.svelte?raw';

describe('поиск из ссылки в Big Picture', () => {
  it('оболочка кладёт текст ссылки в новую страницу каталога черновиком', () => {
    expect(shell).toContain("navigate({ name: 'catalog', draft })");
    expect(shell).toContain('initialDraft={page.page.draft}');
  });

  it('первая загрузка идёт без черновика: запросы читают только query', () => {
    expect(catalogPage).toContain("let query = $state('');");
    expect(catalogPage).toContain('let draft = $state(untrack(() => initialDraft));');
    expect(catalogPage).toContain('catalog.load({ search: query, genre, sort }, nextPage, pageSize)');
    expect(catalogPage).not.toMatch(/search:\s*draft/);
  });

  it('черновик виден на кнопке поиска и уходит в запрос только после подтверждения', () => {
    expect(catalogPage).toContain("{draft || query || bp('bp.catalog.search')}");
    expect(catalogPage).toContain('initialValue: draft || query,');
    expect(catalogPage).toMatch(/if \(result === null\) return;\s*draft = '';\s*query = result\.trim\(\);/);
  });

  it('запрос нигде не вставляется как HTML', () => {
    expect(catalogPage).not.toContain('{@html');
    expect(shell).not.toContain('{@html');
  });
});
