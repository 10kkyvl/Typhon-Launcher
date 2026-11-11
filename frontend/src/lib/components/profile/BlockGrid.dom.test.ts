import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../services/backend', () => ({ inWails: false }));

import BlockGrid from './BlockGrid.svelte';
import { locale, msg } from '../../i18n';
import type { GridBlock } from '../../profile/layoutView';
import { blockLabel } from './blockLabels';

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  locale.set('ru');
});

function text(id: string, index: number, title: string): GridBlock {
  return { id, index, type: 'text', width: 'full', hidden: false, issue: false, body: { kind: 'text', title, body: 'x' } };
}

describe('BlockGrid settings buttons', () => {
  it('the selected block is the current one: its settings button says so and is lit, and is not a toggle', () => {
    const onselect = vi.fn();
    render(BlockGrid, {
      target: app,
      props: { blocks: [text('a', 0, 'About'), text('b', 1, 'Notes')], editing: true, selectedId: 'a', onselect },
    });
    const [selected, other] = screen.getAllByRole('button', { name: msg('profile.blockSettings', { name: blockLabel('text') }) });

    expect(selected.getAttribute('aria-current')).toBe('true');
    expect(selected.classList.contains('lit')).toBe(true);
    expect(selected.hasAttribute('aria-pressed')).toBe(false);
    expect(other.hasAttribute('aria-current')).toBe(false);
    expect(other.classList.contains('lit')).toBe(false);
    expect(other.hasAttribute('aria-pressed')).toBe(false);
  });
});
