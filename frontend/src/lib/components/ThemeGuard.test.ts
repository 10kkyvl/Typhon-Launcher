import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import { guardStep } from './ThemeGuard';
import type { Theme } from '../services/theme';

const theme = (id: string, extra: Partial<Theme> = {}): Theme => ({
  id,
  name: id,
  base: 'dark',
  tokens: {},
  builtIn: false,
  updatedAt: '',
  ...extra,
});

function run(themes: (Theme | null)[]): string[] {
  let seen: string | undefined;
  return themes.map((t) => {
    const step = guardStep(seen, t);
    seen = step.seen;
    return step.action;
  });
}

describe('theme guard', () => {
  it.each([
    { name: 'custom theme applied at startup', themes: [null, theme('mine')], want: ['none', 'none'] },
    { name: 'switch to another custom theme', themes: [null, theme('mine'), theme('other')], want: ['none', 'none', 'show'] },
    { name: 'custom css edited', themes: [null, theme('mine'), theme('mine', { css: 'a{}' })], want: ['none', 'none', 'show'] },
    { name: 'same theme re-applied', themes: [null, theme('mine'), theme('mine')], want: ['none', 'none', 'none'] },
    { name: 'built-in theme hides it', themes: [null, theme('mine'), theme('dark', { builtIn: true })], want: ['none', 'none', 'hide'] },
    { name: 'built-in to custom', themes: [null, theme('dark', { builtIn: true }), theme('mine')], want: ['none', 'hide', 'show'] },
  ])('$name', ({ themes, want }) => {
    expect(run(themes)).toEqual(want);
  });
});
