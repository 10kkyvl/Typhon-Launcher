import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import { renderDom } from '../testing/dom';
import TextLinkHarness from '../testing/TextLinkHarness.svelte';
import source from './TextLink.svelte?raw';

describe('TextLink given an address', () => {
  it('is a real link to it, named by its text, with the attributes it was given', () => {
    renderDom(TextLinkHarness, { onforgot: vi.fn() });
    const link = screen.getByRole('link', { name: 'guide' });
    expect(link.tagName).toBe('A');
    expect(link.getAttribute('href')).toBe('https://example.com/guide');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    expect(link.closest('p')?.textContent).toBe('Read the guide first.');
  });
});

describe('TextLink given an action', () => {
  it('wears the muted tone only when asked', () => {
    renderDom(TextLinkHarness, { onforgot: vi.fn() });
    expect(screen.getByRole('link', { name: 'Library' }).classList.contains('muted')).toBe(true);
    expect(screen.getByRole('link', { name: 'guide' }).classList.contains('muted')).toBe(false);
  });

  it('is a button that submits no form, named by its text, and runs the action on a press', async () => {
    const onforgot = vi.fn();
    const { user } = renderDom(TextLinkHarness, { onforgot });
    const action = screen.getByRole('button', { name: 'Reset it' });
    expect(action.tagName).toBe('BUTTON');
    expect(action.getAttribute('type')).toBe('button');
    expect(action.getAttribute('data-kind')).toBe('forgot');
    expect(screen.queryByRole('link', { name: 'Reset it' })).toBeNull();

    await user.click(action);

    expect(onforgot).toHaveBeenCalledTimes(1);
  });

  it('Tab reaches it in the order of the text; Enter and Space run the action', async () => {
    const onforgot = vi.fn();
    const { user } = renderDom(TextLinkHarness, { onforgot });
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole('link', { name: 'guide' }));
    await user.tab();
    const action = screen.getByRole('button', { name: 'Reset it' });
    expect(document.activeElement).toBe(action);

    await user.keyboard('{Enter}');
    await user.keyboard(' ');

    expect(onforgot).toHaveBeenCalledTimes(2);
  });

  it('disabled, runs nothing and is passed over by Tab', async () => {
    const onforgot = vi.fn();
    const { user } = renderDom(TextLinkHarness, { onforgot, disabled: true });
    const action = screen.getByRole('button', { name: 'Reset it' }) as HTMLButtonElement;
    expect(action.disabled).toBe(true);

    await user.click(action);
    await user.tab();
    await user.tab();

    expect(onforgot).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });
});

// jsdom lays nothing out: what keeps the line as it is and the one focus ring are read from the style.
describe('TextLink in a line of text', () => {
  const css = styleOf(source, 'TextLink.svelte');

  it('takes the font and the line height of the text around it and adds no box of its own', () => {
    const link = ruleBody(css, '.text-link');
    expect(link).toMatchObject({ display: 'inline', padding: '0', border: '0', font: 'inherit', 'line-height': 'inherit', 'vertical-align': 'baseline' });
    expect(Object.keys(link).some((prop) => /^(?:height|min-height|margin)/.test(prop))).toBe(false);
  });

  it('is told from the text by more than its colour, and keeps the focus ring every control has', () => {
    expect(ruleBody(css, '.text-link')['text-decoration-line']).toBe('underline');
    expect(css).not.toMatch(/\boutline\b/);
    expect(css).not.toMatch(/:focus/);
  });

  it('has a muted tone for a link that is part of the furniture (a breadcrumb), which still turns to the text colour on hover', () => {
    expect(ruleBody(css, '.text-link').color).toBe('var(--accent-text)');
    expect(ruleBody(css, '.text-link.muted').color).toBe('var(--text-2)');
    // Muted is a colour only: it keeps the underline, so even in a sentence it is told by more than colour.
    expect(Object.keys(ruleBody(css, '.text-link.muted'))).toEqual(['color']);
    expect(css.indexOf('.text-link.muted')).toBeLessThan(css.indexOf('.text-link:hover:not(:disabled)'));
  });

  it('has a hover and a disabled look, and a disabled one does not change on hover', () => {
    expect(ruleBody(css, '.text-link:hover:not(:disabled)').color).toBe('var(--text)');
    expect(ruleBody(css, '.text-link:disabled')).toMatchObject({ cursor: 'default' });
  });
});
