import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import LayoutEditor from './LayoutEditor.svelte';

function html(layout?: object) {
  return render(LayoutEditor, { props: { layout } } as never).body;
}

describe('LayoutEditor', () => {
  it('lists the sections of all three places', () => {
    const out = html();
    for (const text of ['Боковая панель', 'Верхняя панель', 'Страница игры', 'Основная колонка', 'Боковая колонка', 'Поддержка']) {
      expect(out).toContain(text);
    }
    expect(out).toContain('Достижения');
    expect(out).toContain('Дополнения из Steam');
  });

  it('disables the reset button for the default layout only', () => {
    const resetTag = (out: string) => out.match(/<button[^>]*>(?:<!---->)?Сбросить раскладку/)?.[0] ?? '';
    expect(resetTag(html())).toContain('disabled');
    expect(resetTag(html({ sidebar: { side: 'right' } }))).not.toContain('disabled');
  });

  it('shows a hidden item as hidden', () => {
    expect(html({ gamePage: { hidden: ['reviews'] } })).toMatch(/class="item[^"]*\shidden"/);
  });
});
