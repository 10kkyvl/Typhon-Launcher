import { describe, expect, it } from 'vitest';
import { markError } from '../game/markMessages';

describe('markError', () => {
  it('translates the favorites limit error', () => {
    expect(markError(new Error('favorites limit reached'), 'запасной текст')).toBe('Не больше 6 любимых игр');
  });

  it('falls back for an unrecognized error', () => {
    expect(markError(new Error('something else'), 'запасной текст')).toBe('запасной текст');
  });

  it('falls back for a non-Error value', () => {
    expect(markError('boom', 'запасной текст')).toBe('запасной текст');
  });
});
