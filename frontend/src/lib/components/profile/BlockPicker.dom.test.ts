import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../services/backend', () => ({ inWails: true }));
vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));

import { msg } from '../../i18n';
import { renderDom } from '../../testing/dom';
import { calls, resetBindings } from '../../testing/fakeBinding';
import BlockPicker from './BlockPicker.svelte';

beforeEach(() => {
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
});

describe('BlockPicker', () => {
  it('its button says that it shows the list of blocks below it, and which list, not that it is pressed', async () => {
    const { user } = renderDom(BlockPicker, { layout: { version: 1, blocks: [] }, onadd: vi.fn() });
    const add = screen.getByRole('button', { name: msg('profile.addBlock') });
    expect(add.getAttribute('aria-expanded')).toBe('false');
    expect(add.hasAttribute('aria-pressed')).toBe(false);

    await user.click(add);

    expect(add.getAttribute('aria-expanded')).toBe('true');
    const list = document.getElementById(add.getAttribute('aria-controls') ?? '');
    expect(list?.tagName).toBe('UL');
    expect(list?.querySelectorAll('button').length).toBeGreaterThan(0);

    await user.click(add);

    expect(add.getAttribute('aria-expanded')).toBe('false');
  });
});

describe('BlockPicker, held and handing focus on', () => {
  const genresBlock = { id: 'g', type: 'genres', width: 'half' as const, config: {} };

  it('keeps a kind that cannot be added in the Tab order, held and described by why, and adds nothing on its press', async () => {
    const onadd = vi.fn();
    const { user } = renderDom(BlockPicker, { layout: { version: 1, blocks: [genresBlock] }, onadd });
    await user.click(screen.getByRole('button', { name: msg('profile.addBlock') }));
    const genres = screen.getByRole('button', { name: new RegExp(`^${msg('profile.blockGenres')}`) });

    expect([genres.matches(':disabled'), genres.getAttribute('aria-disabled')]).toEqual([false, 'true']);
    const reason = document.getElementById(genres.getAttribute('aria-describedby') ?? '');
    expect(reason?.textContent).not.toBe('');
    expect(genres.textContent).toContain(reason?.textContent ?? '-');
    await user.click(genres);
    expect(onadd).not.toHaveBeenCalled();
  });

  it('closes the list on a kind added from it and hands focus back to "Add block"', async () => {
    const onadd = vi.fn();
    const { user } = renderDom(BlockPicker, { layout: { version: 1, blocks: [] }, onadd });
    const add = screen.getByRole('button', { name: msg('profile.addBlock') });
    add.focus();

    await user.keyboard('{Enter}');
    await user.tab();
    await user.keyboard('{Enter}');
    expect(onadd).toHaveBeenCalledTimes(1);
    expect(add.getAttribute('aria-expanded')).toBe('false');
    await vi.waitFor(() => expect(document.activeElement).toBe(add));
  });

  it('keeps "Add block" focused while the draft saves, held with the reason', async () => {
    const { rerender } = renderDom(BlockPicker, { layout: { version: 1, blocks: [] }, onadd: vi.fn() });
    const add = screen.getByRole('button', { name: msg('profile.addBlock') });
    add.focus();

    await rerender({ disabled: true });
    expect(document.activeElement).toBe(add);
    expect([add.matches(':disabled'), add.getAttribute('aria-disabled')]).toEqual([false, 'true']);
    expect(document.getElementById(add.getAttribute('aria-describedby') ?? '')?.textContent).toBe(msg('social.saving'));
  });
});
