import { describe, expect, it, onTestFinished } from 'vitest';
import { renderDom } from '../testing/dom';
import { applyStyles } from '../testing/styles';
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

  it('inside a control that already says the progress, it is drawn only: no role, hidden', () => {
    const { container, queryAllByRole } = renderDom(ProgressBar, { value: 40, decorative: true });
    const track = container.querySelector('.track');

    expect(queryAllByRole('progressbar')).toEqual([]);
    expect(track?.getAttribute('aria-hidden')).toBe('true');
    expect(track?.hasAttribute('aria-valuenow')).toBe(false);
    expect(track?.querySelector<HTMLElement>('.fill')?.style.getPropertyValue('--pct')).toBe('40%');
  });

  it('stretches along its row when asked, and only then', () => {
    const fixed = renderDom(ProgressBar, { value: 10, label: 'Fixed' });
    expect(fixed.getByRole('progressbar', { name: 'Fixed' }).classList.contains('stretch')).toBe(false);
    const wide = renderDom(ProgressBar, { value: 10, label: 'Wide', stretch: true });
    expect(wide.getByRole('progressbar', { name: 'Wide' }).classList.contains('stretch')).toBe(true);
  });

  // A kept node would carry the sweep's transform into the shift transition and slide a full bar across.
  it('draws a new fill when the amount becomes known, and keeps it while the value moves', async () => {
    const { getByRole, rerender } = renderDom(ProgressBar, { value: 0, indeterminate: true, label: 'Known' });
    const fill = () => getByRole('progressbar', { name: 'Known' }).querySelector<HTMLElement>('.fill');
    const sweeping = fill();

    await rerender({ value: 40, indeterminate: false, label: 'Known' });
    const known = fill();
    expect(known).not.toBe(sweeping);
    expect(known?.classList.contains('indeterminate')).toBe(false);
    expect(known?.style.getPropertyValue('--pct')).toBe('40%');

    await rerender({ value: 60, indeterminate: false, label: 'Known' });
    expect(fill()).toBe(known);
  });

  it('moves its fill without resizing it: the value is a shift, not a width', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 25, label: 'Shift' });
    const fill = getByRole('progressbar', { name: 'Shift' }).querySelector<HTMLElement>('.fill');

    expect(fill?.style.getPropertyValue('--pct')).toBe('25%');
    expect(fill?.style.width).toBe('');
  });
});

describe('ProgressBar with animations off', () => {
  const css = source.slice(source.indexOf('<style>'));

  // With animations off a still 30 % sweep reads as "30 % done"; the unknown amount fills the track
  // instead. The stripes and the stopped sweep go through var() and shorthands jsdom does not compute:
  // a browser shows them.
  it('under the setting, draws an unknown amount across the whole track, not as a short bar', () => {
    const { container } = renderDom(ProgressBar, { value: 0, indeterminate: true, label: 'Unknown' });
    const fill = container.querySelector('.fill') as HTMLElement;
    applyStyles(fill, source);

    expect(getComputedStyle(fill).width).toBe('30%');
    document.documentElement.classList.add('no-anim');
    onTestFinished(() => document.documentElement.classList.remove('no-anim'));
    expect(getComputedStyle(fill).width).toBe('100%');
  });

  // jsdom applies no @media rule: this one stays a check of the stylesheet's text.
  it('under reduced motion, has the same still look', () => {
    const reduced = /@media \(prefers-reduced-motion: reduce\)\s*\{\s*\.fill\.indeterminate\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
    expect(reduced).toMatch(/animation:\s*none/);
    expect(reduced).toMatch(/width:\s*100%/);
    expect(reduced).toMatch(/repeating-linear-gradient/);
  });

  it('forces nothing with !important and animates no layout property', () => {
    expect(css).not.toMatch(/!important/);
    expect(css).not.toMatch(/transition:[^;]*\bwidth\b/);
  });
});
