import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import CheckboxHarness from '../testing/CheckboxHarness.svelte';
import { description } from '../testing/hints';

const NAME = 'Install when done';
const box = () => screen.getByRole('checkbox', { name: NAME });
const held = () => document.querySelector('output')?.textContent;

describe('Checkbox', () => {
  it('is a checkbox named by its label and described by its description, with nothing else in the name', () => {
    renderDom(CheckboxHarness, { aside: '1.2 GB' });
    expect(box().getAttribute('aria-checked')).toBe('false');
    expect(box().getAttribute('type')).toBe('button');
    expect(description(box())).toBe('Runs the installer after the download 1.2 GB');
  });

  it('a press asks for the other state; the caller that holds it decides what is shown', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked });

    await user.click(box());

    expect(asked.mock.calls).toEqual([[true]]);
    expect(box().getAttribute('aria-checked')).toBe('true');
    await user.click(box());
    expect(asked.mock.calls).toEqual([[true], [false]]);
    expect(box().getAttribute('aria-checked')).toBe('false');
  });

  it('a refused change is not shown: it never writes the value itself', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked, refuse: true });

    await user.click(box());

    expect(asked.mock.calls).toEqual([[true]]);
    expect(box().getAttribute('aria-checked')).toBe('false');
    expect(held()).toBe('false');
  });

  it('Space toggles it; Enter does not, as with a native checkbox; Tab passes through it', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked });
    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(box());

    await user.keyboard(' ');
    expect(box().getAttribute('aria-checked')).toBe('true');
    await user.keyboard('{Enter}');
    expect(box().getAttribute('aria-checked')).toBe('true');
    expect(asked).toHaveBeenCalledTimes(1);

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('a held Space asks once', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked });
    box().focus();

    await user.keyboard('[Space>5/]');

    expect(asked).toHaveBeenCalledTimes(1);
  });

  it('mixed says so, and a press makes it checked', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked, start: 'mixed' });
    expect(box().getAttribute('aria-checked')).toBe('mixed');

    await user.click(box());

    expect(asked.mock.calls).toEqual([[true]]);
    expect(box().getAttribute('aria-checked')).toBe('true');
  });

  it('without onchange it writes the bound value', async () => {
    const { user } = renderDom(CheckboxHarness, { mode: 'bound' });

    await user.click(box());

    expect(held()).toBe('true');
    expect(box().getAttribute('aria-checked')).toBe('true');
  });

  it('disabled, neither a press nor Space changes it, and Tab passes it by', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked, disabled: true });
    expect((box() as HTMLButtonElement).disabled).toBe(true);

    await user.click(box());
    await user.tab();
    await user.tab();

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
    expect(asked).not.toHaveBeenCalled();
    expect(box().getAttribute('aria-checked')).toBe('false');
  });

  it('disabled with a reason stays in the Tab order, says why, and ignores a press and Space', async () => {
    const asked = vi.fn();
    const { user } = renderDom(CheckboxHarness, { asked, disabled: true, reason: 'Needs a library folder' });
    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(box());
    expect(box().getAttribute('aria-disabled')).toBe('true');
    expect(description(box())).toContain('Needs a library folder');

    await user.keyboard(' ');
    await user.click(box());

    expect(asked).not.toHaveBeenCalled();
    expect(box().getAttribute('aria-checked')).toBe('false');
  });

  it('passes attributes on to its control', () => {
    renderDom(CheckboxHarness, {});
    expect(box().getAttribute('data-row')).toBe('install');
  });
});
