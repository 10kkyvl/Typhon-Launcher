import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { user } from '../testing/dom';
import SubmenuHarness from '../testing/SubmenuHarness.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
});

describe('MenuList: one menu at a time', () => {
  it('a menu opened from an item of an open menu keeps that menu open', async () => {
    render(SubmenuHarness, { target: app });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Open' }));

    await keys.click(screen.getByRole('menuitem', { name: 'More' }));

    expect(screen.getByRole('menu', { name: 'Top' })).not.toBeNull();
    expect(screen.getByRole('menu', { name: 'Nested' })).not.toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('menuitem', { name: 'Deep' }));
  });

  it('Escape then closes the nested menu first, and the menu it came from stays', async () => {
    render(SubmenuHarness, { target: app });
    const keys = user();
    await keys.click(screen.getByRole('button', { name: 'Open' }));
    await keys.click(screen.getByRole('menuitem', { name: 'More' }));

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('menu', { name: 'Nested' })).toBeNull();
    expect(screen.getByRole('menu', { name: 'Top' })).not.toBeNull();
  });
});
