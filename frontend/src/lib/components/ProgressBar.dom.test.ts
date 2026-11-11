import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import ProgressBar from './ProgressBar.svelte';
import source from './ProgressBar.svelte?raw';

describe('ProgressBar in a DOM', () => {
  it('is a progress bar named for what it measures, with its value and range', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 30, max: 120, label: 'Hades' });
    const bar = getByRole('progressbar', { name: 'Hades' });

    expect(bar.getAttribute('aria-valuemin')).toBe('0');
    expect(bar.getAttribute('aria-valuemax')).toBe('120');
    expect(bar.getAttribute('aria-valuenow')).toBe('30');
    expect(bar.hasAttribute('aria-busy')).toBe(false);
  });

  it('keeps the value inside its range', () => {
    const over = renderDom(ProgressBar, { value: 250, label: 'Over' });
    expect(over.getByRole('progressbar', { name: 'Over' }).getAttribute('aria-valuenow')).toBe('100');
    const under = renderDom(ProgressBar, { value: -4, label: 'Under' });
    expect(under.getByRole('progressbar', { name: 'Under' }).getAttribute('aria-valuenow')).toBe('0');
  });

  it('exposes the whole percent the page shows, not the float behind it', () => {
    const percent = renderDom(ProgressBar, { value: 0.29 * 100, label: 'Float' });
    expect(percent.getByRole('progressbar', { name: 'Float' }).getAttribute('aria-valuenow')).toBe('28');
    const fraction = renderDom(ProgressBar, { value: 0.29, max: 1, label: 'Fraction' });
    expect(fraction.getByRole('progressbar', { name: 'Fraction' }).getAttribute('aria-valuenow')).toBe('0.28');
    const count = renderDom(ProgressBar, { value: 3, max: 7, label: 'Count' });
    expect(count.getByRole('progressbar', { name: 'Count' }).getAttribute('aria-valuenow')).toBe('3');
  });

  it('reads a value that is not a number as nothing done, and draws it empty', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 0 / 0, label: 'Empty total' });
    const bar = getByRole('progressbar', { name: 'Empty total' });

    expect(bar.getAttribute('aria-valuenow')).toBe('0');
    expect(bar.querySelector<HTMLElement>('.fill')?.style.getPropertyValue('--pct')).toBe('0%');
  });

  it('reads the caller’s words for the value when given them', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 3, max: 10, label: 'Achievements', valueText: '3 of 10' });
    expect(getByRole('progressbar', { name: 'Achievements' }).getAttribute('aria-valuetext')).toBe('3 of 10');
  });

  it('can be named by an element on the page', () => {
    const heading = document.createElement('h3');
    heading.id = 'bar-name';
    heading.textContent = 'Verifying files';
    document.body.append(heading);
    const { getByRole } = renderDom(ProgressBar, { value: 10, labelledby: 'bar-name' });
    expect(getByRole('progressbar', { name: 'Verifying files' })).toBeTruthy();
  });

  it('with the amount unknown it is busy and claims no value', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 0, indeterminate: true, label: 'Installing' });
    const bar = getByRole('progressbar', { name: 'Installing' });

    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(bar.getAttribute('aria-busy')).toBe('true');
    expect(bar.querySelector('.fill')?.classList.contains('indeterminate')).toBe(true);
  });

  it('says its state through a named tone, accent by default', () => {
    const plain = renderDom(ProgressBar, { value: 10, label: 'Plain' });
    expect(plain.getByRole('progressbar', { name: 'Plain' }).dataset.tone).toBe('accent');
    const paused = renderDom(ProgressBar, { value: 10, label: 'Paused', tone: 'muted' });
    expect(paused.getByRole('progressbar', { name: 'Paused' }).dataset.tone).toBe('muted');
  });

  it('stretches along its row when asked, and only then', () => {
    const fixed = renderDom(ProgressBar, { value: 10, label: 'Fixed' });
    expect(fixed.getByRole('progressbar', { name: 'Fixed' }).classList.contains('stretch')).toBe(false);
    const wide = renderDom(ProgressBar, { value: 10, label: 'Wide', stretch: true });
    expect(wide.getByRole('progressbar', { name: 'Wide' }).classList.contains('stretch')).toBe(true);
  });

  it('moves its fill without resizing it: the value is a shift, not a width', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 25, label: 'Shift' });
    const fill = getByRole('progressbar', { name: 'Shift' }).querySelector<HTMLElement>('.fill');

    expect(fill?.style.getPropertyValue('--pct')).toBe('25%');
    expect(fill?.style.width).toBe('');
  });
});

// jsdom applies no component CSS: the still look of an unknown amount is held on the stylesheet.
describe('ProgressBar with animations off', () => {
  const css = source.slice(source.indexOf('<style>'));

  it('has a still look of its own for an unknown amount, under the setting and under reduced motion', () => {
    const still = /:global\(\.no-anim\) \.fill\.indeterminate\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
    expect(still).toMatch(/animation:\s*none/);
    expect(still).toMatch(/width:\s*100%/);
    expect(still).toMatch(/repeating-linear-gradient/);

    const reduced = /@media \(prefers-reduced-motion: reduce\)\s*\{\s*\.fill\.indeterminate\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
    expect(reduced).toMatch(/animation:\s*none/);
    expect(reduced).toMatch(/repeating-linear-gradient/);
  });

  it('forces nothing with !important and animates no layout property', () => {
    expect(css).not.toMatch(/!important/);
    expect(css).not.toMatch(/transition:[^;]*\bwidth\b/);
  });
});
