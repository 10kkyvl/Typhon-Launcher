import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../services/backend', () => ({ inWails: true }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

import BlockGrid from './BlockGrid.svelte';
import { locale, msg } from '../../i18n';
import { calls, resetBindings } from '../../testing/fakeBinding';
import type { GridBlock } from '../../profile/layoutView';
import { blockLabel } from './blockLabels';

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  resetBindings();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  locale.set('ru');
  expect(calls).toEqual([]);
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
