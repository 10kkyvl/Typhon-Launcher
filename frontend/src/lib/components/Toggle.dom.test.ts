import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import ToggleHarness from '../testing/ToggleHarness.svelte';
import Toggle from './Toggle.svelte';

const LABEL = 'Уведомления';

describe('Toggle in a DOM', () => {
  it('a click asks for the other state once; the caller that holds the state decides what is shown', async () => {
    const onchange = vi.fn();
    const { getByRole, user, rerender } = renderDom(Toggle, { label: LABEL, checked: false, onchange });
    const toggle = getByRole('switch', { name: LABEL });

    await user.click(toggle);

    expect(onchange.mock.calls).toEqual([[true]]);
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await rerender({ checked: true });
    expect(toggle.getAttribute('aria-checked')).toBe('true');
  });

  it('Space and Enter on the focused switch ask for the other state', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(Toggle, { label: LABEL, checked: true, onchange });
    const toggle = getByRole('switch', { name: LABEL });

    await user.tab();
    expect(document.activeElement).toBe(toggle);
    await user.keyboard(' ');
    await user.keyboard('{Enter}');

    expect(onchange.mock.calls).toEqual([[false], [false]]);
  });

  it('a held Enter or Space asks once', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(Toggle, { label: LABEL, onchange });
    getByRole('switch', { name: LABEL }).focus();

    await user.keyboard('{Enter>5/}');
    expect(onchange).toHaveBeenCalledTimes(1);
    await user.keyboard('[Space>5/]');
    expect(onchange).toHaveBeenCalledTimes(2);
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

  it('is a button that submits no form and passes attributes on', () => {
    const { getByRole } = renderDom(Toggle, { label: LABEL, id: 'notify', 'data-kind': 'x' });
    const toggle = getByRole('switch', { name: LABEL });
    expect(toggle.getAttribute('type')).toBe('button');
    expect(toggle.id).toBe('notify');
    expect(toggle.getAttribute('data-kind')).toBe('x');
  });
});

describe('Toggle in a settings row', () => {
  it('is named by the row label next to it and described by the row text, with no hidden copy', () => {
    const { getByRole } = renderDom(ToggleHarness, {});
    const toggle = getByRole('switch', { name: 'Minimise to tray' });
    expect(toggle.hasAttribute('aria-label')).toBe(false);
    expect(description(toggle)).toBe('Keeps the launcher running in the background');
  });

  it('a caller that takes the change shows it', async () => {
    const asked = vi.fn();
    const { getByRole, user } = renderDom(ToggleHarness, { asked });
    const toggle = getByRole('switch', { name: 'Minimise to tray' });

    await user.click(toggle);

    expect(asked.mock.calls).toEqual([[true]]);
    expect(toggle.getAttribute('aria-checked')).toBe('true');
  });

  it('a caller that refuses the change keeps the switch on the state it holds', async () => {
    const asked = vi.fn();
    const { getByRole, user } = renderDom(ToggleHarness, { asked, refuse: true });
    const toggle = getByRole('switch', { name: 'Minimise to tray' });

    await user.click(toggle);
    await user.click(toggle);

    expect(asked.mock.calls).toEqual([[true], [true]]);
    expect(toggle.getAttribute('aria-checked')).toBe('false');
  });

  it('a bound switch writes the caller\'s value', async () => {
    const { getByRole, getByText, user } = renderDom(ToggleHarness, { mode: 'bound' });
    const toggle = getByRole('switch', { name: 'Minimise to tray' });

    await user.click(toggle);

    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(getByText('on')).toBeTruthy();
  });
});
