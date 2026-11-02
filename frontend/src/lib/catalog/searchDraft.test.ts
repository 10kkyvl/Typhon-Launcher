import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import source from '../../routes/catalog/Catalog.svelte?raw';
import { navigate, route } from '../stores/router';
import { confirmSearch, openSearchField, typeSearch, type SearchField, type SearchStep } from './searchDraft';

function catalog(params: Record<string, string>) {
  const browse = vi.fn<(search: string) => void>();
  let state: SearchField = openSearchField(params);
  browse(state.search);
  const apply = (step: SearchStep) => {
    state = step;
    if (step.run !== 'none') browse(step.search);
  };
  return {
    browse,
    field: () => state,
    edit(text: string) {
      state = { ...state, field: text };
      apply(typeSearch(state));
    },
    confirm: () => apply(confirmSearch(state)),
  };
}

describe('черновик поиска из ссылки', () => {
  it('Browse с текстом ссылки не вызывается, пока нет подтверждения', () => {
    const page = catalog({ draft: 'Ведьмак 3', nonce: '1' });

    expect(page.field()).toEqual({ field: 'Ведьмак 3', search: '', draft: true });
    expect(page.browse.mock.calls).toEqual([['']]);

    page.confirm();

    expect(page.browse.mock.calls).toEqual([[''], ['Ведьмак 3']]);
    expect(page.field().draft).toBe(false);
  });

  it('правка черновика тоже ничего не отправляет', () => {
    const page = catalog({ draft: 'Ведьмак' });

    page.edit('Ведьмак 3');

    expect(page.browse.mock.calls).toEqual([['']]);
    page.confirm();
    expect(page.browse.mock.calls).toEqual([[''], ['Ведьмак 3']]);
  });

  it('очищенное поле снимает черновик, дальше поиск обычный', () => {
    const page = catalog({ draft: 'Ведьмак' });

    page.edit('');
    expect(page.field().draft).toBe(false);
    expect(page.browse.mock.calls).toEqual([['']]);

    page.edit('Hades');
    expect(page.browse.mock.calls).toEqual([[''], ['Hades']]);
  });

  it('q из верхнего поиска работает как раньше: сразу в запрос, ввод с задержкой', () => {
    const page = catalog({ q: 'Hades' });

    expect(page.field()).toEqual({ field: 'Hades', search: 'Hades', draft: false });
    expect(page.browse.mock.calls).toEqual([['Hades']]);

    expect(typeSearch({ field: 'Hades 2', search: 'Hades', draft: false })).toEqual({
      field: 'Hades 2', search: 'Hades 2', draft: false, run: 'debounced',
    });
  });

  it('подтверждение без черновика не шлёт лишнего запроса', () => {
    expect(confirmSearch({ field: 'Hades', search: 'Hades', draft: false }).run).toBe('none');
  });
});

describe('Catalog.svelte держит черновик вне запросов', () => {
  it('поле привязано к searchField, а не к search', () => {
    expect(source).toContain('<SearchInput bind:value={searchField}');
    expect(source).not.toContain('bind:value={search}');
  });

  it('searchField не попадает ни в один запрос', () => {
    expect(source).not.toMatch(/search:\s*searchField/);
    expect(source).not.toMatch(/queryCatalogGames\([^)]*searchField/);
  });

  it('Enter и кнопка подтверждают, поле в фокусе', () => {
    expect(source).toContain("if (event.key === 'Enter') commitSearch();");
    expect(source).toContain('<Button variant="primary" onclick={commitSearch}>');
    expect(source).toContain('if (searchDraft) searchBox?.focus();');
  });
});

describe('повторная ссылка с тем же текстом', () => {
  it('nonce даёт новый маршрут, и каталог собирается заново', () => {
    navigate('catalog', { draft: 'Hades', nonce: '1' });
    const first = get(route).key;
    navigate('catalog', { draft: 'Hades', nonce: '2' });

    expect(get(route).key).not.toBe(first);
    expect(openSearchField(get(route).params).field).toBe('Hades');
  });
});
