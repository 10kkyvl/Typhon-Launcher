import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import Toggle from './Toggle.svelte';

const LABEL = 'Уведомления';

describe('Toggle in a DOM', () => {
  it('a click turns it on and reports the new value once', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(Toggle, { label: LABEL, onchange });
    const toggle = getByRole('switch', { name: LABEL });

    await user.click(toggle);

    expect(onchange.mock.calls).toEqual([[true]]);
    expect(toggle.getAttribute('aria-checked')).toBe('true');
  });

  it('Space on the focused switch turns it off again', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(Toggle, { label: LABEL, checked: true, onchange });
    const toggle = getByRole('switch', { name: LABEL });

    await user.tab();
    expect(document.activeElement).toBe(toggle);
    await user.keyboard(' ');

    expect(onchange.mock.calls).toEqual([[false]]);
    expect(toggle.getAttribute('aria-checked')).toBe('false');
  });

  it('disabled, neither a click nor Space changes it', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(Toggle, { label: LABEL, disabled: true, onchange });
    const toggle = getByRole('switch', { name: LABEL });

    await user.click(toggle);
    toggle.focus();
    await user.keyboard(' ');

    expect(onchange).not.toHaveBeenCalled();
    expect(toggle.getAttribute('aria-checked')).toBe('false');
  });

  it('shows the state the parent passes in', async () => {
    const { getByRole, rerender } = renderDom(Toggle, { label: LABEL, checked: false });

    await rerender({ checked: true });

    expect(getByRole('switch', { name: LABEL }).getAttribute('aria-checked')).toBe('true');
  });
});
