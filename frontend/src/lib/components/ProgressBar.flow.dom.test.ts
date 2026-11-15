import { describe, expect, it, onTestFinished } from 'vitest';
import { renderDom } from '../testing/dom';
import { ruleBody, styleOf } from '../testing/css';
import { applyStyles } from '../testing/styles';
import ProgressBar from './ProgressBar.svelte';
import source from './ProgressBar.svelte?raw';

const css = styleOf(source, 'ProgressBar.svelte');

function fillOf(props: Record<string, unknown>): HTMLElement {
  const { container } = renderDom(ProgressBar, { value: 40, label: 'Bar', ...props } as never);
  const fill = container.querySelector<HTMLElement>('.fill');
  if (!fill) throw new Error('no fill');
  return fill;
}

/**
 * jsdom computes no style for a pseudo-element, so this reads the cascade by hand: the plain style rules
 * in the document (applyStyles put the component's in) whose selector matches the element once the
 * pseudo-element is cut off. It is drawn when one of them gives it content and none hides it. @media
 * rules are skipped: jsdom applies none.
 */
function drawn(element: Element, pseudo: '::before' | '::after'): boolean {
  const rules = [...document.styleSheets]
    .flatMap((sheet) => [...sheet.cssRules])
    .filter((rule): rule is CSSStyleRule => rule instanceof CSSStyleRule)
    .flatMap((rule) =>
      rule.selectorText
        .split(',')
        .map((selector) => selector.trim())
        .filter((selector) => selector.endsWith(pseudo) && element.matches(selector.slice(0, -pseudo.length)))
        .map(() => rule.style),
    );
  return rules.some((style) => style.getPropertyValue('content') !== '') && !rules.some((style) => style.getPropertyValue('display') === 'none');
}
const streak = (fill: Element) => drawn(fill, '::after');
const cooled = (fill: Element) => drawn(fill, '::before');

describe('a progress bar whose data is flowing', () => {
  it('runs a streak inside the fill while active, and none while idle', () => {
    const idle = fillOf({});
    applyStyles(idle, source);
    expect(idle.classList.contains('active')).toBe(false);
    expect(streak(idle)).toBe(false);

    const flowing = fillOf({ active: true });
    expect(flowing.classList.contains('active')).toBe(true);
    expect(streak(flowing)).toBe(true);
  });

  it('runs no streak while paused or while the amount is unknown, even if active', () => {
    const paused = fillOf({ active: true, paused: true });
    applyStyles(paused, source);
    expect(streak(paused)).toBe(false);

    const unknown = fillOf({ active: true, indeterminate: true });
    expect(unknown.classList.contains('active')).toBe(false);
  });

  it('with animations off, draws no streak', () => {
    const fill = fillOf({ active: true });
    applyStyles(fill, source);
    expect(streak(fill)).toBe(true);
    document.documentElement.classList.add('no-anim');
    onTestFinished(() => document.documentElement.classList.remove('no-anim'));
    expect(streak(fill)).toBe(false);
  });

  // jsdom applies no @media rule: this one stays a check of the stylesheet's text.
  it('under reduced motion, draws no streak', () => {
    const reduced = /@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.fill\.active::after\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
    expect(reduced).toMatch(/display:\s*none/);
  });

  // The band travels to 1.08–1.53 times the drawn width: unclipped it slides off the end of the progress
  // onto the empty track and reads as more done. The fill's box ends where the drawn part ends (it is
  // shifted, not shortened), so the fill clipping its pseudo-elements keeps the band on the progress.
  it('keeps the streak inside the drawn part: the fill clips it', () => {
    const flowing = fillOf({ active: true });
    applyStyles(flowing, source);
    expect(streak(flowing)).toBe(true);
    expect(getComputedStyle(flowing).overflow).toBe('hidden');
  });

  it('runs no streak at 0 %, where nothing is drawn', () => {
    const empty = fillOf({ active: true, value: 0 });
    applyStyles(empty, source);
    expect(streak(empty)).toBe(false);

    const started = fillOf({ active: true, value: 0.5 });
    applyStyles(started, source);
    expect(streak(started)).toBe(true);
  });

  it('moves the streak by transform only, from the start of the drawn part', () => {
    const body = ruleBody(css, '.fill.active::after');
    expect(body.left).toBe('calc(100% - var(--pct, 0%))');
    expect(body.animation).toMatch(/var\(--progress-sweep\)/);
    expect(css).not.toMatch(/transition:[^;]*\b(width|left)\b/);
  });
});

describe('a paused progress bar', () => {
  it('cools its fill, keeps its value and its tone', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 54, label: 'Hades', tone: 'success', paused: true, valueText: 'Paused, 54%' });
    const bar = getByRole('progressbar', { name: 'Hades' });
    const fill = bar.querySelector<HTMLElement>('.fill') as HTMLElement;
    applyStyles(fill, source);

    expect(fill.classList.contains('paused')).toBe(true);
    expect(cooled(fill)).toBe(true);
    expect(bar.dataset.tone).toBe('success');
    expect(bar.getAttribute('aria-valuenow')).toBe('54');
    expect(bar.getAttribute('aria-valuetext')).toBe('Paused, 54%');
  });

  it('is not cooled when it runs', () => {
    const fill = fillOf({});
    applyStyles(fill, source);
    expect(fill.classList.contains('paused')).toBe(false);
    expect(cooled(fill)).toBe(false);
  });
});

describe("a progress bar in the game's colour", () => {
  it('takes the game tone by name', () => {
    const { getByRole } = renderDom(ProgressBar, { value: 10, label: 'Game', tone: 'game' });
    expect(getByRole('progressbar', { name: 'Game' }).dataset.tone).toBe('game');
  });

  it("reads the game's mark where it paints, with the bright accent as the fallback", () => {
    expect(ruleBody(css, ".track[data-tone='game']")).toEqual({
      '--bar': 'var(--game-mark, var(--accent-bright))',
      '--bar-streak': 'var(--game-on-mark, var(--accent-on, var(--accent-on-default)))',
    });
  });
});

describe('every bar drawn today is drawn as before', () => {
  it('the track and the fill keep their rules', () => {
    expect(ruleBody(css, '.track')).toEqual({
      '--bar': 'var(--accent-bright)',
      width: '100%',
      background: 'var(--hover-strong)',
      'border-radius': 'var(--radius-bar)',
      overflow: 'hidden',
    });
    expect(ruleBody(css, '.fill')).toEqual({
      width: '100%',
      height: '100%',
      'border-radius': 'var(--radius-bar)',
      background: 'var(--bar)',
      transform: 'translateX(calc(var(--pct, 0%) - 100%))',
      transition: 'transform var(--progress-step) linear',
    });
  });

  it('a bar without the new props carries none of their classes and draws no streak or cooling', () => {
    const fill = fillOf({ tone: 'muted' });
    applyStyles(fill, source);
    expect([...fill.classList].filter((name) => !name.startsWith('svelte-'))).toEqual(['fill']);
    expect(streak(fill)).toBe(false);
    expect(cooled(fill)).toBe(false);
  });
});
